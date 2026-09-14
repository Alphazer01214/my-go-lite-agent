package layout

import (
	"os"
	"path/filepath"
	"testing"
)

func TestLoadMissingFails(t *testing.T) {
	if _, err := Load(filepath.Join(t.TempDir(), "nope.json")); err == nil {
		t.Fatal("expected error for missing layout")
	}
}

func TestLoadAndValidate(t *testing.T) {
	dir := t.TempDir()
	path := filepath.Join(dir, "layout.json")
	body := `{
	  "pages": [
	    {"slug":"main","title":"Chat","path":"/","slots":[
	      {"id":"sidebar","role":"session-rail","preferred":"session-rail","region":"left"},
	      {"id":"chat","role":"session-view","preferred":"session-view","region":"center"}
	    ]},
	    {"slug":"trace","title":"Trace","path":"/trace","slots":[
	      {"id":"main","role":"session-trace","region":"main"}
	    ]}
	  ]
	}`
	if err := os.WriteFile(path, []byte(body), 0o644); err != nil {
		t.Fatal(err)
	}
	d, err := Load(path)
	if err != nil {
		t.Fatal(err)
	}
	if len(d.Pages) != 2 {
		t.Fatalf("pages=%d", len(d.Pages))
	}
	if d.Pages[0].Slots[0].Role != "session-rail" {
		t.Fatalf("role=%q", d.Pages[0].Slots[0].Role)
	}
}

func TestValidateRequiresMain(t *testing.T) {
	d := Doc{Pages: []Page{{Slug: "other", Path: "/other", Slots: []Slot{{ID: "x"}}}}}
	if err := d.Validate(); err == nil {
		t.Fatal("expected missing main error")
	}
}

func TestMergeAdditive(t *testing.T) {
	base := Doc{Pages: []Page{
		{Slug: "main", Path: "/", Slots: []Slot{{ID: "sidebar", Role: "session-rail"}}},
	}}
	m, err := Merge(base, []Contribution{{
		Plugin: "extra",
		Pages: []Page{
			{Slug: "main", Path: "/", Slots: []Slot{{ID: "addon", Role: "panel"}}},
			{Slug: "tools", Title: "Tools", Path: "/tools", Slots: []Slot{{ID: "body"}}},
		},
	}})
	if err != nil {
		t.Fatal(err)
	}
	if len(m.Pages) != 2 {
		t.Fatalf("pages=%d", len(m.Pages))
	}
	main := m.FindPage("main")
	if len(main.Slots) != 2 || main.Slots[1].ID != "addon" {
		t.Fatalf("main slots=%v", main.Slots)
	}
	if m.FindPage("tools") == nil {
		t.Fatal("contributed page missing")
	}
}

func TestMergeRejectsRedefiningBaseSlot(t *testing.T) {
	base := Doc{Pages: []Page{
		{Slug: "main", Path: "/", Slots: []Slot{{ID: "chat", Role: "session-view"}}},
	}}
	_, err := Merge(base, []Contribution{{
		Plugin: "evil",
		Pages:  []Page{{Slug: "main", Path: "/", Slots: []Slot{{ID: "chat", Role: "other"}}}},
	}})
	if err == nil {
		t.Fatal("expected redefine rejection")
	}
}
