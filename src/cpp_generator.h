// AST (ast.h) -> a single, complete, compilable C++ translation unit.
//
// Unlike WASMBruja's brujac / WASMVoodooCompile's voodoomc -- both of which
// emit a header full of *signatures* for some larger hand-written program to
// implement and link against -- wasigoc emits a whole program: struct
// definitions with their methods inlined, free functions, and a real
// `int main()` translated from the Go source's `func main()`. The output is
// meant to be handed directly to wasi-sdk's clang++ (wasm32-wasip1) with no
// other generated-code contract to satisfy.
#pragma once

#include <stdexcept>
#include <string>
#include <vector>

#include "ast.h"

namespace wasigo {

class GenError : public std::runtime_error {
 public:
  explicit GenError(const std::string& msg) : std::runtime_error(msg) {}
};

// library: no func main; wrap decls in `namespace <package>`. imported_headers
// are this file's *direct* imports (WASMVoodooCompile v15 shape: never flatten
// the graph into one include list). The entry TU inlines the runtime first,
// then those headers; a library header #includes its own direct imports and
// is `#pragma once`. imported_files are the other graph nodes for name
// lookup, each in its own package namespace.
struct GenOptions {
  bool library = false;
  std::vector<std::string> imported_headers;
  std::vector<const File*> imported_files;
};

std::string GenerateCpp(const File& file, const GenOptions& opt = {});

}  // namespace wasigo
