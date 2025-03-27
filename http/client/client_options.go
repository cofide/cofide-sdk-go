// Copyright 2024 Cofide Limited.
// SPDX-License-Identifier: Apache-2.0

package cofide_http

import (
	"context"

	"github.com/cofide/cofide-sdk-go/pkg/id"
)

type ClientOption func(*Client)

func WithSpireAddress(addr string) ClientOption {
	return func(h *Client) {
		h.SpireAddr = addr
	}
}

func WithContext(ctx context.Context) ClientOption {
	return func(h *Client) {
		h.Ctx = ctx
	}
}

func WithSVIDMatch(funcs ...id.MatchFunc) ClientOption {
	return func(h *Client) {
		h.Authorizer = id.AuthorizeMatch(funcs...)
	}
}
