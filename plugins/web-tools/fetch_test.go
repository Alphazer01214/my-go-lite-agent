package main

import (
	"context"
	"net"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	"github.com/tomori/my-go-lite-agent/pluginsdk"
	"github.com/tomori/my-go-lite-agent/pluginsdk/toolcontract"
)

func TestPublicIPRejectsPrivateAndMetadata(t *testing.T) {
	for _, value := range []string{"127.0.0.1", "10.0.0.1", "172.16.0.1", "192.168.1.1",
		"169.254.169.254", "100.64.0.1", "224.0.0.1", "::1", "fc00::1", "fe80::1"} {
		if publicIP(net.ParseIP(value)) {
			t.Fatalf("%s should be blocked", value)
		}
	}
	for _, value := range []string{"8.8.8.8", "1.1.1.1", "2606:4700:4700::1111"} {
		if !publicIP(net.ParseIP(value)) {
			t.Fatalf("%s should be public", value)
		}
	}
}

func TestFetchWithTestClientAndRedirect(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path == "/redirect" {
			http.Redirect(w, r, "/text", http.StatusFound)
			return
		}
		w.Header().Set("Content-Type", "text/plain; charset=utf-8")
		_, _ = w.Write([]byte("hello"))
	}))
	defer server.Close()
	client := &http.Client{CheckRedirect: validateRedirect}
	raw, err := fetchWithClient(context.Background(), fetchIn{URL: server.URL + "/redirect"}, client)
	if err != nil {
		t.Fatal(err)
	}
	result, err := toolcontract.ParseResult(raw)
	if err != nil || result.IsError || result.Content != "hello" {
		t.Fatalf("result = %+v, %v", result, err)
	}
	if result.Meta["redirects"] != float64(1) {
		t.Fatalf("meta = %+v", result.Meta)
	}
}

func TestFetchRejectsUnsupportedContentAndSizeLimit(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		w.Header().Set("Content-Type", "application/octet-stream")
		_, _ = w.Write([]byte(strings.Repeat("x", 100)))
	}))
	defer server.Close()
	raw, err := fetchWithClient(context.Background(), fetchIn{URL: server.URL}, &http.Client{})
	if err != nil {
		t.Fatal(err)
	}
	result, _ := toolcontract.ParseResult(raw)
	if !result.IsError || result.Meta["code"] != "content_type_unsupported" {
		t.Fatalf("result = %+v", result)
	}

	limitServer := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		w.Header().Set("Content-Type", "text/plain")
		_, _ = w.Write([]byte(strings.Repeat("x", 100)))
	}))
	defer limitServer.Close()
	limit := 10
	raw, err = fetchWithClient(context.Background(), fetchIn{URL: limitServer.URL, MaxBytes: &limit}, &http.Client{})
	if err != nil {
		t.Fatal(err)
	}
	result, _ = toolcontract.ParseResult(raw)
	if !result.Truncated || len(result.Content) != 10 {
		t.Fatalf("limit = %+v", result)
	}
}

func TestFetchRejectsLocalhostThroughSafeTransport(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		_, _ = w.Write([]byte("secret"))
	}))
	defer server.Close()
	_, err := fetch(context.Background(), fetchIn{URL: server.URL})
	if pluginsdk.Code(err) != "address_blocked" {
		t.Fatalf("error = %v", err)
	}
}
