package config_test

import (
	"strings"
	"testing"

	"github.com/AlvaroQ/engram-explorer/internal/config"
)

func TestLoad_RefusesNonLoopbackHostByDefault(t *testing.T) {
	for _, host := range []string{"0.0.0.0", "::", "192.168.1.10", "example.com"} {
		t.Run(host, func(t *testing.T) {
			t.Setenv("DASHBOARD_HOST", host)
			t.Setenv("DASHBOARD_ALLOW_REMOTE", "")
			_, err := config.Load()
			if err == nil {
				t.Fatalf("Load() with DASHBOARD_HOST=%q: expected error, got nil", host)
			}
			msg := err.Error()
			for _, want := range []string{"DASHBOARD_HOST", "not a loopback address", "DASHBOARD_ALLOW_REMOTE=true"} {
				if !strings.Contains(msg, want) {
					t.Errorf("error %q missing %q", msg, want)
				}
			}
		})
	}
}

func TestLoad_AcceptsLoopbackHosts(t *testing.T) {
	for _, host := range []string{"127.0.0.1", "127.0.0.2", "::1", "localhost", "LOCALHOST"} {
		t.Run(host, func(t *testing.T) {
			t.Setenv("DASHBOARD_HOST", host)
			cfg, err := config.Load()
			if err != nil {
				t.Fatalf("Load() with DASHBOARD_HOST=%q: unexpected error %v", host, err)
			}
			if cfg.Host != host || cfg.AllowRemote {
				t.Errorf("Host=%q AllowRemote=%v, want %q false", cfg.Host, cfg.AllowRemote, host)
			}
		})
	}
}

func TestLoad_AllowRemoteOverride(t *testing.T) {
	t.Setenv("DASHBOARD_HOST", "0.0.0.0")
	t.Setenv("DASHBOARD_ALLOW_REMOTE", "true")
	cfg, err := config.Load()
	if err != nil {
		t.Fatalf("Load() unexpected error: %v", err)
	}
	if !cfg.AllowRemote || cfg.Host != "0.0.0.0" {
		t.Errorf("AllowRemote=%v Host=%q, want true 0.0.0.0", cfg.AllowRemote, cfg.Host)
	}
}

func TestIsLoopbackHost(t *testing.T) {
	cases := map[string]bool{
		"127.0.0.1": true, "::1": true, "localhost": true, "Localhost": true,
		"0.0.0.0": false, "::": false, "": false, "10.0.0.1": false, "localhost.evil.com": false, "myhost.lan": false, "::ffff:127.0.0.1": true,
	}
	for host, want := range cases {
		if got := config.IsLoopbackHost(host); got != want {
			t.Errorf("IsLoopbackHost(%q) = %v, want %v", host, got, want)
		}
	}
}
