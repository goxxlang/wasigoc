#!/usr/bin/env bash
# bootstrap.sh — build a wasigocvm sysroot into toolchain/
#
# Usage (from repo root, Linux / macOS / WSL):
#   ./toolchain/bootstrap.sh
#   ./toolchain/bootstrap.sh --jobs 8
#   WASI_SDK_REF=wasi-sdk-33 ./toolchain/bootstrap.sh
#
# Installs under:
#   toolchain/          (CMAKE_INSTALL_PREFIX)
#   toolchain/sysroot/  (wasi-sysroot with eh libc++)
#   toolchain/bin/      (clang wrappers)
#
# This is a long build (LLVM + wasi-libc + libc++). Prefer CI or overnight WSL.
set -euo pipefail

ROOT="$(cd "$(dirname "$0")/.." && pwd)"
TC="$ROOT/toolchain"
# Default source/build trees live under $HOME when the repo is on a slow
# mount (WSL /mnt/c). Install prefix stays in-repo so Windows drivers see it.
if [[ -z "${WASIGO_WASI_SDK_SRC:-}" ]]; then
  case "$ROOT" in
    /mnt/*) SRC="$HOME/wasigocvm-wasi-sdk-src" ;;
    *)      SRC="$TC/wasi-sdk-src" ;;
  esac
else
  SRC="$WASIGO_WASI_SDK_SRC"
fi
if [[ -z "${WASIGO_WASI_SDK_BUILD:-}" ]]; then
  case "$ROOT" in
    /mnt/*) BUILD="$HOME/wasigocvm-wasi-sdk-build" ;;
    *)      BUILD="$TC/wasi-sdk-build" ;;
  esac
else
  BUILD="$WASIGO_WASI_SDK_BUILD"
fi
PREFIX="${WASIGO_TOOLCHAIN:-$TC}"
REF="${WASI_SDK_REF:-main}"
JOBS="${WASIGO_BOOTSTRAP_JOBS:-$(nproc 2>/dev/null || sysctl -n hw.ncpu 2>/dev/null || echo 4)}"

# `install`: sysroot + compiler-rt only, using the host clang found below --
# what this script builds by default (~1-2h). No bin/ wrapper scripts come
# out of this; pair the sysroot with an existing wasm32-wasip2-capable
# clang++ (see toolchain/README.md).
# `dist`: wasi-sdk's full standalone SDK, including bin/ wrapper scripts --
# NOT verified by this project yet, may build LLVM/clang from scratch on
# top of the sysroot (much longer). Pass --dist to try it.
BUILD_TARGET=install

while [[ $# -gt 0 ]]; do
  case "$1" in
    --jobs|-j) JOBS="$2"; shift 2 ;;
    --ref) REF="$2"; shift 2 ;;
    --prefix) PREFIX="$2"; shift 2 ;;
    --dist) BUILD_TARGET=dist; shift ;;
    -h|--help)
      sed -n '2,16p' "$0"
      exit 0
      ;;
    *) echo "unknown arg: $1" >&2; exit 2 ;;
  esac
done

mkdir -p "$TC"
echo "[wasigocvm] bootstrap → prefix=$PREFIX jobs=$JOBS ref=$REF"

if [[ ! -d "$SRC/.git" ]]; then
  echo "[wasigocvm] cloning WebAssembly/wasi-sdk ($REF)…"
  git clone --recursive --branch "$REF" \
    https://github.com/WebAssembly/wasi-sdk.git "$SRC" \
    || git clone --recursive https://github.com/WebAssembly/wasi-sdk.git "$SRC"
fi

# Keep submodules current if we already had a checkout.
git -C "$SRC" submodule update --init --recursive

# wasi-sdk's own CMake refuses a non-Clang host compiler (it builds/uses
# clang-based tools like llvm-nm alongside the cross sysroot). Find one
# rather than relying on cc/c++ happening to already be clang.
# Prefer the newest versioned clang (exceptions need >=22, coop-threads
# need >=23 per wasi-sdk-sysroot.cmake) over a possibly-older default
# `clang` symlink.
if [[ -z "${CC:-}" ]]; then
  for c in clang-24 clang-23 clang-22 clang-21 clang-20 clang-19 clang-18 clang; do
    command -v "$c" >/dev/null 2>&1 && { CC="$c"; break; }
  done
fi
if [[ -z "${CXX:-}" ]]; then
  for c in clang++-24 clang++-23 clang++-22 clang++-21 clang++-20 clang++-19 clang++-18 clang++; do
    command -v "$c" >/dev/null 2>&1 && { CXX="$c"; break; }
  done
fi
if [[ -z "${CC:-}" || -z "${CXX:-}" ]]; then
  echo "[wasigocvm] error: need a clang/clang++ host compiler (apt install clang lld)" >&2
  exit 1
fi
export CC CXX

# clang's wasm32-wasip2/wasip3 driver unconditionally shells out to
# `wasm-component-ld` (bytecodealliance, a separate Rust tool) for any
# `-shared` link -- wasi-libc's own shared-object build needs this even
# though wasi-libc's USE_WASM_COMPONENT_LD option is unrelated/OFF. Without
# it every wasip2/wasip3 shared-lib link fails deep into the build with an
# opaque "posix_spawn failed: No such file or directory". wasi-sdk's own CI
# gets it via `cargo install`; we fetch the prebuilt release instead so a
# Rust toolchain isn't a bootstrap prerequisite too.
WCL_VERSION=0.5.30
if ! command -v wasm-component-ld >/dev/null 2>&1 || \
   [[ "$(wasm-component-ld --version 2>/dev/null)" != *"$WCL_VERSION"* ]]; then
  echo "[wasigocvm] fetching wasm-component-ld $WCL_VERSION…"
  WCL_OS="linux"; [[ "$(uname -s)" == Darwin ]] && WCL_OS="macos"
  WCL_ARCH="x86_64"; [[ "$(uname -m)" == arm64 || "$(uname -m)" == aarch64 ]] && WCL_ARCH="aarch64"
  WCL_TMP="$(mktemp -d)"
  curl -sL -o "$WCL_TMP/wcl.tar.gz" \
    "https://github.com/bytecodealliance/wasm-component-ld/releases/download/v${WCL_VERSION}/wasm-component-ld-v${WCL_VERSION}-${WCL_ARCH}-${WCL_OS}.tar.gz"
  tar -xzf "$WCL_TMP/wcl.tar.gz" -C "$WCL_TMP"
  install -m 755 "$WCL_TMP"/wasm-component-ld-*/wasm-component-ld /usr/local/bin/wasm-component-ld
  rm -rf "$WCL_TMP"
  hash -r
fi

# DUAL (not ON): builds both eh/ and noeh/ variants side by side under each
# triple, which is the layout run_golden.cmake / wasigocvm.bat / .sh already
# probe for (${SYSROOT}/include/${triple}/eh/c++/v1). ON alone builds a
# single flat exceptions-only tree with no eh/ subdirectory, which those
# probes silently miss. DUAL needs the same >=22 host clang as ON -- the
# separate, stricter >=23 gate is only for COOP_THREADS_POSSIBLE (threads),
# not this.
echo "[wasigocvm] configuring (WASI_SDK_EXCEPTIONS=DUAL, host compiler $CC)…"
cmake -S "$SRC" -B "$BUILD" \
  -DCMAKE_INSTALL_PREFIX="$PREFIX" \
  -DCMAKE_C_COMPILER="$CC" \
  -DCMAKE_CXX_COMPILER="$CXX" \
  -DWASI_SDK_EXCEPTIONS=DUAL \
  ${CMAKE_GENERATOR:+-G "$CMAKE_GENERATOR"}

echo "[wasigocvm] building + installing (this takes a long time)…"
cmake --build "$BUILD" --target "$BUILD_TARGET" -j "$JOBS"

# wasi-sdk installs sysroot at share/wasi-sysroot; wasigocvm.bat prefers
# toolchain/sysroot. A symlink pointing at a /mnt/* (WSL-visible) path is
# invisible to native Windows tools -- PowerShell/CMake see it as a dead,
# 0-byte reparse point, not a directory, even though WSL/git-bash dereference
# it fine. wasigocvm.bat and CMakeLists.txt are exactly the consumers on the
# other side of that boundary, so always copy when the prefix is under /mnt.
if [[ -d "$PREFIX/share/wasi-sysroot" && ! -e "$PREFIX/sysroot" ]]; then
  case "$PREFIX" in
    /mnt/*) mkdir -p "$PREFIX/sysroot" && cp -a "$PREFIX/share/wasi-sysroot/." "$PREFIX/sysroot/" ;;
    *) ln -sfn "$PREFIX/share/wasi-sysroot" "$PREFIX/sysroot" \
         || { mkdir -p "$PREFIX/sysroot" && cp -a "$PREFIX/share/wasi-sysroot/." "$PREFIX/sysroot/"; } ;;
  esac
fi

# Stamp identity so drivers know this is a wasigocvm install.
mkdir -p "$PREFIX"
cat > "$PREFIX/wasigocvm-toolchain.json" <<EOF
{
  "name": "wasigocvm",
  "exceptions": true,
  "threads": false,
  "borrowed_triple": "wasm32-wasip2",
  "product_define": "WASIGO_GOCVM",
  "built_at": "$(date -u +%Y-%m-%dT%H:%M:%SZ)",
  "wasi_sdk_ref": "$REF"
}
EOF

# Convenience wrapper name (forwards to wasip2 clang until we own a triple).
BIN="$PREFIX/bin"
if [[ -x "$BIN/wasm32-wasip2-clang++" ]]; then
  cat > "$BIN/wasm32-wasigocvm-clang++" <<'WRAP'
#!/usr/bin/env bash
# Temporary: wasigocvm identity wrapper → stock wasip2 clang++ from this prefix.
DIR="$(cd "$(dirname "$0")" && pwd)"
exec "$DIR/wasm32-wasip2-clang++" "$@"
WRAP
  chmod +x "$BIN/wasm32-wasigocvm-clang++"
fi

echo
echo "[wasigocvm] done."
echo "  export WASIGO_TOOLCHAIN=$PREFIX"
echo "  wasigocvm.bat / wasigocvm.sh will prefer this sysroot."
echo "  threads: still false — needs a >=23 host clang for COOP_THREADS_POSSIBLE, then re-bootstrap."
if [[ ! -x "$BIN/wasm32-wasip2-clang++" ]]; then
  echo "  no bin/wasm32-*-clang++ in this install (BUILD_TARGET=$BUILD_TARGET) --"
  echo "  pair this sysroot with an existing wasm32-wasip2-capable clang++ yourself,"
  echo "  or re-run with --dist to try building wasi-sdk's own wrapper scripts."
fi
