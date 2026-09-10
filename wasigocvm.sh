#!/usr/bin/env bash
# wasigocvm.sh — Unix sibling of wasigocvm.bat
set -euo pipefail
ROOT="$(cd "$(dirname "$0")" && pwd)"
CONFIG="${WASIGO_CONFIG:-Release}"
GOFILE=""
OUTWASM=""
EXTRA=()
HOSTBRIDGE=""

usage() {
  cat <<EOF
usage: wasigocvm.sh <file.go> [-o out.wasm] [--host-bridge]
  full libc++ / EH / RTTI, -DWASIGO_GOCVM=1
  prefers \$WASIGO_TOOLCHAIN or $ROOT/toolchain
  --host-bridge: forward os.exec/os.user/syscall/tls.dial to a companion
                 ../shim_sandbox/gocvm_host process over loopback TCP
                 (start it first -- see docs/wasigocvm.md)
EOF
}

while [[ $# -gt 0 ]]; do
  case "$1" in
    -h|--help) usage; exit 0 ;;
    --host-bridge) HOSTBRIDGE=1; shift ;;
    -o) OUTWASM="$2"; shift 2 ;;
    Debug|Release) CONFIG="$1"; shift ;;
    *.go)
      [[ -n "$GOFILE" ]] && { echo "error: extra .go"; exit 2; }
      GOFILE="$1"; shift
      ;;
    *) EXTRA+=("$1"); shift ;;
  esac
done

find_wasigoc() {
  for c in \
    "$ROOT/build/$CONFIG/wasigoc" \
    "$ROOT/build/wasigoc" \
    "$ROOT/build-fork/wasigoc" \
    "$ROOT/wasigoc"
  do
    [[ -x "$c" ]] && { echo "$c"; return; }
  done
}

WASIGOC="$(find_wasigoc || true)"
if [[ -z "${WASIGOC:-}" ]]; then
  echo "error: wasigoc not found — build it first" >&2
  exit 1
fi
[[ -n "$GOFILE" ]] || { usage; exit 0; }
[[ -f "$GOFILE" ]] || { echo "error: not found: $GOFILE" >&2; exit 1; }

GO_ABS="$(cd "$(dirname "$GOFILE")" && pwd)/$(basename "$GOFILE")"
GO_DIR="$(dirname "$GO_ABS")"
GO_BASE="$(basename "$GO_ABS" .go)"
GENCPP="$GO_DIR/${GO_BASE}_gen.cpp"
OUTWASM="${OUTWASM:-$GO_DIR/${GO_BASE}.wasm}"

echo "[wasigoc] $GO_ABS"
"$WASIGOC" "$GO_ABS" -o "$GENCPP" --out-dir "$GO_DIR" "${EXTRA[@]+"${EXTRA[@]}"}"

# --- toolchain resolve ---
SYSROOT=""
CLANG=""
SDK_ROOT=""
if [[ -n "${WASIGO_TOOLCHAIN:-}" && -d "$WASIGO_TOOLCHAIN/sysroot/include" ]]; then
  SYSROOT="$WASIGO_TOOLCHAIN/sysroot"
  SDK_ROOT="$WASIGO_TOOLCHAIN"
elif [[ -d "$ROOT/toolchain/sysroot/include" ]]; then
  SYSROOT="$ROOT/toolchain/sysroot"
  SDK_ROOT="$ROOT/toolchain"
elif [[ -n "${WASI_SDK_PATH:-}" && -d "$WASI_SDK_PATH/share/wasi-sysroot/include" ]]; then
  SYSROOT="$WASI_SDK_PATH/share/wasi-sysroot"
  SDK_ROOT="$WASI_SDK_PATH"
elif [[ -d "${HOME}/wasi-sdk/share/wasi-sysroot/include" ]]; then
  SYSROOT="$HOME/wasi-sdk/share/wasi-sysroot"
  SDK_ROOT="$HOME/wasi-sdk"
fi

pick_clang() {
  local root="$1"
  for name in wasm32-wasigocvm-clang++ wasm32-wasip2-clang++; do
    [[ -x "$root/bin/$name" ]] && { echo "$root/bin/$name"; return; }
  done
}
[[ -n "$SDK_ROOT" ]] && CLANG="$(pick_clang "$SDK_ROOT" || true)"

# A toolchain/ install from bootstrap.sh's default (--target install) has a
# real sysroot but no bin/ of its own -- see toolchain/README.md. Pair that
# sysroot with the stock wasi-sdk's clang++ instead of giving up.
if [[ -z "${CLANG:-}" && -n "$SYSROOT" ]]; then
  for fallback_root in "${WASI_SDK_PATH:-}" "${HOME}/wasi-sdk"; do
    [[ -n "$fallback_root" ]] || continue
    CLANG="$(pick_clang "$fallback_root" || true)"
    [[ -n "$CLANG" ]] && { echo "[wasigocvm] pairing $SYSROOT with clang++ from $fallback_root (no bin/ of its own)"; break; }
  done
fi

if [[ -z "${CLANG:-}" || -z "${SYSROOT:-}" ]]; then
  echo "error: no wasigocvm clang/sysroot — run toolchain/bootstrap.sh or set WASIGO_TOOLCHAIN" >&2
  exit 1
fi

# Prefer wasigocvm layout, then stock wasip2 eh borrow.
TRIPLE=""
for t in wasm32-wasigocvm wasm32-wasip2; do
  if [[ -f "$SYSROOT/include/$t/eh/c++/v1/iostream" ]]; then
    TRIPLE="$t"
    break
  fi
done
if [[ -z "$TRIPLE" ]]; then
  echo "error: full libc++ (eh) missing under $SYSROOT/include" >&2
  exit 1
fi

EH_INC="$SYSROOT/include/$TRIPLE/eh/c++/v1"
EH_LIB="$SYSROOT/lib/$TRIPLE/eh"

HOSTBRIDGE_DEFINE=()
if [[ -n "$HOSTBRIDGE" ]]; then
  HOSTBRIDGE_DEFINE=(-DWASIGOCVM_HOST_BRIDGE=1)
  echo "[wasigocvm] --host-bridge: os.exec/os.user/syscall/tls.dial forward to a companion gocvm_host process"
fi

echo "[wasigocvm] $CLANG ($TRIPLE)"
"$CLANG" -O2 -std=c++20 \
  -fexceptions -frtti \
  -fwasm-exceptions -mllvm -wasm-use-legacy-eh=false \
  -nostdinc++ \
  -isystem "$EH_INC" \
  -isystem "$SYSROOT/include/$TRIPLE" \
  -isystem "$SYSROOT/include" \
  -L "$EH_LIB" -lc++ -lc++abi -lunwind \
  -Wl,--export=__indirect_function_table \
  -I "$GO_DIR" -I "$ROOT/src" \
  -DWASIGO_GOCVM=1 "${HOSTBRIDGE_DEFINE[@]}" \
  -o "$OUTWASM" "$GENCPP"

echo
echo "wasm: $OUTWASM"
echo "run:  ../shim_sandbox/tools/w2g-run.sh $OUTWASM"
