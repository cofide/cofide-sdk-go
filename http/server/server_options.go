// Copyright 2024 Cofide Limited.
// SPDX-License-Identifier: Apache-2.0

package cofide_http_server

import (
	"context"

	"github.com/cofide/cofide-sdk-go/pkg/id"
)

type ServerOption func(*Server)

func WithSPIREAddress(addr string) ServerOption {
	return func(h *Server) {
		h.SPIREAddr = addr
	}
}

func WithContext(ctx context.Context) ServerOption {
	return func(h *Server) {
		h.Ctx = ctx
	}
}

func WithSVIDMatch(funcs ...id.MatchFunc) ServerOption {
	return func(h *Server) {
		h.Authorizer = id.AuthorizeMatch(funcs...)
	}
}
