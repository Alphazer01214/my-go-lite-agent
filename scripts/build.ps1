# Build a release layout under dist/.
# Usage:
#   powershell -File scripts\build.ps1            # incremental (skip unchanged binaries)
#   powershell -File scripts\build.ps1 -Clean     # wipe dist and rebuild everything
#   powershell -File scripts\build.ps1 -Only sandbox,agent
#   powershell -File scripts\build.ps1 -List
#
# Incremental mode hashes each target's module-local Go deps (plus go.mod/go.sum
# and embedded static assets for the server). Unchanged targets reuse the
# existing dist binary; assets (plugin.json/ui/README/examples) always refresh.
#
# The plugin directory is the single source of truth: every plugins/<name>/ owns
# its plugin.json (name/version/provides/consumes/autostart/dependsOn/commands/ui),
# its ui/ assets and its README.md. This script only compiles and copies — it
# never generates manifests, so dist can never drift from the source tree
# (ADR-0021).
param(
    [switch]$Clean,
    [switch]$List,
    [string[]]$Only = @()
)
$ErrorActionPreference = "Stop"

$root = Split-Path -Parent $PSScriptRoot
$dist = Join-Path $root "dist"
$cacheDir = Join-Path $dist ".build-cache"
$go = "go"

# ── shipped plugins ─────────────────────────────────────────────────────────
# Single source of truth: scripts/shipped-plugins.conf (shared with build.sh).
$groups = @{}
foreach ($line in Get-Content (Join-Path $PSScriptRoot "shipped-plugins.conf")) {
    if ($line -match '^\s*(\w+):\s*(.+?)\s*$') {
        $groups[$matches[1]] = @($matches[2] -split '\s+')
    }
}
$corePlugins = @($groups["core"])
$toolsPlugins = @($groups["tools"])
$shippedPlugins = @($corePlugins + $toolsPlugins)

$onlySet = @{}
foreach ($n in $Only) {
    if (-not $n) { continue }
    # -File passes "-Only a,b" as one string; also accept real arrays.
    foreach ($part in ($n -split ',')) {
        $t = $part.Trim()
        if ($t) { $onlySet[$t] = $true }
    }
}

function Should-Build([string]$target) {
    if ($onlySet.Count -eq 0) { return $true }
    if ($onlySet[$target]) { return $true }
    # Accept short names: "sandbox" matches "plugin:sandbox".
    if ($target.StartsWith("plugin:")) {
        $short = $target.Substring("plugin:".Length)
        if ($onlySet[$short]) { return $true }
    }
    if ($onlySet["plugin:$target"]) { return $true }
    return $false
}

# ── user-data preserve (needed when wiping; cheap no-op otherwise) ───────────
$llmDistCfg = Join-Path $dist "plugins\llm-openai\config.json"
$agentDistCfg = Join-Path $dist "plugins\agent\config.json"
$sessDistDir = Join-Path $dist "plugins\session\sessions"
$sandboxPerm = Join-Path $dist "config\permissions.json"
$stash = Join-Path ([IO.Path]::GetTempPath()) ("liteagent-stash-" + [guid]::NewGuid().ToString("N"))
$llmStash = $null
$sessStash = $null
$agentStash = $null
$sandboxStash = $null
if (Test-Path $llmDistCfg) {
    New-Item -ItemType Directory -Path $stash -Force | Out-Null
    $llmStash = Join-Path $stash "llm-config.json"
    Copy-Item $llmDistCfg $llmStash -Force
}
if (Test-Path $agentDistCfg) {
    New-Item -ItemType Directory -Path $stash -Force | Out-Null
    $agentStash = Join-Path $stash "agent-config.json"
    Copy-Item $agentDistCfg $agentStash -Force
}
if (Test-Path $sessDistDir) {
    New-Item -ItemType Directory -Path $stash -Force | Out-Null
    $sessStash = Join-Path $stash "sessions"
    Copy-Item $sessDistDir $sessStash -Recurse -Force
}
if (Test-Path $sandboxPerm) {
    New-Item -ItemType Directory -Path $stash -Force | Out-Null
    $sandboxStash = Join-Path $stash "permissions.json"
    Copy-Item $sandboxPerm $sandboxStash -Force
}

if ($Clean -and (Test-Path $dist)) {
    try {
        Remove-Item -Recurse -Force $dist -ErrorAction Stop
    } catch {
        Write-Warning "dist is locked (cwd or running process); overwriting in place. Close terminals using dist\ and stop liteagent-server.exe for a clean wipe."
    }
}
New-Item -ItemType Directory -Path $dist -Force | Out-Null
New-Item -ItemType Directory -Path (Join-Path $dist "plugins") -Force | Out-Null
New-Item -ItemType Directory -Path (Join-Path $dist "examples") -Force | Out-Null
New-Item -ItemType Directory -Path $cacheDir -Force | Out-Null

$script:rebuildCount = 0
$script:skipCount = 0
$sw = [System.Diagnostics.Stopwatch]::StartNew()

# ── incremental helpers ─────────────────────────────────────────────────────
# Module-local dirs from `go list -deps` (cached by the Go tool).
function Get-LocalDepDirs([string]$pkg) {
    Push-Location $root
    try {
        $out = & $go list -deps -f "{{.Dir}}" $pkg 2>$null
        if ($LASTEXITCODE -ne 0) { return @() }
        $rootPrefix = $root.TrimEnd('\', '/') + [IO.Path]::DirectorySeparatorChar
        $dirs = New-Object System.Collections.Generic.List[string]
        foreach ($d in $out) {
            if (-not $d) { continue }
            $full = $d
            if ($full.StartsWith($rootPrefix, [StringComparison]::OrdinalIgnoreCase)) {
                $dirs.Add($full)
            }
        }
        return $dirs
    } finally {
        Pop-Location
    }
}

function Get-ExtraInputPaths([string]$target, [string]$pkg) {
    $paths = New-Object System.Collections.Generic.List[string]
    $paths.Add((Join-Path $root "go.mod"))
    $paths.Add((Join-Path $root "go.sum"))
    if ($target -eq "liteagent-server") {
        $static = Join-Path $root "web\static"
        if (Test-Path $static) { $paths.Add($static) }
    }
    if ($pkg -like "./plugins/*") {
        $name = Split-Path $pkg -Leaf
        $manifest = Join-Path $root "plugins\$name\plugin.json"
        if (Test-Path $manifest) { $paths.Add($manifest) }
    }
    return $paths
}

function Get-TargetHash([string]$target, [string]$pkg) {
    $files = New-Object System.Collections.Generic.List[string]
    foreach ($dir in Get-LocalDepDirs $pkg) {
        Get-ChildItem -Path $dir -Filter *.go -File -ErrorAction SilentlyContinue | ForEach-Object {
            $files.Add($_.FullName)
        }
    }
    foreach ($p in Get-ExtraInputPaths $target $pkg) {
        if (Test-Path $p -PathType Leaf) {
            $files.Add($p)
        } elseif (Test-Path $p -PathType Container) {
            Get-ChildItem -Path $p -Recurse -File -ErrorAction SilentlyContinue | ForEach-Object {
                $files.Add($_.FullName)
            }
        }
    }
    if ($files.Count -eq 0) { return $null }
    $files.Sort([StringComparer]::OrdinalIgnoreCase)
    $sha = [System.Security.Cryptography.SHA256]::Create()
    try {
        $sb = New-Object System.Text.StringBuilder
        foreach ($f in $files) {
            $info = Get-Item $f -ErrorAction SilentlyContinue
            if (-not $info) { continue }
            $rel = $info.FullName
            if ($rel.StartsWith($root, [StringComparison]::OrdinalIgnoreCase)) {
                $rel = $rel.Substring($root.Length)
            }
            # Content hash of each file (not mtime) so rebuilds are correctness-safe.
            $fs = [System.IO.File]::OpenRead($f)
            try {
                $fileHash = $sha.ComputeHash($fs)
            } finally {
                $fs.Dispose()
            }
            [void]$sb.Append($rel)
            [void]$sb.Append("`n")
            [void]$sb.Append(([BitConverter]::ToString($fileHash) -replace '-', '').ToLowerInvariant())
            [void]$sb.Append("`n")
        }
        $payload = [System.Text.Encoding]::UTF8.GetBytes($sb.ToString())
        $final = $sha.ComputeHash($payload)
        return ([BitConverter]::ToString($final) -replace '-', '').ToLowerInvariant()
    } finally {
        $sha.Dispose()
    }
}

function Get-StampPath([string]$target) {
    $safe = ($target -replace '[\\/:*?"<>|]', '_')
    return Join-Path $cacheDir ($safe + ".sha256")
}

function Test-SkipBuild([string]$target, [string]$outPath, [string]$hash) {
    if ($Clean) { return $false }
    if (-not $hash) { return $false }
    if (-not (Test-Path $outPath)) { return $false }
    $stamp = Get-StampPath $target
    if (-not (Test-Path $stamp)) { return $false }
    $prev = (Get-Content $stamp -Raw).Trim()
    return ($prev -eq $hash)
}

function Write-Stamp([string]$target, [string]$hash) {
    if (-not $hash) { return }
    $stamp = Get-StampPath $target
    [System.IO.File]::WriteAllText($stamp, $hash)
}

function Build-Pkg([string]$target, [string]$pkg, [string]$out) {
    if (-not (Should-Build $target)) {
        Write-Host "skip $target (filtered by -Only)"
        return
    }
    $hash = Get-TargetHash $target $pkg
    if (Test-SkipBuild $target $out $hash) {
        Write-Host "skip $target (unchanged)"
        $script:skipCount++
        return
    }
    Write-Host "build $pkg -> $out"
    & $go build -trimpath -ldflags "-s -w" -o $out $pkg
    if ($LASTEXITCODE -ne 0) { throw "go build $pkg failed" }
    Write-Stamp $target $hash
    $script:rebuildCount++
}

function Copy-Tree([string]$src, [string]$dst) {
    if (Test-Path $dst) {
        Remove-Item -Recurse -Force $dst -ErrorAction SilentlyContinue
    }
    Copy-Item $src $dst -Recurse -Force
}

# Install-PluginDir copies plugins/<name>/ as-is: binary, manifest, ui/ and
# README.md. A manifest without "entry" is a UI-only plugin (no process).
function Install-PluginDir([string]$name) {
    if (-not (Should-Build "plugin:$name") -and -not (Should-Build $name)) {
        Write-Host "skip plugin:$name (filtered by -Only)"
        return
    }
    $src = Join-Path $root "plugins\$name"
    $manifest = Join-Path $src "plugin.json"
    if (-not (Test-Path $manifest)) {
        Write-Warning "skip ${name}: no plugins/${name}/plugin.json"
        return
    }
    $dir = Join-Path $dist "plugins\$name"
    New-Item -ItemType Directory -Path $dir -Force | Out-Null
    $hasEntry = (Get-Content $manifest -Raw) -match '"entry"\s*:'
    if ($hasEntry) {
        Build-Pkg "plugin:$name" "./plugins/$name" (Join-Path $dir "$name.exe")
    }
    Copy-Item $manifest (Join-Path $dir "plugin.json") -Force
    foreach ($extra in @("ui", "README.md", "segments.json", "config.example.json")) {
        $p = Join-Path $src $extra
        if (-not (Test-Path $p)) { continue }
        $dst = Join-Path $dir $extra
        if (Test-Path $p -PathType Container) {
            Copy-Tree $p $dst
        } else {
            Copy-Item $p $dst -Force
        }
    }
}

if ($List) {
    Write-Host "Shipped targets:"
    Write-Host "  liteagent-cli"
    Write-Host "  liteagent-server"
    foreach ($name in $shippedPlugins) {
        $hasEntry = $false
        $manifest = Join-Path $root "plugins\$name\plugin.json"
        if ((Test-Path $manifest) -and ((Get-Content $manifest -Raw) -match '"entry"\s*:')) {
            $hasEntry = $true
        }
        $tag = if ($hasEntry) { "binary" } else { "assets-only" }
        Write-Host "  plugin:$name ($tag)"
    }
    return
}

Push-Location $root
try {
    Build-Pkg "liteagent-cli" "./cmd/liteagent-cli" (Join-Path $dist "liteagent-cli.exe")
    Build-Pkg "liteagent-server" "./cmd/liteagent-server" (Join-Path $dist "liteagent-server.exe")

    # ── plugins ─────────────────────────────────────────────────────────────
    foreach ($name in $shippedPlugins) {
        Install-PluginDir $name
    }

    # Prune plugin dirs that are no longer shipped (keep configs/sessions).
    $keep = @{}
    foreach ($name in $shippedPlugins) { $keep[$name] = $true }
    $keep["llm-openai"] = $true
    $keep["agent"] = $true
    $keep["session"] = $true
    Get-ChildItem (Join-Path $dist "plugins") -Directory -ErrorAction SilentlyContinue | ForEach-Object {
        if (-not $keep[$_.Name]) {
            Write-Host "prune dist/plugins/$($_.Name)"
            Remove-Item -Recurse -Force $_.FullName -ErrorAction SilentlyContinue
        }
    }

    # Restore user data (llm config + agent schemes + sandbox rules + sessions),
    # then seed whatever is still missing from the examples.
    if ($null -ne $llmStash -and -not (Test-Path $llmDistCfg)) {
        New-Item -ItemType Directory -Path (Split-Path -Parent $llmDistCfg) -Force | Out-Null
        Copy-Item $llmStash $llmDistCfg -Force
        Write-Host "llm-openai config: preserved dist config.json (API key survives rebuilds)"
    } elseif ($null -ne $llmStash) {
        # Dist not wiped; config already in place.
    }
    if ($null -ne $agentStash -and -not (Test-Path $agentDistCfg)) {
        New-Item -ItemType Directory -Path (Split-Path -Parent $agentDistCfg) -Force | Out-Null
        Copy-Item $agentStash $agentDistCfg -Force
        Write-Host "agent config: preserved dist config.json (schemes survive rebuilds)"
    }
    if ($null -ne $sessStash) {
        $sessTarget = Join-Path $dist "plugins\session\sessions"
        if (-not (Test-Path $sessTarget)) {
            New-Item -ItemType Directory -Path (Split-Path -Parent $sessTarget) -Force | Out-Null
            Copy-Item $sessStash $sessTarget -Recurse -Force
            Write-Host "sessions: preserved $( (Get-ChildItem -Path $sessTarget -Filter *.jsonl -ErrorAction SilentlyContinue).Count ) file(s)"
        }
    }
    if ($null -ne $sandboxStash -and -not (Test-Path $sandboxPerm)) {
        New-Item -ItemType Directory -Path (Split-Path -Parent $sandboxPerm) -Force | Out-Null
        Copy-Item $sandboxStash $sandboxPerm -Force
        Write-Host "sandbox permissions: preserved dist config/permissions.json"
    }
    if ($null -ne $stash -and (Test-Path $stash)) {
        Remove-Item $stash -Recurse -Force -ErrorAction SilentlyContinue
    }
    # llm-openai config priority: existing dist config, then a repo-local
    # config.json (gitignored), else seed the empty example template.
    # The build never injects or rewrites an API key.
    $repoCfg = Join-Path $root "plugins\llm-openai\config.json"
    if (Test-Path $llmDistCfg) {
        # already present
    } elseif (Test-Path $repoCfg) {
        New-Item -ItemType Directory -Path (Split-Path -Parent $llmDistCfg) -Force | Out-Null
        Copy-Item $repoCfg $llmDistCfg -Force
        Write-Host "llm-openai config: seeded from repo plugins\llm-openai\config.json (kept as-is)"
    } else {
        New-Item -ItemType Directory -Path (Split-Path -Parent $llmDistCfg) -Force | Out-Null
        Copy-Item (Join-Path $root "plugins\llm-openai\config.example.json") $llmDistCfg -Force
        Write-Warning "no llm-openai config yet — set your key via /llm-openai config set apiKey=... or edit dist\plugins\llm-openai\config.json; later rebuilds will keep it"
    }
    # Seed agent config.json once (scheme defaults); later rebuilds keep user edits.
    if (-not (Test-Path $agentDistCfg)) {
        $agentExample = Join-Path $root "plugins\agent\config.example.json"
        if (Test-Path $agentExample) {
            New-Item -ItemType Directory -Path (Split-Path -Parent $agentDistCfg) -Force | Out-Null
            Copy-Item $agentExample $agentDistCfg -Force
            Write-Host "agent config: seeded default schemes (chat/tool_calling/coding)"
        }
    }

    # ── examples & docs ─────────────────────────────────────────────────────
    New-Item -ItemType Directory -Path (Join-Path $dist "config") -Force | Out-Null
    $permCfg = Join-Path $dist "config\permissions.json"
    if (-not (Test-Path $permCfg)) {
        $utf8NoBom = New-Object System.Text.UTF8Encoding $false
        [System.IO.File]::WriteAllText($permCfg, "{`n  `"defaultAction`": `"allow`",`n  `"rules`": []`n}`n", $utf8NoBom)
    }
    # Layout is required at runtime (ADR-0012); ship the base layout with dist.
    Copy-Item (Join-Path $root "layout.json") $dist -Force
    # Author SDK single source (dual export: repo copy + /sdk/ HTTP).
    Copy-Item (Join-Path $root "sdk\lite-agent.js") (Join-Path $dist "lite-agent.js") -Force
    Copy-Item (Join-Path $root "README.md") $dist -Force
    Copy-Item (Join-Path $root "CONTEXT.md") $dist -Force
    Copy-Item (Join-Path $root "plugins\README.md") (Join-Path $dist "plugins\README.md") -Force

    $sw.Stop()
    Write-Host ""
    Write-Host ("Release layout ready: {0}  (rebuilt {1}, skipped {2}, {3:n1}s)" -f $dist, $script:rebuildCount, $script:skipCount, $sw.Elapsed.TotalSeconds)
    Get-ChildItem -Recurse $dist | Where-Object { -not $_.PSIsContainer -and $_.FullName -notlike ($cacheDir + "*") } | ForEach-Object {
        $_.FullName.Substring($dist.Length + 1)
    }
}
finally {
    Pop-Location
}
