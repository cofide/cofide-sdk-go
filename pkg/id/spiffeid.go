// Copyright 2024 Cofide Limited.
// SPDX-License-Identifier: Apache-2.0

package id

import (
	"fmt"
	"sort"
	"strings"

	"github.com/spiffe/go-spiffe/v2/spiffeid"
)

// SPIFFEID is an **opinionated** implementation of the upstream [spiffeid.ID].
// It builds on top of the SPIFFE ID however carries a strong key-value based
// approach to encode the ID with information.
// URLs are used as a string representation of trust data as this can be used
// inside X.509 data easily as URI SAN.
//
// The data is encoded as follows:
// spiffe://<trust domain>/<key 1>/<value 1>/<key 2>/<value 2>
//
// For example:
// spiffe://foo.example.org/ns/production/sa/billing
//
// Verifying these IDs should be done by comparing the trust domain and preferably a minimum set of keys.
// If the SPIFFE ID holds (new) keys that are not known to the verifier, the verifier should ignore these keys.
// This allows for a flexible way to encode data in the SPIFFE ID without breaking the verification.
type SPIFFEID struct {
	// SPIFFE ID
	id spiffeid.ID
}

// NewID creates a SPIFFEID from a trust domain and key-value map.
func NewID(trustDomain string, kv map[string]string) (*SPIFFEID, error) {
	// sort the keys to have a deterministic order
	keys := make([]string, 0, len(kv))
	for k, v := range kv {
		keys = append(keys, k)
		if k == "" || v == "" {
			return nil, fmt.Errorf("empty key or value not allowed")
		}
	}
	sort.Strings(keys)

	pathKV := make([]string, 0, len(kv)*2)
	for _, k := range keys {
		pathKV = append(pathKV, k, kv[k])
	}
	td, err := spiffeid.TrustDomainFromString(trustDomain)
	if err != nil {
		return nil, fmt.Errorf("failed to create trust domain: %w", err)
	}

	path := "/" + strings.Join(pathKV, "/")
	path = strings.TrimSuffix(path, "/")

	id, err := spiffeid.FromPath(td, path)
	if err != nil {
		return nil, fmt.Errorf("failed to create spiffe id: %w", err)
	}
	return &SPIFFEID{id: id}, nil
}

// MustNewID is the same as NewID, but panics on error.
func MustNewID(trustDomain string, kv map[string]string) *SPIFFEID {
	id, err := NewID(trustDomain, kv)
	if err != nil {
		panic(err)
	}
	return id
}

// ParseID parses a SPIFFE ID provided as a string and returns a SPIFFEID.
func ParseID(id string) (*SPIFFEID, error) {
	upstreamID, err := spiffeid.FromString(id)
	if err != nil {
		return nil, fmt.Errorf("failed to parse spiffe id: %w", err)
	}
	svid := &SPIFFEID{id: upstreamID}

	if _, err := svid.ParsePath(); err != nil {
		return nil, fmt.Errorf("failed to parse path: %w", err)
	}

	return svid, nil
}

// MustParseID is the same as ParseID, but panics on error.
func MustParseID(id string) *SPIFFEID {
	svid, err := ParseID(id)
	if err != nil {
		panic(err)
	}
	return svid
}

// ParsePath parses the path component of a SPIFFEID and returns it as a map.
func (s *SPIFFEID) ParsePath() (map[string]string, error) {
	path := s.id.Path()
	path = strings.Trim(path, "/")
	pathParts := strings.Split(path, "/")

	if len(pathParts)%2 != 0 {
		return nil, fmt.Errorf("invalid path, needs to be even in parts: %s", path)
	}

	kv := make(map[string]string)
	for i := 0; i < len(pathParts); i += 2 {
		kv[pathParts[i]] = pathParts[i+1]
	}

	return kv, nil
}

// TrustDomain returns the trust domain of a SPIFFEID as a string.
func (s *SPIFFEID) TrustDomain() string {
	return s.id.TrustDomain().String()
}

// String returns a string representation of the SPIFFEID.
func (s *SPIFFEID) String() string {
	return s.id.String()
}

// ToSpiffeID returns a [spiffeid.ID] representation of the SPIFFEID.
func (s *SPIFFEID) ToSpiffeID() spiffeid.ID {
	return s.id
}
