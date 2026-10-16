package main

import "testing"

func TestHTTPAddress(t *testing.T) {
	t.Setenv("LITEAGENT_HTTP_ADDR", "")
	if got := httpAddress(); got != "127.0.0.1:2333" {
		t.Fatalf("default address = %q", got)
	}

	t.Setenv("LITEAGENT_HTTP_ADDR", "127.0.0.1:9999")
	if got := httpAddress(); got != "127.0.0.1:9999" {
		t.Fatalf("override address = %q", got)
	}
}

func TestServerURL(t *testing.T) {
	for _, test := range []struct {
		address string
		want    string
	}{
		{"127.0.0.1:2333", "http://127.0.0.1:2333/"},
		{"localhost:9999", "http://localhost:9999/"},
		{"[::1]:2333", "http://[::1]:2333/"},
	} {
		if got := serverURL(test.address); got != test.want {
			t.Fatalf("serverURL(%q) = %q, want %q", test.address, got, test.want)
		}
	}
}
