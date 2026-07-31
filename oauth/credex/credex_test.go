// Copyright 2026 Cofide Limited.
// SPDX-License-Identifier: Apache-2.0

package credex_test

import (
	"context"
	"crypto/ecdsa"
	"crypto/elliptic"
	"crypto/rand"
	"errors"
	"net/http"
	"net/http/httptest"
	"sync"
	"testing"
	"time"

	"github.com/go-jose/go-jose/v4"
	"github.com/go-jose/go-jose/v4/jwt"
	"github.com/spiffe/go-spiffe/v2/svid/jwtsvid"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"golang.org/x/oauth2"

	"github.com/cofide/cofide-sdk-go/oauth/credex"
)

func TestTokenSourceExchange(t *testing.T) {
	var requests int
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		requests++
		require.Equal(t, http.MethodPost, r.Method)
		require.Equal(t, "application/x-www-form-urlencoded", r.Header.Get("Content-Type"))
		require.Equal(t, "application/json", r.Header.Get("Accept"))
		require.NoError(t, r.ParseForm())
		assert.Equal(t, "urn:ietf:params:oauth:grant-type:token-exchange", r.Form.Get("grant_type"))
		assert.Equal(t, "urn:ietf:params:oauth:client-assertion-type:jwt-spiffe", r.Form.Get("client_assertion_type"))
		assert.Equal(t, "urn:ietf:params:oauth:token-type:jwt_spiffe", r.Form.Get("subject_token_type"))
		assert.Equal(t, r.Form.Get("client_assertion"), r.Form.Get("subject_token"))
		assert.Equal(t, "legacy-api", r.Form.Get("audience"))
		assert.Equal(t, "read write", r.Form.Get("scope"))
		w.Header().Set("Content-Type", "application/json")
		_, _ = w.Write([]byte(`{"access_token":"downstream-token","token_type":"Bearer","expires_in":3600,"scope":"read write","issued_token_type":"urn:ietf:params:oauth:token-type:access_token"}`))
	}))
	defer server.Close()

	source := newFakeSVIDSource(t)
	tokenSource, err := (&credex.Config{
		TokenURL: server.URL,
		Audience: "legacy-api",
		Scopes:   []string{"read", "write"},
	}).TokenSource(t.Context(), source)
	require.NoError(t, err)

	before := time.Now()
	token, err := tokenSource.Token()
	require.NoError(t, err)
	assert.Equal(t, "downstream-token", token.AccessToken)
	assert.Equal(t, "Bearer", token.TokenType)
	assert.EqualValues(t, 3600, token.ExpiresIn)
	assert.WithinDuration(t, before.Add(time.Hour), token.Expiry, time.Second)
	assert.Equal(t, "read write", token.Extra("scope"))
	assert.Equal(t, "urn:ietf:params:oauth:token-type:access_token", token.Extra("issued_token_type"))

	second, err := tokenSource.Token()
	require.NoError(t, err)
	assert.Same(t, token, second)
	assert.Equal(t, 1, requests)
	assert.Equal(t, []string{server.URL}, source.audiences())
}

func TestTokenSourceRefreshesExpiringToken(t *testing.T) {
	var mu sync.Mutex
	requests := 0
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		mu.Lock()
		requests++
		mu.Unlock()
		_, _ = w.Write([]byte(`{"access_token":"short-lived","token_type":"Bearer","expires_in":1}`))
	}))
	defer server.Close()

	source := newFakeSVIDSource(t)
	tokenSource, err := (&credex.Config{
		TokenURL: server.URL,
		Audience: "legacy-api",
	}).TokenSource(t.Context(), source)
	require.NoError(t, err)

	_, err = tokenSource.Token()
	require.NoError(t, err)
	_, err = tokenSource.Token()
	require.NoError(t, err)

	mu.Lock()
	defer mu.Unlock()
	assert.Equal(t, 2, requests)
}

func TestTokenSourceOAuthError(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		w.WriteHeader(http.StatusForbidden)
		_, _ = w.Write([]byte(`{"error":"access_denied","error_description":"denied by policy"}`))
	}))
	defer server.Close()

	tokenSource, err := (&credex.Config{TokenURL: server.URL, Audience: "legacy-api"}).TokenSource(t.Context(), newFakeSVIDSource(t))
	require.NoError(t, err)
	_, err = tokenSource.Token()

	var retrieveError *oauth2.RetrieveError
	require.ErrorAs(t, err, &retrieveError)
	assert.Equal(t, http.StatusForbidden, retrieveError.Response.StatusCode)
	assert.Equal(t, "access_denied", retrieveError.ErrorCode)
	assert.Equal(t, "denied by policy", retrieveError.ErrorDescription)
}

func TestTokenSourceSVIDError(t *testing.T) {
	wantErr := errors.New("workload API unavailable")
	source := &fakeSVIDSource{err: wantErr}
	tokenSource, err := (&credex.Config{TokenURL: "https://credex.example.com/token", Audience: "legacy-api"}).TokenSource(t.Context(), source)
	require.NoError(t, err)

	_, err = tokenSource.Token()
	require.ErrorIs(t, err, wantErr)
}

func TestConfigValidation(t *testing.T) {
	tests := []struct {
		name   string
		config *credex.Config
		source jwtsvid.Source
		want   string
	}{
		{name: "nil config", source: newFakeSVIDSource(t), want: "config is nil"},
		{name: "nil context", config: &credex.Config{}, source: newFakeSVIDSource(t), want: "context is nil"},
		{name: "nil source", config: &credex.Config{}, want: "JWT-SVID source is nil"},
		{name: "missing token URL", config: &credex.Config{Audience: "api"}, source: newFakeSVIDSource(t), want: "token URL is required"},
		{name: "relative token URL", config: &credex.Config{TokenURL: "/token", Audience: "api"}, source: newFakeSVIDSource(t), want: "must use http or https"},
		{name: "negative early expiry", config: &credex.Config{TokenURL: "https://credex.example.com/token", EarlyExpiry: -time.Second}, source: newFakeSVIDSource(t), want: "early expiry must not be negative"},
		{name: "invalid scope", config: &credex.Config{TokenURL: "https://credex.example.com/token", Audience: "api", Scopes: []string{"read write"}}, source: newFakeSVIDSource(t), want: "invalid scope"},
	}

	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			ctx := t.Context()
			if tc.name == "nil context" {
				ctx = nil
			}
			_, err := tc.config.TokenSource(ctx, tc.source)
			require.ErrorContains(t, err, tc.want)
		})
	}
}

func TestClientAddsBearerToken(t *testing.T) {
	var serverURL string
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		switch r.URL.Path {
		case "/token":
			_, _ = w.Write([]byte(`{"access_token":"downstream-token","token_type":"Bearer","expires_in":3600}`))
		case "/resource":
			assert.Equal(t, "Bearer downstream-token", r.Header.Get("Authorization"))
			w.WriteHeader(http.StatusNoContent)
		default:
			http.NotFound(w, r)
		}
	}))
	defer server.Close()
	serverURL = server.URL

	client, err := (&credex.Config{TokenURL: serverURL + "/token", Audience: "legacy-api"}).Client(t.Context(), newFakeSVIDSource(t))
	require.NoError(t, err)
	resp, err := client.Get(serverURL + "/resource")
	require.NoError(t, err)
	defer func() { _ = resp.Body.Close() }()
	assert.Equal(t, http.StatusNoContent, resp.StatusCode)
}

type fakeSVIDSource struct {
	svid *jwtsvid.SVID
	err  error
	mu   sync.Mutex
	auds []string
}

func newFakeSVIDSource(t *testing.T) *fakeSVIDSource {
	t.Helper()
	key, err := ecdsa.GenerateKey(elliptic.P256(), rand.Reader)
	require.NoError(t, err)
	signer, err := jose.NewSigner(jose.SigningKey{Algorithm: jose.ES256, Key: key}, nil)
	require.NoError(t, err)
	raw, err := jwt.Signed(signer).Claims(map[string]any{
		"sub": "spiffe://example.org/workload",
		"aud": []string{"placeholder"},
		"exp": time.Now().Add(time.Hour).Unix(),
	}).Serialize()
	require.NoError(t, err)
	svid, err := jwtsvid.ParseInsecure(raw, []string{"placeholder"})
	require.NoError(t, err)
	return &fakeSVIDSource{svid: svid}
}

func (s *fakeSVIDSource) FetchJWTSVID(_ context.Context, params jwtsvid.Params) (*jwtsvid.SVID, error) {
	s.mu.Lock()
	defer s.mu.Unlock()
	s.auds = append(s.auds, params.Audience)
	return s.svid, s.err
}

func (s *fakeSVIDSource) audiences() []string {
	s.mu.Lock()
	defer s.mu.Unlock()
	return append([]string(nil), s.auds...)
}
