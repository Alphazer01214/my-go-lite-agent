# Build a Windows release layout under dist/.
# Usage: powershell -File scripts\build.ps1
$ErrorActionPreference = "Stop"

$root = Split-Path -Parent $PSScriptRoot
$dist = Join-Path $root "dist"
$go = "go"

if (Test-Path $dist) {
    Remove-Item -Recurse -Force $dist
}
New-Item -ItemType Directory -Path $dist | Out-Null
New-Item -ItemType Directory -Path (Join-Path $dist "plugins") | Out-Null
New-Item -ItemType Directory -Path (Join-Path $dist "examples") | Out-Null

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
  "protocol": 2,
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
    Build-Pkg "./cmd/host" (Join-Path $dist "host.exe")

    Install-Plugin "session"        "./plugins/session"        '["session"]' "[]" 0 "In-memory session log plugin"
    Install-Plugin "fakellm"        "./plugins/fakellm"        '["llm"]' "[]" 0 "Deterministic fake LLM for tests"
    Install-Plugin "llm-openai"     "./plugins/llm-openai"     '["llm"]' "[]" 120000 "OpenAI-compatible LLM provider" '[{"name":"config","description":"Show or set API key / model / baseURL","usage":"/llm-openai config [get|set key=value]"}]'
    Install-Plugin "echotool"       "./plugins/echotool"       '["tools"]' "[]" 0 "Echo tool with presentation card"
    Install-Plugin "filetools"      "./plugins/filetools"      '["tools"]' "[]" 0 "Read/write workspace files"
    Install-Plugin "contextmanager" "./plugins/contextmanager" '["system-prompt"]' "[]" 0 "System prompt segment assembler"
    Install-Plugin "echo"           "./plugins/echo"           '["echo"]' "[]" 0 "Echo capability plugin"
    Install-Plugin "interceptor"    "./plugins/interceptor"    '["interceptor"]' "[]" 0 "Demo waterfall interceptor"
    Install-Plugin "uidemo"         "./plugins/uidemo"         '[]' "[]" 0 "Web Panel demo (mode switch)"
    Copy-Item (Join-Path $root "plugins\uidemo\ui") (Join-Path $dist "plugins\uidemo\") -Recurse -Force

    Copy-Item (Join-Path $root "plugins\contextmanager\segments.json") (Join-Path $dist "plugins\contextmanager\") -Force
    Copy-Item (Join-Path $root "plugins\llm-openai\config.example.json") (Join-Path $dist "plugins\llm-openai\") -Force

    Copy-Item (Join-Path $root "examples\assembly.json") (Join-Path $dist "examples\") -Force
    Copy-Item (Join-Path $root "examples\assembly-with-tools.json") (Join-Path $dist "examples\") -Force
    Copy-Item (Join-Path $root "examples\chat.json") (Join-Path $dist "examples\") -Force
    Copy-Item (Join-Path $root "examples\agent.json") (Join-Path $dist "examples\") -Force
    Copy-Item (Join-Path $root "README.md") $dist -Force
    Copy-Item (Join-Path $root "CONTEXT.md") $dist -Force

    Write-Host ""
    Write-Host "Release layout ready: $dist"
    Get-ChildItem -Recurse $dist | ForEach-Object { $_.FullName.Substring($dist.Length + 1) }
}
finally {
    Pop-Location
}
