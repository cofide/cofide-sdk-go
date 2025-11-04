// Copyright 2024 Cofide Limited.
// SPDX-License-Identifier: Apache-2.0

package id

import (
	"reflect"
	"testing"

	"github.com/spiffe/go-spiffe/v2/spiffeid"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestParseID(t *testing.T) {
	type args struct {
		id string
	}
	tests := []struct {
		name    string
		args    args
		want    *SPIFFEID
		wantErr bool
	}{
		{
			name: "test parse of a spiffe ID",
			args: args{
				id: "spiffe://example.com/ns/default/sa/default",
			},
			want: &SPIFFEID{
				id: spiffeid.RequireFromPath(spiffeid.RequireTrustDomainFromString("example.com"), "/ns/default/sa/default"),
			},
		},
		{
			name: "test parse of a spiffe ID with incorrect path KV pairs",
			args: args{
				id: "spiffe://example.com/ns/default/sa",
			},
			wantErr: true,
		},
		{
			name: "test parse of a spiffe ID with incorrect path slashes",
			args: args{
				id: "spiffe://example.com/ns/default///sa",
			},
			wantErr: true,
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got, err := ParseID(tt.args.id)
			if (err != nil) != tt.wantErr {
				t.Errorf("ParseID() error = %v, wantErr %v", err, tt.wantErr)
				return
			}
			if !reflect.DeepEqual(got, tt.want) {
				t.Errorf("ParseID() = %v, want %v", got, tt.want)
			}
		})
	}
}

func TestNewID(t *testing.T) {
	type args struct {
		trustDomain string
		kv          map[string]string
	}
	tests := []struct {
		name    string
		args    args
		want    *SPIFFEID
		wantErr bool
	}{
		{
			name: "Create SPIFFE ID",
			args: args{
				trustDomain: "example.com",
				kv:          map[string]string{"ns": "test", "sa": "default"},
			},
			want: &SPIFFEID{
				id: spiffeid.RequireFromPath(spiffeid.RequireTrustDomainFromString("example.com"), "/ns/test/sa/default"),
			},
		},
		{
			name: "Create SPIFFE ID with empty trust domain",
			args: args{
				trustDomain: "",
				kv:          map[string]string{"ns": "test", "sa": "default"},
			},
			wantErr: true,
		},
		{
			name: "Create SPIFFE ID with empty key value",
			args: args{
				trustDomain: "example.com",
				kv:          map[string]string{},
			},
			want: &SPIFFEID{
				id: spiffeid.RequireFromPath(spiffeid.RequireTrustDomainFromString("example.com"), ""),
			},
		},
		{
			name: "Create SPIFFE ID with nil key value",
			args: args{
				trustDomain: "example.com",
				kv:          nil,
			},
			want: &SPIFFEID{
				id: spiffeid.RequireFromPath(spiffeid.RequireTrustDomainFromString("example.com"), ""),
			},
		},
		{
			name: "Create SPIFFE ID with empty key",
			args: args{
				trustDomain: "example.com",
				kv:          map[string]string{"": "test"},
			},
			wantErr: true,
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got, err := NewID(tt.args.trustDomain, tt.args.kv)
			if (err != nil) != tt.wantErr {
				t.Errorf("NewID() error = %v, wantErr %v", err, tt.wantErr)
				return
			}
			if !reflect.DeepEqual(got, tt.want) {
				t.Errorf("NewID() = %v, want %v", got, tt.want)
			}
		})
	}
}

func TestWIMSEID(t *testing.T) {
	tests := []struct {
		name string
		kv   map[string]string
		want string
	}{
		{
			name: "basic",
			kv:   map[string]string{"foo": "bar"},
			want: "wimse://example.org/foo/bar",
		},
		{
			name: "empty",
			kv:   map[string]string{},
			want: "wimse://example.org",
		},
	}
	for _, tt := range tests {
		spiffeID, err := NewID("example.org", tt.kv)
		require.NoError(t, err)
		assert.Equal(t, tt.want, spiffeID.WIMSEIDString())
	}
}
