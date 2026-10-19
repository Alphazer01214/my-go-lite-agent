# Build a release layout under dist/.
# Usage:
#   powershell -File scripts\build.ps1            # incremental (skip unchanged binaries)
#   powershell -File scripts\build.ps1 -Clean     # wipe dist and rebuild everything
#   powershell -File scripts\build.ps1 -Only agent,session
#   powershell -File scripts\build.ps1 -Verify   # go vet + go test (+node) before building
#   powershell -File scripts\build.ps1 -List
#
# Incremental mode hashes each target's module-local Go deps (plus go.mod/go.sum,
# the plugin manifest, and — for the server — the go:embed'ed WebUI assets under
# internal\webui\static). Unchanged targets reuse the
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
    [switch]$Verify,
    [string[]]$Only = @()
)
$ErrorActionPreference = "Stop"

$root = Split-Path -Parent $PSScriptRoot
$dist = Join-Path $root "dist"
$cacheDir = Join-Path $dist ".build-cache"
$webStatic = Join-Path $root "internal\webui\static"
$go = "go"

# Release stamp: git describe when available, else "dev". Injected into the
# server binary via -X so a dist build is always identifiable.
$version = "dev"
try { $described = & git -C $root describe --tags --always --dirty 2>$null } catch { $described = $null }
if ($described) { $version = ("$described").Trim() }

# ── shipped plugins ─────────────────────────────────────────────────────────
# Single source of truth: scripts/shipped-plugins.conf (shared with build.sh).
$groups = @{}
foreach ($line in Get-Content (Join-Path $PSScriptRoot "shipped-plugins.conf")) {
    if ($line -match '^\s*(\w+):\s*(.+?)\s*$') {
        $groups[$matches[1]] = @($matches[2] -split '\s+')
    }
}
$corePlugins = @($groups["core"] | Where-Object { $_ })
$toolsPlugins = @($groups["tools"] | Where-Object { $_ })
$shippedPlugins = @($corePlugins + $toolsPlugins | Where-Object { $_ })

# Listed in the conf but not a mountable plugin yet (no plugin.json). Warn once
# instead of once per target; a missing plugin never aborts the build.
$missingPlugins = @($shippedPlugins | Where-Object { -not (Test-Path (Join-Path $root "plugins\$_\plugin.json")) })

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
    # Accept short names: "agent" matches "plugin:agent".
    if ($target.StartsWith("plugin:")) {
        $short = $target.Substring("plugin:".Length)
        if ($onlySet[$short]) { return $true }
    }
    if ($onlySet["plugin:$target"]) { return $true }
    return $false
}

# Copy an optional repo asset into dist; missing source warns and skips (never aborts).
function Copy-DistAsset([string]$rel, [string]$dst) {
    $src = Join-Path $root $rel
    if (Test-Path $src) {
        Copy-Item $src $dst -Force
    } else {
        Write-Warning "skip missing dist asset: $rel"
    }
}

# ── pre-flight verification (opt-in) ────────────────────────────────────────
# Fail before touching dist\. go test also warms the module build cache, which
# makes the subsequent `go build` calls cheaper.
if ($Verify) {
    Write-Host "verify: go vet ./..."
    & $go vet ./...
    if ($LASTEXITCODE -ne 0) { throw "go vet failed" }
    Write-Host "verify: go test ./..."
    & $go test ./...
    if ($LASTEXITCODE -ne 0) { throw "go test failed" }
    $mjs = Get-ChildItem -Path (Join-Path $root "internal\webui") -Filter *.test.mjs -File -ErrorAction SilentlyContinue
    if ((-not $mjs) -or (-not (Get-Command node -ErrorAction SilentlyContinue))) {
        Write-Warning "verify: node not found or no browser tests - skipped"
    } else {
        Write-Host "verify: node --test internal/webui/*.test.mjs"
        & node --test @($mjs.FullName)
        if ($LASTEXITCODE -ne 0) { throw "node tests failed" }
    }
}
if ($missingPlugins.Count -gt 0) {
    Write-Warning ("shipped-plugins.conf lists plugin(s) without plugins/<name>/plugin.json: " + ($missingPlugins -join ", "))
}

# ── user-data preserve (needed when wiping; cheap no-op otherwise) ───────────
$llmDistCfg = Join-Path $dist "plugins\llm-openai\config.json"
$agentDistCfg = Join-Path $dist "plugins\agent\config.json"
$sessDistDir = Join-Path $dist "plugins\session\sessions"
$sessionWorkspaceFile = Join-Path $sessDistDir "workspaces.json"
$legacyWorkspaceFile = Join-Path $dist "plugins\workspace\workspaces.json"
$stash = Join-Path ([IO.Path]::GetTempPath()) ("liteagent-stash-" + [guid]::NewGuid().ToString("N"))
$llmStash = $null
$sessStash = $null
$legacyWorkspaceStash = $null
$agentStash = $null
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
if (Test-Path $legacyWorkspaceFile) {
    New-Item -ItemType Directory -Path $stash -Force | Out-Null
    $legacyWorkspaceStash = Join-Path $stash "legacy-workspaces.json"
    Copy-Item $legacyWorkspaceFile $legacyWorkspaceStash -Force
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
        # The server go:embeds internal/webui/static; without these files in the
        # hash a shell/asset-only edit would leave a stale dist binary behind.
        if (Test-Path $webStatic) { $paths.Add($webStatic) }
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
    $ldflags = "-s -w"
    if ($target -eq "liteagent-server") { $ldflags = "-s -w -X main.version=$version" }
    & $go build -trimpath -ldflags $ldflags -o $out $pkg
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
    if ($missingPlugins.Count -gt 0) {
        Write-Warning ("no plugin.json: " + ($missingPlugins -join ", "))
    }
    return
}

Push-Location $root
try {
    Build-Pkg "liteagent-server" "./cmd/liteagent-server" (Join-Path $dist "liteagent-server.exe")

    # ── plugins ─────────────────────────────────────────────────────────────
    foreach ($name in $shippedPlugins) {
        Install-PluginDir $name
    }

    # Workspace Registry moved into session. Preserve an existing legacy file
    # before pruning the old standalone plugin directory.
    if ((Test-Path $legacyWorkspaceFile) -and -not (Test-Path $sessionWorkspaceFile)) {
        New-Item -ItemType Directory -Path $sessDistDir -Force | Out-Null
        Copy-Item $legacyWorkspaceFile $sessionWorkspaceFile -Force
        Write-Host "workspace registry: migrated dist/plugins/workspace into session"
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
            $stale = Get-StampPath "plugin:$($_.Name)"
            if (Test-Path $stale) { Remove-Item $stale -Force -ErrorAction SilentlyContinue }
        }
    }

    # Restore user data (llm config + agent schemes + sessions),
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
        New-Item -ItemType Directory -Path $sessTarget -Force | Out-Null
        $stashedSessions = @(Get-ChildItem -LiteralPath $sessStash -Force -ErrorAction SilentlyContinue)
        if ($stashedSessions.Count -gt 0) {
            Copy-Item (Join-Path $sessStash "*") $sessTarget -Recurse -Force
        }
        Write-Host "sessions: preserved $( (Get-ChildItem -Path $sessTarget -Filter *.jsonl -ErrorAction SilentlyContinue).Count ) file(s)"
    }
    if ($null -ne $legacyWorkspaceStash -and -not (Test-Path $sessionWorkspaceFile)) {
        New-Item -ItemType Directory -Path $sessDistDir -Force | Out-Null
        Copy-Item $legacyWorkspaceStash $sessionWorkspaceFile -Force
        Write-Host "workspace registry: restored legacy data into session"
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
    } elseif (Test-Path (Join-Path $root "plugins\llm-openai\config.example.json")) {
        New-Item -ItemType Directory -Path (Split-Path -Parent $llmDistCfg) -Force | Out-Null
        Copy-Item (Join-Path $root "plugins\llm-openai\config.example.json") $llmDistCfg -Force
        Write-Warning "no llm-openai config yet — set your key via /llm-openai config set apiKey=... or edit dist\plugins\llm-openai\config.json; later rebuilds will keep it"
    } else {
        Write-Warning "no config template for llm-openai (dist keeps no config.json)"
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

    # ── docs ────────────────────────────────────────────────────────────────
    Copy-DistAsset "README.md" $dist
    Copy-DistAsset "CONTEXT.md" $dist
    Copy-DistAsset "plugins\README.md" (Join-Path $dist "plugins\README.md")

    $sw.Stop()
    Write-Host ""
    Write-Host ("Release layout ready: {0}  (rebuilt {1}, skipped {2}, {3:n1}s, version {4})" -f $dist, $script:rebuildCount, $script:skipCount, $sw.Elapsed.TotalSeconds, $version)
    Get-ChildItem -Recurse $dist | Where-Object { -not $_.PSIsContainer -and $_.FullName -notlike ($cacheDir + "*") } | ForEach-Object {
        $_.FullName.Substring($dist.Length + 1)
    }
}
finally {
    Pop-Location
}
