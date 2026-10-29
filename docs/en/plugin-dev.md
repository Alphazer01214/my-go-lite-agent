# Plugin Development

A plugin is a directory package. The directory name, `plugin.json.name` and the name passed to `pluginsdk.NewPlugin` must match.

_English edition of [docs/platform/plugin-dev.md](../platform/plugin-dev.md). If the two diverge, the Chinese document is authoritative._

## 1. Package layout

```text
plugins/<name>/
  main.go
  plugin.json
  config.json
  errors.go
  types.go
  plugin.go
  README.md
  communication.md
  ui/
    main.js
    main.css
```

| File | Required | Responsibility |
|---|---|---|
| `main.go` | yes | register handlers and start the runtime |
| `plugin.json` | yes | static surface: identity, capabilities, dependencies, UI, settings |
| `config.json` | as needed | writable runtime config |
| `errors.go` | yes | error codes and constructors for this plugin |
| `types.go` | yes | contract types shared across features |
| `plugin.go` | yes | plugin struct, config, locks, lifecycle |
| `README.md` | yes | purpose, config, methods |
| `communication.md` | yes | methods, payloads, events, frontend contract |
| `ui/` | as needed | web components and static assets |

Business code is split by concern into `<feature>.go` files.

## 2. main.go

```go
package main

import (
    "context"
    "encoding/json"

    "github.com/tomori/my-go-lite-agent/pluginsdk"
)

// handlePing: func(context.Context, *pluginsdk.Request) (json.RawMessage, error)
func main() {
    p := pluginsdk.NewPlugin("example")
    p.Register("example", "ping", handlePing)
    if err := p.Serve(); err != nil {
        panic(err)
    }
}
```

Plugins import `pluginsdk` only. Business code never hand-writes the frame loop. Test fixtures may use `protocol` directly.

`Register` accepts a `pluginsdk.HandleFunc`.
The handler receives `ctx` and `*pluginsdk.Request`.
It returns `json.RawMessage` and `error`.

The handler owns request decoding with `pluginsdk.Decode`.
It also owns JSON encoding of the business response.

`NewHandler` and `Handle` remain compatibility symbols.
They are not part of the standard registration path.

## 3. Configuration boundary

`plugin.json` is static metadata. `config.json` is runtime configuration.

```text
plugin.json   written by people and the host
config.json   read and written by the plugin runtime
```

Real secrets never go into `plugin.json`. A local `config.json` is not committed; the repository ships `config.example.json` instead.

A configuration update should:

1. persist to the plugin's private config file;
2. return a redacted result;
3. not modify the config snapshot of a running call;
4. take effect from the next call.

## 4. Method documentation

Every plugin's `communication.md` covers at least:

| Section | Content |
|---|---|
| Capabilities | the list of `capability.method` |
| Method payloads | JSON fields and requiredness |
| Responses | result shape |
| Events | id, channel, payload |
| Call graph | Mermaid sequence diagram |
| Web UI | components, state, commands |
| Errors | stable error codes |
| Status | implemented vs. target contract |

Do not present Go private struct names as public protocol. Documents use JSON field names.

## 5. Frontend components

A plugin UI entry defines a custom element. Tag names must be prefixed with the plugin name.

```js
class ExamplePanel extends HTMLElement {
  setData(state) {
    this.state = state;
    this.render();
  }

  runCommand(command, payload) {
    if (command === "refresh") {
      this.refresh();
    }
  }

  connectedCallback() {
    this.render();
  }

  disconnectedCallback() {
    this.cleanup();
  }
}

customElements.define("example-panel", ExamplePanel);
```

Components use shadow DOM for style isolation. A component never reads or writes plugin storage directly and never establishes a second routing layer.

## 6. Testing

Plugin tests should cover:

- payload decoding;
- successful responses;
- stable error codes;
- config persistence and redaction;
- event payloads;
- pure projection and transformation functions;
- cancellation and error paths.

## 7. Build

The build script only compiles and copies; it never generates `plugin.json`.

A release directory contains at least:

```text
dist/plugins/<name>/
  <entry>
  plugin.json
  config.json
  README.md
  communication.md
  ui/
```

The build never writes real API keys. Existing user configuration is preserved on rebuild.

## 8. Prohibited

- calling another plugin directly;
- addressing business capabilities by plugin name;
- storing persistent records in UI state;
- treating transient stream deltas as persistent records;
- letting the browser set `from`;
- keeping secrets in `plugin.json`;
- replacing `plugin.json` registration with a README.
