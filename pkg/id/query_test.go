// Copyright 2024 Cofide Limited.
// SPDX-License-Identifier: Apache-2.0

package id

import (
	"testing"
)

func TestSPIFFEID_Matches(t *testing.T) {
	type args struct {
		funcs []MatchFunc
	}
	tests := []struct {
		name    string
		id      *SPIFFEID
		args    args
		wantErr bool
	}{
		{
			name: "Simple KV match",
			id:   MustParseID("spiffe://example.org/key1/value1/key2/value2"),
			args: args{
				funcs: []MatchFunc{
					Equals("key1", "value1"),
					Equals("key2", "value2"),
				},
			},
			wantErr: false,
		},
		{
			name: "Simple KV mismatch",
			id:   MustParseID("spiffe://example.org/key1/value1/key2/value2"),
			args: args{
				funcs: []MatchFunc{
					Equals("key1", "value1"),
					Equals("key2", "value3"),
				},
			},
			wantErr: true,
		},
		{
			name: "Simple OR",
			id:   MustParseID("spiffe://example.org/ns/kube-system/sa/default/deploy/coredns"),
			args: args{
				funcs: []MatchFunc{
					Equals("ns", "kube-system"),
					Or(Equals("deploy", "kube-dns"), Equals("deploy", "coredns")),
				},
			},
			wantErr: false,
		},
		{
			name: "Simple OR mismatch",
			id:   MustParseID("spiffe://example.org/ns/kube-system/sa/default/deploy/coredns"),
			args: args{
				funcs: []MatchFunc{
					Equals("ns", "kube-system"),
					Or(Equals("deploy", "kube-dns"), Equals("deploy", "kube-proxy")),
				},
			},
			wantErr: true,
		},
		{
			name: "Simple Glob",
			id:   MustParseID("spiffe://example.org/ns/kube-system/sa/default/deploy/coredns"),
			args: args{
				funcs: []MatchFunc{
					MatchGlob("deploy", "core*"),
				},
			},
			wantErr: false,
		},
		{
			name: "Simple Glob mismatch",
			id:   MustParseID("spiffe://example.org/ns/kube-system/sa/default/deploy/coredns"),
			args: args{
				funcs: []MatchFunc{
					MatchGlob("deploy", "kube*"),
				},
			},
			wantErr: true,
		},
		{
			name: "Simple isEmpty",
			id:   MustParseID("spiffe://example.org/ns/kube-system/sa/default/deploy/coredns"),
			args: args{
				funcs: []MatchFunc{
					IsEmpty("cluster"),
				},
			},
			wantErr: false,
		},
		{
			name: "Simple isEmpty mismatch",
			id:   MustParseID("spiffe://example.org/ns/kube-system/sa/default/deploy/coredns"),
			args: args{
				funcs: []MatchFunc{
					IsEmpty("ns"),
				},
			},
			wantErr: true,
		},
		{
			name: "Simple isNotEmpty",
			id:   MustParseID("spiffe://example.org/ns/kube-system/sa/default/deploy/coredns"),
			args: args{
				funcs: []MatchFunc{
					IsNotEmpty("ns"),
				},
			},
			wantErr: false,
		},
		{
			name: "Simple isNotEmpty mismatch",
			id:   MustParseID("spiffe://example.org/ns/kube-system/sa/default/deploy/coredns"),
			args: args{
				funcs: []MatchFunc{
					IsNotEmpty("cluster"),
				},
			},
			wantErr: true,
		},
		{
			name: "Simple Not",
			id:   MustParseID("spiffe://example.org/ns/kube-system/sa/default/deploy/coredns"),
			args: args{
				funcs: []MatchFunc{
					Not(IsEmpty("ns")),
				},
			},
			wantErr: false,
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			s := tt.id
			err := s.Matches(tt.args.funcs...)
			if (err != nil) != tt.wantErr {
				t.Errorf("SPIFFEID.Matches() error = %v, wantErr %v", err, tt.wantErr)
				return
			}
		})
	}
}
