// Copyright 2024 Cofide Limited.
// SPDX-License-Identifier: Apache-2.0

package cofide_http

import (
	"crypto/tls"
	"io"
	"log/slog"
	"net/http"
	"net/url"
	"os"
	"time"

	"github.com/cofide/cofide-sdk-go/internal/spirehelper"
	"github.com/spiffe/go-spiffe/v2/spiffetls/tlsconfig"

	"github.com/cofide/cofide-sdk-go/internal/transport"
	"github.com/cofide/cofide-sdk-go/internal/xds"
)

type Client struct {
	// internal HTTP client
	http *http.Client

	*spirehelper.SPIREHelper

	/** FROM THIS POINT ALL PROPERTIES COME FROM net/http **/

	// Transport specifies the mechanism by which individual
	// HTTP requests are made.
	// If nil, DefaultTransport is used.
	Transport http.RoundTripper

	// CheckRedirect specifies the policy for handling redirects.
	// If CheckRedirect is not nil, the client calls it before
	// following an HTTP redirect. The arguments req and via are
	// the upcoming request and the requests made already, oldest
	// first. If CheckRedirect returns an error, the Client's Get
	// method returns both the previous Response (with its Body
	// closed) and CheckRedirect's error (wrapped in a url.Error)
	// instead of issuing the Request req.
	// As a special case, if CheckRedirect returns ErrUseLastResponse,
	// then the most recent response is returned with its body
	// unclosed, along with a nil error.
	//
	// If CheckRedirect is nil, the Client uses its default policy,
	// which is to stop after 10 consecutive requests.
	CheckRedirect func(req *http.Request, via []*http.Request) error

	// Jar specifies the cookie jar.
	//
	// The Jar is used to insert relevant cookies into every
	// outbound Request and is updated with the cookie values
	// of every inbound Response. The Jar is consulted for every
	// redirect that the Client follows.
	//
	// If Jar is nil, cookies are only sent if they are explicitly
	// set on the Request.
	Jar http.CookieJar

	// Timeout specifies a time limit for requests made by this
	// Client. The timeout includes connection time, any
	// redirects, and reading the response body. The timer remains
	// running after Get, Head, Post, or Do return and will
	// interrupt reading of the Response.Body.
	//
	// A Timeout of zero means no timeout.
	//
	// The Client cancels requests to the underlying Transport
	// as if the Request's Context ended.
	//
	// For compatibility, the Client will also use the deprecated
	// CancelRequest method on Transport if found. New
	// RoundTripper implementations should use the Request's Context
	// for cancellation instead of implementing CancelRequest.
	Timeout time.Duration
}

func NewClient(opts ...ClientOption) *Client {
	c := &Client{
		SPIREHelper: spirehelper.NewSPIREHelper(),
	}

	for _, opt := range opts {
		opt(c)
	}

	// Ensure SPIRE is ready in order to use the x509Source and craft the
	// tlsConfig for the custom transport
	c.EnsureSPIRE()
	c.WaitReady()

	tlsConfig := tlsconfig.MTLSClientConfig(c.X509Source, c.BundleSource, c.Authorizer)
	c.Transport = createTransport(tlsConfig)

	return c
}

func createTransport(tlsConfig *tls.Config) http.RoundTripper {
	if !isXDSEnabled() {
		return &http.Transport{TLSClientConfig: tlsConfig}
	}

	xdsServer := os.Getenv("EXPERIMENTAL_XDS_SERVER_URI")
	if xdsServer == "" {
		return &http.Transport{TLSClientConfig: tlsConfig}
	}

	xdsClient, err := xds.NewXDSClient(xds.XDSClientConfig{
		ServerURI: xdsServer,
		NodeID:    "node",
	})
	if err != nil {
		slog.Error("failed to create xDS client, falling back to default transport", "error", err)
		return &http.Transport{TLSClientConfig: tlsConfig}
	}

	return transport.NewCofideTransport(xdsClient, tlsConfig)
}

func isXDSEnabled() bool {
	return os.Getenv("EXPERIMENTAL_ENABLE_XDS") == "true"
}

func (c *Client) getHttp() *http.Client {
	if c.http != nil {
		c.http.CheckRedirect = c.CheckRedirect
		c.http.Jar = c.Jar
		c.http.Timeout = c.Timeout

		return c.http
	}

	c.http = &http.Client{
		Transport:     c.Transport,
		CheckRedirect: c.CheckRedirect,
		Jar:           c.Jar,
		Timeout:       c.Timeout,
	}

	return c.http
}

func secureURL(u string) string {
	parsed, err := url.Parse(u)
	if err != nil {
		return u
	}

	if parsed.Scheme == "http" {
		parsed.Scheme = "https"
	}

	return parsed.String()
}

func (c *Client) CloseIdleConnections() {
	c.getHttp().CloseIdleConnections()
}

func (c *Client) Do(req *http.Request) (*http.Response, error) {
	c.EnsureSPIRE()
	c.WaitReady()

	if req.URL.Scheme == "http" {
		req.URL.Scheme = "https"
	}

	return c.getHttp().Do(req)
}

func (c *Client) Get(url string) (resp *http.Response, err error) {
	c.EnsureSPIRE()
	c.WaitReady()

	return c.getHttp().Get(secureURL(url))
}

func (c *Client) Head(url string) (resp *http.Response, err error) {
	c.EnsureSPIRE()
	c.WaitReady()

	return c.getHttp().Head(secureURL(url))
}

func (c *Client) Post(url, contentType string, body io.Reader) (resp *http.Response, err error) {
	c.EnsureSPIRE()
	c.WaitReady()

	return c.getHttp().Post(secureURL(url), contentType, body)
}

func (c *Client) PostForm(url string, data url.Values) (resp *http.Response, err error) {
	c.EnsureSPIRE()
	c.WaitReady()

	return c.getHttp().PostForm(secureURL(url), data)
}
