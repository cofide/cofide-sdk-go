package transport

import (
	"crypto/tls"
	"fmt"
	"net"
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
		xdsClient: client,
		tlsConfig: tlsConfig,
		baseTransport: &http.Transport{
			DialTLS: func(network, addr string) (net.Conn, error) {
				// Extract hostname from addr for SNI
				host, _, err := net.SplitHostPort(addr)
				if err != nil {
					host = addr
				}

				// Check if we have a custom resolution for this host
				endpoints, err := client.GetEndpoints(host)
				if err == nil && len(endpoints) > 0 {
					endpoint := selectEndpoint(endpoints)

					// Clone the TLS config to avoid modifying the original
					customTLSConfig := tlsConfig.Clone()

					// Set ServerName for SNI to the original hostname
					customTLSConfig.ServerName = host

					// Connect to resolved IP:port instead but use original hostname for SNI
					targetAddr := fmt.Sprintf("%s:%d", endpoint.Host, endpoint.Port)
					conn, err := tls.Dial(network, targetAddr, customTLSConfig)
					return conn, err
				}

				// Fall back to standard behavior with the original TLS config
				// but make sure ServerName is set correctly
				customTLSConfig := tlsConfig.Clone()
				customTLSConfig.ServerName = host
				return tls.Dial(network, addr, customTLSConfig)
			},
		},
	}
}

func (t *CofideTransport) RoundTrip(req *http.Request) (*http.Response, error) {
	return t.baseTransport.RoundTrip(req)
}

func selectEndpoint(endpoints []xds.Endpoint) xds.Endpoint {
	// Simple round-robin for now
	// TODO: could be enhanced with weighted selection
	return endpoints[0]
}
