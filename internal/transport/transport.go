package transport

import (
	"context"
	"crypto/tls"
	"fmt"
	"net"
	"net/http"

	"github.com/cofide/cofide-sdk-go/internal/xds"
)

type CofideTransport struct {
	xdsClient     *xds.XDSClient
	baseTransport http.RoundTripper
}

func NewCofideTransport(client *xds.XDSClient, tlsConfig *tls.Config) *CofideTransport {
	// Create a transport with a custom dialer
	baseTransport := &http.Transport{
		TLSClientConfig: tlsConfig,
		// Create a custom dialer that handles hostname resolution
		DialContext: func(ctx context.Context, network, addr string) (net.Conn, error) {
			// Extract host and port
			host, _, err := net.SplitHostPort(addr)
			if err != nil {
				// Fall back to standard dialing
				dialer := &net.Dialer{}
				return dialer.DialContext(ctx, network, addr)
			}

			// Try to resolve endpoint
			endpoints, err := client.GetEndpoints(host)
			if err != nil || len(endpoints) == 0 {
				// Fall back to standard dialing
				dialer := &net.Dialer{}
				return dialer.DialContext(ctx, network, addr)
			}

			// Select endpoint
			endpoint := selectEndpoint(endpoints)

			// Dial using resolved endpoint
			dialer := &net.Dialer{}
			return dialer.DialContext(ctx, network, fmt.Sprintf("%s:%d", endpoint.Host, endpoint.Port))
		},
	}

	return &CofideTransport{
		xdsClient:     client,
		baseTransport: baseTransport,
	}
}

func (t *CofideTransport) RoundTrip(req *http.Request) (*http.Response, error) {
	// The ServerName in TLS config will be automatically set to req.URL.Hostname()
	// by the http.Transport implementation
	return t.baseTransport.RoundTrip(req)
}

func selectEndpoint(endpoints []xds.Endpoint) xds.Endpoint {
	// Simple round-robin for now
	// TODO: could be enhanced with weighted selection
	return endpoints[0]
}
