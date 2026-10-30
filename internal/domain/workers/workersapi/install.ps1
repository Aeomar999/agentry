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
# The server's own CA, when it is served with one (-https): the download
# trusts it alone, and joining keeps it. Both are '' when it needs none.
$ca = {{CA}}
$caFingerprint = {{CA_FINGERPRINT}}
$bin = if ($env:AGENTRY_BIN) { $env:AGENTRY_BIN } else { Join-Path $env:LOCALAPPDATA 'Agentry\bin' }
$arch = if ($env:PROCESSOR_ARCHITECTURE -eq 'ARM64') { 'arm64' } else { 'amd64' }
$exe = Join-Path $bin 'agentry.exe'

# Downloads uri to path. With the server's own CA it uses curl.exe, which
# comes with Windows 10 and later and can trust that CA alone; Schannel's
# revocation check fails for a CA that publishes no list.
function Save-Download($uri, $path) {
  if (-not $ca) {
    Invoke-WebRequest -UseBasicParsing -Uri $uri -OutFile $path
    return
  }
  $caFile = (New-TemporaryFile).FullName
  try {
    Set-Content $caFile $ca
    curl.exe -fsSL --cacert $caFile --ssl-no-revoke -o $path $uri
    if ($LASTEXITCODE -ne 0) { throw "curl.exe exited with $LASTEXITCODE" }
  } finally {
    Remove-Item -Force -ErrorAction SilentlyContinue $caFile
  }
}

Write-Host "Installing agentry in $bin"
New-Item -ItemType Directory -Force -Path $bin | Out-Null
try {
  Save-Download "$server/worker/agentry?code=$code&os=windows&arch=$arch" "$exe.part"
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
$join = @('worker', 'join')
if ($caFingerprint) { $join += '-ca-fingerprint', $caFingerprint }
if ($env:AGENTRY_WORKER_NAME) { $join += '-name', $env:AGENTRY_WORKER_NAME }
& $exe @join $server $code
if ($LASTEXITCODE -ne 0) { exit $LASTEXITCODE }
& $exe worker start

# Put agentry on your PATH, for new terminals.
$path = [Environment]::GetEnvironmentVariable('Path', 'User')
if (-not (($path -split ';') -contains $bin)) {
  [Environment]::SetEnvironmentVariable('Path', ($path.TrimEnd(';') + ';' + $bin).TrimStart(';'), 'User')
  Write-Host "Added $bin to your PATH; open a new terminal to use agentry."
}
Write-Host ''
