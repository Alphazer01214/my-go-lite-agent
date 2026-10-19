#!/usr/bin/env bash
# Build a release layout under dist/ for macOS (and Linux).
# Usage:
#   bash scripts/build.sh                 # incremental (skip unchanged binaries)
#   bash scripts/build.sh --clean         # wipe dist and rebuild everything
#   bash scripts/build.sh --verify        # go vet + go test (+node) before building
#   bash scripts/build.sh --only agent,session
#   bash scripts/build.sh --list
#
# Incremental mode hashes each target's module-local Go deps (plus go.mod/go.sum,
# the plugin manifest, and — for the server — the go:embed'ed WebUI assets under
# internal/webui/static). Unchanged targets reuse the existing dist binary;
# assets (plugin.json/ui/README/examples) always refresh.
#
# The plugin directory is the single source of truth: every plugins/<name>/ owns
# its plugin.json (name/version/provides/consumes/autostart/dependsOn/commands/ui),
# its ui/ assets and its README.md. This script only compiles and copies — it
# never generates manifests, so dist can never drift from the source tree
# (ADR-0021).
set -euo pipefail

ROOT="$(cd "$(dirname "$0")/.." && pwd)"
DIST="$ROOT/dist"
CACHE_DIR="$DIST/.build-cache"
WEB_STATIC="$ROOT/internal/webui/static"
CLEAN=0
LIST=0
VERIFY=0
ONLY=""

while [[ $# -gt 0 ]]; do
  case "$1" in
    --clean|-c) CLEAN=1; shift ;;
    --list) LIST=1; shift ;;
    --verify|-V) VERIFY=1; shift ;;
    --only) ONLY="${2:-}"; shift 2 ;;
    --only=*) ONLY="${1#*=}"; shift ;;
    *) echo "unknown flag: $1" >&2; exit 2 ;;
  esac
done

# ── shipped plugins ─────────────────────────────────────────────────────────
# Single source of truth: scripts/shipped-plugins.conf (shared with build.ps1).
# Bash 3.2 + `set -u`: empty-array `${arr[@]}` is unbound, so expand via the
# ${arr[@]+"${arr[@]}"} idiom. `|| [[ -n $line ]]` also picks up a conf last
# line with no trailing newline (read returns non-zero on that EOF otherwise
# and the line would be skipped).
CORE_PLUGINS=()
TOOLS_PLUGINS=()
while IFS= read -r line || [[ -n "$line" ]]; do
  case "$line" in
    core:*)
      __rest="${line#core:}"
      CORE_PLUGINS=($__rest)
      ;;
    tools:*)
      __rest="${line#tools:}"
      TOOLS_PLUGINS=($__rest)
      ;;
  esac
done < "$ROOT/scripts/shipped-plugins.conf"
SHIPPED_PLUGINS=(${CORE_PLUGINS[@]+"${CORE_PLUGINS[@]}"} ${TOOLS_PLUGINS[@]+"${TOOLS_PLUGINS[@]}"})

# Listed in the conf but not a mountable plugin yet (no plugin.json). Warn once
# instead of once per target; a missing plugin never aborts the build.
MISSING_PLUGINS=""
for name in ${SHIPPED_PLUGINS[@]+"${SHIPPED_PLUGINS[@]}"}; do
  [[ -f "$ROOT/plugins/$name/plugin.json" ]] || MISSING_PLUGINS="$MISSING_PLUGINS $name"
done
MISSING_PLUGINS="${MISSING_PLUGINS# }"

should_build() {
  local target="$1"
  [[ -z "$ONLY" ]] && return 0
  [[ ",$ONLY," == *",$target,"* ]] && return 0
  # Accept short names: "agent" matches "plugin:agent".
  if [[ "$target" == plugin:* ]]; then
    local short="${target#plugin:}"
    [[ ",$ONLY," == *",$short,"* ]] && return 0
  fi
  [[ ",$ONLY," == *",plugin:$target,"* ]] && return 0
  return 1
}

# Copy an optional repo asset into dist; missing source warns and skips (never aborts).
copy_dist_asset() {
  local rel="$1" dst="$2"
  if [[ -e "$ROOT/$rel" ]]; then
    cp "$ROOT/$rel" "$dst"
  else
    echo "warning: skip missing dist asset: $rel" >&2
  fi
}

START_TS=$(date +%s)
REBUILT=0
SKIPPED=0

# ── pre-flight verification (opt-in) ────────────────────────────────────────
# Fail before touching dist/. go test also warms the module build cache, which
# makes the subsequent `go build` calls cheaper.
if [[ $VERIFY -eq 1 ]]; then
  echo "verify: go vet ./..."
  (cd "$ROOT" && go vet ./...)
  echo "verify: go test ./..."
  (cd "$ROOT" && go test ./...)
  if command -v node >/dev/null 2>&1; then
    echo "verify: node --test internal/webui/*.test.mjs"
    (cd "$ROOT" && node --test internal/webui/*.test.mjs)
  else
    echo "verify: node not found — skipped browser tests" >&2
  fi
fi

if [[ -n "$MISSING_PLUGINS" ]]; then
  echo "warning: shipped-plugins.conf lists plugin(s) without plugins/<name>/plugin.json: $MISSING_PLUGINS" >&2
fi

# ── user-data preserve (needed when wiping; cheap no-op otherwise) ───────────
STASH="$(mktemp -d)"
LLM_DIST_CFG="$DIST/plugins/llm-openai/config.json"
AGENT_DIST_CFG="$DIST/plugins/agent/config.json"
SESS_DIST_DIR="$DIST/plugins/session/sessions"
SESSION_WORKSPACE_FILE="$SESS_DIST_DIR/workspaces.json"
LEGACY_WORKSPACE_FILE="$DIST/plugins/workspace/workspaces.json"
[[ -f "$LLM_DIST_CFG" ]] && cp "$LLM_DIST_CFG" "$STASH/llm-config.json"
[[ -f "$AGENT_DIST_CFG" ]] && cp "$AGENT_DIST_CFG" "$STASH/agent-config.json"
[[ -d "$SESS_DIST_DIR" ]] && cp -R "$SESS_DIST_DIR" "$STASH/sessions"
[[ -f "$LEGACY_WORKSPACE_FILE" ]] && cp "$LEGACY_WORKSPACE_FILE" "$STASH/legacy-workspaces.json"

if [[ $CLEAN -eq 1 && -d "$DIST" ]]; then
  echo "removing previous dist/ (configs + sessions stashed)"
  rm -rf "$DIST"
fi
mkdir -p "$DIST/plugins" "$CACHE_DIR"

# ── incremental helpers ─────────────────────────────────────────────────────
local_dep_dirs() {
  local pkg="$1"
  (cd "$ROOT" && go list -deps -f '{{.Dir}}' "$pkg" 2>/dev/null) | awk -v root="$ROOT/" 'index($0, root)==1'
}

extra_input_paths() {
  local target="$1" pkg="$2"
  echo "$ROOT/go.mod"
  echo "$ROOT/go.sum"
  # The server go:embeds internal/webui/static; without these files in the hash
  # a shell/asset-only edit would leave a stale dist binary behind.
  if [[ "$target" == "liteagent-server" && -d "$WEB_STATIC" ]]; then
    find "$WEB_STATIC" -type f 2>/dev/null
  fi
  if [[ "$pkg" == ./plugins/* ]]; then
    local name="${pkg#./plugins/}"
    [[ -f "$ROOT/plugins/$name/plugin.json" ]] && echo "$ROOT/plugins/$name/plugin.json"
  fi
  return 0
}

get_target_hash() {
  local target="$1" pkg="$2"
  local tmp
  tmp="$(mktemp)"
  {
    local_dep_dirs "$pkg" | while read -r dir; do
      [[ -d "$dir" ]] || continue
      find "$dir" -maxdepth 1 -name '*.go' -type f 2>/dev/null
    done
    extra_input_paths "$target" "$pkg" | while read -r p; do
      if [[ -f "$p" ]]; then
        echo "$p"
      elif [[ -d "$p" ]]; then
        find "$p" -type f 2>/dev/null
      fi
    done
  } | sed "s|^$ROOT/||" | sort -u | while read -r rel; do
    local abs="$ROOT/$rel"
    [[ -f "$abs" ]] || continue
    # path + content hash
    printf '%s\n' "$rel"
    if command -v sha256sum >/dev/null 2>&1; then
      sha256sum "$abs" | awk '{print $1}'
    else
      shasum -a 256 "$abs" | awk '{print $1}'
    fi
  done >"$tmp"
  if command -v sha256sum >/dev/null 2>&1; then
    sha256sum "$tmp" | awk '{print $1}'
  else
    shasum -a 256 "$tmp" | awk '{print $1}'
  fi
  rm -f "$tmp"
}

stamp_path() {
  local safe="${1//\//_}"
  safe="${safe//:/_}"
  echo "$CACHE_DIR/${safe}.sha256"
}

test_skip_build() {
  local target="$1" out="$2" hash="$3"
  [[ $CLEAN -eq 1 ]] && return 1
  [[ -z "$hash" ]] && return 1
  [[ -f "$out" ]] || return 1
  local stamp
  stamp="$(stamp_path "$target")"
  [[ -f "$stamp" ]] || return 1
  [[ "$(cat "$stamp")" == "$hash" ]]
}

write_stamp() {
  local target="$1" hash="$2"
  [[ -z "$hash" ]] && return 0
  printf '%s' "$hash" >"$(stamp_path "$target")"
}

# Release stamp: git describe when available, else "dev". Injected into the
# server binary via -X so a dist build is always identifiable.
VERSION="$(git -C "$ROOT" describe --tags --always --dirty 2>/dev/null || true)"
[[ -n "$VERSION" ]] || VERSION="dev"

build_pkg() {
  local target="$1" pkg="$2" out="$3"
  if ! should_build "$target"; then
    echo "skip $target (filtered by --only)"
    return 0
  fi
  local hash
  hash="$(get_target_hash "$target" "$pkg")"
  if test_skip_build "$target" "$out" "$hash"; then
    echo "skip $target (unchanged)"
    SKIPPED=$((SKIPPED + 1))
    return 0
  fi
  echo "build $pkg -> $out"
  local ldflags="-s -w"
  [[ "$target" == "liteagent-server" ]] && ldflags="-s -w -X main.version=$VERSION"
  go build -trimpath -ldflags "$ldflags" -o "$out" "$pkg"
  write_stamp "$target" "$hash"
  REBUILT=$((REBUILT + 1))
}

copy_tree() {
  local src="$1" dst="$2"
  rm -rf "$dst"
  cp -R "$src" "$dst"
}

# install_plugin_dir copies plugins/<name>/ as-is: binary, manifest, ui/ assets,
# README.md and per-plugin extra files. Source manifests are Windows-first
# ("entry": "<name>.exe"); the Unix dist gets the bare name.
install_plugin_dir() {
  local name="$1"
  local src="$ROOT/plugins/$name"
  local dir="$DIST/plugins/$name"

  if ! should_build "plugin:$name" && ! should_build "$name"; then
    echo "skip plugin:$name (filtered by --only)"
    return 0
  fi
  if [[ ! -f "$src/plugin.json" ]]; then
    return 0
  fi
  mkdir -p "$dir"
  if grep -q '"entry"' "$src/plugin.json"; then
    build_pkg "plugin:$name" "./plugins/$name" "$dir/$name"
  fi
  cp "$src/plugin.json" "$dir/plugin.json"
  sed -i.bak 's/"entry": *"'"$name"'\.exe"/"entry": "'"$name"'"/' "$dir/plugin.json"
  rm -f "$dir/plugin.json.bak"
  for extra in ui README.md segments.json config.example.json; do
    if [[ -d "$src/$extra" ]]; then
      copy_tree "$src/$extra" "$dir/$extra"
    elif [[ -f "$src/$extra" ]]; then
      cp "$src/$extra" "$dir/$extra"
    fi
  done
  return 0
}

if [[ $LIST -eq 1 ]]; then
  echo "Shipped targets:"
  echo "  liteagent-server"
  for name in ${SHIPPED_PLUGINS[@]+"${SHIPPED_PLUGINS[@]}"}; do
    tag="assets-only"
    if [[ -f "$ROOT/plugins/$name/plugin.json" ]] && grep -q '"entry"' "$ROOT/plugins/$name/plugin.json"; then
      tag="binary"
    fi
    echo "  plugin:$name ($tag)"
  done
  [[ -n "$MISSING_PLUGINS" ]] && echo "  (no plugin.json: $MISSING_PLUGINS)" >&2
  rm -rf "$STASH"
  exit 0
fi

# ── main binaries ────────────────────────────────────────────────────────────
pushd "$ROOT" >/dev/null

build_pkg "liteagent-server" "./cmd/liteagent-server" "$DIST/liteagent-server"

# ── plugins ──────────────────────────────────────────────────────────────────
for name in ${SHIPPED_PLUGINS[@]+"${SHIPPED_PLUGINS[@]}"}; do
  install_plugin_dir "$name"
done

# Workspace Registry moved into session. Preserve an existing legacy file
# before pruning the old standalone plugin directory.
if [[ -f "$LEGACY_WORKSPACE_FILE" && ! -f "$SESSION_WORKSPACE_FILE" ]]; then
  mkdir -p "$SESS_DIST_DIR"
  cp "$LEGACY_WORKSPACE_FILE" "$SESSION_WORKSPACE_FILE"
  echo "workspace registry: migrated dist/plugins/workspace into session"
fi

# Prune plugin dirs that are no longer shipped (keep user-data names).
if [[ -d "$DIST/plugins" ]]; then
  for dir in "$DIST/plugins"/*; do
    [[ -d "$dir" ]] || continue
    base="$(basename "$dir")"
    keep=0
    for name in ${SHIPPED_PLUGINS[@]+"${SHIPPED_PLUGINS[@]}"}; do
      [[ "$base" == "$name" ]] && keep=1 && break
    done
    if [[ $keep -eq 0 ]]; then
      echo "prune dist/plugins/$base"
      rm -rf "$dir"
      rm -f "$(stamp_path "plugin:$base")"
    fi
  done
fi

# ── config restore / seed ───────────────────────────────────────────────────
if [[ -f "$STASH/llm-config.json" && ! -f "$LLM_DIST_CFG" ]]; then
  mkdir -p "$(dirname "$LLM_DIST_CFG")"
  cp "$STASH/llm-config.json" "$LLM_DIST_CFG"
  echo "llm-openai config: preserved dist config.json (API key survives rebuilds)"
fi
if [[ -f "$STASH/agent-config.json" && ! -f "$AGENT_DIST_CFG" ]]; then
  mkdir -p "$(dirname "$AGENT_DIST_CFG")"
  cp "$STASH/agent-config.json" "$AGENT_DIST_CFG"
  echo "agent config: preserved dist config.json (schemes survive rebuilds)"
fi
if [[ -d "$STASH/sessions" ]]; then
  mkdir -p "$DIST/plugins/session"
  cp -R "$STASH/sessions/." "$SESS_DIST_DIR/"
  echo "sessions: preserved $(find "$DIST/plugins/session/sessions" -name '*.jsonl' 2>/dev/null | wc -l | tr -d ' ') file(s)"
fi
if [[ -f "$STASH/legacy-workspaces.json" && ! -f "$SESSION_WORKSPACE_FILE" ]]; then
  mkdir -p "$SESS_DIST_DIR"
  cp "$STASH/legacy-workspaces.json" "$SESSION_WORKSPACE_FILE"
  echo "workspace registry: restored legacy data into session"
fi
rm -rf "$STASH"

# Config priority: existing dist config, then a repo-local config.json
# (gitignored), else the plugin's config.example.json. The build never injects
# or rewrites an API key.
seed_dist_config() {
  local name="$1" dst="$2"
  [[ -f "$dst" ]] && return 0
  local src="$ROOT/plugins/$name"
  if [[ -f "$src/config.json" ]]; then
    mkdir -p "$(dirname "$dst")"
    cp "$src/config.json" "$dst"
    echo "$name config: seeded from repo plugins/$name/config.json (kept as-is)"
  elif [[ -f "$src/config.example.json" ]]; then
    mkdir -p "$(dirname "$dst")"
    cp "$src/config.example.json" "$dst"
    echo "$name config: seeded from plugins/$name/config.example.json"
  else
    echo "warning: no config template for $name (dist keeps no config.json)" >&2
  fi
}
seed_dist_config "llm-openai" "$LLM_DIST_CFG"
seed_dist_config "agent" "$AGENT_DIST_CFG"

# ── docs ─────────────────────────────────────────────────────────────────────
copy_dist_asset "README.md" "$DIST/"
copy_dist_asset "CONTEXT.md" "$DIST/"
copy_dist_asset "plugins/README.md" "$DIST/plugins/README.md"

popd >/dev/null

END_TS=$(date +%s)
echo ""
echo "Release layout ready: $DIST  (rebuilt $REBUILT, skipped $SKIPPED, $((END_TS - START_TS))s, version $VERSION)"
find "$DIST" -type f ! -path "$CACHE_DIR/*" | sed "s|^$DIST/||" | sort
