package cofide_http

import (
	"context"
	"io"
	"net/http"
	"net/url"
	"os"
	"time"

	"github.com/cofide/cofide-sdk-go/internal/spirehelper"
	"github.com/spiffe/go-spiffe/v2/spiffetls/tlsconfig"
)

type Client struct {
	// internal HTTP client
	http *http.Client

	*spirehelper.SpireHelper

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

		SpireHelper: &spirehelper.SpireHelper{
			Ctx:        context.Background(),
			SpireAddr:  "unix:///tmp/spire.sock",
			Authorizer: tlsconfig.AuthorizeAny(),
		},
	}

	if os.Getenv("SPIFFE_ENDPOINT_SOCKET") != "" {
		c.SpireAddr = os.Getenv("SPIFFE_ENDPOINT_SOCKET")
	}

	for _, opt := range opts {
		opt(c)
	}

	return c
}

func (c *Client) getHttp() *http.Client {
	if c.http != nil {
		c.http.CheckRedirect = c.CheckRedirect
		c.http.Jar = c.Jar
		c.http.Timeout = c.Timeout

		return c.http
	}

	tlsConfig := tlsconfig.MTLSClientConfig(c.X509Source, c.X509Source, c.Authorizer)

	c.http = &http.Client{
		Transport: &http.Transport{
			TLSClientConfig: tlsConfig,
		},
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
	c.EnsureSpire()
	c.WaitReady()

	if req.URL.Scheme == "http" {
		req.URL.Scheme = "https"
	}

	return c.getHttp().Do(req)
}

func (c *Client) Get(url string) (resp *http.Response, err error) {
	c.EnsureSpire()
	c.WaitReady()

	return c.getHttp().Get(secureURL(url))
}

func (c *Client) Head(url string) (resp *http.Response, err error) {
	c.EnsureSpire()
	c.WaitReady()

	return c.getHttp().Head(secureURL(url))
}

func (c *Client) Post(url, contentType string, body io.Reader) (resp *http.Response, err error) {
	c.EnsureSpire()
	c.WaitReady()

	return c.getHttp().Post(secureURL(url), contentType, body)
}

func (c *Client) PostForm(url string, data url.Values) (resp *http.Response, err error) {
	c.EnsureSpire()
	c.WaitReady()

	return c.getHttp().PostForm(secureURL(url), data)
}
