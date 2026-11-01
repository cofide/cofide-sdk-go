# cofide-sdk-go

Go SDK for workloads that authenticate with [SPIFFE](https://spiffe.io/) identity. It
wraps `net/http` clients and servers with mutual TLS using the workload's X.509-SVID
from the SPIFFE Workload API, and adds helpers for building, parsing, and authorizing
SPIFFE IDs.

> Experimental. The API may change.

## Features

- **Mutual TLS clients**: a drop-in replacement for `http.Client` that presents the
  workload's X.509-SVID, rotates it automatically, and authorizes servers by SPIFFE ID.
- **Mutual TLS servers**: wraps an `http.Server` so that it serves TLS with the
  workload's X.509-SVID and authorizes clients by SPIFFE ID.
- **SPIFFE IDs**: build and parse key/value SPIFFE IDs such as
  `spiffe://example.org/ns/production/sa/billing`, and match them with composable
  rules (`Equals`, `MatchGlob`, `Or`, `Not`, …).
- **Endpoint discovery** (experimental): resolve the client's upstream hosts through an
  xDS server.

## Usage

Call a server over mutual TLS, authorizing it by SPIFFE ID:

```go
package main

import (
	"log"

	cofidehttp "github.com/cofide/cofide-sdk-go/http/client"
	"github.com/cofide/cofide-sdk-go/pkg/id"
)

func main() {
	// Blocks until the Workload API has issued an X.509-SVID.
	client, err := cofidehttp.NewClient(
		cofidehttp.WithSVIDMatch(id.Equals("ns", "production"), id.Equals("sa", "billing")),
	)
	if err != nil {
		log.Fatal(err)
	}

	resp, err := client.Get("https://billing.example.org/things")
	if err != nil {
		log.Fatal(err)
	}
	defer resp.Body.Close()
}
```

Serve over mutual TLS, authorizing clients by SPIFFE ID:

```go
package main

import (
	"log"
	"net/http"

	cofidehttpserver "github.com/cofide/cofide-sdk-go/http/server"
	"github.com/cofide/cofide-sdk-go/pkg/id"
	"github.com/spiffe/go-spiffe/v2/svid/x509svid"
)

func main() {
	mux := http.NewServeMux()
	mux.HandleFunc("/things", func(w http.ResponseWriter, r *http.Request) {
		peer, err := x509svid.IDFromCert(r.TLS.PeerCertificates[0])
		if err != nil {
			http.Error(w, "not an X.509-SVID", http.StatusUnauthorized)
			return
		}
		log.Printf("request from %s", peer)
	})

	server := cofidehttpserver.NewServer(
		&http.Server{Addr: ":8443", Handler: mux},
		cofidehttpserver.WithSVIDMatch(id.Equals("ns", "production")),
	)
	// Always serves TLS; the certificate and key come from the Workload API.
	log.Fatal(server.ListenAndServe())
}
```

Without `WithSVIDMatch`, clients and servers accept any peer whose X.509-SVID chains to
the workload's trust bundle. Name the peers you expect.

The client upgrades `http://` URLs to `https://`. Both the client and the server expose
the workload's own SPIFFE ID with `GetIdentity()`.

## SPIFFE IDs

A common convention for SPIFFE ID paths is for them to be key/value pairs `spiffe://<trust domain>/<key 1>/<value 1>/<key 2>/<value 2>`.
The `pkg/id` provides utilities to construct, parse and verify the path of such a SPIFFE ID.

```go
sid, err := id.NewID("example.org", map[string]string{"ns": "production", "sa": "billing"})
// spiffe://example.org/ns/production/sa/billing (keys are sorted)

err = sid.Matches(
	id.Equals("ns", "production"),
	id.Or(id.Equals("sa", "billing"), id.MatchGlob("sa", "billing-*")),
	id.IsEmpty("cluster"),
)
```

| Matcher | Matches IDs where |
|---|---|
| `Equals(key, value)` | `key` has exactly `value` |
| `MatchGlob(key, pattern)` | `key` is present and matches a [glob](https://github.com/gobwas/glob) pattern |
| `IsEmpty(key)` | `key` is absent or empty |
| `IsNotEmpty(key)` | `key` has a value |
| `Or(matchers...)` | any of `matchers` match |
| `Not(matcher)` | `matcher` does not match |

`id.AuthorizeMatch(matchers...)` turns matchers into a go-spiffe
[`tlsconfig.Authorizer`](https://pkg.go.dev/github.com/spiffe/go-spiffe/v2/spiffetls/tlsconfig#Authorizer)
for use outside this SDK. `WithSVIDMatch` uses it. All matchers must match.

## Install

Requires Go 1.26 or later.

```sh
go get github.com/cofide/cofide-sdk-go
```

## Configuration

Clients and servers are configured with options. The Workload API address can also be
set from the environment.

| Variable | Option | Purpose |
|---|---|---|
| `SPIFFE_ENDPOINT_SOCKET` | `WithSPIREAddress` | Workload API socket; default `unix:///tmp/spire.sock` |
| | `WithContext` | Context for the Workload API connection; default `context.Background()` |
| | `WithSVIDMatch` | Authorize peers by SPIFFE ID; default: any peer in the trust bundle |
| | `WithXDS` | Client only: xDS server URI for endpoint discovery |
| | `WithXDSNodeID` | Client only: xDS node ID; default `node` |

The SDK retries the Workload API with backoff until it issues an X.509-SVID.
`NewClient` waits for it. A server waits when it starts serving.

### Endpoint discovery

With `WithXDS`, the client subscribes to the xDS server's endpoints for each host it
calls (cluster `<host>_cluster`). It dials a discovered endpoint and keeps the hostname
for TLS. Discovery starts on the first request to a host, so until endpoints arrive, or
when none are known, the client dials the host directly.

## Development

```sh
just test        # run the tests
just test-race   # with the race detector
just lint        # golangci-lint
```

## Licence

Apache-2.0. See [LICENSE](LICENSE).

Copyright 2024 Cofide Limited.
