package app

import (
	"fmt"
	"net"
	"os"
	"os/signal"
	"strings"
	"syscall"

	"github.com/tomori/my-go-lite-agent/assembly"
	"github.com/tomori/my-go-lite-agent/discovery"
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
func runWebAndOptionalREPL(pluginsDir, assemblyPath, addr string, withREPL bool, dump *bool) error {
	cfg, err := assembly.Load(assemblyPath)
	if err != nil {
		return err
	}
	res := discovery.Scan(pluginsDir)
	if len(res.Errors) > 0 {
		printDiscovery(res)
		return fmt.Errorf("discovery failed before assembly")
	}
	plan := assembly.Resolve(cfg, res)
	printRejected(plan.Rejected)
	if len(plan.Missing) > 0 {
		return fmt.Errorf("assembly references unknown plugins: %s", strings.Join(plan.Missing, ", "))
	}
	if dump != nil && *dump {
		dumpAssembly(plan, res)
	}
	srv, err := serve.Start(plan.Mounted)
	if err != nil {
		return err
	}
	defer func() { _ = srv.Close() }()

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
		Addr:         bind,
		PluginsDir:   pluginsDir,
		Plan:         plan,
		Srv:          srv,
		CommandPlane: webCommandPlane{cp: cp},
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
