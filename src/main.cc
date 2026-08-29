// wasigoc: the Go++ compiler. Go++ source -> C++ for wasm32-wasip1. Usage:
//
//   wasigoc input.go -o output.cpp
//           [--import-dir=DIR ...] [--out-dir=DIR] [--import-out=PATH=NAME ...]
//
// Imports follow WASMVoodooCompile: each imported package keeps its own
// `package` name as a C++ namespace and gets its own generated header
// (never flattened into the entry file's namespace). Each header #includes
// only its direct imports. A path may be a single .go file or a directory of
// .go files (one package). Path search: relative imports, go.mod replace,
// importer dir, go.mod module prefix, --import-dir, bundled stdlib. See README.
#include "cpp_generator.h"
#include "lexer.h"
#include "module_loader.h"
#include "parser.h"

#include <cstdio>
#include <filesystem>
#include <fstream>
#include <map>
#include <optional>
#include <sstream>
#include <string>
#include <system_error>
#include <vector>

#ifndef WASIGO_STDLIB_PATH
#define WASIGO_STDLIB_PATH ""
#endif

namespace {

std::optional<std::string> TryReadFile(const std::string& path) {
  std::ifstream in(path, std::ios::binary);
  if (!in.is_open()) return std::nullopt;
  std::ostringstream ss;
  ss << in.rdbuf();
  return ss.str();
}

std::vector<std::string> ListGoFiles(const std::string& dir) {
  std::vector<std::string> out;
  std::error_code ec;
  std::filesystem::path p(dir);
  if (!std::filesystem::is_directory(p, ec)) return out;
  for (const auto& entry : std::filesystem::directory_iterator(p, ec)) {
    if (ec) break;
    std::error_code fec;
    if (!entry.is_regular_file(fec)) continue;
    const std::string name = entry.path().filename().string();
    if (name.size() < 3 || name.compare(name.size() - 3, 3, ".go") != 0) continue;
    if (name.size() >= 8 && name.compare(name.size() - 8, 8, "_test.go") == 0) continue;
    out.push_back(entry.path().generic_string());
  }
  return out;
}

void WriteFile(const std::string& path, const std::string& content) {
  std::ofstream out(path, std::ios::binary);
  if (!out.is_open()) {
    throw std::runtime_error("cannot open '" + path + "' for writing");
  }
  out << content;
}

void PrintUsage(const char* argv0) {
  std::fprintf(stderr,
               "usage: %s <input.go> -o <output.cpp> [--import-dir=DIR] "
               "[--out-dir=DIR] [--import-out=PATH=NAME]\n",
               argv0);
}

std::string DirName(const std::string& path) {
  size_t slash = path.find_last_of("/\\");
  if (slash == std::string::npos) return "";
  return path.substr(0, slash);
}

std::string JoinDirFile(const std::string& dir, const std::string& filename) {
  if (dir.empty()) return filename;
  char last = dir.back();
  if (last == '/' || last == '\\') return dir + filename;
  return dir + "/" + filename;
}

// "examples/geom/point.go" -> "examples_point_gen.hpp"
// "examples/geom" (a package directory) -> "examples_geom_gen.hpp" (parent
// dir folded in -- see below)
//
// The immediate parent directory is folded into the filename, not just the
// last path segment, because two *different* packages can legitimately
// share a package-directory basename -- real Go's own stdlib does exactly
// this (go/scanner vs text/scanner, net/http/pprof vs runtime/pprof) -- and
// every imported package's generated header lands in the same shared
// --out-dir (e.g. this project's golden-test build, which funnels every
// example program's transitive imports into one shared generated/
// directory). Basename-only collided silently: no build error, just one
// header's content clobbering the other's on disk, whichever package
// happened to be generated last -- found when adding stdlib/text/scanner
// alongside the pre-existing stdlib/go/scanner, which broke go/scanner's
// own already-passing golden test (and everything downstream of it:
// go/parser, go/printer, go/format, go/types) purely from build-order
// luck, not from any change to their own source.
std::string DeriveGenFilename(const std::string& go_path) {
  std::string base = go_path;
  while (!base.empty() && (base.back() == '/' || base.back() == '\\')) base.pop_back();
  size_t slash = base.find_last_of("/\\");
  std::string parent;
  if (slash != std::string::npos) {
    parent = base.substr(0, slash);
    base = base.substr(slash + 1);
  }
  const std::string ext = ".go";
  if (base.size() > ext.size() &&
      base.compare(base.size() - ext.size(), ext.size(), ext) == 0) {
    base = base.substr(0, base.size() - ext.size());
  }
  size_t parent_slash = parent.find_last_of("/\\");
  std::string parent_base =
      parent_slash != std::string::npos ? parent.substr(parent_slash + 1) : parent;
  if (!parent_base.empty()) {
    return parent_base + "_" + base + "_gen.hpp";
  }
  return base + "_gen.hpp";
}

using ImportOutOverrides = std::map<std::string, std::string>;

std::string GenFilenameFor(const std::string& resolved_path,
                           const ImportOutOverrides& overrides) {
  auto it = overrides.find(resolved_path);
  if (it != overrides.end()) return it->second;
  return DeriveGenFilename(resolved_path);
}

}  // namespace

int main(int argc, char** argv) {
  std::string input_path;
  std::string output_path;
  std::string out_dir;
  std::vector<std::string> import_dirs;
  ImportOutOverrides import_outs;

  for (int i = 1; i < argc; ++i) {
    std::string arg = argv[i];
    if (arg == "-o" && i + 1 < argc) {
      output_path = argv[++i];
    } else if (arg.rfind("--import-dir=", 0) == 0) {
      import_dirs.push_back(arg.substr(13));
    } else if (arg == "--import-dir" && i + 1 < argc) {
      import_dirs.push_back(argv[++i]);
    } else if (arg.rfind("--out-dir=", 0) == 0) {
      out_dir = arg.substr(10);
    } else if (arg == "--out-dir" && i + 1 < argc) {
      out_dir = argv[++i];
    } else if (arg.rfind("--import-out=", 0) == 0) {
      std::string spec = arg.substr(13);
      size_t eq = spec.find('=');
      if (eq == std::string::npos) {
        std::fprintf(stderr, "--import-out needs PATH=NAME\n");
        return 2;
      }
      import_outs[spec.substr(0, eq)] = spec.substr(eq + 1);
    } else if (arg == "-h" || arg == "--help") {
      PrintUsage(argv[0]);
      return 0;
    } else if (!arg.empty() && arg[0] == '-') {
      std::fprintf(stderr, "unknown flag '%s'\n", arg.c_str());
      PrintUsage(argv[0]);
      return 2;
    } else if (input_path.empty()) {
      input_path = arg;
    } else {
      std::fprintf(stderr, "unexpected extra argument '%s'\n", arg.c_str());
      PrintUsage(argv[0]);
      return 2;
    }
  }

  if (input_path.empty() || output_path.empty()) {
    PrintUsage(argv[0]);
    return 2;
  }
  if (out_dir.empty()) out_dir = DirName(output_path);

  {
    std::string stdlib = WASIGO_STDLIB_PATH;
    if (!stdlib.empty()) {
      bool already = false;
      for (const std::string& d : import_dirs) {
        if (d == stdlib) {
          already = true;
          break;
        }
      }
      if (!already) import_dirs.insert(import_dirs.begin(), stdlib);
    }
  }

  try {
    wasigo::LoadedGraph graph =
        wasigo::LoadModuleGraph(input_path, import_dirs, TryReadFile, ListGoFiles);
    if (graph.empty()) {
      std::fprintf(stderr, "%s: empty module graph\n", input_path.c_str());
      return 1;
    }

    // Write imported files first (dependency order, entry last) as headers
    // in their own package namespace -- WASMVoodooCompile's per-file shape.
    // Each header #includes only its *direct* imports (voodoo v15); the
    // include graph nests, it is never flattened into the entry TU.
    std::map<std::string, std::string> path_to_hdr;
    for (size_t i = 0; i + 1 < graph.size(); ++i) {
      path_to_hdr[graph[i].resolved_path] =
          GenFilenameFor(graph[i].resolved_path, import_outs);
    }
    auto DirectHeaders = [&](const wasigo::LoadedFile& lf) {
      std::vector<std::string> hs;
      for (const std::string& dep : lf.direct_resolved) {
        auto it = path_to_hdr.find(dep);
        if (it != path_to_hdr.end()) hs.push_back(it->second);
      }
      return hs;
    };

    for (size_t i = 0; i + 1 < graph.size(); ++i) {
      if (graph[i].file.package_name == "main") {
        throw wasigo::LoadError("imported file '" + graph[i].resolved_path +
                                "' is package main; only the entry file may be");
      }
      wasigo::GenOptions lib;
      lib.library = true;
      lib.imported_headers = DirectHeaders(graph[i]);
      for (size_t j = 0; j + 1 < graph.size(); ++j) {
        if (j != i) lib.imported_files.push_back(&graph[j].file);
      }
      std::string hdr = GenerateCpp(graph[i].file, lib);
      WriteFile(JoinDirFile(out_dir, path_to_hdr[graph[i].resolved_path]), hdr);
    }

    wasigo::GenOptions opt;
    const wasigo::File& entry = graph.back().file;
    opt.library = entry.package_name != "main";
    opt.imported_headers = DirectHeaders(graph.back());
    for (size_t i = 0; i + 1 < graph.size(); ++i) {
      opt.imported_files.push_back(&graph[i].file);
    }
    std::string cpp = wasigo::GenerateCpp(entry, opt);
    WriteFile(output_path, cpp);
  } catch (const wasigo::LexError& e) {
    std::fprintf(stderr, "%s: lex error: %s\n", input_path.c_str(), e.what());
    return 1;
  } catch (const wasigo::ParseError& e) {
    std::fprintf(stderr, "%s: parse error: %s\n", input_path.c_str(), e.what());
    return 1;
  } catch (const wasigo::GenError& e) {
    std::fprintf(stderr, "%s: codegen error: %s\n", input_path.c_str(), e.what());
    return 1;
  } catch (const wasigo::LoadError& e) {
    std::fprintf(stderr, "%s: load error: %s\n", input_path.c_str(), e.what());
    return 1;
  } catch (const std::exception& e) {
    std::fprintf(stderr, "%s: %s\n", input_path.c_str(), e.what());
    return 1;
  }

  return 0;
}
