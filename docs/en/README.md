# Documentation

The core contracts are available in English. Everything else lives in the Chinese documentation tree.

## English

| Document | Contents |
|---|---|
| [Architecture](architecture.md) | Host and plugins, routing, the call pipeline, Web UI, scope |
| [Wire protocol](protocol.md) | Frames, message types, routing, host methods, error codes |
| [Plugin development](plugin-dev.md) | Package layout, SDK entry point, components, testing, build |

## Chinese

The platform documentation under `docs/` is written in Chinese and remains the source of truth for anything not translated here.

| Document | Contents |
|---|---|
| [docs/README.md](../README.md) | Full index |
| [platform/protocol.md](../platform/protocol.md) | Frame protocol |
| [platform/pluginsdk.md](../platform/pluginsdk.md) | SDK reference |
| [platform/manifest.md](../platform/manifest.md) | Manifest, UI and settings contracts |
| [platform/webui.md](../platform/webui.md) | Web UI and browser bridge |
| [platform/plugin-dev.md](../platform/plugin-dev.md) | Plugin development |
| [platform/build.md](../platform/build.md) | Build, incremental cache, dist layout |
| [design/](../design/) | Composition model, plugin designs, performance notes |
| [prompts/](../prompts/) | Prompt contract and writing rules |

## Where to start

- **Writing a plugin** — [plugin-dev.md](plugin-dev.md), then read `plugins/file-tools/` as a worked example.
- **Embedding the runtime** — [protocol.md](protocol.md) and [architecture.md](architecture.md).
- **Understanding the context pipeline** — [design/composition-model-context-tools-sandbox.md](../design/composition-model-context-tools-sandbox.md) *(Chinese)*.
