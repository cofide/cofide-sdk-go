package transport

import (
	"crypto/tls"
	"fmt"
	"net/http"

	"github.com/cofide/cofide-sdk-go/internal/xds"
)

type CofideTransport struct {
	xdsClient     *xds.XDSClient
	baseTransport http.RoundTripper
	tlsConfig     *tls.Config
}

func NewCofideTransport(client *xds.XDSClient, tlsConfig *tls.Config) *CofideTransport {
	return &CofideTransport{
		xdsClient:     client,
		tlsConfig:     tlsConfig,
		baseTransport: http.DefaultTransport,
	}
}

func (t *CofideTransport) RoundTrip(req *http.Request) (*http.Response, error) {
	service := req.URL.Hostname()

	endpoints, err := t.xdsClient.GetEndpoints(service)
	if err != nil {
		// Fall back to direct call
		return t.baseTransport.RoundTrip(req)
	}

	// Clone request to modify it
	outReq := req.Clone(req.Context())

	// Select endpoint (simple round-robin for now)
	endpoint := selectEndpoint(endpoints)
	outReq.URL.Host = fmt.Sprintf("%s:%d", endpoint.Host, endpoint.Port)

	return t.baseTransport.RoundTrip(outReq)
}

func selectEndpoint(endpoints []xds.Endpoint) xds.Endpoint {
	// Simple round-robin for now
	// TODO: could be enhanced with weighted selection
	return endpoints[0]
}
