package main_test

import (
	"fmt"
	"strings"
)

// L0 diagnostic argv helpers (ADR-0030): domain startup flags were removed;
// tests address plugins by name via -invoke.

func invokeArgs(plugin, cap, method, payload string) []string {
	if payload == "" {
		payload = `{}`
	}
	return []string{
		"-invoke", plugin,
		"-frame-cap", cap,
		"-frame-method", method,
		"-invoke-payload", payload,
	}
}

func invokeTurn(input string) []string {
	return invokeArgs("agent", "loop", "turn", fmt.Sprintf(`{"input":%q}`, input))
}

func invokeSession(method, payload string) []string {
	return invokeArgs("session", "session", method, payload)
}

func invokeContext(method, payload string) []string {
	return invokeArgs("context-manager", "context", method, payload)
}

func invokeAgent(method, payload string) []string {
	return invokeArgs("agent", "agent", method, payload)
}

func hasSub(ss []string, sub string) bool {
	return strings.Contains(strings.Join(ss, " "), sub)
}
