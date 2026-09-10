# bootstrap.ps1 — launch wasigocvm sysroot build via WSL (or print next steps).
#
#   .\toolchain\bootstrap.ps1
#   .\toolchain\bootstrap.ps1 -Jobs 8
#   .\toolchain\bootstrap.ps1 -Ref wasi-sdk-33
param(
  [int]$Jobs = 0,
  [string]$Ref = "main",
  [switch]$DryRun
)

$ErrorActionPreference = "Stop"
$Root = Split-Path -Parent $PSScriptRoot
if ($Jobs -le 0) { $Jobs = [Environment]::ProcessorCount }

$wsl = Get-Command wsl -ErrorAction SilentlyContinue
if (-not $wsl) {
  Write-Host @"
No WSL found. Options:
  1) Install WSL + cmake/ninja/git, then re-run this script
  2) On Linux CI: ./toolchain/bootstrap.sh
  3) Copy a prebuilt WASIGO_TOOLCHAIN tree into $Root\toolchain\

Until then wasigocvm.bat falls back to %USERPROFILE%\wasi-sdk (eh/ tree).
"@
  exit 1
}

$unixRoot = (wsl -e wslpath -a $Root).Trim()
$args = @("./toolchain/bootstrap.sh", "--jobs", "$Jobs", "--ref", $Ref)
Write-Host "[wasigocvm] wsl → $unixRoot ; $($args -join ' ')"
Write-Host "[wasigocvm] source/build default to ~/wasigocvm-wasi-sdk-* (not /mnt/c)"
if ($DryRun) { exit 0 }

# Long build — stream output. LLVM clone+compile can take hours.
wsl -e bash -lc "cd '$unixRoot' && chmod +x toolchain/bootstrap.sh && $($args -join ' ')"
exit $LASTEXITCODE
