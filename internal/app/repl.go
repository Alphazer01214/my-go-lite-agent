package app

import (
	"fmt"
	"os"
	"os/signal"
	"strings"
	"syscall"

	"github.com/tomori/my-go-lite-agent/serve"
)

// runREPL mounts Plugins and runs an interactive multi-turn loop on one Session.
func runREPL(pluginsDir, assemblyPath string, dump *bool, workspace string) error {
	plan, _, err := resolveAssembly(pluginsDir, assemblyPath, dump != nil && *dump)
	if err != nil {
		return err
	}
	srv, err := startMounted(pluginsDir, plan)
	if err != nil {
		return err
	}
	defer func() { _ = srv.Close() }()

	if workspace != "" {
		// Session 初值 (ADR-0020): soft — session plugin may be absent.
		_, _ = srv.CallByCap(serve.SessionCap, "create", serve.MarshalPayload(map[string]any{
			"sessionId": "default", "workspace": workspace,
		}))
	}
	serve.RegisterApproval(srv, cliToolApproval)

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
	sch := agentSchemeLabel(srv)
	banner := "lite agent REPL — type a message; /help for commands; Tab completes /commands; /exit to leave."
	if sch != "" {
		banner = fmt.Sprintf("lite agent REPL — scheme: %s (/agent config set defaultScheme=… to switch)\n%s", sch, banner)
	}
	fmt.Println(banner)
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
			// Surface scheme changes immediately after /agent config set.
			if strings.HasPrefix(line, "/agent ") {
				if sch := agentSchemeLabel(srv); sch != "" {
					fmt.Printf("agent scheme: %s\n", sch)
				}
			}
			continue
		}
		r := &turnRenderer{}
		restore := wireRenderer(srv, r)
		r.begin()
		if sch := agentSchemeLabel(srv); sch != "" {
			fmt.Printf("[scheme: %s]\n", sch)
		}
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
