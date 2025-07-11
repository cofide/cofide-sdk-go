// Copyright 2024 Cofide Limited.
// SPDX-License-Identifier: Apache-2.0

package backoff

import (
	"sync"
	"time"
)

// Backoff is a simple exponential backoff implementation.
type Backoff struct {
	InitialDelay time.Duration
	MaxDelay     time.Duration
	n            int

	mutex sync.Mutex
}

type BackoffOption func(*Backoff)

func WithInitialDelay(d time.Duration) BackoffOption {
	return func(b *Backoff) {
		b.InitialDelay = d
	}
}

func WithMaxDelay(d time.Duration) BackoffOption {
	return func(b *Backoff) {
		b.MaxDelay = d
	}
}

func NewBackoff(opts ...BackoffOption) *Backoff {
	b := &Backoff{
		InitialDelay: time.Millisecond * 200,
		MaxDelay:     10 * time.Second,
		n:            0,
	}

	for _, opt := range opts {
		opt(b)
	}

	return b
}

// Duration returns the next wait period for the backoff. Not goroutine-safe.
func (b *Backoff) Duration() time.Duration {
	b.mutex.Lock()
	defer b.mutex.Unlock()

	d := b.InitialDelay << b.n
	// Check for overflow (d becomes non-positive) or if it exceeds MaxDelay.
	if d < 0 || d > b.MaxDelay {
		d = b.MaxDelay
	}

	b.n++
	return time.Duration(d)
}

// Reset resets the backoff's state.
func (b *Backoff) Reset() {
	b.n = 0
}
