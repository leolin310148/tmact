package main

import "testing"

func TestResolvePprofAddr(t *testing.T) {
	cases := []struct {
		name    string
		flag    string
		flagSet bool
		env     string
		want    string
		wantErr bool
	}{
		{name: "off by default"},
		{name: "ipv4 loopback flag", flag: "127.0.0.1:6061", flagSet: true, want: "127.0.0.1:6061"},
		{name: "ipv6 loopback flag", flag: "[::1]:6061", flagSet: true, want: "[::1]:6061"},
		{name: "localhost flag", flag: "localhost:6061", flagSet: true, want: "localhost:6061"},
		{name: "env fallback", env: "127.0.0.1:6062", want: "127.0.0.1:6062"},
		{name: "explicit empty flag beats env", flag: "", flagSet: true, env: "127.0.0.1:6062", want: ""},
		{name: "wildcard rejected", flag: "0.0.0.0:6061", flagSet: true, wantErr: true},
		{name: "empty host rejected", flag: ":6061", flagSet: true, wantErr: true},
		{name: "lan ip rejected", env: "100.120.181.54:6061", wantErr: true},
		{name: "missing port rejected", flag: "127.0.0.1", flagSet: true, wantErr: true},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			t.Setenv(statusdPprofEnv, tc.env)
			got, err := resolvePprofAddr(tc.flag, tc.flagSet)
			if tc.wantErr {
				if err == nil {
					t.Fatalf("want error, got %q", got)
				}
				return
			}
			if err != nil || got != tc.want {
				t.Fatalf("got %q, %v; want %q", got, err, tc.want)
			}
		})
	}
}
