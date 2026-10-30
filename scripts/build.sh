#!/usr/bin/env bash
# Build a release layout under dist/ for macOS (and Linux).
# Usage:
#   bash scripts/build.sh                 # incremental (skip unchanged binaries)
#   bash scripts/build.sh --clean         # wipe dist and rebuild everything
#   bash scripts/build.sh --verify        # go vet before building
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
#
# Future plugin checklist:
#   1. plugins/<name>/plugin.json (entry/provides/requires/ui);
#   2. ui/<entry> and the custom elements it defines;
#   3. config.example.json when the plugin has runtime config;
#   4. add the name to scripts/shipped-plugins.conf;
#   5. docs under plugins/<name>/ and tests.
# Handoff: docs/archive/progress/2026-10-30-agent-session-llm-handoff.md
# Runtime data is preserved generically: every plugin's root-level *.json
# (except plugin.json/config.example.json/segments.json) and its sessions/
# and data/ directories survive a clean build. Missing config.json is seeded
# from the repo-local config.json or config.example.json.
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
OPTIONAL_PLUGINS=()
while IFS= read -r line || [[ -n "$line" ]]; do
  line="${line%$'\r'}"
  case "$line" in
    core:*)
      __rest="${line#core:}"
      CORE_PLUGINS=($__rest)
      ;;
    tools:*)
      __rest="${line#tools:}"
      TOOLS_PLUGINS=($__rest)
      ;;
    optional:*)
      __rest="${line#optional:}"
      OPTIONAL_PLUGINS=($__rest)
      ;;
  esac
done < "$ROOT/scripts/shipped-plugins.conf"
SHIPPED_PLUGINS=(${CORE_PLUGINS[@]+"${CORE_PLUGINS[@]}"} ${TOOLS_PLUGINS[@]+"${TOOLS_PLUGINS[@]}"} ${OPTIONAL_PLUGINS[@]+"${OPTIONAL_PLUGINS[@]}"})

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
# Fail before touching dist/.
if [[ $VERIFY -eq 1 ]]; then
  echo "verify: go vet ./..."
  (cd "$ROOT" && go vet ./...)
fi

if [[ -n "$MISSING_PLUGINS" ]]; then
  echo "warning: shipped-plugins.conf lists plugin(s) without plugins/<name>/plugin.json: $MISSING_PLUGINS" >&2
fi

# ── user-data preserve (needed when wiping; cheap no-op otherwise) ───────────
# Generic rule: every plugin's root-level *.json is runtime data, except the
# files the build itself ships (plugin.json / config.example.json /
# segments.json). Directories named sessions/ or data/ are runtime data too.
# The legacy standalone workspace registry is handled separately below.
STASH="$(mktemp -d)"
PLUGIN_STASH="$STASH/plugins"
SESS_DIST_DIR="$DIST/plugins/session/sessions"
SESSION_WORKSPACE_FILE="$SESS_DIST_DIR/workspaces.json"
LEGACY_WORKSPACE_FILE="$DIST/plugins/workspace/workspaces.json"
if [[ -d "$DIST/plugins" ]]; then
  for dir in "$DIST/plugins"/*; do
    [[ -d "$dir" ]] || continue
    name="$(basename "$dir")"
    # The legacy workspace plugin is migrated into session separately.
    [[ "$name" == "workspace" ]] && continue
    target="$PLUGIN_STASH/$name"
    copied=0
    for file in "$dir"/*.json; do
      [[ -f "$file" ]] || continue
      base="$(basename "$file")"
      case "$base" in
        plugin.json|config.example.json|segments.json) continue ;;
      esac
      mkdir -p "$target"
      cp "$file" "$target/$base"
      copied=1
    done
    for data in sessions data; do
      if [[ -d "$dir/$data" ]]; then
        mkdir -p "$target"
        cp -R "$dir/$data" "$target/$data"
        copied=1
      fi
    done
    if [[ $copied -eq 1 ]]; then
      echo "preserve runtime data: plugins/$name"
    fi
  done
fi
if [[ -f "$LEGACY_WORKSPACE_FILE" ]]; then
  mkdir -p "$STASH"
  cp "$LEGACY_WORKSPACE_FILE" "$STASH/legacy-workspaces.json"
fi

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
    # model-context go:embeds config.example.json as the shipped prompt default.
    [[ -f "$ROOT/plugins/$name/config.example.json" ]] && echo "$ROOT/plugins/$name/config.example.json"
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
  # Best-effort UI entry validation: catch a manifest that points at a file
  # the plugin package does not ship. Uses python3 or node when available.
  ui_entry=""
  if command -v python3 >/dev/null 2>&1; then
    ui_entry="$(python3 -c 'import json,sys; d=json.load(open(sys.argv[1], encoding="utf-8")); print(((d.get("ui") or {}).get("entry") or ""))' "$src/plugin.json" 2>/dev/null || true)"
  elif command -v node >/dev/null 2>&1; then
    ui_entry="$(node -e 'const fs=require("fs");const d=JSON.parse(fs.readFileSync(process.argv[1],"utf8"));process.stdout.write((d.ui&&d.ui.entry)||"")' "$src/plugin.json" 2>/dev/null || true)"
  fi
  if [[ -n "$ui_entry" && ! -f "$src/$ui_entry" ]]; then
    echo "warning: plugin:$name ui.entry is missing: $ui_entry" >&2
  fi
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

# ── runtime data restore / config seed ──────────────────────────────────────
# Restore runtime data for plugins that are still shipped. A plugin removed
# from shipped-plugins.conf was pruned above and is not resurrected.
if [[ -d "$PLUGIN_STASH" ]]; then
  for src in "$PLUGIN_STASH"/*; do
    [[ -d "$src" ]] || continue
    name="$(basename "$src")"
    keep=0
    for shipped in ${SHIPPED_PLUGINS[@]+"${SHIPPED_PLUGINS[@]}"}; do
      [[ "$name" == "$shipped" ]] && keep=1 && break
    done
    [[ $keep -eq 1 ]] || continue
    dst="$DIST/plugins/$name"
    mkdir -p "$dst"
    for file in "$src"/*.json; do
      [[ -f "$file" ]] || continue
      base="$(basename "$file")"
      [[ -f "$dst/$base" ]] || cp "$file" "$dst/$base"
    done
    for data in sessions data; do
      if [[ -d "$src/$data" ]]; then
        mkdir -p "$dst/$data"
        cp -R "$src/$data/." "$dst/$data/" 2>/dev/null || true
      fi
    done
  done
fi

# Seed a missing config.json for every shipped plugin with an example. The
# repo-local config.json wins; the build never injects or rewrites an API key.
for name in ${SHIPPED_PLUGINS[@]+"${SHIPPED_PLUGINS[@]}"}; do
  src="$ROOT/plugins/$name/config.example.json"
  dst="$DIST/plugins/$name/config.json"
  [[ -f "$src" ]] || continue
  [[ -f "$dst" ]] && continue
  mkdir -p "$(dirname "$dst")"
  repo="$ROOT/plugins/$name/config.json"
  if [[ -f "$repo" ]]; then
    cp "$repo" "$dst"
    echo "$name config: seeded from repo plugins/$name/config.json (kept as-is)"
  else
    cp "$src" "$dst"
    echo "$name config: seeded from plugins/$name/config.example.json"
  fi
done
if [[ -f "$DIST/plugins/llm-openai/config.json" ]] && ! grep -q '"models"' "$DIST/plugins/llm-openai/config.json"; then
  echo "warning: llm-openai config.json uses the legacy flat format; rewrite it as models[] + default_model" >&2
fi
if [[ -d "$SESS_DIST_DIR" ]]; then
  echo "sessions: preserved $(find "$SESS_DIST_DIR" -name '*.jsonl' 2>/dev/null | wc -l | tr -d ' ') file(s)"
fi
if [[ -f "$STASH/legacy-workspaces.json" && ! -f "$SESSION_WORKSPACE_FILE" ]]; then
  mkdir -p "$SESS_DIST_DIR"
  cp "$STASH/legacy-workspaces.json" "$SESSION_WORKSPACE_FILE"
  echo "workspace registry: restored legacy data into session"
fi
rm -rf "$STASH"

# ── docs ─────────────────────────────────────────────────────────────────────
copy_dist_asset "README.md" "$DIST/"
copy_dist_asset "CONTEXT.md" "$DIST/"
copy_dist_asset "plugins/README.md" "$DIST/plugins/README.md"

popd >/dev/null

END_TS=$(date +%s)
echo ""
echo "Release layout ready: $DIST  (rebuilt $REBUILT, skipped $SKIPPED, $((END_TS - START_TS))s, version $VERSION)"
echo "handoff: docs/archive/progress/2026-10-30-agent-session-llm-handoff.md"
find "$DIST" -type f ! -path "$CACHE_DIR/*" | sed "s|^$DIST/||" | sort
