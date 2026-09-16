//go:build !windows

package app

// enableVirtualTerminal is a no-op outside Windows (ANSI is already active).
func enableVirtualTerminal() {}
