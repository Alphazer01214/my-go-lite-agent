// Package app hosts the liteagent-server runtime: flag dispatch, assembly
// mounting helpers, the command plane, and the Web Medium launcher.
// liteagent-server is a thin main over it.
package app

import (
	"encoding/json"
	"fmt"
	"os"
	"strings"

	"github.com/tomori/my-go-lite-agent/assembly"
	"github.com/tomori/my-go-lite-agent/discovery"
	"github.com/tomori/my-go-lite-agent/serve"
)

var progName = "liteagent"

func fatal(err error) {
	fmt.Fprintf(os.Stderr, "%s: %v\n", progName, err)
	os.Exit(1)
}

// resolveAssembly scans plugins and builds the mount plan (ADR-0021).
// The daily path is Autostart roots + dependsOn closure; -assembly is a
// deprecated no-op (its file is ignored — assembly files no longer act as a
// mount whitelist). Soft-fail (ADR-0017/0022): discovery errors print but do
// not abort.
func resolveAssembly(pluginsDir, assemblyPath string, dump bool) (assembly.Plan, assembly.Config, error) {
	if assemblyPath != "" {
		fmt.Fprintf(os.Stderr, "warn: -assembly is deprecated (ADR-0021) and ignored; Autostart+dependsOn is used (file: %s)\n", assemblyPath)
	}
	res := discovery.Scan(pluginsDir)
	for _, e := range res.Errors {
		fmt.Fprintf(os.Stderr, "discover error: %v\n", e)
	}
	plan := assembly.ResolveAutostart(res)
	printRejected(plan.Rejected)
	if len(plan.Missing) > 0 {
		fmt.Fprintf(os.Stderr, "assembly references unknown plugins (continuing): %s\n", strings.Join(plan.Missing, ", "))
	}
	if dump {
		dumpAssembly(plan)
	}
	return plan, assembly.Config{}, nil
}

// startMounted starts the Server for plan and attaches the Discovery catalog
// so ensurePlugins can mount later (ADR-0023). Plugin-switch denylist (ADR-0032)
// is applied before launch.
func startMounted(pluginsDir string, plan assembly.Plan) (*serve.Server, error) {
	res := discovery.Scan(pluginsDir)
	disabled := serve.LoadPluginSwitchFile(serve.SwitchPath(pluginsDir))
	mounted := serve.FilterMountedFound(plan.Mounted, disabled)
	srv, err := serve.Start(mounted)
	if err != nil {
		return nil, err
	}
	srv.SetCatalog(res)
	// SetPluginsDir reloads the switch store into Host memory.
	srv.SetPluginsDir(pluginsDir)
	srv.SetDisabledSet(disabled)
	return srv, nil
}

func dumpAssembly(plan assembly.Plan) {
	fmt.Println("assembly:")
	for _, p := range plan.Mounted {
		m := p.Manifest
		fmt.Printf("  mounted name=%s dir=%s entry=%s provides=[%s]\n",
			m.Name, p.Dir, m.Entry, strings.Join(m.Provides, ","))
	}
	for _, p := range plan.Unmounted {
		m := p.Manifest
		fmt.Printf("  available name=%s mounted=false\n", m.Name)
	}
}

func printRejected(rejected []assembly.Rejected) {
	for _, r := range rejected {
		fmt.Fprintf(os.Stderr, "reject plugin %s: %s\n", r.Name, r.Reason)
	}
}

// probeCommandFaces warns when a Plugin declares commands[] but has no commands.call handler.
func probeCommandFaces(srv *serve.Server, mounted []discovery.Found) {
	for _, p := range mounted {
		if len(p.Manifest.Commands) == 0 {
			continue
		}
		_, err := serve.CallByFace(srv, p.Manifest.Name, "commands", "call", json.RawMessage(`{"command":"__probe__","args":""}`))
		if err == nil {
			continue
		}
		if strings.Contains(err.Error(), "method_not_found") {
			fmt.Fprintf(os.Stderr, "warn: plugin %s declares commands but has no commands.call handler\n", p.Manifest.Name)
		}
	}
}
