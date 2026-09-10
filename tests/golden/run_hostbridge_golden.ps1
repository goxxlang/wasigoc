# Compiles+runs one wasigocvm golden that needs the companion native
# gocvm_host process (WASIGOCVM_HOST_BRIDGE): os.exec/os.user/syscall/
# tls.dial topics that categorically cannot run inside the wasm32 sandbox.
# check_wasm.cmake can't drive this alone -- it runs one static COMMAND, but
# this needs a process started before wasmtime and torn down after -- so this
# script does the compile (via run_golden.cmake -DCOMPILE_ONLY=1, to reuse
# its flag logic instead of duplicating it), starts gocvm_host.exe on a
# throwaway port, waits for it to accept, runs wasmtime with network access
# granted and WASIGOCVM_HOST_ADDR pointing at that port, then compares
# stdout against the *real* pid of the gocvm_host.exe process this run
# started (syscall.Getpid() forwards to gocvm_host, which answers with its
# own GetCurrentProcessId() -- see shim_sandbox/src/sapi/real_win.cc -- so
# there is no fixed expected string to compare against; a match proves the
# call round-tripped through the real host bridge, not the wasm32-local
# "no bridge" fallback (which would print 1 instead)).
param(
  [Parameter(Mandatory)] [string]$Cmake,
  [Parameter(Mandatory)] [string]$RunGoldenScript,
  [Parameter(Mandatory)] [string]$ClangXX,
  [Parameter(Mandatory)] [string]$Sysroot,
  [Parameter(Mandatory)] [string]$IncludeDir,
  [Parameter(Mandatory)] [string]$SrcDir,
  [Parameter(Mandatory)] [string]$CppFile,
  [Parameter(Mandatory)] [string]$WasmFile,
  [Parameter(Mandatory)] [string]$Wasmtime,
  [Parameter(Mandatory)] [string]$HostExe,
  [int]$Port = 47829
)

$ErrorActionPreference = 'Stop'

function Fail([string]$msg) {
  Write-Error "[hostbridge] $msg"
  exit 1
}

Write-Host "[hostbridge] compiling $CppFile -> $WasmFile (WASIGOCVM_HOST_BRIDGE=1)"
# Splatted, not a backtick-continued inline call: an inline multi-line `&`
# invocation here was observed to make cmake's execute_process silently
# mis-receive args (compile fails with a WHOLLY EMPTY stdout/stderr, not a
# normal clang diagnostic) even though the same args printed correctly --
# root cause not pinned down, but @cmakeArgs splatting reproduces cleanly
# every time and the inline form does not. If touching this, keep splatting.
$cmakeArgs = @(
  "-DCLANGXX=$ClangXX",
  "-DSYSROOT=$Sysroot",
  "-DINCLUDE_DIR=$IncludeDir",
  "-DSRC_DIR=$SrcDir",
  "-DCPP_FILE=$CppFile",
  "-DWASM_FILE=$WasmFile",
  "-DWASI_PREVIEW=p2",
  "-DWASIGO_CXXLIB=full",
  "-DEXTRA_DEFINES=WASIGOCVM_HOST_BRIDGE=1",
  "-DCOMPILE_ONLY=1",
  "-P", $RunGoldenScript
)
& $Cmake @cmakeArgs
if ($LASTEXITCODE -ne 0) { Fail "compile step failed (exit $LASTEXITCODE)" }

# gocvm_host.exe's main() does exactly ONE accept() then serves that single
# connection and exits (see shim_sandbox/src/gocvm_host.cc's own comment:
# "One process serves exactly one wasm-guest connection then exits"). A
# real TCP connect-then-close readiness probe would itself BE that one
# accepted connection, leaving nothing left for the wasm guest to talk to
# (this was tried and reliably reproduced the guest falling back to "no
# bridge" -- pid 1 -- even though the compile/start steps all succeeded).
# So wait for its own stderr banner ("listening on ...", printed right
# after listen() succeeds, before accept()) instead of touching the socket.
Write-Host "[hostbridge] starting $HostExe on 127.0.0.1:$Port"
$hostErrFile = [System.IO.Path]::GetTempFileName()
$hostOutFile = [System.IO.Path]::GetTempFileName()
$hostProc = Start-Process -FilePath $HostExe -ArgumentList "$Port" -PassThru -WindowStyle Hidden `
    -RedirectStandardError $hostErrFile -RedirectStandardOutput $hostOutFile
$hostPid = $hostProc.Id

$deadline = (Get-Date).AddSeconds(5)
$ready = $false
while ((Get-Date) -lt $deadline) {
  if ($hostProc.HasExited) { break }
  $banner = Get-Content -Path $hostErrFile -Raw -ErrorAction SilentlyContinue
  if ($banner -and $banner -match 'listening on') {
    $ready = $true
    break
  }
  Start-Sleep -Milliseconds 100
}
if (-not $ready) {
  if (-not $hostProc.HasExited) { Stop-Process -Id $hostPid -Force -ErrorAction SilentlyContinue }
  $banner = Get-Content -Path $hostErrFile -Raw -ErrorAction SilentlyContinue
  Remove-Item $hostErrFile, $hostOutFile -Force -ErrorAction SilentlyContinue
  Fail "gocvm_host.exe (pid $hostPid) never started listening on 127.0.0.1:${Port}: $banner"
}

Write-Host "[hostbridge] running $WasmFile under wasmtime (expect real host pid $hostPid back)"
$wasmtimeArgs = @(
  "run", "--dir=.::.", "-W", "timeout=30s", "-S", "inherit-network", "-S", "tcp",
  "--env", "WASIGOCVM_HOST_ADDR=127.0.0.1:$Port", $WasmFile
)
$actualLines = & $Wasmtime @wasmtimeArgs
$wasmtimeExit = $LASTEXITCODE

# gocvm_host.exe serves exactly one connection then exits on its own; this
# is just a defensive backstop if wasmtime crashed before ever connecting.
Start-Sleep -Milliseconds 200
if (-not $hostProc.HasExited) { Stop-Process -Id $hostPid -Force -ErrorAction SilentlyContinue }
Remove-Item $hostErrFile, $hostOutFile -Force -ErrorAction SilentlyContinue

if ($wasmtimeExit -ne 0) {
  Fail "wasmtime exited $wasmtimeExit"
}

$actualTrim = (($actualLines -join "`n")).Trim()
$expectedTrim = "$hostPid"
if ($actualTrim -ne $expectedTrim) {
  Fail ("output mismatch: expected real gocvm_host pid '$expectedTrim', got '$actualTrim' " +
        "(pid 1 means the host bridge never actually engaged -- check WASIGOCVM_HOST_BRIDGE wiring)")
}

Write-Host "[hostbridge] OK: guest received real host pid $actualTrim"
exit 0
