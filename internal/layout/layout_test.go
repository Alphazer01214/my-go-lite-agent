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
	    {"slug":"main","title":"Main","path":"/","slots":[
	      {"id":"top","role":"panel","region":"top"},
	      {"id":"bottom","role":"panel","region":"bottom"},
	      {"id":"left","role":"panel","region":"left"},
	      {"id":"center","role":"panel","region":"center"},
	      {"id":"right","role":"panel","region":"right"}
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
	if len(d.Pages) != 1 {
		t.Fatalf("pages=%d", len(d.Pages))
	}
	if len(d.Pages[0].Slots) != 5 || d.Pages[0].Slots[3].ID != "center" {
		t.Fatalf("slots=%v", d.Pages[0].Slots)
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
		{Slug: "main", Path: "/", Slots: []Slot{{ID: "left", Role: "panel"}}},
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
	var main *Page
	for i := range m.Pages {
		if m.Pages[i].Slug == "main" {
			main = &m.Pages[i]
		}
	}
	if main == nil || len(main.Slots) != 2 || main.Slots[1].ID != "addon" {
		t.Fatalf("main slots=%v", m.Pages)
	}
	hasTools := false
	for _, p := range m.Pages {
		if p.Slug == "tools" {
			hasTools = true
		}
	}
	if !hasTools {
		t.Fatal("contributed page missing")
	}
}

func TestMergeRejectsRedefiningBaseSlot(t *testing.T) {
	base := Doc{Pages: []Page{
		{Slug: "main", Path: "/", Slots: []Slot{{ID: "left", Role: "panel"}}},
	}}
	_, err := Merge(base, []Contribution{{
		Plugin: "evil",
		Pages:  []Page{{Slug: "main", Path: "/", Slots: []Slot{{ID: "left", Role: "other"}}}},
	}})
	if err == nil {
		t.Fatal("expected redefine rejection")
	}
}
