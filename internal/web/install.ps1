# Makes this Windows machine an Agentry worker: installs the agentry command,
# joins the server with a one-time code, and starts the worker in the
# background, started again when you log in. Run again on a machine that has
# joined, it updates agentry and restarts.
#
#   AGENTRY_WORKER_NAME  what to call this machine (default: its name)
#   AGENTRY_BIN          where agentry goes (default: %LOCALAPPDATA%\Agentry\bin)
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
  Invoke-WebRequest -UseBasicParsing -Uri "$server/worker/agentry?code=$code&os=windows&arch=$arch" -OutFile "$exe.part"
} catch {
  Remove-Item -Force -ErrorAction SilentlyContinue "$exe.part"
  Write-Error "The server has no agentry for windows/$arch."
  exit 1
}
# Windows won't replace a program that runs: a worker running already stops
# first, and the old agentry (Claude Code may be running it) is moved aside.
if (Test-Path $exe) {
  & $exe worker stop 2>$null | Out-Null
  Remove-Item -Force -ErrorAction SilentlyContinue "$exe.old"
  Move-Item -Force $exe "$exe.old"
}
Move-Item -Force "$exe.part" $exe

foreach ($tool in 'git', 'claude') {
  if (-not (Get-Command $tool -ErrorAction SilentlyContinue)) { Write-Host "Note: $tool isn't installed here; agents need it." }
}

# Joins, or, if this machine has joined already, leaves the code unused.
if ($env:AGENTRY_WORKER_NAME) { & $exe worker join -name $env:AGENTRY_WORKER_NAME $server $code } else { & $exe worker join $server $code }
if ($LASTEXITCODE -ne 0) { exit $LASTEXITCODE }
& $exe worker start

# Put agentry on your PATH, for new terminals.
$path = [Environment]::GetEnvironmentVariable('Path', 'User')
if (-not (($path -split ';') -contains $bin)) {
  [Environment]::SetEnvironmentVariable('Path', ($path.TrimEnd(';') + ';' + $bin).TrimStart(';'), 'User')
  Write-Host "Added $bin to your PATH; open a new terminal to use agentry."
}
Write-Host ''
