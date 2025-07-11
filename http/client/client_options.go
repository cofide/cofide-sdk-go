// Copyright 2024 Cofide Limited.
// SPDX-License-Identifier: Apache-2.0

package cofide_http

import (
	"context"

	"github.com/cofide/cofide-sdk-go/pkg/id"
)

type ClientOption func(*Client)

func WithSPIREAddress(addr string) ClientOption {
	return func(h *Client) {
		h.SPIREAddr = addr
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

func WithXDS(serverURI string) ClientOption {
	return func(c *Client) {
		c.xdsServerURI = serverURI
	}
}

func WithXDSNodeID(nodeID string) ClientOption {
	return func(c *Client) {
		c.xdsNodeID = nodeID
	}
}
