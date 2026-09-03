@echo off
setlocal EnableExtensions EnableDelayedExpansion
cd /d "%~dp0"

rem goclang++.bat — compile a Go++ program to a NATIVE host executable
rem with clang++, instead of wasm32-wasip1. Same wasigoc frontend as
rem compile.bat, different backend compiler and no WASI restrictions
rem (real exceptions, real threads, real sockets are all available here
rem -- this is plain host clang++, not the wasi-sdk wasm-only build).
rem That's what unlocks linking ~/shim_sandbox directly: shim_sandbox's
rem own Shim/ABAC/s2 host layer needs real OS access wasm32-wasip1
rem can't grant; net.Pipe()-based code doesn't care which backend it's
rem compiled with.
rem
rem Requires wasigoc.exe already built (see compile.bat).
rem
rem shim_sandbox + ABAC are ON BY DEFAULT: gocvm.Call's ErrorState
rem machinery (runtime.hpp) surfaces every bridge failure -- including
rem an internal bridge panic or an ABAC deny -- as a real Go error
rem instead of ever aborting the process (see docs/design-log.md's
rem GocVM diary), so there is no longer a safety reason to keep it
rem opt-in. If ../shim_sandbox isn't found or isn't built, this quietly
rem falls back to a bridge-less build (gocvm.Call reports "no host
rem bridge registered", same as always) rather than failing -- pass
rem --shim-sandbox explicitly to make a missing/unbuilt shim_sandbox a
rem hard error instead.
rem
rem   goclang++.bat hello.go                    -> hello.exe, gocvm+ABAC on
rem   goclang++.bat hello.go -o out.exe
rem   goclang++.bat foo.go --no-shim-sandbox     no gocvm.Call bridge at all
rem   goclang++.bat foo.go --no-abac             gocvm.Call bridge, no -DW2G_ABAC_SYSTEM=1
rem   goclang++.bat foo.go --shim-sandbox        same as default, but a missing
rem                                               shim_sandbox is now a hard error
rem
rem clang++ ABI note: this targets x86_64-w64-mingw32 (the MinGW ABI),
rem matching the mingw g++ that builds wasigoc.exe and shim_sandbox's
rem libw2g.a in this workspace -- so the same libw2g.a links straight
rem in, no shim_sandbox rebuild needed. Override with GOCLANG_TARGET if
rem your toolchain differs (e.g. an MSVC-ABI clang-cl setup, which would
rem also need shim_sandbox rebuilt with that same ABI).

set "GOFILE="
set "OUTEXE="
set "USE_SHIM=1"
set "USE_ABAC=1"
set "EXPLICIT_SHIM="
set "CLANG_EXTRA="

:parse
if "%~1"=="" goto :parsed
if /I "%~1"=="-h" goto :usage
if /I "%~1"=="--help" goto :usage
if /I "%~1"=="/?" goto :usage
if /I "%~1"=="-o" (
  if "%~2"=="" (
    echo error: -o requires a path
    exit /b 2
  )
  set "OUTEXE=%~2"
  shift
  shift
  goto :parse
)
if /I "%~1"=="--shim-sandbox" (
  set "USE_SHIM=1"
  set "EXPLICIT_SHIM=1"
  shift
  goto :parse
)
if /I "%~1"=="--no-shim-sandbox" (
  set "USE_SHIM="
  set "USE_ABAC="
  shift
  goto :parse
)
if /I "%~1"=="--abac" (
  set "USE_ABAC=1"
  shift
  goto :parse
)
if /I "%~1"=="--no-abac" (
  set "USE_ABAC="
  shift
  goto :parse
)
if /I "%~x1"==".go" (
  if defined GOFILE (
    echo error: extra .go file "%~1" — pass one entry file
    exit /b 2
  )
  set "GOFILE=%~1"
  shift
  goto :parse
)
set "CLANG_EXTRA=!CLANG_EXTRA! %1"
shift
goto :parse

:parsed
if not defined GOFILE goto :usage

call :find_wasigoc
if not defined WASIGOC_EXE (
  echo error: wasigoc.exe not found — run compile.bat first to build it
  exit /b 1
)

call :find_clang
if not defined CLANGXX (
  echo error: no native clang++ found
  echo looked at CLANGXX%%, %%LLVM_DIR%%\bin, "C:\Program Files\LLVM\bin", and PATH
  exit /b 1
)

if not exist "%GOFILE%" (
  echo error: not found: %GOFILE%
  exit /b 1
)

for %%I in ("%GOFILE%") do (
  set "GO_ABS=%%~fI"
  set "GO_DIR=%%~dpI"
  set "GO_BASE=%%~nI"
)
if "%GO_DIR:~-1%"=="\" set "GO_DIR=%GO_DIR:~0,-1%"

set "GENCPP=%GO_DIR%\%GO_BASE%_gen.cpp"
if not defined OUTEXE set "OUTEXE=%GO_DIR%\%GO_BASE%.exe"

echo.
echo [wasigoc] %GO_ABS%
echo          -o %GENCPP%
"%WASIGOC_EXE%" "%GO_ABS%" -o "%GENCPP%" --out-dir "%GO_DIR%"
if errorlevel 1 (
  echo wasigoc failed
  exit /b 1
)

set "TARGET_TRIPLE=%GOCLANG_TARGET%"
if not defined TARGET_TRIPLE set "TARGET_TRIPLE=x86_64-w64-mingw32"

set "SHIM_FLAGS="
if defined USE_SHIM (
  set "SHIM_DIR=%SHIM_SANDBOX_DIR%"
  if not defined SHIM_DIR if exist "..\shim_sandbox\include\w2g\pipe.h" set "SHIM_DIR=..\shim_sandbox"
  if not defined SHIM_DIR (
    if defined EXPLICIT_SHIM (
      echo error: --shim-sandbox given but no shim_sandbox found
      echo set SHIM_SANDBOX_DIR, or check out shim_sandbox next to go++ ^(..\shim_sandbox^)
      exit /b 1
    )
    echo [shim_sandbox] not found next to go++ -- building without gocvm ^(gocvm.Call will report "no host bridge registered"^). Pass --shim-sandbox to make a missing shim_sandbox a hard error, or set SHIM_SANDBOX_DIR.
    set "USE_SHIM="
  )
)
if defined USE_SHIM if not exist "!SHIM_DIR!\build\libw2g.a" (
  if defined EXPLICIT_SHIM (
    echo error: !SHIM_DIR!\build\libw2g.a not found — build shim_sandbox first
    echo   cd !SHIM_DIR! ^&^& cmake -B build ^&^& cmake --build build
    exit /b 1
  )
  echo [shim_sandbox] !SHIM_DIR! found but not built -- building without gocvm. Run: cd !SHIM_DIR! ^&^& cmake -B build ^&^& cmake --build build
  set "USE_SHIM="
)
if defined USE_SHIM (
  rem -lws2_32 -liphlpapi: shim_sandbox's stub topics (net.dial/listen/
  rem tcp.bind/udp.bind/tls.dial) do real Winsock work (src/sapi/
  rem real_win.cc) instead of always answering "not supported" -- link
  rem them so hand-written client code using w2g::WasiNet still links.
  rem
  rem -DWASIGO_GOCVM_BRIDGE=1: makes generated code's gocvm.Call(...)
  rem (wasigo::gocvm in runtime.hpp -- the one dispatch gate real Go++
  rem stdlib source calls, e.g. os/exec.Command(...).Output()) reach
  rem shim_sandbox's real backends instead of always answering "no host
  rem bridge registered". Wired at startup by wasigo::set_os_args calling
  rem shim_sandbox's wasigo_gocvm_install_bridge() (src/gocvm_bridge.cc).
  rem -lsecur32 -lcrypt32: real TLS (Schannel/SSPI, src/sapi/tls_win.cc).
  rem -lnetapi32: os/user Lookup/LookupId (NetUserGetInfo, src/sapi/
  rem real_win.cc).
  set "SHIM_FLAGS=-I "!SHIM_DIR!\include" "!SHIM_DIR!\build\libw2g.a" -lpthread -lws2_32 -liphlpapi -lsecur32 -lcrypt32 -lnetapi32 -DWASIGO_GOCVM_BRIDGE=1"
  if defined USE_ABAC set "SHIM_FLAGS=!SHIM_FLAGS! -DW2G_ABAC_SYSTEM=1"
  echo [shim_sandbox] !SHIM_DIR!
)

echo.
echo [clang++] %CLANGXX% --target=%TARGET_TRIPLE%
echo           -o %OUTEXE%
"%CLANGXX%" --target=%TARGET_TRIPLE% -std=c++20 -O2 -I "%GO_DIR%" -I src ^
  "%GENCPP%" !SHIM_FLAGS! %CLANG_EXTRA% -o "%OUTEXE%"
if errorlevel 1 (
  echo clang++ failed
  exit /b 1
)
echo.
echo native exe: %OUTEXE%
exit /b 0

:usage
echo usage: goclang++.bat ^<input.go^> [-o out.exe] [--no-shim-sandbox] [--no-abac] [--shim-sandbox] [clang++ flags...]
echo.
echo   Compiles a Go++ program to a native host .exe via clang++ instead
echo   of wasm32-wasip1 -- unrestricted C++ (exceptions, threads, real
echo   sockets), so ~/shim_sandbox links straight in.
echo   Needs wasigoc.exe already built (compile.bat).
echo.
echo   shim_sandbox + ABAC are linked by DEFAULT (a missing/unbuilt
echo   shim_sandbox silently falls back to no bridge instead of failing).
echo   --shim-sandbox: same default, but a missing/unbuilt shim_sandbox
echo                   is now a hard error instead of a silent fallback.
echo   --no-shim-sandbox: don't link gocvm's bridge at all.
echo   --no-abac: link the bridge but skip -DW2G_ABAC_SYSTEM=1.
exit /b 0

:find_wasigoc
set "WASIGOC_EXE="
if exist "build\wasigoc.exe" set "WASIGOC_EXE=%cd%\build\wasigoc.exe"
if not defined WASIGOC_EXE if exist "build\Release\wasigoc.exe" set "WASIGOC_EXE=%cd%\build\Release\wasigoc.exe"
if not defined WASIGOC_EXE if exist "build\Debug\wasigoc.exe" set "WASIGOC_EXE=%cd%\build\Debug\wasigoc.exe"
if not defined WASIGOC_EXE if exist "build-fork\wasigoc.exe" set "WASIGOC_EXE=%cd%\build-fork\wasigoc.exe"
if not defined WASIGOC_EXE if exist "wasigoc.exe" set "WASIGOC_EXE=%cd%\wasigoc.exe"
exit /b 0

:find_clang
set "CLANGXX="
if defined CLANGXX_PATH if exist "%CLANGXX_PATH%" set "CLANGXX=%CLANGXX_PATH%"
if not defined CLANGXX if exist "C:\Program Files\LLVM\bin\clang++.exe" set "CLANGXX=C:\Program Files\LLVM\bin\clang++.exe"
if not defined CLANGXX if defined LLVM_DIR if exist "%LLVM_DIR%\bin\clang++.exe" set "CLANGXX=%LLVM_DIR%\bin\clang++.exe"
if not defined CLANGXX (
  where clang++ >nul 2>&1
  if not errorlevel 1 for /f "delims=" %%C in ('where clang++') do if not defined CLANGXX set "CLANGXX=%%C"
)
exit /b 0
