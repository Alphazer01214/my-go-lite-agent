package app

import (
	"fmt"
	"os"
	"os/signal"
	"strings"
	"syscall"

	"github.com/tomori/my-go-lite-agent/assembly"
	"github.com/tomori/my-go-lite-agent/discovery"
	"github.com/tomori/my-go-lite-agent/serve"
)

// runREPL mounts Plugins and runs an interactive multi-turn loop on one Session.
func runREPL(pluginsDir, assemblyPath string, dump *bool) error {
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
		dumpAssembly(plan)
	}
	srv, err := serve.Start(plan.Mounted)
	if err != nil {
		return err
	}
	defer func() { _ = srv.Close() }()

	probeCommandFaces(srv, plan.Mounted)

	cp := newCommandPlane(srv, pluginsDir, plan)

	// Clean shutdown on Ctrl+C so plugin processes are not orphaned.
	sigCh := make(chan os.Signal, 1)
	signal.Notify(sigCh, os.Interrupt, syscall.SIGTERM)
	go func() {
		<-sigCh
		fmt.Println("\nshutting down…")
		_ = srv.Close()
		os.Exit(0)
	}()

	return runREPLLoop(srv, cp)
}

// runREPLLoop is the interactive stdin loop (shared by -repl and -serve -repl).
func runREPLLoop(srv *serve.Server, cp *commandPlane) error {
	fmt.Println("lite agent REPL — type a message; /help for commands; Tab completes /commands; /exit to leave.")
	for {
		line, err := readLineRaw("> ", cp.completeSlash)
		if err != nil {
			if err.Error() == "interrupted" {
				fmt.Println("\nshutting down…")
				_ = srv.Close()
				return nil
			}
			fmt.Println()
			break
		}
		line = strings.TrimSpace(line)
		if line == "" {
			continue
		}
		if strings.HasPrefix(line, "/") {
			quit, err := cp.handle(line)
			if err != nil {
				fmt.Fprintf(os.Stderr, "error: %v\n", err)
				continue
			}
			if quit {
				break
			}
			continue
		}
		r := &turnRenderer{}
		restore := wireRenderer(srv, r)
		r.begin()
		out, err := srv.RunTurn(line)
		if err != nil {
			r.end("")
			restore()
			fmt.Fprintf(os.Stderr, "error: %v\n", err)
			continue
		}
		r.end(out.Assistant)
		restore()
	}
	return nil
}
