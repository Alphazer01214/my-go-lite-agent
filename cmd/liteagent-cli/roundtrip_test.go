package main_test

import (
	"os/exec"
	"strings"
	"testing"
)

func TestHostEchoRoundtrip(t *testing.T) {
	root := moduleRoot(t)
	hostBin := buildPkg(t, root, "./cmd/host")
	echoBin := buildPkg(t, root, "./plugins/echo")

	cmd := exec.Command(hostBin, "-plugin", echoBin)
	cmd.Dir = root
	out, err := cmd.CombinedOutput()
	if err != nil {
		t.Fatalf("host exit: %v\n%s", err, out)
	}
	s := string(out)
	if !strings.Contains(s, "ok id=1") {
		t.Fatalf("missing ok line: %s", s)
	}
	if !strings.Contains(s, `"hello":"world"`) {
		t.Fatalf("missing echoed payload: %s", s)
	}
	if strings.Contains(s, "host:") {
		t.Fatalf("unexpected host error output: %s", s)
	}
}

func TestHostMissingFlags(t *testing.T) {
	root := moduleRoot(t)
	hostBin := buildPkg(t, root, "./cmd/host")

	cmd := exec.Command(hostBin)
	out, err := cmd.CombinedOutput()
	if err == nil {
		t.Fatalf("want non-zero exit, got success: %s", out)
	}
	if !strings.Contains(string(out), "-plugin") && !strings.Contains(string(out), "-discover") {
		t.Fatalf("want usage hint, got: %s", out)
	}
}

