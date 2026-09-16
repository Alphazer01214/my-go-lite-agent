#!/usr/bin/env bash
# arch-check.sh - ADR acceptance criteria as a runnable progress meter.
#
# Each check encodes one rule from ADR-0026..0029 (see those files for the
# rationale). At the refactor baseline most checks FAIL by design; each zone
# (Z1..Z5) flips its checks to green. Exit 0 only when all pass.
#
# Usage: bash scripts/arch-check.sh

set -u
cd "$(dirname "$0")/.."

PYBIN="$(command -v python3 || command -v python)"
TMP=".workbuddy/.arch-tmp.$$"
mkdir -p "$TMP"

PASS=0
FAIL=0
FAILED=()

report() { # report <ok:0|1> <name>
  if [ "$1" = "0" ]; then
    printf '  [PASS] %s\n' "$2"; PASS=$((PASS+1))
  else
    printf '  [FAIL] %s\n' "$2"; FAIL=$((FAIL+1)); FAILED+=("$2")
  fi
}

echo "arch-check: ADR acceptance criteria (0026..0029)"
echo "=================================================="
echo

# C1 (ADR-0026 rule 1): no plugin-name literals in Host code (prod files).
# Strict by design: cap literals like "session"/"agent" must go through
# serve constants; probe defaults bound to "echo" must be removed.
echo "C1  No plugin-name literals in Host prod code (ADR-0026)"
hit=0
for d in plugins/*/; do
  name="$(basename "$d")"
  [ -f "$d/plugin.json" ] || continue
  if grep -rq "\"$name\"" --include="*.go" --exclude="*_test.go" serve/ web/ internal/; then
    printf '      literal "%s" in Host code\n' "$name"
    hit=1
  fi
done
report "$hit" "no plugin-name literals"
echo

# C2 (ADR-0027): every capability Host calls is declared by some manifest,
# via provides (routed caps) or hostFaces (addressed faces).
echo "C2  Caps Host calls are declared (provides | hostFaces)"
{
  grep -rhoE 'Cap:[[:space:]]*"[a-z-]+"' --include="*.go" --exclude="*_test.go" serve/ web/ internal/
  grep -rhoE 'CallByCap\("[a-z-]+"' --include="*.go" --exclude="*_test.go" serve/ web/ internal/
  grep -rhoE 'CallByPlugin\("[a-z-]+"[[:space:]]*,[[:space:]]*"[a-z-]+"' --include="*.go" --exclude="*_test.go" serve/ web/ internal/ \
    | sed -E 's/^[^"]*"[a-z-]+"[^"]*"([a-z-]+)"/\1/'   # keep the CAP arg, drop the plugin arg
} | sed -E 's/[^"]*"([a-z-]+)"/\1/' | sort -u > "$TMP/host_caps"
# Declared caps = provides ∪ hostFaces fields only (NOT commands[] entries,
# whose "name":"config" would false-pass a whole-file grep).
"$PYBIN" - > "$TMP/declared" <<'PY'
import glob, json
for f in glob.glob('plugins/*/plugin.json'):
    try:
        m = json.load(open(f, encoding='utf-8'))
    except Exception:
        continue
    for c in (m.get('provides') or []) + (m.get('hostFaces') or []):
        print(c)
PY
sort -u "$TMP/declared" -o "$TMP/declared"
undeclared=0
while IFS= read -r cap; do
  [ -n "$cap" ] || continue
  case "$cap" in
    host|agent) continue ;;                       # host-owned pseudo caps
  esac
  if ! grep -qx "$cap" "$TMP/declared"; then
    printf '      cap "%s" used by Host, declared by no manifest\n' "$cap"
    undeclared=1
  fi
done < "$TMP/host_caps"
report "$undeclared" "caps used by Host are declared"
echo

# C3 (ADR-0026): every entry in hostUsedCapabilities has a real call site.
echo "C3  hostUsedCapabilities entries all have call sites"
dead=0
for cap in $(grep -oE 'serve\.[A-Za-z]+Cap\b' web/server.go | sed 's/serve\.//' | sort -u); do
  uses="$(grep -nE "\b$cap\b" serve/*.go | grep -vE ':[0-9]+:[[:space:]]*(//|$)' | grep -cvE "const $cap\b")"
  if [ "$uses" -eq 0 ]; then
    printf '      %s declared in hostUsedCapabilities but never used\n' "$cap"
    dead=1
  fi
done
report "$dead" "no dead entries in hostUsedCapabilities"
echo

# C4 (ADR-0016 allowlist): serve exported Server methods. QuerySessionFacts /
# DeriveMessages / AppendSessionFacts & friends are deliberately NOT allowed:
# they are the Z3 removal set (internal helpers or CallByCap in the Medium).
echo "C4  serve exported methods within ADR-0016 allowlist"
ALLOW='^(Start|SetCatalog|SetPluginsDir|MountedPluginNames|DegradedNames|ToolOwners|Cards|Panels|Subscribe|CallByCap|Close|MarshalPayload|RunTurn|RunTurnOn|CancelTurnOn|IsRunningOn|RunningSessions|StatusForSession|TurnCancelledOn|AgentRequest|AgentInject|EnsurePlugins)$'
oversized=0
while IFS= read -r m; do
  printf '%s' "$m" | grep -qE "$ALLOW" || {
    printf '      exported beyond allowlist: %s\n' "$m"
    oversized=1
  }
done < <(grep -hoE '^func \(s \*Server\) [A-Z][A-Za-z]+' serve/*.go | sed -E 's/.*\) //')
report "$oversized" "no methods beyond ADR-0016 allowlist"
echo

# C5 (ADR-0017): single launch failure must not take down the whole host.
echo "C5  Start does not Close host on launch failure (ADR-0017)"
closes="$(awk '/^func Start\(/,/^}/' serve/serve.go | grep -c 's\.Close()')"
if [ "$closes" -gt 0 ]; then report 1 "Start soft-fail"; else report 0 "Start soft-fail"; fi
echo

# C6 (ADR-0023): every scheme's allowedTools are provided by its closure.
echo "C6  Scheme allowedTools covered by dependsPlugins closure"
"$PYBIN" - <<'PY' > "$TMP/c6"; rc=$?
import re, sys

def block(src, start):
    depth = 0
    for i in range(start, len(src)):
        if src[i] == '{': depth += 1
        elif src[i] == '}':
            depth -= 1
            if depth == 0:
                return src[start:i+1]
    return ''

src = open('plugins/agent/config.go', encoding='utf-8').read()
m = re.search(r'func defaultSchemes\(\)[^{]*\{', src)
body = block(src, m.end()-1) if m else ''

tool_names = set()
for d in ('filetools','shelltools','webtools','skill-manager','echotool'):
    try:
        t = open('plugins/%s/main.go' % d, encoding='utf-8').read()
        tool_names |= set(re.findall(r'"name":\s*"([a-z_]+)"', t))
    except OSError:
        pass

bad = 0
for sm in re.finditer(r'"([a-z_]+)":\s*\{', body):
    entry = block(body, sm.end()-1)
    name = sm.group(1)
    m_all = re.search(r'AllowedTools:\s*&?(\[\]string\{([^}]*)\}|[A-Za-z_][A-Za-z0-9_]*)', entry)
    if not m_all:
        continue
    if m_all.group(2) is not None:
        tools_raw = m_all.group(2)
    else:
        var = m_all.group(1)
        vdef = re.search(r'\b%s\s*:?=\s*\[\]string\{([^}]*)\}' % var, src)
        if not vdef:
            continue
        tools_raw = vdef.group(1)
    tools = set(re.findall(r'"([a-z_]+)"', tools_raw)) - {'todo','run_subagent'}
    m_dep = re.search(r'DependsPlugins:\s*([^,\n]+)', entry)
    dep_raw = m_dep.group(1) if m_dep else ''
    deps = [] if 'nil' in dep_raw else re.findall(r'"([a-z-]+)"', dep_raw)
    if tools and not deps:
        print('      scheme "%s" declares allowedTools %s but pulls no plugins'
              % (name, sorted(tools)))
        bad = 1
sys.exit(bad)
PY
report "$rc" "allowedTools covered by dependsPlugins"
echo

# C7 (ADR-0022): degraded must be recoverable.
echo "C7  degraded recoverable (reconcile + clear path)"
r=1
if grep -q 'reconcileConsumes' serve/*.go && grep -qE 'delete\(s\.degraded' serve/*.go; then
  r=0
fi
report "$r" "reconcileConsumes + degraded clear path"
echo

# C8 (ADR-0021): -assembly must not act as a mount whitelist.
echo "C8  -assembly not a mount whitelist (ADR-0021)"
legacy="$(grep -c 'assembly\.Resolve(cfg' internal/app/app.go)"
if [ "$legacy" -gt 0 ]; then report 1 "no legacy whitelist"; else report 0 "no legacy whitelist"; fi
echo

# C9 (ADR-0028): auto-compact doc and code aligned.
echo "C9  Auto-compact accepted as default (ADR-0028)"
r=0
[ -f docs/adr/0028-auto-compact-as-default.md ] || r=1
grep -q 'maybeAutoCompact' plugins/agent/main.go || r=1
report "$r" "ADR-0028 exists and code wired"
echo

# C10 (ADR-0026 rule 2): wire types defined once (pluginsdk is the source).
echo "C10 Wire types defined once"
dup=0
for t in 'type PanelOp struct' 'type SummaryPair struct' 'type RenderIntent struct'; do
  n="$(grep -rl "$t" serve/ pluginsdk/ 2>/dev/null | wc -l)"
  if [ "$n" -gt 1 ]; then
    printf '      %s defined in %s packages\n' "$t" "$n"
    dup=1
  fi
done
report "$dup" "no duplicated wire types"
echo

# C11 (ADR-0029): no single-slot On* hooks.
echo "C11 No single-slot On* hooks (ADR-0029)"
slots="$(grep -cE '^\s+On[A-Za-z]+[[:space:]]+func' serve/serve.go)"
if [ "$slots" -gt 0 ]; then
  printf '      %d single-slot On* fields in Server\n' "$slots"
  report 1 "no single-slot On* fields"
else
  report 0 "no single-slot On* fields"
fi
echo

# C12: every shipped plugin has a mount path.
echo "C12 Every shipped plugin has a mount path"
nomount=0
for f in plugins/*/plugin.json; do
  name="$(basename "$(dirname "$f")")"
  grep -q '"autostart":[[:space:]]*true' "$f" && continue
  grep -q "\"$name\"" plugins/agent/config.go && continue
  uionly="$("$PYBIN" -c "import json; m=json.load(open(r'$f', encoding='utf-8')); print(1 if not m.get('entry') and m.get('ui') else 0)" 2>/dev/null || echo 0)"
  if [ "$uionly" != "1" ]; then
    printf '      %s: no autostart, not in any scheme, not UI-only\n' "$name"
    nomount=1
  fi
done
report "$nomount" "all shipped plugins mountable"
echo

echo "=================================================="
printf 'arch-check: %d passed, %d failed\n\n' "$PASS" "$FAIL"
if [ "$FAIL" -gt 0 ]; then
  echo "pending (flip these to green zone by zone):"
  for c in "${FAILED[@]}"; do printf '  - %s\n' "$c"; done
  exit 1
fi
echo "all acceptance criteria met."
exit 0
