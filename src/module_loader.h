// Resolves a .go file's `import "path"` statements, transitively, the way
// WASMVoodooCompile's module_loader resolves `import "path.voodoom"`:
// each *package* keeps its own declarations and its `package` name (that
// becomes a C++ namespace). Files in one directory with the same package
// clause are one package (Go), merged into one graph node / one header.
// See README "Imports".
#pragma once

#include "ast.h"

#include <functional>
#include <optional>
#include <stdexcept>
#include <string>
#include <vector>

namespace wasigo {

class LoadError : public std::runtime_error {
 public:
  explicit LoadError(const std::string& msg) : std::runtime_error(msg) {}
};

using FileReader = std::function<std::optional<std::string>(const std::string& path)>;
// Full paths of *.go files in a directory (not *_test.go). Empty if `dir`
// is not a readable directory.
using DirLister = std::function<std::vector<std::string>(const std::string& dir)>;

struct LoadedFile {
  std::string resolved_path;  // file path, or directory path for a package dir
  File file;
  // Resolved paths of this package's own (non-builtin) imports -- WASMVoodooCompile's
  // "each header #includes only its direct imports", never the flattened graph.
  std::vector<std::string> direct_resolved;
};

using LoadedGraph = std::vector<LoadedFile>;

bool IsBuiltinImport(const std::string& path);

LoadedGraph LoadModuleGraph(const std::string& entry_path,
                            const std::vector<std::string>& import_dirs,
                            const FileReader& read_file,
                            const DirLister& list_dir = {});

}  // namespace wasigo
