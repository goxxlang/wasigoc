cmake_minimum_required(VERSION 3.16)
project(wasigoc LANGUAGES CXX)

set(CMAKE_CXX_STANDARD 20)
set(CMAKE_CXX_STANDARD_REQUIRED ON)
set(CMAKE_CXX_EXTENSIONS OFF)

# wasigoc is a host-side dev tool (like WASMBruja's brujac / WASMVoodooCompile's
# voodoomc), built with whatever compiler this build is using -- it never runs
# inside WASI itself. Its OUTPUT is what targets WASI: each golden test below
# feeds a generated .cpp to a real wasi-sdk clang++ (wasm32-wasip1) to prove
# the whole from-Go pipeline actually produces a valid wasm module, not just
# plausible-looking C++.

add_library(wasigoc_lib
  src/lexer.cc
  src/parser.cc
  src/cpp_generator.cc
  src/module_loader.cc
)
target_include_directories(wasigoc_lib PUBLIC ${CMAKE_CURRENT_SOURCE_DIR}/src)
# wasigoc inlines src/runtime.hpp into every generated TU so the user's
# clang++ line stays "one file in, one wasm out".
file(TO_CMAKE_PATH "${CMAKE_CURRENT_SOURCE_DIR}/src/runtime.hpp" WASIGO_RUNTIME_PATH)
target_compile_definitions(wasigoc_lib PRIVATE "WASIGO_RUNTIME_PATH=\"${WASIGO_RUNTIME_PATH}\"")

if(MSVC)
  target_compile_options(wasigoc_lib PRIVATE /W4)
else()
  target_compile_options(wasigoc_lib PRIVATE -Wall -Wextra -Wpedantic)
endif()

add_executable(wasigoc src/main.cc)
target_link_libraries(wasigoc PRIVATE wasigoc_lib)
file(TO_CMAKE_PATH "${CMAKE_CURRENT_SOURCE_DIR}/stdlib" WASIGO_STDLIB_PATH)
target_compile_definitions(wasigoc PRIVATE "WASIGO_STDLIB_PATH=\"${WASIGO_STDLIB_PATH}\"")

add_executable(wasigo_runtime_smoketest tests/runtime_smoketest.cc)
target_include_directories(wasigo_runtime_smoketest PRIVATE ${CMAKE_CURRENT_SOURCE_DIR}/src)
add_test(NAME runtime_smoketest COMMAND wasigo_runtime_smoketest)

add_executable(wasigo_frontend_test tests/frontend_test.cc)
target_link_libraries(wasigo_frontend_test PRIVATE wasigoc_lib)
add_test(NAME frontend_test COMMAND wasigo_frontend_test)

enable_testing()

# ---- Locate wasi-sdk (optional) ---------------------------------------------
# Golden tests still run without it -- they just stop at "wasigoc produced
# plausible C++" instead of "wasi-sdk actually turned it into a wasm module".
if(NOT DEFINED WASIGO_WASI_SDK_PATH)
  if(DEFINED ENV{WASI_SDK_PATH})
    set(WASIGO_WASI_SDK_PATH "$ENV{WASI_SDK_PATH}")
  elseif(DEFINED ENV{USERPROFILE} AND EXISTS "$ENV{USERPROFILE}/wasi-sdk")
    set(WASIGO_WASI_SDK_PATH "$ENV{USERPROFILE}/wasi-sdk")
  elseif(DEFINED ENV{HOME} AND EXISTS "$ENV{HOME}/wasi-sdk")
    set(WASIGO_WASI_SDK_PATH "$ENV{HOME}/wasi-sdk")
  endif()
endif()

set(WASIGO_HAVE_WASI_SDK FALSE)
if(WASIGO_WASI_SDK_PATH)
  find_program(WASIGO_CLANGXX
    NAMES wasm32-wasip1-clang++ wasm32-wasip1-clang++.exe clang++ clang++.exe
    PATHS "${WASIGO_WASI_SDK_PATH}/bin"
    NO_DEFAULT_PATH
  )
  if(WASIGO_CLANGXX)
    set(WASIGO_HAVE_WASI_SDK TRUE)
    set(WASIGO_WASI_SYSROOT "${WASIGO_WASI_SDK_PATH}/share/wasi-sysroot")
    message(STATUS "wasigo: using wasi-sdk clang++ at ${WASIGO_CLANGXX}")
  endif()
endif()
if(NOT WASIGO_HAVE_WASI_SDK)
  message(STATUS "wasigo: no wasi-sdk found (set WASI_SDK_PATH or -DWASIGO_WASI_SDK_PATH=...) "
                  "-- golden tests will transpile but not compile to wasm")
endif()

find_program(WASIGO_WASMTIME NAMES wasmtime wasmtime.exe)

# ---- Golden tests ------------------------------------------------------------
# Each one: wasigoc translates examples/<dir>/<name>.go, then (if wasi-sdk was
# found) that output is compiled straight to a wasm32-wasip1 module. The
# ctest entry then checks the module is well-formed and, if wasmtime is also
# on PATH, that running it produces exactly the expected stdout.
set(WASIGO_GEN_DIR "${CMAKE_CURRENT_BINARY_DIR}/generated")
file(MAKE_DIRECTORY "${WASIGO_GEN_DIR}")

function(wasigo_add_golden name go_rel_path expected_output)
  set(gen_cpp "${WASIGO_GEN_DIR}/${name}.cpp")
  set(go_src "${CMAKE_CURRENT_SOURCE_DIR}/examples/${go_rel_path}")

  add_custom_command(
    OUTPUT "${gen_cpp}"
    COMMAND wasigoc "${go_src}" -o "${gen_cpp}" --out-dir "${WASIGO_GEN_DIR}"
    DEPENDS wasigoc "${go_src}" "${CMAKE_CURRENT_SOURCE_DIR}/src/runtime.hpp" ${ARGN}
    COMMENT "wasigoc: generating ${name}.cpp"
    VERBATIM
  )

  add_custom_target(wasigo_generate_${name} ALL DEPENDS "${gen_cpp}")

  # Host-side proof the generated TU is real C++. Always runs, even if
  # wasi-sdk is missing. Wasm proof is the separate `_golden` test, which
  # uses wasm32-wasip1-clang++ with the noeh include order in run_golden.cmake.
  add_executable(wasigo_native_${name} "${gen_cpp}")
  add_dependencies(wasigo_native_${name} wasigo_generate_${name})
  target_include_directories(wasigo_native_${name} PRIVATE "${WASIGO_GEN_DIR}")
  add_test(NAME ${name}_native COMMAND wasigo_native_${name})

  if(WASIGO_HAVE_WASI_SDK)
    set(gen_wasm "${WASIGO_GEN_DIR}/${name}.wasm")
    # A fixture file at tests/golden/expected/<name>.txt (exact bytes, real
    # newlines) takes priority over the inline expected_output string when
    # present -- for output containing literal backslash-n/backslash-t/
    # backslash-quote sequences (e.g. printed JSON), which an inline string
    # can't represent through run_golden.cmake's own "\\n"-means-newline
    # substitution without ambiguity.
    set(expected_file "${CMAKE_CURRENT_SOURCE_DIR}/tests/golden/expected/${name}.txt")
    set(_expected_file_arg)
    if(EXISTS "${expected_file}")
      set(_expected_file_arg "-DEXPECTED_OUTPUT_FILE=${expected_file}")
    endif()
    add_test(
      NAME ${name}_golden
      COMMAND ${CMAKE_COMMAND}
              -DCLANGXX=${WASIGO_CLANGXX}
              -DSYSROOT=${WASIGO_WASI_SYSROOT}
              -DINCLUDE_DIR=${WASIGO_GEN_DIR}
              -DCPP_FILE=${gen_cpp}
              -DWASM_FILE=${gen_wasm}
              -DWASMTIME=${WASIGO_WASMTIME}
              "-DEXPECTED_OUTPUT=${expected_output}"
              ${_expected_file_arg}
              -P "${CMAKE_CURRENT_SOURCE_DIR}/tests/golden/run_golden.cmake"
    )
  endif()
endfunction()

wasigo_add_golden(hello "hello/hello.go" "hello, wasi\\n")
wasigo_add_golden(fib "fib/fib.go"
  "fib(0) = 0\\nfib(1) = 1\\nfib(2) = 1\\nfib(3) = 2\\nfib(4) = 3\\nfib(5) = 5\\nfib(6) = 8\\nfib(7) = 13\\nfib(8) = 21\\nfib(9) = 34\\n10 / 2 = 5\\nerror: division by zero\\n")
wasigo_add_golden(structs "structs/point.go"
  "sum before scale: 5\\nx: 10 y: 15\\nsum after scale: 25\\norigin sum: 0\\n")
wasigo_add_golden(collections "collections/collections.go"
  "len: 6\\nsum: 21\\nnums[0] = 1\\nnums[1] = 2\\nnums[2] = 3\\nnums[3] = 4\\nnums[4] = 5\\nnums[5] = 6\\nalice is 30\\ndave not found\\ntotal age: 95\\n")
wasigo_add_golden(rosetta "rosetta/rosetta.go" "7\\nred 0 green 1 blue 2\\nsum 6\\nrecovered boom\\n")
wasigo_add_golden(importpkg "importpkg/main.go" "sum: 5\\n"
  "${CMAKE_CURRENT_SOURCE_DIR}/examples/geom/point.go"
  "${CMAKE_CURRENT_SOURCE_DIR}/examples/geom/origin.go")
wasigo_add_golden(stdlibpkg "stdlibpkg/main.go" "HI\\n42\\nyes\\n2\\n1 2 3\\nc.go\\nabs\\n"
  "${CMAKE_CURRENT_SOURCE_DIR}/stdlib/strings/strings.go"
  "${CMAKE_CURRENT_SOURCE_DIR}/stdlib/strconv/strconv.go"
  "${CMAKE_CURRENT_SOURCE_DIR}/stdlib/unicode/utf8/utf8.go"
  "${CMAKE_CURRENT_SOURCE_DIR}/stdlib/sort/sort.go"
  "${CMAKE_CURRENT_SOURCE_DIR}/stdlib/path/path.go"
  "${CMAKE_CURRENT_SOURCE_DIR}/stdlib/math/math.go")
wasigo_add_golden(asyncval "asyncval/main.go" "42\\n")
wasigo_add_golden(iface "iface/main.go" "sum: 5\\nx: 2\\nnot string\\n")
wasigo_add_golden(lang "lang/main.go" "3\\n7\\ntwo-or-three\\n9\\n0 71\\n1 111\\nGo\\n4\\n8\\n3\\n")
wasigo_add_golden(otigen "otigen/main.go" "0\\n2\\n7\\n3\\n")
wasigo_add_golden(literals "literals/main.go" "255\\n8\\n10\\n8\\n0.5\\nA\\nA\\n7\\n937\\n10\\n7\\n4\\n8\\n15.5\\n1\\n100\\n")
wasigo_add_golden(rangchan "rangchan/main.go" "6\\n")
wasigo_add_golden(more "more/main.go" "PT\\n5\\n15\\n4\\nargs\\nhi\\n")
wasigo_add_golden(iowrite "iowrite/main.go" "2\\n2\\n"
  "${CMAKE_CURRENT_SOURCE_DIR}/stdlib/io/io.go")
wasigo_add_golden(ctrl "ctrl/main.go" "11\\n1\\n5\\neq\\n6\\n9\\nhi\\ntrue\\ns 1\\n"
  "${CMAKE_CURRENT_SOURCE_DIR}/stdlib/strings/strings.go"
  "${CMAKE_CURRENT_SOURCE_DIR}/stdlib/strconv/strconv.go"
  "${CMAKE_CURRENT_SOURCE_DIR}/stdlib/time/time.go")
wasigo_add_golden(anyval "anyval/main.go" "7\\n1\\n3\\n0 0 0 3\\n0\\n5\\n8 7 2\\n5 np\\nhi\\nc.go\\nis\\npf\\n"
  "${CMAKE_CURRENT_SOURCE_DIR}/stdlib/bytes/bytes.go"
  "${CMAKE_CURRENT_SOURCE_DIR}/stdlib/strconv/strconv.go"
  "${CMAKE_CURRENT_SOURCE_DIR}/stdlib/path/filepath/filepath.go")
wasigo_add_golden(modnest "modnest/main.go" "sum: 5\\nred: 1\\n"
  "${CMAKE_CURRENT_SOURCE_DIR}/examples/modnest/go.mod"
  "${CMAKE_CURRENT_SOURCE_DIR}/examples/modnest/shape/point.go"
  "${CMAKE_CURRENT_SOURCE_DIR}/examples/modnest/shape/palette/palette.go")
wasigo_add_golden(newpkgs "newpkgs/main.go"
  "-1\\n0\\n1\\n1 2 3\\ntrue\\nfalse\\n1\\n3\\n1\\n3 2 1\\ntrue\\nfalse\\n1\\n1\\ntrue\\ntrue\\nfalse\\ntrue\\nA\\n3\\n63\\n8\\n16\\n3\\n0\\n2\\n3\\n3\\n1\\n"
  "${CMAKE_CURRENT_SOURCE_DIR}/stdlib/cmp/cmp.go"
  "${CMAKE_CURRENT_SOURCE_DIR}/stdlib/slices/slices.go"
  "${CMAKE_CURRENT_SOURCE_DIR}/stdlib/maps/maps.go"
  "${CMAKE_CURRENT_SOURCE_DIR}/stdlib/unicode/unicode.go"
  "${CMAKE_CURRENT_SOURCE_DIR}/stdlib/math/bits/bits.go"
  "${CMAKE_CURRENT_SOURCE_DIR}/stdlib/container/list/list.go"
  "${CMAKE_CURRENT_SOURCE_DIR}/stdlib/container/ring/ring.go"
  "${CMAKE_CURRENT_SOURCE_DIR}/stdlib/sync/sync.go")
wasigo_add_golden(fillins "fillins/main.go"
  "hi\\nhi\\n3 a b c\\nkey value true\\nbbb\\ntrue\\n4\\n42 \\n42 \\nff\\nhello world\\nhi\\nk v true\\na/c\\ntrue\\n/a/b/ c.go\\na/b/c\\n1 2 3 4 5\\n2\\n3\\n4\\n1024\\ntrue\\ntrue\\n1\\n1\\ntrue\\n"
  "${CMAKE_CURRENT_SOURCE_DIR}/stdlib/strings/strings.go"
  "${CMAKE_CURRENT_SOURCE_DIR}/stdlib/strconv/strconv.go"
  "${CMAKE_CURRENT_SOURCE_DIR}/stdlib/bytes/bytes.go"
  "${CMAKE_CURRENT_SOURCE_DIR}/stdlib/io/io.go"
  "${CMAKE_CURRENT_SOURCE_DIR}/stdlib/path/path.go"
  "${CMAKE_CURRENT_SOURCE_DIR}/stdlib/path/filepath/filepath.go"
  "${CMAKE_CURRENT_SOURCE_DIR}/stdlib/sort/sort.go"
  "${CMAKE_CURRENT_SOURCE_DIR}/stdlib/math/math.go"
  "${CMAKE_CURRENT_SOURCE_DIR}/stdlib/unicode/utf8/utf8.go")
wasigo_add_golden(morepkgs "morepkgs/main.go"
  "6869\\nhi \\naGk=\\nhello \\n1 65\\n0\\n1\\ndone 1 true\\n"
  "${CMAKE_CURRENT_SOURCE_DIR}/stdlib/encoding/hex/hex.go"
  "${CMAKE_CURRENT_SOURCE_DIR}/stdlib/encoding/base64/base64.go"
  "${CMAKE_CURRENT_SOURCE_DIR}/stdlib/log/log.go"
  "${CMAKE_CURRENT_SOURCE_DIR}/stdlib/unicode/utf16/utf16.go"
  "${CMAKE_CURRENT_SOURCE_DIR}/stdlib/container/heap/heap.go")
wasigo_add_golden(errwrap "errwrap/main.go"
  "lookup : not found\\ntrue\\ntrue\\ne1\\ne2\\ntrue\\ntrue\\n")
wasigo_add_golden(fprint "fprint/main.go"
  "hi 1 true\\na b\\n5-x-false\\nbuffered 42\\n7\\n"
  "${CMAKE_CURRENT_SOURCE_DIR}/stdlib/bytes/bytes.go")
wasigo_add_golden(readers "readers/main.go"
  "hello bytes\\nhello strings\\n6  copied\\n88\\n65\\n88 X\\n"
  "${CMAKE_CURRENT_SOURCE_DIR}/stdlib/bytes/bytes.go"
  "${CMAKE_CURRENT_SOURCE_DIR}/stdlib/strings/strings.go"
  "${CMAKE_CURRENT_SOURCE_DIR}/stdlib/io/io.go")
# os.File touches the real filesystem (WriteFile/Open/Create). wasmtime is
# invoked with --dir=.::. (see check_wasm.cmake) so the sandbox has a
# preopened cwd and these ops actually succeed under the wasm runtime, not
# just under host g++.
wasigo_add_golden(osfile "osfile/main.go"
  "\\nhello file \\n\\nhello file \\n\\n\\n7 \\nwritten\\ntrue\\n"
  "${CMAKE_CURRENT_SOURCE_DIR}/stdlib/io/io.go")
wasigo_add_golden(timepkg "timepkg/main.go"
  "1700000000\\n2023 11 14\\n22 13 20\\n2\\n2023-11-14 22:13:20 UTC\\n5400000000000\\ntrue true true\\n1.5s\\n250ms\\n3h0m0s\\n0s\\n"
  "${CMAKE_CURRENT_SOURCE_DIR}/stdlib/time/time.go")
wasigo_add_golden(bufiopkg "bufiopkg/main.go"
  "1 line one\\n2 line two\\n3 line three\\n\\nhello world\\n"
  "${CMAKE_CURRENT_SOURCE_DIR}/stdlib/bufio/bufio.go"
  "${CMAKE_CURRENT_SOURCE_DIR}/stdlib/bytes/bytes.go"
  "${CMAKE_CURRENT_SOURCE_DIR}/stdlib/strings/strings.go")
wasigo_add_golden(atomicpkg "atomicpkg/main.go"
  "8\\ntrue 100\\nfalse 100\\n15\\n15 0\\ntrue\\nhi\\n"
  "${CMAKE_CURRENT_SOURCE_DIR}/stdlib/sync/atomic/atomic.go")
wasigo_add_golden(binrand "binrand/main.go"
  "305419896\\n305419896\\n12345678901234\\n1 2\\n0\\n49\\n23\\n0\\ntrue\\n5\\nfalse\\n"
  "${CMAKE_CURRENT_SOURCE_DIR}/stdlib/encoding/binary/binary.go"
  "${CMAKE_CURRENT_SOURCE_DIR}/stdlib/math/rand/rand.go"
  "${CMAKE_CURRENT_SOURCE_DIR}/stdlib/time/time.go")
wasigo_add_golden(flagpkg "flagpkg/main.go"
  "world 1 false\\n0\\n"
  "${CMAKE_CURRENT_SOURCE_DIR}/stdlib/flag/flag.go"
  "${CMAKE_CURRENT_SOURCE_DIR}/stdlib/strconv/strconv.go"
  "${CMAKE_CURRENT_SOURCE_DIR}/stdlib/strings/strings.go")
wasigo_add_golden(contextpkg "contextpkg/main.go"
  "cancelled: context canceled\\nalice\\ntrue\\ntrue\\n"
  "${CMAKE_CURRENT_SOURCE_DIR}/stdlib/context/context.go")
wasigo_add_golden(urlpkg "urlpkg/main.go"
  "hello+world%2Ffoo%3F\\nhello world/foo? \\na%20b/c\\n\\nhttps example.com /path/to/thing a=1&b=two frag\\nhttps://example.com/path/to/thing?a=1&b=two#frag\\n\\n1 two\\ntrue /just/a/path\\n"
  "${CMAKE_CURRENT_SOURCE_DIR}/stdlib/net/url/url.go")
# No EXPECTED_OUTPUT: the real output has embedded quotes/tabs/backslash-n
# JSON-escape sequences that are fragile to get exactly right through this
# macro's own "\\n"-means-newline substitution (see run_golden.cmake) --
# already thoroughly verified by hand instead (generate + host g++ compile
# + run, all 18 output lines checked, including round-tripping the escaped
# string back out byte-for-byte); this still proves the wasi-sdk structural
# compile succeeds.
wasigo_add_golden(jsonpkg "jsonpkg/main.go" ""
  "${CMAKE_CURRENT_SOURCE_DIR}/stdlib/encoding/json/json.go")
wasigo_add_golden(execpkg "execpkg/main.go"
  "3\\ntrue\\nexec: not supported on wasm32-wasip1 (WASI preview 1 has no subprocess support)\\ntrue true\\ntrue\\n"
  "${CMAKE_CURRENT_SOURCE_DIR}/stdlib/os/exec/exec.go")
wasigo_add_golden(regexppkg "regexppkg/main.go"
  "true\\nfalse\\nfalse\\n123\\n2\\n123\\n456\\ntrue\\nfalse\\na b c d\\n4\\na\\nb\\nc \\nd\\ntrue\\ntrue\\nfalse\\ntrue\\nfalse\\ntrue\\ntrue \\n"
  "${CMAKE_CURRENT_SOURCE_DIR}/stdlib/regexp/regexp.go")
wasigo_add_golden(textprotopkg "textprotopkg/main.go"
  "Content-Type\\nX-Custom-Header\\ntext/plain\\n2\\none\\ntwo\\ntrue\\ntrue\\nexample.com\\ntext/plain\\nhello world\\nbody follows\\ntrue\\ntrue\\n"
  "${CMAKE_CURRENT_SOURCE_DIR}/stdlib/net/textproto/textproto.go"
  "${CMAKE_CURRENT_SOURCE_DIR}/stdlib/strings/strings.go")
wasigo_add_golden(mailpkg "mailpkg/main.go"
  "true\\nAlice Smith\\nalice@example.com\\nAlice Smith <alice@example.com>\\ntrue\\ntrue\\nbob@example.com\\nbob@example.com\\ntrue\\ntrue\\ntrue\\n3\\nAlice <alice@example.com>\\nbob@example.com\\nCarol X\\ncarol@example.com\\n"
  "${CMAKE_CURRENT_SOURCE_DIR}/stdlib/net/mail/mail.go")
wasigo_add_golden(fnvpkg "fnvpkg/main.go"
  "3069866343\\n1335831723\\n8883723591023973575\\n11831194018420276491\\n2166136261\\n4\\n1\\n"
  "${CMAKE_CURRENT_SOURCE_DIR}/stdlib/hash/fnv/fnv.go")
wasigo_add_golden(crc32pkg "crc32pkg/main.go"
  "0\\n907060870\\n1095738169\\n907060870\\n0\\n4\\n1\\n"
  "${CMAKE_CURRENT_SOURCE_DIR}/stdlib/hash/crc32/crc32.go")
wasigo_add_golden(adler32pkg "adler32pkg/main.go"
  "1\\n300286872\\n103547413\\n300286872\\n4\\n4\\n"
  "${CMAKE_CURRENT_SOURCE_DIR}/stdlib/hash/adler32/adler32.go")
wasigo_add_golden(md5pkg "md5pkg/main.go"
  "d41d8cd98f00b204e9800998ecf8427e\\n900150983cd24fb0d6963f7d28e17f72\\n9e107d9d372bb6826bd81d3542a419d6\\n900150983cd24fb0d6963f7d28e17f72\\n9e107d9d372bb6826bd81d3542a419d6\\n900150983cd24fb0d6963f7d28e17f72\\n16\\n64\\n"
  "${CMAKE_CURRENT_SOURCE_DIR}/stdlib/crypto/md5/md5.go"
  "${CMAKE_CURRENT_SOURCE_DIR}/stdlib/encoding/hex/hex.go")
wasigo_add_golden(sha256pkg "sha256pkg/main.go"
  "e3b0c44298fc1c149afbf4c8996fb92427ae41e4649b934ca495991b7852b855\\nba7816bf8f01cfea414140de5dae2223b00361a396177a9cb410ff61f20015ad\\nd7a8fbb307d7809469ca9abcb0082e4f8d5651e46d3cdb762d02d0bf37c9e592\\nba7816bf8f01cfea414140de5dae2223b00361a396177a9cb410ff61f20015ad\\nd7a8fbb307d7809469ca9abcb0082e4f8d5651e46d3cdb762d02d0bf37c9e592\\nba7816bf8f01cfea414140de5dae2223b00361a396177a9cb410ff61f20015ad\\n32\\n64\\n"
  "${CMAKE_CURRENT_SOURCE_DIR}/stdlib/crypto/sha256/sha256.go"
  "${CMAKE_CURRENT_SOURCE_DIR}/stdlib/encoding/hex/hex.go")
wasigo_add_golden(sha1pkg "sha1pkg/main.go"
  "da39a3ee5e6b4b0d3255bfef95601890afd80709\\na9993e364706816aba3e25717850c26c9cd0d89d\\n2fd4e1c67a2d28fced849ee1bb76e7391b93eb12\\na9993e364706816aba3e25717850c26c9cd0d89d\\n2fd4e1c67a2d28fced849ee1bb76e7391b93eb12\\na9993e364706816aba3e25717850c26c9cd0d89d\\n20\\n64\\n"
  "${CMAKE_CURRENT_SOURCE_DIR}/stdlib/crypto/sha1/sha1.go"
  "${CMAKE_CURRENT_SOURCE_DIR}/stdlib/encoding/hex/hex.go")
wasigo_add_golden(reflectpkg "reflectpkg/main.go"
  "true\\n3\\nX\\nY\\nName\\ntrue\\n3\\ntrue\\norigin\\nPoint\\ntrue\\ntrue\\n42\\ntrue\\nhello\\ntrue\\ntrue\\n")
wasigo_add_golden(netpkg "netpkg/main.go"
  "true\\ntrue\\nfalse\\nfalse\\ntrue\\ntrue\\ntrue\\n5\\ntrue\\nhello\\n6\\ntrue\\n world\\ntrue\\ntrue\\ntrue\\ntrue\\nhi\\ntrue\\nping\\n127.0.0.1\\n80\\n[::1]:443\\ntrue\\npong\\n"
  "${CMAKE_CURRENT_SOURCE_DIR}/stdlib/net/net.go"
  "${CMAKE_CURRENT_SOURCE_DIR}/stdlib/strings/strings.go")
wasigo_add_golden(httppkg "httppkg/main.go"
  "true\\n200\\nok\\n200\\nPOST /echo hi\\n404\\nnot found\\n"
  "${CMAKE_CURRENT_SOURCE_DIR}/stdlib/net/http/http.go"
  "${CMAKE_CURRENT_SOURCE_DIR}/stdlib/net/net.go"
  "${CMAKE_CURRENT_SOURCE_DIR}/stdlib/strings/strings.go"
  "${CMAKE_CURRENT_SOURCE_DIR}/stdlib/strconv/strconv.go")
wasigo_add_golden(tokenpkg "tokenpkg/main.go"
  "IDENT\\n+\\nfunc\\n<-\\ntrue\\ntrue\\ntrue\\ntrue\\nfalse\\ntrue\\ntrue\\nfalse\\ntrue\\n"
  "${CMAKE_CURRENT_SOURCE_DIR}/stdlib/go/token/token.go")
# No inline EXPECTED_OUTPUT: the real output has embedded literal
# newlines (each SEMICOLON token's literal text is "\n") that would
# collide with this macro's own "\\n"-means-newline substitution the
# same way jsonpkg's did -- see tests/golden/expected/scannerpkg.txt.
wasigo_add_golden(scannerpkg "scannerpkg/main.go" ""
  "${CMAKE_CURRENT_SOURCE_DIR}/stdlib/go/scanner/scanner.go"
  "${CMAKE_CURRENT_SOURCE_DIR}/stdlib/go/token/token.go")
wasigo_add_golden(parserpkg "parserpkg/main.go"
  "true\\ntrue\\ntrue\\ntrue\\ntrue\\ntrue\\ntrue\\ntrue\\nPrintln\\n2\\ntrue\\ntrue\\ntrue\\nmain\\n1\\ntrue\\nadd\\n2\\na\\nint\\n1\\nint\\n4\\ntrue\\ntrue\\ntrue\\ntrue\\ntrue\\n"
  "${CMAKE_CURRENT_SOURCE_DIR}/stdlib/go/parser/parser.go"
  "${CMAKE_CURRENT_SOURCE_DIR}/stdlib/go/ast/ast.go"
  "${CMAKE_CURRENT_SOURCE_DIR}/stdlib/go/scanner/scanner.go"
  "${CMAKE_CURRENT_SOURCE_DIR}/stdlib/go/token/token.go")
# No inline EXPECTED_OUTPUT: multi-line printed/formatted source with
# embedded tabs -- same escaping fragility as jsonpkg/scannerpkg, see
# tests/golden/expected/{printerpkg,formatpkg}.txt.
wasigo_add_golden(printerpkg "printerpkg/main.go" ""
  "${CMAKE_CURRENT_SOURCE_DIR}/stdlib/go/printer/printer.go"
  "${CMAKE_CURRENT_SOURCE_DIR}/stdlib/go/parser/parser.go"
  "${CMAKE_CURRENT_SOURCE_DIR}/stdlib/go/ast/ast.go"
  "${CMAKE_CURRENT_SOURCE_DIR}/stdlib/go/scanner/scanner.go"
  "${CMAKE_CURRENT_SOURCE_DIR}/stdlib/go/token/token.go")
wasigo_add_golden(formatpkg "formatpkg/main.go" ""
  "${CMAKE_CURRENT_SOURCE_DIR}/stdlib/go/format/format.go"
  "${CMAKE_CURRENT_SOURCE_DIR}/stdlib/go/printer/printer.go"
  "${CMAKE_CURRENT_SOURCE_DIR}/stdlib/go/parser/parser.go"
  "${CMAKE_CURRENT_SOURCE_DIR}/stdlib/go/ast/ast.go"
  "${CMAKE_CURRENT_SOURCE_DIR}/stdlib/go/scanner/scanner.go"
  "${CMAKE_CURRENT_SOURCE_DIR}/stdlib/go/token/token.go")
wasigo_add_golden(typespkg "typespkg/main.go"
  "true\\n*int\\ntrue\\n[]string\\nfalse\\nint\\nstring\\nbool\\nerror\\nint\\nerror\\n*int\\ntrue\\ntrue\\n5\\ntrue\\ntrue\\ntrue\\ntrue\\ntrue\\nint\\nfloat64\\ntrue\\ntrue\\ntrue\\ntrue\\nmap[string]int\\ntrue\\nint\\ntrue\\n[]int\\ntrue\\ntrue\\n5\\ntrue\\ntrue\\ntrue\\ntrue\\ntrue\\ntrue\\ntrue\\nfalse\\nDuration\\ntrue\\ninterface{Read([]int)int}\\nfalse\\nfalse\\nSet[int]\\ntrue\\n5\\ntrue\\ntrue\\n1\\ntrue\\ntrue\\ntrue\\nDuration\\nfalse\\ntrue\\n5\\ntrue\\ntrue\\ntrue\\ntrue\\ntrue\\nstring\\nSet[int]\\nfalse\\ntrue\\nint\\n"
  "${CMAKE_CURRENT_SOURCE_DIR}/stdlib/go/types/types.go"
  "${CMAKE_CURRENT_SOURCE_DIR}/stdlib/go/parser/parser.go"
  "${CMAKE_CURRENT_SOURCE_DIR}/stdlib/go/ast/ast.go"
  "${CMAKE_CURRENT_SOURCE_DIR}/stdlib/go/scanner/scanner.go"
  "${CMAKE_CURRENT_SOURCE_DIR}/stdlib/go/token/token.go")
wasigo_add_golden(crc64pkg "crc64pkg/main.go"
  "true\\ntrue\\ntrue\\ntrue\\n8\\n1\\n8\\ntrue\\ntrue\\n"
  "${CMAKE_CURRENT_SOURCE_DIR}/stdlib/hash/crc64/crc64.go")
# No inline EXPECTED_OUTPUT: real output has embedded double-quotes
# (StringVar.String()/Map.String() quote their contents), same escaping
# fragility as jsonpkg -- see tests/golden/expected/expvarpkg.txt.
wasigo_add_golden(debugpkg "debugpkg/main.go"
  "true\\ntrue\\ntrue\\ntrue\\ntrue\\nsurvived\\n"
  "${CMAKE_CURRENT_SOURCE_DIR}/stdlib/runtime/debug/debug.go")
wasigo_add_golden(signalpkg "signalpkg/main.go"
  "no signal received, as expected\\nall signal functions callable without crashing\\n"
  "${CMAKE_CURRENT_SOURCE_DIR}/stdlib/os/signal/signal.go")
wasigo_add_golden(tarpkg "tarpkg/main.go"
  "true\\ntrue\\ntrue\\ntrue\\ntrue\\ntrue\\ntrue\\ntrue\\ntrue\\ntrue\\ntrue\\ntrue\\n"
  "${CMAKE_CURRENT_SOURCE_DIR}/stdlib/archive/tar/tar.go")
wasigo_add_golden(pempkg "pempkg/main.go"
  "true\\ntrue\\ntrue\\ntrue\\ntrue\\ntrue\\ntrue\\ntrue\\ntrue\\ntrue\\ntrue\\ntrue\\ntrue\\ntrue\\n"
  "${CMAKE_CURRENT_SOURCE_DIR}/stdlib/encoding/pem/pem.go"
  "${CMAKE_CURRENT_SOURCE_DIR}/stdlib/encoding/base64/base64.go")
wasigo_add_golden(slogpkg "slogpkg/main.go"
  "true\\ntrue\\ntrue\\ntrue\\ntrue\\ntrue\\ntrue\\ntrue\\ntrue\\n"
  "${CMAKE_CURRENT_SOURCE_DIR}/stdlib/log/slog/slog.go"
  "${CMAKE_CURRENT_SOURCE_DIR}/stdlib/bytes/bytes.go")
wasigo_add_golden(subtlepkg "subtlepkg/main.go"
  "true\\ntrue\\ntrue\\ntrue\\ntrue\\ntrue\\ntrue\\ntrue\\ntrue\\ntrue\\ntrue\\ntrue\\ntrue\\ntrue\\n"
  "${CMAKE_CURRENT_SOURCE_DIR}/stdlib/crypto/subtle/subtle.go")
wasigo_add_golden(userpkg "userpkg/main.go"
  "true\\ntrue\\ntrue\\n"
  "${CMAKE_CURRENT_SOURCE_DIR}/stdlib/os/user/user.go")
wasigo_add_golden(htmlpkg "htmlpkg/main.go"
  "true\\ntrue\\ntrue\\ntrue\\ntrue\\ntrue\\ntrue\\ntrue\\ntrue\\ntrue\\n"
  "${CMAKE_CURRENT_SOURCE_DIR}/stdlib/html/html.go"
  "${CMAKE_CURRENT_SOURCE_DIR}/stdlib/strconv/strconv.go")
wasigo_add_golden(bigpkg "bigpkg/main.go"
  "true\\ntrue\\ntrue\\ntrue\\ntrue\\ntrue\\ntrue\\ntrue\\ntrue\\ntrue\\ntrue\\ntrue\\ntrue\\ntrue\\ntrue\\ntrue\\ntrue\\ntrue\\ntrue\\ntrue\\ntrue\\ntrue\\ntrue\\ntrue\\ntrue\\ntrue\\ntrue\\ntrue\\ntrue\\ntrue\\ntrue\\ntrue\\ntrue\\ntrue\\ntrue\\ntrue\\ntrue\\ntrue\\ntrue\\ntrue\\ntrue\\ntrue\\n"
  "${CMAKE_CURRENT_SOURCE_DIR}/stdlib/math/big/big.go"
  "${CMAKE_CURRENT_SOURCE_DIR}/stdlib/strconv/strconv.go")
wasigo_add_golden(runtimepkg "runtimepkg/main.go"
  "wasip1\\nwasm\\n1\\n1\\n1\\n1\\ngo++wasigoc\\nsurvived\\n"
  "${CMAKE_CURRENT_SOURCE_DIR}/stdlib/runtime/runtime.go")
wasigo_add_golden(mimepkg "mimepkg/main.go"
  "true\\ntrue\\ntrue\\ntrue\\ntrue\\ntrue\\ntrue\\n"
  "${CMAKE_CURRENT_SOURCE_DIR}/stdlib/mime/mime.go")
wasigo_add_golden(tabwriterpkg "tabwriterpkg/main.go"
  "true\\ntrue\\ntrue\\n"
  "${CMAKE_CURRENT_SOURCE_DIR}/stdlib/text/tabwriter/tabwriter.go")
wasigo_add_golden(expvarpkg "expvarpkg/main.go" ""
  "${CMAKE_CURRENT_SOURCE_DIR}/stdlib/expvar/expvar.go"
  "${CMAKE_CURRENT_SOURCE_DIR}/stdlib/strconv/strconv.go")
wasigo_add_golden(rc4pkg "rc4pkg/main.go"
  "true\\ntrue\\ntrue\\ntrue\\ntrue\\ntrue\\n"
  "${CMAKE_CURRENT_SOURCE_DIR}/stdlib/crypto/rc4/rc4.go"
  "${CMAKE_CURRENT_SOURCE_DIR}/stdlib/encoding/hex/hex.go")
wasigo_add_golden(sha512pkg "sha512pkg/main.go"
  "true\\ntrue\\ntrue\\ntrue\\ntrue\\n64\\n128\\n"
  "${CMAKE_CURRENT_SOURCE_DIR}/stdlib/crypto/sha512/sha512.go"
  "${CMAKE_CURRENT_SOURCE_DIR}/stdlib/encoding/hex/hex.go")
wasigo_add_golden(ioutilpkg "ioutilpkg/main.go"
  "true\\nhello ioutil\\ntrue\\ntrue\\nround trip\\n"
  "${CMAKE_CURRENT_SOURCE_DIR}/stdlib/io/ioutil/ioutil.go"
  "${CMAKE_CURRENT_SOURCE_DIR}/stdlib/io/io.go")
wasigo_add_golden(maphashpkg "maphashpkg/main.go"
  "true\\ntrue\\ntrue\\ntrue\\ntrue\\ntrue\\n8\\ntrue\\ntrue\\n"
  "${CMAKE_CURRENT_SOURCE_DIR}/stdlib/hash/maphash/maphash.go"
  "${CMAKE_CURRENT_SOURCE_DIR}/stdlib/time/time.go")
wasigo_add_golden(base32pkg "base32pkg/main.go"
  "true\\ntrue\\ntrue\\ntrue\\ntrue\\ntrue\\ntrue\\ntrue\\ntrue\\ntrue\\ntrue\\ntrue\\ntrue\\ntrue\\ntrue\\ntrue\\ntrue\\n"
  "${CMAKE_CURRENT_SOURCE_DIR}/stdlib/encoding/base32/base32.go")
wasigo_add_golden(csvpkg "csvpkg/main.go"
  "true\\n3\\ntrue\\ntrue\\ntrue\\ntrue\\ntrue\\ntrue\\ntrue\\ntrue\\ntrue\\ntrue\\n2\\ntrue\\ntrue\\ntrue\\ntrue\\ntrue\\ntrue\\ntrue\\ntrue\\n1\\ntrue\\n"
  "${CMAKE_CURRENT_SOURCE_DIR}/stdlib/encoding/csv/csv.go")
wasigo_add_golden(constraintpkg "constraintpkg/main.go"
  "true\\nfalse\\ntrue\\ntrue\\nlinux && amd64\\ntrue\\nfalse\\ntrue\\n(linux || darwin) && !cgo\\ntrue\\nfalse\\nfalse\\ntrue\\ntrue\\ntrue\\nfalse\\ntrue\\ntrue\\nfalse\\ntrue\\ntrue\\ntrue\\n"
  "${CMAKE_CURRENT_SOURCE_DIR}/stdlib/go/build/constraint/constraint.go")
wasigo_add_golden(hmacpkg "hmacpkg/main.go"
  "true\\ntrue\\ntrue\\ntrue\\ntrue\\ntrue\\nfalse\\n"
  "${CMAKE_CURRENT_SOURCE_DIR}/stdlib/crypto/hmac/hmac.go"
  "${CMAKE_CURRENT_SOURCE_DIR}/stdlib/crypto/md5/md5.go"
  "${CMAKE_CURRENT_SOURCE_DIR}/stdlib/crypto/sha1/sha1.go"
  "${CMAKE_CURRENT_SOURCE_DIR}/stdlib/crypto/sha256/sha256.go"
  "${CMAKE_CURRENT_SOURCE_DIR}/stdlib/encoding/hex/hex.go")
# No inline EXPECTED_OUTPUT: printed source has embedded tabs, see
# tests/golden/expected/astpkg.txt. Exercises the extended grammar
# (var/const decls, pointer/slice/map types, composite literals,
# address-of/dereference, range-for, switch/case/default) end to end
# through parser -> ast -> printer.
wasigo_add_golden(astpkg "astpkg/main.go" ""
  "${CMAKE_CURRENT_SOURCE_DIR}/stdlib/go/printer/printer.go"
  "${CMAKE_CURRENT_SOURCE_DIR}/stdlib/go/parser/parser.go"
  "${CMAKE_CURRENT_SOURCE_DIR}/stdlib/go/ast/ast.go"
  "${CMAKE_CURRENT_SOURCE_DIR}/stdlib/go/scanner/scanner.go"
  "${CMAKE_CURRENT_SOURCE_DIR}/stdlib/go/token/token.go")
wasigo_add_golden(versionpkg "versionpkg/main.go"
  "true\\ntrue\\nfalse\\nfalse\\ngo1.21\\ngo1.21\\n-1\\n0\\n1\\n-1\\n"
  "${CMAKE_CURRENT_SOURCE_DIR}/stdlib/go/version/version.go")
wasigo_add_golden(rand2pkg "rand2pkg/main.go"
  "49\\n66\\n0\\n49\\ntrue\\n5\\nfalse\\ntrue\\ntrue\\n"
  "${CMAKE_CURRENT_SOURCE_DIR}/stdlib/math/rand/v2/rand.go")
wasigo_add_golden(suffixarraypkg "suffixarraypkg/main.go"
  "2\\n1 3\\n1\\n0\\n0\\nbanana\\n"
  "${CMAKE_CURRENT_SOURCE_DIR}/stdlib/index/suffixarray/suffixarray.go"
  "${CMAKE_CURRENT_SOURCE_DIR}/stdlib/sort/sort.go")
# textscannerpkg's output contains literal `"` bytes (a printed quoted
# token) which would collide with EXPECTED_OUTPUT's own "\\n" substitution
# -- same fragility as jsonpkg/scannerpkg -- see
# tests/golden/expected/textscannerpkg.txt.
wasigo_add_golden(imagepkg "imagepkg/main.go"
  "4 3\\nfalse\\ntrue\\nfalse\\n5 5 9 8\\n3 3 5 5\\n0 0 8 8\\n3 2\\n10 20 30 255\\n0 0 0 0\\n"
  "${CMAKE_CURRENT_SOURCE_DIR}/stdlib/image/image.go"
  "${CMAKE_CURRENT_SOURCE_DIR}/stdlib/image/color/color.go")
wasigo_add_golden(lzwpkg "lzwpkg/main.go"
  "true\\ntrue\\ntrue\\ntrue\\ntrue\\ntrue\\ntrue\\ntrue\\ntrue\\n"
  "${CMAKE_CURRENT_SOURCE_DIR}/stdlib/compress/lzw/lzw.go")
wasigo_add_golden(colorpkg "colorpkg/main.go"
  "65535 0 0 65535\\ntrue\\n76\\ntrue\\n255 255 255 255\\n128 0 0 128\\n0 255\\n"
  "${CMAKE_CURRENT_SOURCE_DIR}/stdlib/image/color/color.go")
wasigo_add_golden(doccommentpkg "doccommentpkg/main.go"
  "4\\ntrue\\n2\\nFoo does the thing.\\nIt returns a value.\\ntrue\\nDetails\\ntrue\\n2\\ntrue\\n2\\n    codeLine1()\\n"
  "${CMAKE_CURRENT_SOURCE_DIR}/stdlib/go/doc/comment/comment.go")
wasigo_add_golden(encodingpkg "encodingpkg/main.go"
  "3,4\\ntrue\\n3 4\\n2\\n3 4\\n"
  "${CMAKE_CURRENT_SOURCE_DIR}/stdlib/encoding/encoding.go")
wasigo_add_golden(synpkg "synpkg/main.go"
  "true\\ntrue\\n3\\na b c\\ntrue\\ntrue\\ntrue\\n2\\ntrue\\n97 99\\n0 96\\n4\\ntrue\\n1\\ntrue\\n48 57\\nfalse\\n"
  "${CMAKE_CURRENT_SOURCE_DIR}/stdlib/regexp/syntax/syntax.go")
wasigo_add_golden(multipartpkg "multipartpkg/main.go"
  "BOUNDARY123\\ntrue\\nname\\n\\nwasigo\\ntrue\\nupload\\nhello.txt\\nhello world\\nfalse\\n"
  "${CMAKE_CURRENT_SOURCE_DIR}/stdlib/mime/multipart/multipart.go")
wasigo_add_golden(constantpkg "constantpkg/main.go"
  "Int\\n10 true\\n2\\n9.5\\n-7\\ntrue\\nfalse\\nfoobar\\nfalse\\ntrue\\n-1\\n0\\n1\\nUnknown\\n"
  "${CMAKE_CURRENT_SOURCE_DIR}/stdlib/go/constant/constant.go"
  "${CMAKE_CURRENT_SOURCE_DIR}/stdlib/go/token/token.go")
wasigo_add_golden(aespkg "aespkg/main.go"
  "true\\n16\\n69c4e0d86a7b0430d8cdb78070b4c55a\\n00112233445566778899aabbccddeeff\\nfalse\\n"
  "${CMAKE_CURRENT_SOURCE_DIR}/stdlib/crypto/cipher/cipher.go"
  "${CMAKE_CURRENT_SOURCE_DIR}/stdlib/crypto/aes/aes.go"
  "${CMAKE_CURRENT_SOURCE_DIR}/stdlib/encoding/hex/hex.go")
wasigo_add_golden(despkg "despkg/main.go"
  "true\\n8\\n85e813540f0ab405\\n0123456789abcdef\\nfalse\\n8ca64de9c1b123a7\\n"
  "${CMAKE_CURRENT_SOURCE_DIR}/stdlib/crypto/cipher/cipher.go"
  "${CMAKE_CURRENT_SOURCE_DIR}/stdlib/crypto/des/des.go"
  "${CMAKE_CURRENT_SOURCE_DIR}/stdlib/encoding/hex/hex.go")
wasigo_add_golden(ascii85pkg "ascii85pkg/main.go"
  "9jqo^BlbD-BleB1DJ+*+F(f,q/0JhKF<GL>Cj@.4Gp\$d7F!,L7@<6@)\\nMan is distinguished, not only by his reason\\ntrue\\ntrue\\nzBOu!rDZ\\n"
  "${CMAKE_CURRENT_SOURCE_DIR}/stdlib/encoding/ascii85/ascii85.go")
wasigo_add_golden(textscannerpkg "textscannerpkg/main.go" ""
  "${CMAKE_CURRENT_SOURCE_DIR}/stdlib/text/scanner/scanner.go"
  "${CMAKE_CURRENT_SOURCE_DIR}/stdlib/strings/strings.go")
# qppkg's output contains literal \r bytes (CRLF line breaks the Writer
# emits) which would collide with EXPECTED_OUTPUT's own "\\n" substitution
# -- same fragility as jsonpkg -- see tests/golden/expected/qppkg.txt.
wasigo_add_golden(qppkg "qppkg/main.go" ""
  "${CMAKE_CURRENT_SOURCE_DIR}/stdlib/mime/quotedprintable/quotedprintable.go"
  "${CMAKE_CURRENT_SOURCE_DIR}/stdlib/bytes/bytes.go"
  "${CMAKE_CURRENT_SOURCE_DIR}/stdlib/io/io.go")
wasigo_add_golden(flatepkg "flatepkg/main.go"
  "true\\ntrue\\ntrue\\ntrue\\ntrue\\ntrue\\ntrue\\ntrue\\ntrue\\n"
  "${CMAKE_CURRENT_SOURCE_DIR}/stdlib/compress/flate/flate.go"
  "${CMAKE_CURRENT_SOURCE_DIR}/stdlib/bytes/bytes.go"
  "${CMAKE_CURRENT_SOURCE_DIR}/stdlib/io/io.go")
wasigo_add_golden(zlibpkg "zlibpkg/main.go"
  "true\\ntrue\\ntrue\\ntrue\\n"
  "${CMAKE_CURRENT_SOURCE_DIR}/stdlib/compress/zlib/zlib.go"
  "${CMAKE_CURRENT_SOURCE_DIR}/stdlib/compress/flate/flate.go"
  "${CMAKE_CURRENT_SOURCE_DIR}/stdlib/hash/adler32/adler32.go"
  "${CMAKE_CURRENT_SOURCE_DIR}/stdlib/bytes/bytes.go"
  "${CMAKE_CURRENT_SOURCE_DIR}/stdlib/io/io.go")
wasigo_add_golden(gzippkg "gzippkg/main.go"
  "true\\ntrue\\ntrue\\ntrue\\ntrue\\n"
  "${CMAKE_CURRENT_SOURCE_DIR}/stdlib/compress/gzip/gzip.go"
  "${CMAKE_CURRENT_SOURCE_DIR}/stdlib/compress/flate/flate.go"
  "${CMAKE_CURRENT_SOURCE_DIR}/stdlib/hash/crc32/crc32.go"
  "${CMAKE_CURRENT_SOURCE_DIR}/stdlib/bytes/bytes.go"
  "${CMAKE_CURRENT_SOURCE_DIR}/stdlib/io/io.go")
wasigo_add_golden(zippkg "zippkg/main.go"
  "true\\ntrue\\ntrue\\ntrue\\ntrue\\ntrue\\ntrue\\ntrue\\ntrue\\ntrue\\ntrue\\ntrue\\ntrue\\n"
  "${CMAKE_CURRENT_SOURCE_DIR}/stdlib/archive/zip/zip.go"
  "${CMAKE_CURRENT_SOURCE_DIR}/stdlib/compress/flate/flate.go"
  "${CMAKE_CURRENT_SOURCE_DIR}/stdlib/hash/crc32/crc32.go"
  "${CMAKE_CURRENT_SOURCE_DIR}/stdlib/bytes/bytes.go"
  "${CMAKE_CURRENT_SOURCE_DIR}/stdlib/io/io.go")
wasigo_add_golden(pngpkg "pngpkg/main.go"
  "true\\ntrue\\ntrue\\ntrue\\ntrue\\ntrue\\ntrue\\n"
  "${CMAKE_CURRENT_SOURCE_DIR}/stdlib/image/png/png.go"
  "${CMAKE_CURRENT_SOURCE_DIR}/stdlib/image/image.go"
  "${CMAKE_CURRENT_SOURCE_DIR}/stdlib/image/color/color.go"
  "${CMAKE_CURRENT_SOURCE_DIR}/stdlib/compress/zlib/zlib.go"
  "${CMAKE_CURRENT_SOURCE_DIR}/stdlib/compress/flate/flate.go"
  "${CMAKE_CURRENT_SOURCE_DIR}/stdlib/hash/crc32/crc32.go"
  "${CMAKE_CURRENT_SOURCE_DIR}/stdlib/hash/adler32/adler32.go"
  "${CMAKE_CURRENT_SOURCE_DIR}/stdlib/bytes/bytes.go"
  "${CMAKE_CURRENT_SOURCE_DIR}/stdlib/io/io.go")
wasigo_add_golden(palettepkg "palettepkg/main.go"
  "216\\n0\\n0\\n0\\n255\\n255\\n255\\n0\\n255\\n255\\n"
  "${CMAKE_CURRENT_SOURCE_DIR}/stdlib/image/color/palette/palette.go"
  "${CMAKE_CURRENT_SOURCE_DIR}/stdlib/image/color/color.go")
wasigo_add_golden(drawpkg "drawpkg/main.go"
  "true\\ntrue\\ntrue\\ntrue\\ntrue\\ntrue\\ntrue\\ntrue\\ntrue\\n"
  "${CMAKE_CURRENT_SOURCE_DIR}/stdlib/image/draw/draw.go"
  "${CMAKE_CURRENT_SOURCE_DIR}/stdlib/image/image.go"
  "${CMAKE_CURRENT_SOURCE_DIR}/stdlib/image/color/color.go")
wasigo_add_golden(gifpkg "gifpkg/main.go"
  "true true\\ntrue true\\ntrue true\\ntrue\\n"
  "${CMAKE_CURRENT_SOURCE_DIR}/stdlib/image/gif/gif.go"
  "${CMAKE_CURRENT_SOURCE_DIR}/stdlib/image/image.go"
  "${CMAKE_CURRENT_SOURCE_DIR}/stdlib/image/color/color.go"
  "${CMAKE_CURRENT_SOURCE_DIR}/stdlib/compress/lzw/lzw.go"
  "${CMAKE_CURRENT_SOURCE_DIR}/stdlib/bytes/bytes.go"
  "${CMAKE_CURRENT_SOURCE_DIR}/stdlib/io/io.go")
wasigo_add_golden(jpegpkg "jpegpkg/main.go"
  "true\\ntrue\\ntrue\\ntrue\\ntrue\\n"
  "${CMAKE_CURRENT_SOURCE_DIR}/stdlib/image/jpeg/jpeg.go"
  "${CMAKE_CURRENT_SOURCE_DIR}/stdlib/image/image.go"
  "${CMAKE_CURRENT_SOURCE_DIR}/stdlib/image/color/color.go"
  "${CMAKE_CURRENT_SOURCE_DIR}/stdlib/bytes/bytes.go"
  "${CMAKE_CURRENT_SOURCE_DIR}/stdlib/io/io.go")
wasigo_add_golden(sqlpkg "sqlpkg/main.go"
  "true\\ntrue\\n1\\ntrue\\ntrue\\ntrue\\ntrue\\n2 2 bob\\ntrue\\ntrue\\n1 alice\\ntrue\\ntrue\\n3\\ntrue\\ntrue\\ntrue\\ntrue\\n4\\ntrue\\ntrue\\ntrue\\ntrue\\ntrue\\ntrue\\ntrue\\n2\\n"
  "${CMAKE_CURRENT_SOURCE_DIR}/stdlib/database/sql/sql.go"
  "${CMAKE_CURRENT_SOURCE_DIR}/stdlib/database/sql/driver/driver.go"
  "${CMAKE_CURRENT_SOURCE_DIR}/stdlib/io/io.go")
wasigo_add_golden(xmlpkg "xmlpkg/main.go"
  "true\\ntrue\\ntrue\\n"
  "${CMAKE_CURRENT_SOURCE_DIR}/stdlib/encoding/xml/xml.go"
  "${CMAKE_CURRENT_SOURCE_DIR}/stdlib/strconv/strconv.go")
wasigo_add_golden(pepkg "pepkg/main.go"
  "true\\ntrue\\ntrue\\ntrue\\ntrue\\ntrue\\ntrue\\ntrue\\ntrue\\ntrue\\ntrue\\ntrue\\n"
  "${CMAKE_CURRENT_SOURCE_DIR}/stdlib/debug/pe/pe.go")
wasigo_add_golden(elfpkg "elfpkg/main.go"
  "true\\ntrue\\ntrue\\ntrue\\ntrue\\ntrue\\ntrue\\ntrue\\ntrue\\ntrue\\ntrue\\ntrue\\n"
  "${CMAKE_CURRENT_SOURCE_DIR}/stdlib/debug/elf/elf.go")
wasigo_add_golden(templatepkg "templatepkg/main.go"
  "true\\ntrue\\ntrue\\ntrue\\ntrue\\ntrue\\ntrue\\n"
  "${CMAKE_CURRENT_SOURCE_DIR}/stdlib/text/template/template.go"
  "${CMAKE_CURRENT_SOURCE_DIR}/stdlib/strings/strings.go"
  "${CMAKE_CURRENT_SOURCE_DIR}/stdlib/strconv/strconv.go")
wasigo_add_golden(testingpkg "testingpkg/main.go"
  "--- PASS: Add\\ntrue\\nintentional failure\\n--- FAIL: Fail\\nfalse\\n"
  "${CMAKE_CURRENT_SOURCE_DIR}/stdlib/testing/testing.go")
wasigo_add_golden(htmltemplatepkg "htmltemplatepkg/main.go"
  "true\\ntrue\\n"
  "${CMAKE_CURRENT_SOURCE_DIR}/stdlib/html/template/template.go"
  "${CMAKE_CURRENT_SOURCE_DIR}/stdlib/html/html.go"
  "${CMAKE_CURRENT_SOURCE_DIR}/stdlib/strings/strings.go"
  "${CMAKE_CURRENT_SOURCE_DIR}/stdlib/strconv/strconv.go")
wasigo_add_golden(iotestpkg "iotestpkg/main.go"
  "true\\ntrue\\ntrue\\ntrue\\ntrue\\ntrue\\ntrue\\n"
  "${CMAKE_CURRENT_SOURCE_DIR}/stdlib/testing/iotest/iotest.go"
  "${CMAKE_CURRENT_SOURCE_DIR}/stdlib/strings/strings.go"
  "${CMAKE_CURRENT_SOURCE_DIR}/stdlib/io/io.go")
wasigo_add_golden(fstestpkg "fstestpkg/main.go"
  "true\\ntrue\\n"
  "${CMAKE_CURRENT_SOURCE_DIR}/stdlib/testing/fstest/fstest.go"
  "${CMAKE_CURRENT_SOURCE_DIR}/stdlib/io/fs/fs.go"
  "${CMAKE_CURRENT_SOURCE_DIR}/stdlib/io/io.go"
  "${CMAKE_CURRENT_SOURCE_DIR}/stdlib/unicode/utf8/utf8.go")
wasigo_add_golden(machopkg "machopkg/main.go"
  "true\\ntrue\\ntrue\\ntrue\\ntrue\\ntrue\\ntrue\\ntrue\\ntrue\\ntrue\\ntrue\\n"
  "${CMAKE_CURRENT_SOURCE_DIR}/stdlib/debug/macho/macho.go")
wasigo_add_golden(fspkg "fspkg/main.go"
  "hello fs\\ntrue\\ntrue\\na.txt\\n8\\nfalse\\ntrue\\ntrue\\ntrue\\ntrue\\nfalse\\nfalse\\nfalse\\nfalse\\nfalse\\n"
  "${CMAKE_CURRENT_SOURCE_DIR}/stdlib/io/fs/fs.go"
  "${CMAKE_CURRENT_SOURCE_DIR}/stdlib/io/io.go"
  "${CMAKE_CURRENT_SOURCE_DIR}/stdlib/unicode/utf8/utf8.go")
wasigo_add_golden(hashpkg "hashpkg/main.go"
  "16\\n16\\n150536843\\n4\\n3398089811\\n4337451929027903488\\n16\\n"
  "${CMAKE_CURRENT_SOURCE_DIR}/stdlib/hash/hash.go"
  "${CMAKE_CURRENT_SOURCE_DIR}/stdlib/hash/adler32/adler32.go"
  "${CMAKE_CURRENT_SOURCE_DIR}/stdlib/hash/crc32/crc32.go"
  "${CMAKE_CURRENT_SOURCE_DIR}/stdlib/hash/crc64/crc64.go"
  "${CMAKE_CURRENT_SOURCE_DIR}/stdlib/crypto/md5/md5.go")
wasigo_add_golden(plan9objpkg "plan9objpkg/main.go"
  "true\\ntrue\\ntrue\\ntrue\\ntrue\\ntrue\\ntrue\\ntrue\\ntrue\\ntrue\\ntrue\\ntrue\\ntrue\\ntrue\\ntrue\\ntrue\\n"
  "${CMAKE_CURRENT_SOURCE_DIR}/stdlib/debug/plan9obj/plan9obj.go")
wasigo_add_golden(templateparsepkg "templateparsepkg/main.go"
  "true\\ntrue\\ntrue\\ntrue\\ntrue\\ntrue\\ntrue\\ntrue\\ntrue\\ntrue\\ntrue\\ntrue\\ntrue\\ntrue\\ntrue\\ntrue\\n"
  "${CMAKE_CURRENT_SOURCE_DIR}/stdlib/text/template/parse/parse.go"
  "${CMAKE_CURRENT_SOURCE_DIR}/stdlib/text/template/template.go"
  "${CMAKE_CURRENT_SOURCE_DIR}/stdlib/strings/strings.go"
  "${CMAKE_CURRENT_SOURCE_DIR}/stdlib/strconv/strconv.go")
wasigo_add_golden(runtimeobspkg "runtimeobspkg/main.go"
  "true\\ntrue\\ntrue\\ntrue\\ntrue\\ntrue\\ntrue\\ntrue\\ntrue\\ntrue\\ntrue\\ntrue\\ntrue\\n"
  "${CMAKE_CURRENT_SOURCE_DIR}/stdlib/runtime/metrics/metrics.go"
  "${CMAKE_CURRENT_SOURCE_DIR}/stdlib/runtime/pprof/pprof.go"
  "${CMAKE_CURRENT_SOURCE_DIR}/stdlib/runtime/trace/trace.go"
  "${CMAKE_CURRENT_SOURCE_DIR}/stdlib/runtime/coverage/coverage.go"
  "${CMAKE_CURRENT_SOURCE_DIR}/stdlib/bytes/bytes.go")
wasigo_add_golden(godocpkg "godocpkg/main.go"
  "true\\ntrue\\ntrue\\ntrue\\ntrue\\ntrue\\ntrue\\ntrue\\ntrue\\ntrue\\ntrue\\ntrue\\ntrue\\ntrue\\n"
  "${CMAKE_CURRENT_SOURCE_DIR}/stdlib/go/doc/doc.go"
  "${CMAKE_CURRENT_SOURCE_DIR}/stdlib/go/parser/parser.go"
  "${CMAKE_CURRENT_SOURCE_DIR}/stdlib/go/ast/ast.go"
  "${CMAKE_CURRENT_SOURCE_DIR}/stdlib/go/scanner/scanner.go"
  "${CMAKE_CURRENT_SOURCE_DIR}/stdlib/go/token/token.go")
wasigo_add_golden(buildinfopkg "buildinfopkg/main.go"
  "true\\ntrue\\ntrue\\ntrue\\ntrue\\ntrue\\n"
  "${CMAKE_CURRENT_SOURCE_DIR}/stdlib/debug/buildinfo/buildinfo.go"
  "${CMAKE_CURRENT_SOURCE_DIR}/stdlib/encoding/hex/hex.go"
  "${CMAKE_CURRENT_SOURCE_DIR}/stdlib/strings/strings.go")
wasigo_add_golden(rsapkg "rsapkg/main.go"
  "true\\ntrue\\ntrue\\ntrue\\n"
  "${CMAKE_CURRENT_SOURCE_DIR}/stdlib/crypto/rsa/rsa.go"
  "${CMAKE_CURRENT_SOURCE_DIR}/stdlib/math/big/big.go")
wasigo_add_golden(bzip2pkg "bzip2pkg/main.go"
  "true\\ntrue\\ntrue\\ntrue\\ntrue\\ntrue\\n"
  "${CMAKE_CURRENT_SOURCE_DIR}/stdlib/compress/bzip2/bzip2.go"
  "${CMAKE_CURRENT_SOURCE_DIR}/stdlib/encoding/hex/hex.go"
  "${CMAKE_CURRENT_SOURCE_DIR}/stdlib/bytes/bytes.go"
  "${CMAKE_CURRENT_SOURCE_DIR}/stdlib/io/io.go"
  "${CMAKE_CURRENT_SOURCE_DIR}/stdlib/strings/strings.go")
wasigo_add_golden(cryptopkg "cryptopkg/main.go"
  "true\\ntrue\\ntrue\\ntrue\\ntrue\\ntrue\\ntrue\\n"
  "${CMAKE_CURRENT_SOURCE_DIR}/stdlib/crypto/crypto.go")
wasigo_add_golden(sha3pkg "sha3pkg/main.go"
  "true\\ntrue\\ntrue\\ntrue\\ntrue\\n"
  "${CMAKE_CURRENT_SOURCE_DIR}/stdlib/crypto/sha3/sha3.go"
  "${CMAKE_CURRENT_SOURCE_DIR}/stdlib/encoding/hex/hex.go")
wasigo_add_golden(hkdfpkg "hkdfpkg/main.go"
  "true\\ntrue\\ntrue\\n"
  "${CMAKE_CURRENT_SOURCE_DIR}/stdlib/crypto/hkdf/hkdf.go"
  "${CMAKE_CURRENT_SOURCE_DIR}/stdlib/crypto/hmac/hmac.go"
  "${CMAKE_CURRENT_SOURCE_DIR}/stdlib/crypto/sha256/sha256.go"
  "${CMAKE_CURRENT_SOURCE_DIR}/stdlib/encoding/hex/hex.go")
wasigo_add_golden(pbkdf2pkg "pbkdf2pkg/main.go"
  "true\\ntrue\\n"
  "${CMAKE_CURRENT_SOURCE_DIR}/stdlib/crypto/pbkdf2/pbkdf2.go"
  "${CMAKE_CURRENT_SOURCE_DIR}/stdlib/crypto/hmac/hmac.go"
  "${CMAKE_CURRENT_SOURCE_DIR}/stdlib/crypto/sha256/sha256.go"
  "${CMAKE_CURRENT_SOURCE_DIR}/stdlib/encoding/hex/hex.go")
wasigo_add_golden(dsapkg "dsapkg/main.go"
  "true\\ntrue\\ntrue\\ntrue\\n"
  "${CMAKE_CURRENT_SOURCE_DIR}/stdlib/crypto/dsa/dsa.go"
  "${CMAKE_CURRENT_SOURCE_DIR}/stdlib/math/big/big.go")
wasigo_add_golden(ecpkg "ecpkg/main.go"
  "true\\ntrue\\ntrue\\ntrue\\ntrue\\ntrue\\ntrue\\ntrue\\n"
  "${CMAKE_CURRENT_SOURCE_DIR}/stdlib/crypto/elliptic/elliptic.go"
  "${CMAKE_CURRENT_SOURCE_DIR}/stdlib/crypto/ecdh/ecdh.go"
  "${CMAKE_CURRENT_SOURCE_DIR}/stdlib/crypto/ecdsa/ecdsa.go"
  "${CMAKE_CURRENT_SOURCE_DIR}/stdlib/math/big/big.go")
wasigo_add_golden(ed25519pkg "ed25519pkg/main.go"
  "true\\ntrue\\ntrue\\ntrue\\ntrue\\n"
  "${CMAKE_CURRENT_SOURCE_DIR}/stdlib/crypto/ed25519/ed25519.go"
  "${CMAKE_CURRENT_SOURCE_DIR}/stdlib/crypto/sha512/sha512.go"
  "${CMAKE_CURRENT_SOURCE_DIR}/stdlib/math/big/big.go")
wasigo_add_golden(randpkg "randpkg/main.go"
  "true\\ntrue\\ntrue\\ntrue\\n"
  "${CMAKE_CURRENT_SOURCE_DIR}/stdlib/crypto/rand/rand.go"
  "${CMAKE_CURRENT_SOURCE_DIR}/stdlib/time/time.go")
wasigo_add_golden(tlspkg "tlspkg/main.go"
  "true\\ntrue\\ntrue\\ntrue\\ntrue\\n"
  "${CMAKE_CURRENT_SOURCE_DIR}/stdlib/crypto/tls/tls.go")
wasigo_add_golden(x509pkg "x509pkg/main.go"
  "true\\ntrue\\ntrue\\ntrue\\ntrue\\n"
  "${CMAKE_CURRENT_SOURCE_DIR}/stdlib/crypto/x509/x509.go"
  "${CMAKE_CURRENT_SOURCE_DIR}/stdlib/crypto/x509/pkix/pkix.go"
  "${CMAKE_CURRENT_SOURCE_DIR}/stdlib/math/big/big.go")
wasigo_add_golden(dwarfpkg "dwarfpkg/main.go"
  "true\\ntrue\\ntrue\\ntrue\\ntrue\\ntrue\\ntrue\\ntrue\\ntrue\\n"
  "${CMAKE_CURRENT_SOURCE_DIR}/stdlib/debug/dwarf/dwarf.go"
  "${CMAKE_CURRENT_SOURCE_DIR}/stdlib/debug/gosym/gosym.go")
wasigo_add_golden(embedpkg "embedpkg/main.go"
  "true\\ntrue\\n"
  "${CMAKE_CURRENT_SOURCE_DIR}/stdlib/embed/embed.go"
  "${CMAKE_CURRENT_SOURCE_DIR}/stdlib/io/fs/fs.go")
wasigo_add_golden(asn1pkg "asn1pkg/main.go"
  "true\\ntrue\\ntrue\\ntrue\\ntrue\\ntrue\\ntrue\\ntrue\\ntrue\\ntrue\\n"
  "${CMAKE_CURRENT_SOURCE_DIR}/stdlib/encoding/asn1/asn1.go")
wasigo_add_golden(gobpkg "gobpkg/main.go"
  "true\\ntrue\\ntrue\\ntrue\\n"
  "${CMAKE_CURRENT_SOURCE_DIR}/stdlib/encoding/gob/gob.go")
wasigo_add_golden(gobuildpkg "gobuildpkg/main.go"
  "true\\ntrue\\ntrue\\ntrue\\ntrue\\ntrue\\ntrue\\ntrue\\n"
  "${CMAKE_CURRENT_SOURCE_DIR}/stdlib/go/build/build.go"
  "${CMAKE_CURRENT_SOURCE_DIR}/stdlib/go/importer/importer.go"
  "${CMAKE_CURRENT_SOURCE_DIR}/stdlib/go/parser/parser.go"
  "${CMAKE_CURRENT_SOURCE_DIR}/stdlib/go/ast/ast.go"
  "${CMAKE_CURRENT_SOURCE_DIR}/stdlib/go/scanner/scanner.go"
  "${CMAKE_CURRENT_SOURCE_DIR}/stdlib/go/token/token.go")
wasigo_add_golden(cmplxpkg "cmplxpkg/main.go"
  "true\\ntrue\\ntrue\\ntrue\\ntrue\\ntrue\\ntrue\\ntrue\\ntrue\\n"
  "${CMAKE_CURRENT_SOURCE_DIR}/stdlib/math/cmplx/cmplx.go"
  "${CMAKE_CURRENT_SOURCE_DIR}/stdlib/math/math.go")
wasigo_add_golden(quickpkg "quickpkg/main.go"
  "true\\ntrue\\ntrue\\ntrue\\n"
  "${CMAKE_CURRENT_SOURCE_DIR}/stdlib/testing/quick/quick.go"
  "${CMAKE_CURRENT_SOURCE_DIR}/stdlib/testing/slogtest/slogtest.go"
  "${CMAKE_CURRENT_SOURCE_DIR}/stdlib/log/slog/slog.go"
  "${CMAKE_CURRENT_SOURCE_DIR}/stdlib/bytes/bytes.go"
  "${CMAKE_CURRENT_SOURCE_DIR}/stdlib/math/rand/rand.go"
  "${CMAKE_CURRENT_SOURCE_DIR}/stdlib/strings/strings.go")
wasigo_add_golden(unsafepkg "unsafepkg/main.go"
  "true\\ntrue\\n"
  "${CMAKE_CURRENT_SOURCE_DIR}/stdlib/unsafe/unsafe.go")
wasigo_add_golden(structspkg "structspkg/main.go"
  "true\\ntrue\\n"
  "${CMAKE_CURRENT_SOURCE_DIR}/stdlib/structs/structs.go")
wasigo_add_golden(syscallpkg "syscallpkg/main.go"
  "true\\ntrue\\ntrue\\ntrue\\ntrue\\ntrue\\ntrue\\ntrue\\n"
  "${CMAKE_CURRENT_SOURCE_DIR}/stdlib/syscall/syscall.go")
