# Compiles one wasigoc-generated .cpp and validates at ctest time.
#
# Required: CLANGXX, CPP_FILE, WASM_FILE
# Optional: SYSROOT, WASMTIME, EXPECTED_OUTPUT, WASI_PREVIEW (p1|p2),
#           SRC_DIR, WASMTIME_WASI, WASIGO_CXXLIB (noeh|full),
#           EXTRA_DEFINES (semicolon list, each becomes -D<item>),
#           COMPILE_ONLY (skip the run/check step -- for callers, like
#           run_hostbridge_golden.ps1, that need to start a companion
#           process between compiling and running the wasm)
#
# WASIGO_CXXLIB=full is the wasigocvm contract: eh libc++, RTTI, standard
# WASM exceptions (not legacy). Default remains noeh for stock p1 goldens.

if(NOT DEFINED CLANGXX OR NOT DEFINED CPP_FILE OR NOT DEFINED WASM_FILE)
  message(FATAL_ERROR "run_golden.cmake: CLANGXX, CPP_FILE, and WASM_FILE are required")
endif()

if(NOT DEFINED SYSROOT OR SYSROOT STREQUAL "")
  get_filename_component(_clang_bin "${CLANGXX}" DIRECTORY)
  set(SYSROOT "${_clang_bin}/../share/wasi-sysroot")
endif()

if(NOT DEFINED WASI_PREVIEW OR WASI_PREVIEW STREQUAL "")
  set(WASI_PREVIEW p1)
endif()
if(WASI_PREVIEW STREQUAL "p2")
  # Prefer an explicit wasigocvm triple layout if the sysroot has one.
  if(EXISTS "${SYSROOT}/include/wasm32-wasigocvm/eh/c++/v1/iostream")
    set(_triple wasm32-wasigocvm)
  else()
    set(_triple wasm32-wasip2)
  endif()
else()
  set(_triple wasm32-wasip1)
endif()

if(NOT DEFINED WASIGO_CXXLIB OR WASIGO_CXXLIB STREQUAL "")
  if(WASI_PREVIEW STREQUAL "p2")
    set(WASIGO_CXXLIB full)
  else()
    set(WASIGO_CXXLIB noeh)
  endif()
endif()

get_filename_component(_clang_name "${CLANGXX}" NAME)
set(_target_flag)
if(NOT _clang_name MATCHES "wasm32-")
  set(_target_flag --target=${_triple})
endif()

set(_include_flag)
if(DEFINED INCLUDE_DIR AND NOT INCLUDE_DIR STREQUAL "")
  set(_include_flag -I "${INCLUDE_DIR}")
endif()
if(DEFINED SRC_DIR AND NOT SRC_DIR STREQUAL "")
  list(APPEND _include_flag -I "${SRC_DIR}")
endif()

set(_cxx_flags -O2 -std=c++20)
set(_link_flags)
set(_isystem_flags)
if(WASIGO_CXXLIB STREQUAL "full")
  # wasigocvm: full libc++, RTTI, standard (non-legacy) WASM EH
  list(APPEND _cxx_flags -fexceptions -frtti -fwasm-exceptions
       -mllvm -wasm-use-legacy-eh=false -DWASIGO_GOCVM=1)
  set(_isystem_flags
    -nostdinc++
    -isystem "${SYSROOT}/include/${_triple}/eh/c++/v1"
    -isystem "${SYSROOT}/include/${_triple}"
    -isystem "${SYSROOT}/include"
  )
  list(APPEND _link_flags
    -L "${SYSROOT}/lib/${_triple}/eh"
    -lc++ -lc++abi -lunwind
    -Wl,--export=__indirect_function_table
  )
  # Host must grant -W exceptions (see check_wasm.cmake WASMTIME_WASM)
  if(NOT DEFINED WASMTIME_WASM OR WASMTIME_WASM STREQUAL "")
    set(WASMTIME_WASM "exceptions")
  endif()
else()
  list(APPEND _cxx_flags -fno-exceptions)
  set(_isystem_flags
    -nostdinc++
    -isystem "${SYSROOT}/include/${_triple}/noeh/c++/v1"
    -isystem "${SYSROOT}/include/${_triple}"
    -isystem "${SYSROOT}/include"
  )
endif()

if(DEFINED EXTRA_DEFINES AND NOT EXTRA_DEFINES STREQUAL "")
  foreach(_d IN LISTS EXTRA_DEFINES)
    if(NOT _d STREQUAL "")
      list(APPEND _cxx_flags "-D${_d}")
    endif()
  endforeach()
endif()

execute_process(
  COMMAND "${CLANGXX}" ${_target_flag}
          ${_cxx_flags}
          ${_isystem_flags}
          ${_include_flag}
          ${_link_flags}
          -o "${WASM_FILE}" "${CPP_FILE}"
  OUTPUT_VARIABLE compile_output
  ERROR_VARIABLE compile_error
  RESULT_VARIABLE compile_result
)
if(NOT compile_result EQUAL 0)
  message(FATAL_ERROR
    "wasi-sdk clang++ failed (${_triple}, cxxlib=${WASIGO_CXXLIB}):\n${compile_output}\n${compile_error}")
endif()

if(NOT COMPILE_ONLY)
  include("${CMAKE_CURRENT_LIST_DIR}/check_wasm.cmake")
endif()
