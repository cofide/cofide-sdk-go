// Copyright 2024 Cofide Limited.
// SPDX-License-Identifier: Apache-2.0

package cofide_http_server

import (
	"context"
	"net"
	"net/http"

	"github.com/cofide/cofide-sdk-go/internal/spirehelper"
	"github.com/spiffe/go-spiffe/v2/spiffetls/tlsconfig"
)

type Server struct {
	// internal HTTP server
	http *http.Server

	// consumer given http server
	upstreamHTTP *http.Server

	*spirehelper.SPIREHelper
}

func NewServer(server *http.Server, opts ...ServerOption) *Server {
	s := &Server{
		upstreamHTTP: server,
		SPIREHelper:  spirehelper.NewSPIREHelper(context.Background()),
	}

	for _, opt := range opts {
		opt(s)
	}

	return s
}

func (s *Server) getHttp() *http.Server {
	if s.http != nil {
		s.http.Handler = s.upstreamHTTP.Handler
		s.http.Addr = s.upstreamHTTP.Addr
		s.http.ReadTimeout = s.upstreamHTTP.ReadTimeout
		s.http.ReadHeaderTimeout = s.upstreamHTTP.ReadHeaderTimeout
		s.http.WriteTimeout = s.upstreamHTTP.WriteTimeout
		s.http.IdleTimeout = s.upstreamHTTP.IdleTimeout
		s.http.MaxHeaderBytes = s.upstreamHTTP.MaxHeaderBytes
		s.http.ConnState = s.upstreamHTTP.ConnState
		s.http.ErrorLog = s.upstreamHTTP.ErrorLog
		s.http.BaseContext = s.upstreamHTTP.BaseContext
		s.http.ConnContext = s.upstreamHTTP.ConnContext
		s.http.DisableGeneralOptionsHandler = s.upstreamHTTP.DisableGeneralOptionsHandler

		return s.http
	}

	tlsConfig := tlsconfig.MTLSServerConfig(s.X509Source, s.X509Source, s.Authorizer)

	s.http = &http.Server{
		TLSConfig: tlsConfig,

		Handler:                      s.upstreamHTTP.Handler,
		Addr:                         s.upstreamHTTP.Addr,
		ReadTimeout:                  s.upstreamHTTP.ReadTimeout,
		ReadHeaderTimeout:            s.upstreamHTTP.ReadHeaderTimeout,
		WriteTimeout:                 s.upstreamHTTP.WriteTimeout,
		IdleTimeout:                  s.upstreamHTTP.IdleTimeout,
		MaxHeaderBytes:               s.upstreamHTTP.MaxHeaderBytes,
		ConnState:                    s.upstreamHTTP.ConnState,
		ErrorLog:                     s.upstreamHTTP.ErrorLog,
		BaseContext:                  s.upstreamHTTP.BaseContext,
		ConnContext:                  s.upstreamHTTP.ConnContext,
		DisableGeneralOptionsHandler: s.upstreamHTTP.DisableGeneralOptionsHandler,
	}

	return s.http
}

func (w *Server) Close() error {
	return w.getHttp().Close()
}

func (w *Server) ListenAndServe() error {
	return w.ListenAndServeTLS("", "") // certs and keys verridden by SPIRE
}

func (w *Server) ListenAndServeTLS(_, _ string) error {
	w.EnsureSPIRE()
	w.WaitReady()
	return w.getHttp().ListenAndServeTLS("", "") // certs and keys verridden by SPIRE
}

func (w *Server) RegisterOnShutdown(f func()) {
	w.EnsureSPIRE()
	w.WaitReady()
	w.getHttp().RegisterOnShutdown(f)
}

func (w *Server) Serve(l net.Listener) error {
	return w.ServeTLS(l, "", "") // certs and keys verridden by SPIRE
}

func (w *Server) ServeTLS(l net.Listener, _, _ string) error {
	w.EnsureSPIRE()
	w.WaitReady()
	return w.getHttp().ServeTLS(l, "", "") // certs and keys verridden by SPIRE
}

func (w *Server) SetKeepAlivesEnabled(v bool) {
	w.getHttp().SetKeepAlivesEnabled(v)
}

func (w *Server) Shutdown(ctx context.Context) error {
	return w.getHttp().Shutdown(ctx)
}
