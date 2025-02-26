// Copyright 2024 Cofide Limited.
// SPDX-License-Identifier: Apache-2.0

package id

import (
	"crypto/x509"
	"errors"
	"fmt"

	"github.com/gobwas/glob"
	"github.com/spiffe/go-spiffe/v2/spiffeid"
	"github.com/spiffe/go-spiffe/v2/spiffetls/tlsconfig"
)

// MatchFunc is a function that can be called to determine whether the path
// component of a SPIFFEID matches a given constraint. The function should
// return nil if the constraint matches, or an error otherwise.
type MatchFunc func(kv map[string]string) error

// Matches applies a set of MatchFunc functions to a SPIFFEID and returns the
// combined match result. If no MatchFunc returns an error, then Match returns
// nil. Otherwise an error is returned.
func (s *SPIFFEID) Matches(funcs ...MatchFunc) error {
	kv, err := s.ParsePath()
	if err != nil {
		return err
	}

	for _, f := range funcs {
		err := f(kv)
		if err != nil {
			return err
		}
	}

	return nil
}

// AuthorizeMatch returns a [tlsconfig.Authorizer] that authorizes an ID when
// it matches all of the provided MatchFunc.
func AuthorizeMatch(funcs ...MatchFunc) tlsconfig.Authorizer {
	return func(id spiffeid.ID, verifiedChains [][]*x509.Certificate) error {
		sid, err := ParseID(id.String())
		if err != nil {
			return err
		}

		err = sid.Matches(funcs...)
		if err != nil {
			return err
		}
		return nil
	}
}

// Equals returns a MatchFunc that matches any ID that contains the specified
// key/value pair.
func Equals(key, value string) MatchFunc {
	return func(kv map[string]string) error {
		if val, ok := kv[key]; !ok || val != value {
			return fmt.Errorf("key %s does not match value %s", key, value)
		}

		return nil
	}
}

// IsEmptyKey returns a MatchFunc that matches any ID that contains the
// specified key with an empty value.
func IsEmpty(key string) MatchFunc {
	return func(kv map[string]string) error {
		if kv[key] != "" {
			return fmt.Errorf("key %s is not empty", key)
		}

		return nil
	}
}

// IsNotEmpty returns a MatchFunc that matches any ID that contains the
// specified key with a non-empty value.
func IsNotEmpty(key string) MatchFunc {
	return func(kv map[string]string) error {
		if kv[key] == "" {
			return fmt.Errorf("key %s is empty", key)
		}

		return nil
	}
}

// MatchGlob returns a MatchFunc that matches any ID that contains the
// specified key with a value matching the specified glob pattern.
func MatchGlob(key, globStr string) MatchFunc {
	return func(kv map[string]string) error {
		g, err := glob.Compile(globStr)
		if err != nil {
			return fmt.Errorf("failed to compile glob %q: %w", globStr, err)
		}
		if _, ok := kv[key]; !ok {
			return fmt.Errorf("key %q not found", key)
		}
		if !g.Match(kv[key]) {
			return fmt.Errorf("key %q with value %q does not match glob %q", key, kv[key], globStr)
		}

		return nil
	}
}

// Or returns a MatchFunc that combines the specified MatchFunc using a logical
// OR.
func Or(funcs ...MatchFunc) MatchFunc {
	return func(kv map[string]string) error {
		errs := make([]error, 0, len(funcs))
		for _, f := range funcs {
			err := f(kv)
			errs = append(errs, err)
		}

		hasPassedTest := false
		for _, err := range errs {
			if err == nil {
				hasPassedTest = true
				break
			}
		}

		if !hasPassedTest {
			return fmt.Errorf("none of the tests passed")
		}

		return nil
	}
}

// Not returns a MatchFunc that logically inverts the result of the specified
// MatchFunc.
func Not(f MatchFunc) MatchFunc {
	return func(kv map[string]string) error {
		err := f(kv)
		if err == nil {
			return errors.New("Did not receive an error in NOT call")
		}

		return nil
	}
}
