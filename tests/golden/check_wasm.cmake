# Validates one wasigoc-generated .wasm module. Invoked via `cmake -P` from
# a ctest entry (see the wasigo_add_golden() function in CMakeLists.txt).
#
# Required: WASM_FILE
# Optional: WASMTIME (path to a wasmtime executable), and either
#   EXPECTED_OUTPUT (the program's expected stdout, with literal "\n"
#   standing in for a newline) or EXPECTED_OUTPUT_FILE (a fixture file
#   compared byte-for-byte, no escaping -- use this when the real output
#   itself contains literal backslash-n/t/" sequences, e.g. printed JSON,
#   which would collide with EXPECTED_OUTPUT's own substitution). If
#   WASMTIME and one of these is given, the module is actually run and its
#   output is checked; otherwise this just confirms a well-formed wasm
#   module was produced, since a wasm runtime may not be installed
#   everywhere this project is built.

if(NOT DEFINED WASM_FILE)
  message(FATAL_ERROR "check_wasm.cmake: WASM_FILE not set")
endif()
if(NOT EXISTS "${WASM_FILE}")
  message(FATAL_ERROR "wasigoc/wasi-sdk did not produce ${WASM_FILE}")
endif()

if(WASMTIME AND NOT WASMTIME STREQUAL "WASMTIME-NOTFOUND")
  # --dir=.::. preopens the current directory into the guest as "." so
  # os.File goldens (WriteFile/Create/Open against a relative scratch-file
  # name) get real WASI filesystem access under wasmtime's sandbox, instead
  # of failing every op with "cannot create file" -- harmless for goldens
  # that don't touch the filesystem at all.
  execute_process(
    COMMAND "${WASMTIME}" run --dir=.::. "${WASM_FILE}"
    OUTPUT_VARIABLE actual_output
    RESULT_VARIABLE run_result
  )
  if(NOT run_result EQUAL 0)
    message(FATAL_ERROR "${WASM_FILE} exited with code ${run_result} under wasmtime")
  endif()
  if(DEFINED EXPECTED_OUTPUT_FILE)
    file(READ "${EXPECTED_OUTPUT_FILE}" expected)
    if(NOT actual_output STREQUAL expected)
      message(FATAL_ERROR
        "output mismatch for ${WASM_FILE}\n--- expected (${EXPECTED_OUTPUT_FILE}) ---\n${expected}\n--- actual ---\n${actual_output}")
    endif()
  elseif(DEFINED EXPECTED_OUTPUT)
    string(REPLACE "\\n" "\n" expected "${EXPECTED_OUTPUT}")
    if(NOT actual_output STREQUAL expected)
      message(FATAL_ERROR
        "output mismatch for ${WASM_FILE}\n--- expected ---\n${expected}\n--- actual ---\n${actual_output}")
    endif()
  endif()
else()
  file(READ "${WASM_FILE}" magic LIMIT 4 HEX)
  if(NOT magic STREQUAL "0061736d")
    message(FATAL_ERROR "${WASM_FILE} doesn't start with the wasm magic number (got 0x${magic})")
  endif()
endif()
