package assembly

import (
	"os"
	"path/filepath"
	"testing"

	"github.com/tomori/my-go-lite-agent/discovery"
	"github.com/tomori/my-go-lite-agent/plugin"
)

func TestResolveMountSet(t *testing.T) {
	res := discovery.Result{
		Plugins: []discovery.Found{
			{Dir: "/p/a", Manifest: plugin.Manifest{Name: "alpha", Version: "0.1.0", Protocol: 1, Entry: "a"}},
			{Dir: "/p/b", Manifest: plugin.Manifest{Name: "beta", Version: "0.1.0", Protocol: 1, Entry: "b"}},
		},
	}
	plan := Resolve(Config{Plugins: []string{"alpha", "ghost"}}, res)

	if len(plan.Mounted) != 1 || plan.Mounted[0].Manifest.Name != "alpha" {
		t.Fatalf("mounted: %+v", plan.Mounted)
	}
	if len(plan.Unmounted) != 1 || plan.Unmounted[0].Manifest.Name != "beta" {
		t.Fatalf("unmounted: %+v", plan.Unmounted)
	}
	if len(plan.Missing) != 1 || plan.Missing[0] != "ghost" {
		t.Fatalf("missing: %+v", plan.Missing)
	}
}

func TestLoadAssembly(t *testing.T) {
	dir := t.TempDir()
	path := filepath.Join(dir, "assembly.json")
	if err := os.WriteFile(path, []byte(`{"plugins":["alpha"]}`), 0o644); err != nil {
		t.Fatal(err)
	}
	cfg, err := Load(path)
	if err != nil {
		t.Fatal(err)
	}
	if len(cfg.Plugins) != 1 || cfg.Plugins[0] != "alpha" {
		t.Fatalf("cfg: %+v", cfg)
	}
}
