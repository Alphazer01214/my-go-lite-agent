package main_test

import (
	"os/exec"
	"strings"
	"testing"
)

func TestHostMissingFlags(t *testing.T) {
	root := moduleRoot(t)
	hostBin := buildPkg(t, root, "./cmd/liteagent-cli")

	cmd := exec.Command(hostBin)
	out, err := cmd.CombinedOutput()
	if err == nil {
		t.Fatalf("want non-zero exit, got success: %s", out)
	}
	if !strings.Contains(string(out), "-plugin") && !strings.Contains(string(out), "-discover") {
		t.Fatalf("want usage hint, got: %s", out)
	}
}
