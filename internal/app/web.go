package app

import (
	"fmt"
	"net"
	"os"
	"os/signal"
	"strings"
	"syscall"

	"github.com/tomori/my-go-lite-agent/assembly"
	"github.com/tomori/my-go-lite-agent/layout"
	"github.com/tomori/my-go-lite-agent/serve"
	"github.com/tomori/my-go-lite-agent/web"
)

type webCommandPlane struct {
	cp *commandPlane
}

func (w webCommandPlane) HandleOut(line string) (string, bool, error) {
	return w.cp.handleOut(line)
}
func (w webCommandPlane) Complete(prefix string) []string { return w.cp.completeSlash(prefix) }

// runWebAndOptionalREPL mounts Plugins, starts the Web Medium, and optionally the CLI REPL.
func runWebAndOptionalREPL(pluginsDir, assemblyPath, addr, layoutPath string, withREPL bool, dump *bool) error {
	plan, cfg, err := resolveAssembly(pluginsDir, assemblyPath, dump != nil && *dump)
	if err != nil {
		return err
	}

	if layoutPath == "" {
		layoutPath = "layout.json"
	}
	baseLayout, err := layout.Load(layoutPath)
	if err != nil {
		return err
	}
	var contribs []layout.Contribution
	for _, c := range assembly.ManifestUIContributions(plan) {
		var pages []layout.Page
		for _, p := range c.Pages {
			page := layout.Page{Slug: p.Slug, Title: p.Title, Path: p.Path}
			for _, s := range p.Slots {
				page.Slots = append(page.Slots, layout.Slot{ID: s.ID, Role: s.Role, Preferred: s.Preferred, Region: s.Region})
			}
			pages = append(pages, page)
		}
		contribs = append(contribs, layout.Contribution{Plugin: c.Plugin, Pages: pages})
	}
	merged, err := layout.Merge(baseLayout, contribs)
	if err != nil {
		return err
	}
	uiMounts, err := assembly.ResolveUIMounts(cfg, plan)
	if err != nil {
		return err
	}

	srv, err := serve.Start(plan.Mounted)
	if err != nil {
		return err
	}
	defer func() { _ = srv.Close() }()

	defaultWS := ""
	if cwd, err := os.Getwd(); err == nil {
		defaultWS = cwd
	}
	_ = srv.SetSessionWorkspace("default", defaultWS)

	probeCommandFaces(srv, plan.Mounted)
	cp := newCommandPlane(srv, pluginsDir, plan)

	bind := addr
	if strings.HasPrefix(bind, ":") {
		bind = "127.0.0.1" + bind
	}
	ln, err := net.Listen("tcp", bind)
	if err != nil {
		return fmt.Errorf("listen %s: %w", bind, err)
	}
	hs := web.New(web.Options{
		Addr:             bind,
		PluginsDir:       pluginsDir,
		Plan:             plan,
		Srv:              srv,
		CommandPlane:     webCommandPlane{cp: cp},
		Layout:           merged,
		UIMounts:         uiMounts,
		DefaultWorkspace: defaultWS,
	})
	go func() {
		if err := hs.Serve(ln); err != nil {
			fmt.Fprintf(os.Stderr, "web: %v\n", err)
		}
	}()
	fmt.Printf("web medium on http://%s\n", bind)

	sigCh := make(chan os.Signal, 1)
	signal.Notify(sigCh, os.Interrupt, syscall.SIGTERM)
	go func() {
		<-sigCh
		fmt.Println("\nshutting down…")
		_ = hs.Close()
		_ = srv.Close()
		os.Exit(0)
	}()

	if withREPL {
		return runREPLLoop(srv, cp)
	}
	select {}
}
