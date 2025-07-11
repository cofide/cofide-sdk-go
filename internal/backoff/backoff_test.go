// Copyright 2024 Cofide Limited.
// SPDX-License-Identifier: Apache-2.0

package backoff

import (
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
)

func TestBackoff_defaults(t *testing.T) {
	backoff := NewBackoff()
	expectedDurations := []time.Duration{
		200 * time.Millisecond,
		400 * time.Millisecond,
		800 * time.Millisecond,
	}
	for _, expected := range expectedDurations {
		assert.Equal(t, expected, backoff.Duration())
	}
	backoff.Reset()
	assert.Equal(t, 200*time.Millisecond, backoff.Duration())
}

func TestBackoff_maxDelay(t *testing.T) {
	backoff := NewBackoff(WithInitialDelay(time.Second), WithMaxDelay(5*time.Second))
	expectedDurations := []time.Duration{
		time.Second,
		2 * time.Second,
		4 * time.Second,
		5 * time.Second,
		5 * time.Second,
	}
	for _, expected := range expectedDurations {
		assert.Equal(t, expected, backoff.Duration())
	}
	backoff.Reset()
	assert.Equal(t, time.Second, backoff.Duration())
}
