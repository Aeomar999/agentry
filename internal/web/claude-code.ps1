# Gives Claude Code on this Windows machine Agentry's tools, for every repo:
# installs the agentry command, connects it to the server with a one-time
# code, and adds it to Claude Code with its status line. Run again with a
# new command from the Integrations page, it updates agentry and connects
# afresh.
#
#   AGENTRY_BIN  where agentry goes (default: %LOCALAPPDATA%\Agentry\bin)
$ErrorActionPreference = 'Stop'
$ProgressPreference = 'SilentlyContinue'

$server = {{SERVER}}
$code = {{CODE}}
$bin = if ($env:AGENTRY_BIN) { $env:AGENTRY_BIN } else { Join-Path $env:LOCALAPPDATA 'Agentry\bin' }
$arch = if ($env:PROCESSOR_ARCHITECTURE -eq 'ARM64') { 'arm64' } else { 'amd64' }
$exe = Join-Path $bin 'agentry.exe'

Write-Host "Installing agentry in $bin"
New-Item -ItemType Directory -Force -Path $bin | Out-Null
try {
  Invoke-WebRequest -UseBasicParsing -Uri "$server/integrations/agentry?code=$code&os=windows&arch=$arch" -OutFile "$exe.part"
} catch {
  Remove-Item -Force -ErrorAction SilentlyContinue "$exe.part"
  Write-Error "The server has no agentry for windows/$arch."
  exit 1
}
# A running agentry (a worker, or Claude Code's) can't be replaced: it is moved aside.
if (Test-Path $exe) {
  Remove-Item -Force -ErrorAction SilentlyContinue "$exe.old"
  Move-Item -Force $exe "$exe.old"
}
Move-Item -Force "$exe.part" $exe

& $exe mcp connect $server $code
if ($LASTEXITCODE -ne 0) { exit $LASTEXITCODE }

if (Get-Command claude -ErrorAction SilentlyContinue) {
  & $exe mcp install
} else {
  Write-Host "Claude Code isn't installed here. Install it, then run: $exe mcp install"
}

$path = [Environment]::GetEnvironmentVariable('Path', 'User')
if (-not (($path -split ';') -contains $bin)) {
  [Environment]::SetEnvironmentVariable('Path', ($path.TrimEnd(';') + ';' + $bin).TrimStart(';'), 'User')
  Write-Host "Added $bin to your PATH; open a new terminal to use agentry."
}
Write-Host ''
Write-Host 'Done. Start claude in any repo and ask it for your Agentry board.'
