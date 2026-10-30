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
# The server's own CA, when it is served with one (-https): the download
# trusts it alone, and connecting keeps it. Both are '' when it needs none.
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
  Save-Download "$server/integrations/agentry?code=$code&os=windows&arch=$arch" "$exe.part"
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

$connect = @('mcp', 'connect')
if ($caFingerprint) { $connect += '-ca-fingerprint', $caFingerprint }
& $exe @connect $server $code
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
