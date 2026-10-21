//go:build windows

package main

import (
	"strings"
	"testing"

	"golang.org/x/sys/windows"
)

func TestDecodeOutputBytes(t *testing.T) {
	utf8Input := []byte("中文测试")
	if got := decodeOutputBytes(utf8Input); string(got) != "中文测试" {
		t.Fatalf("valid UTF-8 changed: %q", got)
	}
	if windows.GetACP() != 936 {
		t.Skipf("active code page is %d, not GBK", windows.GetACP())
	}
	gbk := []byte{0xD6, 0xD0, 0xCE, 0xC4, 0xB2, 0xE2, 0xCA, 0xD4}
	got := string(decodeOutputBytes(gbk))
	if got != "中文测试" {
		t.Fatalf("gbk decode = %q", got)
	}
	if strings.ContainsRune(got, '\uFFFD') {
		t.Fatalf("gbk decode kept replacement runes: %q", got)
	}
}
