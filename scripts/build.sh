#!/usr/bin/env bash
# Build a release layout under dist/ for macOS (and Linux).
# Usage: bash scripts/build.sh
set -euo pipefail

ROOT="$(cd "$(dirname "$0")/.." && pwd)"
DIST="$ROOT/dist"

# ── clean ────────────────────────────────────────────────────────────────────
if [[ -d "$DIST" ]]; then
    echo "removing previous dist/"
    rm -rf "$DIST"
fi
mkdir -p "$DIST/plugins" "$DIST/examples"

# ── helpers ──────────────────────────────────────────────────────────────────
build_pkg() {
    local pkg="$1" out="$2"
    echo "build $pkg -> $out"
    go build -trimpath -ldflags "-s -w" -o "$out" "$pkg"
}

install_plugin() {
    local name="$1" pkg="$2" provides="$3"
    local consumes="${4:-[]}" timeoutMs="${5:-0}" description="${6:-}" commands="${7:-}"

    local dir="$DIST/plugins/$name"
    mkdir -p "$dir"
    build_pkg "$pkg" "$dir/$name"

    local extra=""
    if [[ "$timeoutMs" -gt 0 ]]; then
        extra+=$',\n  "timeoutMs": '"$timeoutMs"
    fi
    if [[ -n "$description" ]]; then
        extra+=$',\n  "description": "'"$description"'"'
    fi
    if [[ -n "$commands" ]]; then
        extra+=$',\n  "commands": '"$commands"
    fi

    cat > "$dir/plugin.json" <<EOF
{
  "name": "$name",
  "version": "0.1.0",
  "protocol": 2,
  "provides": $provides,
  "consumes": $consumes,
  "entry": "$name"$extra
}
EOF
}

# ── main binaries ────────────────────────────────────────────────────────────
pushd "$ROOT" >/dev/null

build_pkg "./cmd/liteagent-cli"   "$DIST/liteagent-cli"
build_pkg "./cmd/liteagent-server" "$DIST/liteagent-server"

# ── plugins ──────────────────────────────────────────────────────────────────
install_plugin "session"        "./plugins/session"        '["session"]' "[]" 0 \
    "File-backed session log plugin (JSONL) with the session trace Web view" \
    '[{"name":"dump-trace","description":"Export Session Log facts as JSON","usage":"/session dump-trace [sessionId]"},{"name":"list","description":"List sessions","usage":"/session list"},{"name":"derive","description":"Print Model Context from Session Log","usage":"/session derive [sessionId]"},{"name":"current","description":"Show Current Session id","usage":"/session current"}]'
# session ships its own manifest (ui mounts: session-trace) and its UI entry
cp "$ROOT/plugins/session/plugin.json" "$DIST/plugins/session/"
cp -r "$ROOT/plugins/session/ui"        "$DIST/plugins/session/"
# source manifest uses .exe entry (Windows dev); strip for Unix dist
sed -i.bak 's/"entry": "session\.exe"/"entry": "session"/' "$DIST/plugins/session/plugin.json"
rm -f "$DIST/plugins/session/plugin.json.bak"

install_plugin "llm-openai"     "./plugins/llm-openai"     '["llm"]' "[]" 120000 \
    "OpenAI-compatible LLM provider" \
    '[{"name":"config","description":"Show or set API key / model / baseURL","usage":"/llm-openai config [get|set key=value]"}]'

install_plugin "echotool"       "./plugins/echotool"       '["tools"]' "[]" 0 \
    "Echo tool with presentation card"

install_plugin "filetools"      "./plugins/filetools"      '["tools"]' "[]" 0 \
    "Read/write workspace files"

install_plugin "context-manager" "./plugins/context-manager" '["system-prompt","context"]' "[]" 0 \
    "Context Manager: system prompt + prepare/compact/usage" \
    '[{"name":"usage","description":"Last prepare Context Usage","usage":"/context-manager usage [sessionId]"},{"name":"list","description":"Model Context messages from last prepare","usage":"/context-manager list [sessionId]"},{"name":"skills","description":"List registered skills","usage":"/context-manager skills"}]'

install_plugin "echo"           "./plugins/echo"           '["echo"]' "[]" 0 \
    "Echo capability plugin"

install_plugin "uidemo"         "./plugins/uidemo"         '[]' "[]" 0 \
    "Web Panel Component reference"
# uidemo ships its own manifest (ui.entry + ui.mounts) and its UI entry module
cp "$ROOT/plugins/uidemo/plugin.json" "$DIST/plugins/uidemo/"
cp -r "$ROOT/plugins/uidemo/ui"        "$DIST/plugins/uidemo/"
# source manifest uses .exe entry (Windows dev); strip for Unix dist
sed -i.bak 's/"entry": "uidemo\.exe"/"entry": "uidemo"/' "$DIST/plugins/uidemo/plugin.json"
rm -f "$DIST/plugins/uidemo/plugin.json.bak"

# ── extra plugin assets ─────────────────────────────────────────────────────
cp "$ROOT/plugins/context-manager/segments.json" "$DIST/plugins/context-manager/"
cp "$ROOT/plugins/llm-openai/config.example.json" "$DIST/plugins/llm-openai/"

LLM_CFG="$ROOT/plugins/llm-openai/config.json"
if [[ ! -f "$LLM_CFG" ]]; then
    echo "WARNING: plugins/llm-openai/config.json not found — dist will not include API key"
else
    cp "$LLM_CFG" "$DIST/plugins/llm-openai/"
    echo "copied llm-openai config.json (with apiKey) -> dist/plugins/llm-openai/"
fi

# ── examples & docs ─────────────────────────────────────────────────────────
cp "$ROOT/examples/assembly.json"           "$DIST/examples/"
cp "$ROOT/examples/assembly-with-tools.json" "$DIST/examples/"
cp "$ROOT/examples/chat.json"               "$DIST/examples/"
cp "$ROOT/examples/agent.json"              "$DIST/examples/"
cp "$ROOT/README.md"                         "$DIST/"
cp "$ROOT/CONTEXT.md"                        "$DIST/"

popd >/dev/null

echo ""
echo "Release layout ready: $DIST"
find "$DIST" -type f | sed "s|^$DIST/||" | sort
