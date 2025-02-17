package transport

import (
	"crypto/tls"
	"net"
	"net/http"
	"sync"
	"time"

	"github.com/cofide/cofide-sdk-go/internal/xds"
)

type CofideTransport struct {
	xdsClient     *xds.XDSClient
	endpointCache sync.Map // serviceName -> endpoint
	BaseTransport http.RoundTripper
	tlsConfig     *tls.Config
}

func NewCofideTransport(client *xds.XDSClient, tlsConfig *tls.Config) *CofideTransport {
	return &CofideTransport{
		xdsClient:     client,
		tlsConfig:     tlsConfig,
		BaseTransport: http.DefaultTransport,
	}
}

func (t *CofideTransport) RoundTrip(req *http.Request) (*http.Response, error) {
	var transport http.RoundTripper
	if t.BaseTransport != nil {
		transport = t.BaseTransport
	} else {
		transport = http.DefaultTransport
	}

	if t.tlsConfig != nil {
		// Create a new transport with the custom TLS config.
		transport = &http.Transport{
			DialContext: (&net.Dialer{
				Timeout:   30 * time.Second,
				KeepAlive: 30 * time.Second,
			}).DialContext,
			ForceAttemptHTTP2:     true,
			MaxIdleConns:          100,
			IdleConnTimeout:       90 * time.Second,
			TLSHandshakeTimeout:   10 * time.Second,
			ExpectContinueTimeout: 1 * time.Second,
			TLSClientConfig:       t.tlsConfig,
		}
	}
	return transport.RoundTrip(req)
}
