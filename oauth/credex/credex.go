// Copyright 2026 Cofide Limited.
// SPDX-License-Identifier: Apache-2.0

// Package credex obtains OAuth 2.0 access tokens from Credex using a SPIFFE
// JWT-SVID. The returned token sources integrate with golang.org/x/oauth2.
package credex

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"net/http"
	"net/url"
	"strings"
	"time"

	"github.com/spiffe/go-spiffe/v2/svid/jwtsvid"
	"golang.org/x/oauth2"
)

const (
	grantTypeTokenExchange    = "urn:ietf:params:oauth:grant-type:token-exchange"
	clientAssertionJWTSpiffe  = "urn:ietf:params:oauth:client-assertion-type:jwt-spiffe"
	tokenTypeJWTSpiffe        = "urn:ietf:params:oauth:token-type:jwt_spiffe"
	defaultEarlyExpiry        = 10 * time.Second
	maxTokenResponseBodyBytes = 1 << 20
)

// Config configures an OAuth Bridge exchange through Credex.
//
// Credex authenticates the workload using a JWT-SVID, obtains an access token
// from the downstream authorization server selected by policy, and returns
// that token unchanged.
type Config struct {
	// TokenURL is the Credex OAuth token endpoint.
	TokenURL string

	// JWTSVIDAudience is the audience used when fetching the JWT-SVID that
	// authenticates the workload to Credex. When empty, TokenURL is used. Set
	// this when the externally advertised token endpoint differs from the URL
	// used to reach Credex from the workload's network.
	JWTSVIDAudience string

	// Audience is the target audience requested from the downstream
	// authorization server. Credex uses it, together with Scopes and the
	// workload identity, when selecting an exchange policy.
	Audience string

	// Scopes are the OAuth scopes requested from the downstream authorization
	// server.
	Scopes []string

	// HTTPClient is used to call the Credex token endpoint. When nil,
	// http.DefaultClient is used.
	HTTPClient *http.Client

	// EarlyExpiry controls how early a cached access token is refreshed. When
	// zero, a ten-second safety margin is used.
	EarlyExpiry time.Duration
}

// TokenSource returns an oauth2.TokenSource which obtains and refreshes access
// tokens through Credex. The source is safe for concurrent use.
//
// The caller owns source and must close it when it is also an io.Closer, as is
// the case for workloadapi.JWTSource.
func (c *Config) TokenSource(ctx context.Context, source jwtsvid.Source) (oauth2.TokenSource, error) {
	if ctx == nil {
		return nil, errors.New("credex: context is nil")
	}
	if source == nil {
		return nil, errors.New("credex: JWT-SVID source is nil")
	}
	if err := c.validate(); err != nil {
		return nil, err
	}

	httpClient := c.HTTPClient
	if httpClient == nil {
		httpClient = http.DefaultClient
	}

	sourceImpl := &tokenSource{
		ctx:             ctx,
		svidSource:      source,
		httpClient:      httpClient,
		tokenURL:        c.TokenURL,
		jwtSVIDAudience: c.jwtSVIDAudience(),
		audience:        c.Audience,
		scopes:          append([]string(nil), c.Scopes...),
	}

	earlyExpiry := c.EarlyExpiry
	if earlyExpiry == 0 {
		earlyExpiry = defaultEarlyExpiry
	}

	return oauth2.ReuseTokenSourceWithExpiry(nil, sourceImpl, earlyExpiry), nil
}

func (c *Config) jwtSVIDAudience() string {
	if c.JWTSVIDAudience != "" {
		return c.JWTSVIDAudience
	}
	return c.TokenURL
}

// Client returns an HTTP client which obtains bearer tokens from Credex and
// refreshes them as required. The client is valid for the lifetime of ctx.
func (c *Config) Client(ctx context.Context, source jwtsvid.Source) (*http.Client, error) {
	tokenSource, err := c.TokenSource(ctx, source)
	if err != nil {
		return nil, err
	}
	return oauth2.NewClient(ctx, tokenSource), nil
}

func (c *Config) validate() error {
	if c == nil {
		return errors.New("credex: config is nil")
	}
	if c.TokenURL == "" {
		return errors.New("credex: token URL is required")
	}
	parsed, err := url.Parse(c.TokenURL)
	if err != nil {
		return fmt.Errorf("credex: invalid token URL: %w", err)
	}
	if parsed.Scheme != "http" && parsed.Scheme != "https" {
		return errors.New("credex: token URL must use http or https")
	}
	if parsed.Host == "" {
		return errors.New("credex: token URL must be absolute")
	}
	if c.EarlyExpiry < 0 {
		return errors.New("credex: early expiry must not be negative")
	}
	for _, scope := range c.Scopes {
		if scope == "" || strings.ContainsAny(scope, " \t\r\n") {
			return fmt.Errorf("credex: invalid scope %q", scope)
		}
	}
	return nil
}

type tokenSource struct {
	ctx             context.Context
	svidSource      jwtsvid.Source
	httpClient      *http.Client
	tokenURL        string
	jwtSVIDAudience string
	audience        string
	scopes          []string
}

func (s *tokenSource) Token() (*oauth2.Token, error) {
	svid, err := s.svidSource.FetchJWTSVID(s.ctx, jwtsvid.Params{Audience: s.jwtSVIDAudience})
	if err != nil {
		return nil, fmt.Errorf("credex: fetching JWT-SVID: %w", err)
	}
	if svid == nil || svid.Marshal() == "" {
		return nil, errors.New("credex: JWT-SVID source returned an empty SVID")
	}

	encodedSVID := svid.Marshal()
	form := url.Values{
		"grant_type":            {grantTypeTokenExchange},
		"client_assertion_type": {clientAssertionJWTSpiffe},
		"client_assertion":      {encodedSVID},
		"subject_token":         {encodedSVID},
		"subject_token_type":    {tokenTypeJWTSpiffe},
	}
	if s.audience != "" {
		form.Set("audience", s.audience)
	}
	if len(s.scopes) > 0 {
		form.Set("scope", strings.Join(s.scopes, " "))
	}

	req, err := http.NewRequestWithContext(s.ctx, http.MethodPost, s.tokenURL, strings.NewReader(form.Encode()))
	if err != nil {
		return nil, fmt.Errorf("credex: building token request: %w", err)
	}
	req.Header.Set("Content-Type", "application/x-www-form-urlencoded")
	req.Header.Set("Accept", "application/json")

	resp, err := s.httpClient.Do(req)
	if err != nil {
		return nil, fmt.Errorf("credex: requesting token: %w", err)
	}
	defer func() { _ = resp.Body.Close() }()

	body, err := io.ReadAll(io.LimitReader(resp.Body, maxTokenResponseBodyBytes+1))
	if err != nil {
		return nil, fmt.Errorf("credex: reading token response: %w", err)
	}
	if len(body) > maxTokenResponseBodyBytes {
		return nil, errors.New("credex: token response exceeds 1 MiB")
	}

	if resp.StatusCode < http.StatusOK || resp.StatusCode >= http.StatusMultipleChoices {
		return nil, newRetrieveError(resp, body)
	}

	var wire tokenResponse
	if err := json.Unmarshal(body, &wire); err != nil {
		return nil, fmt.Errorf("credex: decoding token response: %w", err)
	}
	if wire.Error != "" {
		return nil, newRetrieveError(resp, body)
	}
	if wire.AccessToken == "" {
		return nil, errors.New("credex: token response is missing access_token")
	}
	if wire.ExpiresIn < 0 {
		return nil, errors.New("credex: token response contains a negative expires_in")
	}

	token := &oauth2.Token{
		AccessToken: wire.AccessToken,
		TokenType:   wire.TokenType,
		ExpiresIn:   wire.ExpiresIn,
	}
	if wire.ExpiresIn > 0 {
		token.Expiry = time.Now().Add(time.Duration(wire.ExpiresIn) * time.Second)
	}

	return token.WithExtra(map[string]any{
		"scope":             wire.Scope,
		"issued_token_type": wire.IssuedTokenType,
	}), nil
}

type tokenResponse struct {
	AccessToken      string `json:"access_token"`
	TokenType        string `json:"token_type"`
	ExpiresIn        int64  `json:"expires_in"`
	Scope            string `json:"scope"`
	IssuedTokenType  string `json:"issued_token_type"`
	Error            string `json:"error"`
	ErrorDescription string `json:"error_description"`
	ErrorURI         string `json:"error_uri"`
}

func newRetrieveError(resp *http.Response, body []byte) *oauth2.RetrieveError {
	var wire tokenResponse
	_ = json.Unmarshal(body, &wire)
	return &oauth2.RetrieveError{
		Response:         resp,
		Body:             body,
		ErrorCode:        wire.Error,
		ErrorDescription: wire.ErrorDescription,
		ErrorURI:         wire.ErrorURI,
	}
}
