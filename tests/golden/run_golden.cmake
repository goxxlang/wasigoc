# Compiles one wasigoc-generated .cpp with wasi-sdk's wasm32-wasip1 clang++
# and validates the result, all at `ctest` time rather than at `cmake --build`
# time -- so a broken or missing wasi-sdk install fails only this one test,
# not the whole build (wasigoc itself, and transpiling every example to C++,
# never depends on wasi-sdk at all).
#
# Required: CLANGXX, CPP_FILE, WASM_FILE
# Optional: SYSROOT (derived from CLANGXX if omitted), WASMTIME, EXPECTED_OUTPUT

if(NOT DEFINED CLANGXX OR NOT DEFINED CPP_FILE OR NOT DEFINED WASM_FILE)
  message(FATAL_ERROR "run_golden.cmake: CLANGXX, CPP_FILE, and WASM_FILE are required")
endif()

if(NOT DEFINED SYSROOT OR SYSROOT STREQUAL "")
  get_filename_component(_clang_bin "${CLANGXX}" DIRECTORY)
  set(SYSROOT "${_clang_bin}/../share/wasi-sysroot")
endif()

get_filename_component(_clang_name "${CLANGXX}" NAME)
set(_target_flag)
if(NOT _clang_name MATCHES "wasm32-")
  set(_target_flag --target=wasm32-wasip1)
endif()

# wasi-sdk 34 (LLVM 23) puts both noeh/c++/v1 and c++/v1 on the default
# include path. libc++'s own ctype.h/errno.h then win over wasi-libc's,
# and iostream fails to compile. -nostdinc++ + the noeh tree + wasi-libc
# is the include order that actually works. -fno-exceptions matches the
# noeh headers and our panic mapping (goto/abort, not __cxa_throw).
set(_include_flag)
if(DEFINED INCLUDE_DIR AND NOT INCLUDE_DIR STREQUAL "")
  set(_include_flag -I "${INCLUDE_DIR}")
endif()

execute_process(
  COMMAND "${CLANGXX}" ${_target_flag}
          -O2 -std=c++20 -fno-exceptions
          -nostdinc++
          -isystem "${SYSROOT}/include/wasm32-wasip1/noeh/c++/v1"
          -isystem "${SYSROOT}/include/wasm32-wasip1"
          -isystem "${SYSROOT}/include"
          ${_include_flag}
          -o "${WASM_FILE}" "${CPP_FILE}"
  OUTPUT_VARIABLE compile_output
  ERROR_VARIABLE compile_error
  RESULT_VARIABLE compile_result
)
if(NOT compile_result EQUAL 0)
  message(FATAL_ERROR
    "wasi-sdk clang++ failed to compile ${CPP_FILE}:\n${compile_output}\n${compile_error}")
endif()

include("${CMAKE_CURRENT_LIST_DIR}/check_wasm.cmake")
