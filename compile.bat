@echo off
setlocal EnableExtensions EnableDelayedExpansion
cd /d "%~dp0"

rem compile — build wasigoc (host) and optionally compile a Go++ program
rem to wasm32-wasip1. Matches CMakeLists.txt and docs/build.md.
rem
rem   compile                              build wasigoc (Release)
rem   compile Debug                        build wasigoc (Debug)
rem   compile examples\hello\hello.go compile Go++ -> .wasm
rem   compile hello.go -o out.wasm        same, explicit wasm path

set "CONFIG=Release"
set "GOFILE="
set "OUTWASM="
set "WASIGOC_EXTRA="

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
  set "OUTWASM=%~2"
  shift
  shift
  goto :parse
)
if /I "%~1"=="Debug" (
  set "CONFIG=Debug"
  shift
  goto :parse
)
if /I "%~1"=="Release" (
  set "CONFIG=Release"
  shift
  goto :parse
)
if /I "%~1"=="MinSizeRel" (
  set "CONFIG=MinSizeRel"
  shift
  goto :parse
)
if /I "%~1"=="RelWithDebInfo" (
  set "CONFIG=RelWithDebInfo"
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
set "WASIGOC_EXTRA=!WASIGOC_EXTRA! %1"
shift
goto :parse

:parsed
call :build_wasigoc
if errorlevel 1 exit /b 1
if not defined GOFILE (
  echo.
  echo wasigoc: !WASIGOC_EXE!
  echo usage: compile ^<file.go^> [-o out.wasm]
  exit /b 0
)
call :compile_go
exit /b %ERRORLEVEL%

:usage
echo usage: compile [Debug^|Release] [^<input.go^> [-o out.wasm] [wasigoc flags...]]
echo.
echo   no .go file   build the host compiler wasigoc.exe
echo   input.go      transpile with wasigoc, then wasm32-wasip1-clang++
echo.
echo wasi-sdk is found at WASI_SDK_PATH, %%USERPROFILE%%\wasi-sdk, or
echo the same search CMakeLists.txt uses. Do not use a bare
echo clang++ --target=wasm32-wasip1; this script uses the noeh include
echo order from docs/build.md.
exit /b 0

:build_wasigoc
set "WASIGOC_EXE="
call :find_wasigoc
if defined WASIGOC_EXE if not defined GOFILE goto :do_build
if defined WASIGOC_EXE if defined GOFILE (
  echo using existing wasigoc: !WASIGOC_EXE!
  exit /b 0
)

:do_build
where cmake >nul 2>&1
if errorlevel 1 goto :build_direct

echo [1/2] cmake configure
if not exist "build\CMakeCache.txt" (
  cmake -S . -B build
  if errorlevel 1 (
    echo cmake configure failed
    exit /b 1
  )
)

echo [2/2] cmake --build --target wasigoc --config %CONFIG%
cmake --build build --config %CONFIG% --target wasigoc
if errorlevel 1 (
  echo cmake build failed
  exit /b 1
)
call :find_wasigoc
if not defined WASIGOC_EXE (
  echo error: wasigoc.exe not found after build
  exit /b 1
)
echo built !WASIGOC_EXE!
exit /b 0

:build_direct
echo cmake not on PATH — compiling wasigoc directly
set "CXX="
where g++ >nul 2>&1
if not errorlevel 1 set "CXX=g++"
if not defined CXX (
  where clang++ >nul 2>&1
  if not errorlevel 1 set "CXX=clang++"
)
if not defined CXX (
  echo error: need cmake, g++, or clang++ to build wasigoc
  exit /b 1
)

set "ROOT=%cd%"
set "ROOT_FWD=%ROOT:\=/%"
set "WASIGOC_EXE=%ROOT%\wasigoc.exe"
"%CXX%" -std=c++20 -O2 -Wall -Wextra -Wpedantic -I src ^
  -DWASIGO_RUNTIME_PATH=\"%ROOT_FWD%/src/runtime.hpp\" ^
  -DWASIGO_STDLIB_PATH=\"%ROOT_FWD%/stdlib\" ^
  src/lexer.cc src/parser.cc src/cpp_generator.cc src/module_loader.cc src/main.cc ^
  -o "%WASIGOC_EXE%"
if errorlevel 1 (
  echo direct compile failed
  exit /b 1
)
echo built %WASIGOC_EXE%
exit /b 0

:find_wasigoc
set "WASIGOC_EXE="
if exist "build\%CONFIG%\wasigoc.exe" set "WASIGOC_EXE=%cd%\build\%CONFIG%\wasigoc.exe"
if not defined WASIGOC_EXE if exist "build\wasigoc.exe" set "WASIGOC_EXE=%cd%\build\wasigoc.exe"
if not defined WASIGOC_EXE if exist "build\Debug\wasigoc.exe" set "WASIGOC_EXE=%cd%\build\Debug\wasigoc.exe"
if not defined WASIGOC_EXE if exist "build\Release\wasigoc.exe" set "WASIGOC_EXE=%cd%\build\Release\wasigoc.exe"
if not defined WASIGOC_EXE if exist "wasigoc.exe" set "WASIGOC_EXE=%cd%\wasigoc.exe"
exit /b 0

:compile_go
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
if not defined OUTWASM set "OUTWASM=%GO_DIR%\%GO_BASE%.wasm"

echo.
echo [wasigoc] %GO_ABS%
echo          -o %GENCPP%
"%WASIGOC_EXE%" "%GO_ABS%" -o "%GENCPP%" --out-dir "%GO_DIR%" %WASIGOC_EXTRA%
if errorlevel 1 (
  echo wasigoc failed
  exit /b 1
)

call :find_wasi_sdk
if not defined WASI_CLANG (
  echo.
  echo wasigoc wrote %GENCPP%
  echo no wasi-sdk found — set WASI_SDK_PATH or install to %%USERPROFILE%%\wasi-sdk
  echo skipping wasm compile. Flags: docs\build.md
  exit /b 0
)

set "SYS=%WASI_ROOT%\share\wasi-sysroot"
echo.
echo [wasi-sdk] %WASI_CLANG%
echo            -o %OUTWASM%
"%WASI_CLANG%" -O2 -std=c++20 -fno-exceptions ^
  -nostdinc++ ^
  -isystem "%SYS%\include\wasm32-wasip1\noeh\c++\v1" ^
  -isystem "%SYS%\include\wasm32-wasip1" ^
  -isystem "%SYS%\include" ^
  -I "%GO_DIR%" ^
  -o "%OUTWASM%" "%GENCPP%"
if errorlevel 1 (
  echo wasm32-wasip1-clang++ failed
  exit /b 1
)
echo.
echo wasm: %OUTWASM%
exit /b 0

:find_wasi_sdk
set "WASI_ROOT="
set "WASI_CLANG="
if defined WASI_SDK_PATH if exist "%WASI_SDK_PATH%\bin" set "WASI_ROOT=%WASI_SDK_PATH%"
if not defined WASI_ROOT if exist "%USERPROFILE%\wasi-sdk\bin" set "WASI_ROOT=%USERPROFILE%\wasi-sdk"
if not defined WASI_ROOT if exist "%HOME%\wasi-sdk\bin" set "WASI_ROOT=%HOME%\wasi-sdk"
if not defined WASI_ROOT exit /b 0
if exist "%WASI_ROOT%\bin\wasm32-wasip1-clang++.exe" set "WASI_CLANG=%WASI_ROOT%\bin\wasm32-wasip1-clang++.exe"
if not defined WASI_CLANG if exist "%WASI_ROOT%\bin\clang++.exe" set "WASI_CLANG=%WASI_ROOT%\bin\clang++.exe"
exit /b 0
