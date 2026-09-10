@echo off
setlocal EnableExtensions EnableDelayedExpansion
cd /d "%~dp0"

rem wasigocvm — Go++ -> C++ -> wasm using OUR toolchain contract.
rem Product define: -DWASIGO_GOCVM=1. Prefers toolchain\ over stock wasi-sdk.
rem
rem   wasigocvm.bat examples\httppkg\main.go
rem   wasigocvm.bat hello.go -o out.wasm

set "CONFIG=Release"
set "GOFILE="
set "OUTWASM="
set "WASIGOC_EXTRA="
set "HOSTBRIDGE="

:parse
if "%~1"=="" goto :parsed
if /I "%~1"=="-h" goto :usage
if /I "%~1"=="--help" goto :usage
if /I "%~1"=="/?" goto :usage
if /I "%~1"=="--host-bridge" (set "HOSTBRIDGE=1" & shift & goto :parse)
if /I "%~1"=="-o" (
  if "%~2"=="" (echo error: -o requires a path & exit /b 2)
  set "OUTWASM=%~2"
  shift & shift & goto :parse
)
if /I "%~1"=="Debug" (set "CONFIG=Debug" & shift & goto :parse)
if /I "%~1"=="Release" (set "CONFIG=Release" & shift & goto :parse)
if /I "%~x1"==".go" (
  if defined GOFILE (echo error: extra .go file & exit /b 2)
  set "GOFILE=%~1" & shift & goto :parse
)
set "WASIGOC_EXTRA=!WASIGOC_EXTRA! %1"
shift
goto :parse

:parsed
call :find_wasigoc
if not defined WASIGOC_EXE (
  echo building wasigoc...
  call compile.bat %CONFIG%
  if errorlevel 1 exit /b 1
  call :find_wasigoc
)
if not defined WASIGOC_EXE (echo error: wasigoc.exe not found & exit /b 1)
if not defined GOFILE (
  echo wasigocvm: full libc++ / eh / rtti / WASIGO_GOCVM ^(docs\wasigocvm.md^)
  echo usage: wasigocvm.bat ^<file.go^> [-o out.wasm]
  exit /b 0
)
if not exist "%GOFILE%" (echo error: not found: %GOFILE% & exit /b 1)

for %%I in ("%GOFILE%") do (
  set "GO_ABS=%%~fI"
  set "GO_DIR=%%~dpI"
  set "GO_BASE=%%~nI"
)
if "%GO_DIR:~-1%"=="\" set "GO_DIR=%GO_DIR:~0,-1%"
set "GENCPP=%GO_DIR%\%GO_BASE%_gen.cpp"
if not defined OUTWASM set "OUTWASM=%GO_DIR%\%GO_BASE%.wasm"

echo [wasigoc] %GO_ABS%
"%WASIGOC_EXE%" "%GO_ABS%" -o "%GENCPP%" --out-dir "%GO_DIR%" %WASIGOC_EXTRA%
if errorlevel 1 exit /b 1

call :find_toolchain
if not defined WASIGO_CLANG (
  echo error: no wasigocvm clang — set WASIGO_TOOLCHAIN or run toolchain\bootstrap.ps1
  echo          ^(fallback: %%USERPROFILE%%\wasi-sdk with eh/^)
  exit /b 1
)

call :pick_triple
if not defined WASIGO_TRIPLE (
  echo error: full libc++ ^(eh^) missing under %WASIGO_SYSROOT%\include
  echo        run toolchain\bootstrap.ps1 or use a wasi-sdk with eh/
  exit /b 1
)

set "EH_INC=%WASIGO_SYSROOT%\include\%WASIGO_TRIPLE%\eh\c++\v1"
set "EH_LIB=%WASIGO_SYSROOT%\lib\%WASIGO_TRIPLE%\eh"

set "HOSTBRIDGE_DEFINE="
if defined HOSTBRIDGE set "HOSTBRIDGE_DEFINE=-DWASIGOCVM_HOST_BRIDGE=1"

echo [wasigocvm] %WASIGO_CLANG%
echo             triple=%WASIGO_TRIPLE%  full libc++ + rtti + standard wasm EH
if defined HOSTBRIDGE echo             --host-bridge: os.exec/os.user/syscall/tls.dial forward to a companion gocvm_host process
"%WASIGO_CLANG%" -O2 -std=c++20 ^
  -fexceptions -frtti ^
  -fwasm-exceptions -mllvm -wasm-use-legacy-eh=false ^
  -nostdinc++ ^
  -isystem "%EH_INC%" ^
  -isystem "%WASIGO_SYSROOT%\include\%WASIGO_TRIPLE%" ^
  -isystem "%WASIGO_SYSROOT%\include" ^
  -L "%EH_LIB%" -lc++ -lc++abi -lunwind ^
  -Wl,--export=__indirect_function_table ^
  -I "%GO_DIR%" -I "%~dp0src" ^
  -DWASIGO_GOCVM=1 %HOSTBRIDGE_DEFINE% ^
  -o "%OUTWASM%" "%GENCPP%"
if errorlevel 1 (
  echo wasigocvm clang++ failed
  exit /b 1
)
echo.
echo wasm: %OUTWASM%
echo run:  ..\shim_sandbox\tools\w2g-run.bat "%OUTWASM%"
exit /b 0

:usage
echo usage: wasigocvm.bat ^<input.go^> [-o out.wasm] [--host-bridge] [wasigoc flags...]
echo.
echo Own target: full libc++, RTTI, WASM EH, Oilpan + type_key, gocvm net.
echo Prefers WASIGO_TOOLCHAIN / toolchain\sysroot; else stock wasi-sdk eh.
echo Threads need a pthread-capable sysroot ^(toolchain\bootstrap^).
echo --host-bridge: forward os.exec/os.user/syscall/tls.dial to a companion
echo                ..\shim_sandbox\gocvm_host.exe over loopback TCP ^(start it
echo                first -- see docs\wasigocvm.md^). Net topics ^(net/tls.dial
echo                excluded^) already run in-guest without this flag.
exit /b 0

:find_wasigoc
set "WASIGOC_EXE="
if exist "build\%CONFIG%\wasigoc.exe" set "WASIGOC_EXE=%cd%\build\%CONFIG%\wasigoc.exe"
if not defined WASIGOC_EXE if exist "build\wasigoc.exe" set "WASIGOC_EXE=%cd%\build\wasigoc.exe"
if not defined WASIGOC_EXE if exist "build\Release\wasigoc.exe" set "WASIGOC_EXE=%cd%\build\Release\wasigoc.exe"
if not defined WASIGOC_EXE if exist "build-fork\wasigoc.exe" set "WASIGOC_EXE=%cd%\build-fork\wasigoc.exe"
if not defined WASIGOC_EXE if exist "wasigoc.exe" set "WASIGOC_EXE=%cd%\wasigoc.exe"
exit /b 0

:find_toolchain
set "WASIGO_CLANG="
set "WASIGO_SYSROOT="
set "WASIGO_SDK_ROOT="
rem 1) Custom wasigocvm sysroot
if defined WASIGO_TOOLCHAIN if exist "%WASIGO_TOOLCHAIN%\sysroot\include" (
  set "WASIGO_SYSROOT=%WASIGO_TOOLCHAIN%\sysroot"
  set "WASIGO_SDK_ROOT=%WASIGO_TOOLCHAIN%"
)
if not defined WASIGO_SYSROOT if exist "%~dp0toolchain\sysroot\include" (
  set "WASIGO_SYSROOT=%~dp0toolchain\sysroot"
  set "WASIGO_SDK_ROOT=%~dp0toolchain"
)
rem share\wasi-sysroot layout from bootstrap install prefix
if not defined WASIGO_SYSROOT if defined WASIGO_TOOLCHAIN if exist "%WASIGO_TOOLCHAIN%\share\wasi-sysroot\include" (
  set "WASIGO_SYSROOT=%WASIGO_TOOLCHAIN%\share\wasi-sysroot"
  set "WASIGO_SDK_ROOT=%WASIGO_TOOLCHAIN%"
)
rem 2) Stock wasi-sdk borrow
if not defined WASIGO_SYSROOT if defined WASI_SDK_PATH if exist "%WASI_SDK_PATH%\share\wasi-sysroot\include" (
  set "WASIGO_SYSROOT=%WASI_SDK_PATH%\share\wasi-sysroot"
  set "WASIGO_SDK_ROOT=%WASI_SDK_PATH%"
)
if not defined WASIGO_SYSROOT if exist "%USERPROFILE%\wasi-sdk\share\wasi-sysroot\include" (
  set "WASIGO_SYSROOT=%USERPROFILE%\wasi-sdk\share\wasi-sysroot"
  set "WASIGO_SDK_ROOT=%USERPROFILE%\wasi-sdk"
)
if not defined WASIGO_SYSROOT exit /b 0
call :pick_clang "%WASIGO_SDK_ROOT%"
rem A bootstrap.sh --target install run (the default) has a real sysroot
rem but no bin\ of its own (see toolchain\README.md) -- pair that sysroot
rem with the stock wasi-sdk's clang++ instead of giving up.
if not defined WASIGO_CLANG if defined WASI_SDK_PATH (
  call :pick_clang "%WASI_SDK_PATH%"
  if defined WASIGO_CLANG echo [wasigocvm] pairing %WASIGO_SYSROOT% with clang++ from %WASI_SDK_PATH% ^(no bin\ of its own^)
)
if not defined WASIGO_CLANG if exist "%USERPROFILE%\wasi-sdk\bin" (
  call :pick_clang "%USERPROFILE%\wasi-sdk"
  if defined WASIGO_CLANG echo [wasigocvm] pairing %WASIGO_SYSROOT% with clang++ from %USERPROFILE%\wasi-sdk ^(no bin\ of its own^)
)
exit /b 0

:pick_clang
set "WASIGO_CLANG="
if exist "%~1\bin\wasm32-wasigocvm-clang++.exe" set "WASIGO_CLANG=%~1\bin\wasm32-wasigocvm-clang++.exe"
if not defined WASIGO_CLANG if exist "%~1\bin\wasm32-wasigocvm-clang++" set "WASIGO_CLANG=%~1\bin\wasm32-wasigocvm-clang++"
if not defined WASIGO_CLANG if exist "%~1\bin\wasm32-wasip2-clang++.exe" set "WASIGO_CLANG=%~1\bin\wasm32-wasip2-clang++.exe"
if not defined WASIGO_CLANG if exist "%~1\bin\wasm32-wasip2-clang++" set "WASIGO_CLANG=%~1\bin\wasm32-wasip2-clang++"
exit /b 0

:pick_triple
set "WASIGO_TRIPLE="
if exist "%WASIGO_SYSROOT%\include\wasm32-wasigocvm\eh\c++\v1\iostream" set "WASIGO_TRIPLE=wasm32-wasigocvm"
if not defined WASIGO_TRIPLE if exist "%WASIGO_SYSROOT%\include\wasm32-wasip2\eh\c++\v1\iostream" set "WASIGO_TRIPLE=wasm32-wasip2"
exit /b 0
