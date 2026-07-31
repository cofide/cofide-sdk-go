// Copyright 2026 Cofide Limited.
// SPDX-License-Identifier: Apache-2.0

package credex_test

import (
	"context"
	"log"

	"github.com/spiffe/go-spiffe/v2/workloadapi"

	"github.com/cofide/cofide-sdk-go/oauth/credex"
)

func ExampleConfig_Client() {
	ctx := context.Background()

	jwtSource, err := workloadapi.NewJWTSource(ctx)
	if err != nil {
		log.Fatal(err)
	}
	defer func() { _ = jwtSource.Close() }()

	config := &credex.Config{
		TokenURL: "https://credex.example.com/token",
		Audience: "legacy-api",
		Scopes:   []string{"data:read"},
	}
	client, err := config.Client(ctx, jwtSource)
	if err != nil {
		log.Fatal(err)
	}

	// The client obtains and refreshes a downstream OAuth access token through
	// Credex, then adds it to requests as an Authorization bearer token.
	_, _ = client.Get("https://legacy-api.example.com/data")
}
