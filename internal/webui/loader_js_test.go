package webui

import (
	"os/exec"
	"path/filepath"
	"testing"
)

func TestLoaderJavaScript(t *testing.T) {
	node, err := exec.LookPath("node")
	if err != nil {
		t.Skip("node is not installed")
	}

	command := exec.Command(node, "--test", filepath.Join(".", "loader.test.mjs"))
	output, err := command.CombinedOutput()
	if err != nil {
		t.Fatalf("loader tests failed: %v\n%s", err, output)
	}
}
