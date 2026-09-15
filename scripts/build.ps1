# Build a Windows release layout under dist/.
# Usage: powershell -File scripts\build.ps1
$ErrorActionPreference = "Stop"

$root = Split-Path -Parent $PSScriptRoot
$dist = Join-Path $root "dist"
$go = "go"

# Preserve the user's llm-openai config (API key) across the dist wipe below.
$llmDistCfg = Join-Path $dist "plugins\llm-openai\config.json"
$llmStash = $null
if (Test-Path $llmDistCfg) {
    $llmStash = Join-Path ([IO.Path]::GetTempPath()) ("llm-openai-config-" + [guid]::NewGuid().ToString("N") + ".json")
    Copy-Item $llmDistCfg $llmStash -Force
}

if (Test-Path $dist) {
    try {
        Remove-Item -Recurse -Force $dist -ErrorAction Stop
    } catch {
        Write-Warning "dist is locked (cwd or running process); overwriting in place. Close terminals using dist\ and stop liteagent-server.exe for a clean wipe."
    }
}
New-Item -ItemType Directory -Path $dist -Force | Out-Null
New-Item -ItemType Directory -Path (Join-Path $dist "plugins") -Force | Out-Null
New-Item -ItemType Directory -Path (Join-Path $dist "examples") -Force | Out-Null

function Build-Pkg([string]$pkg, [string]$out) {
    Write-Host "build $pkg -> $out"
    & $go build -trimpath -ldflags "-s -w" -o $out $pkg
    if ($LASTEXITCODE -ne 0) { throw "go build $pkg failed" }
}

function Install-Plugin([string]$name, [string]$pkg, [string]$provides, [string]$consumes = "[]", [int]$timeoutMs = 0, [string]$description = "", [string]$commands = "") {
    $dir = Join-Path $dist "plugins\$name"
    New-Item -ItemType Directory -Path $dir | Out-Null
    $exe = Join-Path $dir "$name.exe"
    Build-Pkg $pkg $exe
    $extra = ""
    if ($timeoutMs -gt 0) {
        $extra += ",`n  `"timeoutMs`": $timeoutMs"
    }
    if ($description -ne "") {
        $extra += ",`n  `"description`": `"$description`""
    }
    if ($commands -ne "") {
        $extra += ",`n  `"commands`": $commands"
    }
    $manifest = @"
{
  "name": "$name",
  "version": "0.1.0",
  "protocol": 3,
  "provides": $provides,
  "consumes": $consumes,
  "entry": "$name.exe"$extra
}
"@
    # UTF-8 without BOM — Host's JSON parser rejects a BOM.
    $utf8NoBom = New-Object System.Text.UTF8Encoding $false
    [System.IO.File]::WriteAllText((Join-Path $dir "plugin.json"), $manifest, $utf8NoBom)
}

Push-Location $root
try {
    Build-Pkg "./cmd/liteagent-cli" (Join-Path $dist "liteagent-cli.exe")
    Build-Pkg "./cmd/liteagent-server" (Join-Path $dist "liteagent-server.exe")

    Install-Plugin "session"        "./plugins/session"        '["session"]' "[]" 0 "File-backed session log plugin (JSONL) with the session trace Web view" '[{"name":"dump-trace","description":"Export Session Log facts as JSON","usage":"/session dump-trace [sessionId]"},{"name":"list","description":"List sessions","usage":"/session list"},{"name":"derive","description":"Print Model Context from Session Log","usage":"/session derive [sessionId]"},{"name":"current","description":"Show Current Session id","usage":"/session current"}]'
    # session ships its own manifest (ui mounts: session-trace) and its UI Entry.
    Copy-Item (Join-Path $root "plugins\session\plugin.json") (Join-Path $dist "plugins\session\") -Force
    Copy-Item (Join-Path $root "plugins\session\ui") (Join-Path $dist "plugins\session\") -Recurse -Force
    Install-Plugin "llm-openai"     "./plugins/llm-openai"     '["llm"]' "[]" 120000 "OpenAI-compatible LLM provider" '[{"name":"config","description":"Show or set API key / model / baseURL","usage":"/llm-openai config [get|set key=value]"}]'
    Install-Plugin "echotool"       "./plugins/echotool"       '["tools"]' "[]" 0 "Echo tool with presentation card"
    Install-Plugin "filetools"      "./plugins/filetools"      '["tools"]' "[]" 0 "Read/write workspace files"
    Install-Plugin "context-manager" "./plugins/context-manager" '["system-prompt","context"]' "[]" 0 "Context Manager: system prompt + prepare/compact/usage" '[{"name":"usage","description":"Last prepare Context Usage","usage":"/context-manager usage [sessionId]"},{"name":"list","description":"Model Context messages from last prepare","usage":"/context-manager list [sessionId]"},{"name":"skills","description":"List registered skills","usage":"/context-manager skills"}]'
    Install-Plugin "echo"           "./plugins/echo"           '["echo"]' "[]" 0 "Echo capability plugin"
    Install-Plugin "uidemo"         "./plugins/uidemo"         '[]' "[]" 0 "Web Panel Component reference"
    # uidemo ships its own manifest (ui.entry + ui.mounts) and its UI Entry module.
    Copy-Item (Join-Path $root "plugins\uidemo\plugin.json") (Join-Path $dist "plugins\uidemo\") -Force
    Copy-Item (Join-Path $root "plugins\uidemo\ui") (Join-Path $dist "plugins\uidemo\") -Recurse -Force

    Copy-Item (Join-Path $root "plugins\context-manager\segments.json") (Join-Path $dist "plugins\context-manager\") -Force
    Copy-Item (Join-Path $root "plugins\llm-openai\config.example.json") (Join-Path $dist "plugins\llm-openai\") -Force
    # llm-openai config priority: keep what the user already runs (dist), then a
    # repo-local config.json (gitignored), else seed the empty example template.
    # The build never injects or rewrites an API key.
    $repoCfg = Join-Path $root "plugins\llm-openai\config.json"
    if ($null -ne $llmStash) {
        Copy-Item $llmStash $llmDistCfg -Force
        Remove-Item $llmStash -Force
        Write-Host "llm-openai config: preserved dist config.json (API key survives rebuilds)"
    } elseif (Test-Path $repoCfg) {
        Copy-Item $repoCfg $llmDistCfg -Force
        Write-Host "llm-openai config: seeded from repo plugins\llm-openai\config.json (kept as-is)"
    } else {
        Copy-Item (Join-Path $root "plugins\llm-openai\config.example.json") $llmDistCfg -Force
        Write-Warning "no llm-openai config yet — set your key via /llm-openai config set apiKey=... or edit dist\plugins\llm-openai\config.json; later rebuilds will keep it"
    }

    Copy-Item (Join-Path $root "examples\assembly.json") (Join-Path $dist "examples\") -Force
    Copy-Item (Join-Path $root "examples\assembly-with-tools.json") (Join-Path $dist "examples\") -Force
    Copy-Item (Join-Path $root "examples\chat.json") (Join-Path $dist "examples\") -Force
    Copy-Item (Join-Path $root "examples\agent.json") (Join-Path $dist "examples\") -Force
    # Layout is required at runtime (ADR-0012); ship the base layout with dist.
    Copy-Item (Join-Path $root "layout.json") $dist -Force
    # Author SDK single source (dual export: repo copy + /sdk/ HTTP).
    Copy-Item (Join-Path $root "sdk\lite-agent.js") (Join-Path $dist "lite-agent.js") -Force
    Copy-Item (Join-Path $root "README.md") $dist -Force
    Copy-Item (Join-Path $root "CONTEXT.md") $dist -Force

    Write-Host ""
    Write-Host "Release layout ready: $dist"
    Get-ChildItem -Recurse $dist | ForEach-Object { $_.FullName.Substring($dist.Length + 1) }
}
finally {
    Pop-Location
}
