package main

import (
	"context"
	"fmt"
	"net"
	"net/http"
	"time"
)

func newSafeClient() *http.Client {
	transport := &http.Transport{
		Proxy:                 nil,
		ForceAttemptHTTP2:     true,
		MaxIdleConns:          16,
		IdleConnTimeout:       30 * time.Second,
		TLSHandshakeTimeout:   10 * time.Second,
		ResponseHeaderTimeout: 15 * time.Second,
		DialContext:           safeDialContext,
	}
	return &http.Client{Transport: transport, CheckRedirect: validateRedirect}
}

func validateRedirect(req *http.Request, via []*http.Request) error {
	if len(via) >= 5 {
		return errRedirectLimit
	}
	_, err := validateURL(req.URL.String())
	return err
}

func safeDialContext(ctx context.Context, network, address string) (net.Conn, error) {
	host, port, err := net.SplitHostPort(address)
	if err != nil {
		return nil, err
	}
	addresses, err := net.DefaultResolver.LookupIPAddr(ctx, host)
	if err != nil {
		return nil, err
	}
	if len(addresses) == 0 {
		return nil, fmt.Errorf("no addresses for %s", host)
	}
	var selected net.IP
	for _, address := range addresses {
		if !publicIP(address.IP) {
			return nil, fmt.Errorf("address_blocked: %s", address.IP)
		}
		if selected == nil {
			selected = address.IP
		}
	}
	dialer := net.Dialer{Timeout: 10 * time.Second, KeepAlive: 30 * time.Second}
	return dialer.DialContext(ctx, network, net.JoinHostPort(selected.String(), port))
}

func publicIP(ip net.IP) bool {
	if ip == nil {
		return false
	}
	if v4 := ip.To4(); v4 != nil {
		if v4[0] == 0 || v4[0] == 10 || v4[0] == 127 {
			return false
		}
		if v4[0] == 169 && v4[1] == 254 {
			return false
		}
		if v4[0] == 172 && v4[1] >= 16 && v4[1] <= 31 {
			return false
		}
		if v4[0] == 192 && v4[1] == 168 {
			return false
		}
		if v4[0] == 100 && v4[1] >= 64 && v4[1] <= 127 {
			return false
		}
		if v4[0] == 192 && v4[1] == 0 && v4[2] == 0 {
			return false
		}
		if v4[0] == 198 && (v4[1] == 18 || v4[1] == 19) {
			return false
		}
		if v4[0] >= 224 {
			return false
		}
		return true
	}
	if ip.IsLoopback() || ip.IsUnspecified() || ip.IsPrivate() ||
		ip.IsLinkLocalUnicast() || ip.IsLinkLocalMulticast() || ip.IsMulticast() {
		return false
	}
	return true
}
