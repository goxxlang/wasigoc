#include "module_loader.h"

#include "lexer.h"
#include "parser.h"

#include <algorithm>
#include <cctype>
#include <sstream>
#include <string>
#include <unordered_set>
#include <utility>
#include <vector>

namespace wasigo {

namespace {

std::string DirName(const std::string& path) {
  size_t slash = path.find_last_of("/\\");
  if (slash == std::string::npos) return "";
  return path.substr(0, slash);
}

std::string JoinPath(const std::string& dir, const std::string& rel) {
  if (dir.empty()) return rel;
  char last = dir.back();
  if (last == '/' || last == '\\') return dir + rel;
  return dir + "/" + rel;
}

std::string NormalizePath(const std::string& path) {
  std::string unified = path;
  for (char& c : unified) {
    if (c == '\\') c = '/';
  }
  std::string prefix;
  size_t start = 0;
  if (unified.size() >= 3 && std::isalpha(static_cast<unsigned char>(unified[0])) &&
      unified[1] == ':' && unified[2] == '/') {
    prefix = unified.substr(0, 3);
    start = 3;
  } else if (!unified.empty() && unified[0] == '/') {
    prefix = "/";
    start = 1;
  }

  std::vector<std::string> stack;
  size_t i = start;
  while (i <= unified.size()) {
    size_t next = unified.find('/', i);
    std::string seg =
        unified.substr(i, next == std::string::npos ? std::string::npos : next - i);
    if (!seg.empty() && seg != ".") {
      if (seg == "..") {
        if (!stack.empty() && stack.back() != "..") {
          stack.pop_back();
        } else if (prefix.empty()) {
          stack.push_back("..");
        }
      } else {
        stack.push_back(seg);
      }
    }
    if (next == std::string::npos) break;
    i = next + 1;
  }

  std::string result = prefix;
  for (size_t k = 0; k < stack.size(); ++k) {
    if (k) result += "/";
    result += stack[k];
  }
  return result.empty() ? "." : result;
}

bool EndsWithGo(const std::string& p) {
  return p.size() >= 3 && p.compare(p.size() - 3, 3, ".go") == 0;
}

File ParseGo(const std::string& path, const std::string& content) {
  try {
    return Parse(Tokenize(content));
  } catch (const LexError& e) {
    throw LoadError("in '" + path + "': " + e.what());
  } catch (const ParseError& e) {
    throw LoadError("in '" + path + "': " + e.what());
  }
}

File MergePackageMove(std::vector<File>& parts, const std::vector<std::string>& paths) {
  if (parts.empty()) throw LoadError("empty package directory");
  File out = std::move(parts[0]);
  for (size_t i = 1; i < parts.size(); ++i) {
    File& f = parts[i];
    if (f.package_name != out.package_name) {
      throw LoadError("package name mismatch: '" + paths[0] + "' is package " +
                      out.package_name + " but '" + paths[i] + "' is package " +
                      f.package_name);
    }
    for (auto& s : f.structs) out.structs.push_back(std::move(s));
    for (auto& n : f.interfaces) out.interfaces.push_back(std::move(n));
    for (auto& a : f.aliases) out.aliases.push_back(std::move(a));
    for (auto& g : f.globals) out.globals.push_back(std::move(g));
    for (auto& fn : f.funcs) out.funcs.push_back(std::move(fn));
    for (auto& imp : f.imports) {
      bool seen = false;
      for (auto& e : out.imports) {
        if (e == imp) {
          seen = true;
          break;
        }
      }
      if (!seen) out.imports.push_back(imp);
    }
    for (auto& spec : f.import_specs) {
      bool seen = false;
      for (auto& e : out.import_specs) {
        if (e.path == spec.path && e.local == spec.local) {
          seen = true;
          break;
        }
      }
      if (!seen) out.import_specs.push_back(spec);
    }
  }
  return out;
}

struct GoMod {
  std::string module;  // `module example.com/app`
  // replace old => new; `new` starting with ./ or ../ is relative to module_root.
  std::vector<std::pair<std::string, std::string>> replace;
};

std::string TrimWs(const std::string& s) {
  size_t a = 0;
  while (a < s.size() && std::isspace(static_cast<unsigned char>(s[a]))) a++;
  size_t b = s.size();
  while (b > a && std::isspace(static_cast<unsigned char>(s[b - 1]))) b--;
  return s.substr(a, b - a);
}

std::string StripGoModComment(const std::string& line) {
  size_t c = line.find("//");
  if (c == std::string::npos) return line;
  return line.substr(0, c);
}

GoMod ParseGoMod(const std::string& content) {
  GoMod gm;
  bool in_replace_block = false;
  std::istringstream in(content);
  std::string raw;
  while (std::getline(in, raw)) {
    if (!raw.empty() && raw.back() == '\r') raw.pop_back();
    std::string line = TrimWs(StripGoModComment(raw));
    if (line.empty()) continue;
    if (in_replace_block) {
      if (line == ")") {
        in_replace_block = false;
        continue;
      }
      size_t arrow = line.find("=>");
      if (arrow == std::string::npos) continue;
      std::string from = TrimWs(line.substr(0, arrow));
      std::string to = TrimWs(line.substr(arrow + 2));
      if (!from.empty() && !to.empty()) gm.replace.push_back({from, to});
      continue;
    }
    if (line.rfind("module ", 0) == 0) {
      gm.module = TrimWs(line.substr(7));
      continue;
    }
    if (line == "replace (") {
      in_replace_block = true;
      continue;
    }
    if (line.rfind("replace ", 0) == 0) {
      std::string rest = TrimWs(line.substr(8));
      size_t arrow = rest.find("=>");
      if (arrow == std::string::npos) continue;
      std::string from = TrimWs(rest.substr(0, arrow));
      std::string to = TrimWs(rest.substr(arrow + 2));
      if (!from.empty() && !to.empty()) gm.replace.push_back({from, to});
    }
  }
  return gm;
}

std::string FindModuleRoot(const std::string& start_dir, const FileReader& read_file) {
  std::string dir = NormalizePath(start_dir);
  for (int i = 0; i < 64; ++i) {
    if (read_file(JoinPath(dir, "go.mod"))) return dir;
    std::string parent = DirName(dir);
    if (parent.empty() || parent == dir) break;
    dir = parent;
  }
  return "";
}

bool HasPrefixPath(const std::string& path, const std::string& prefix) {
  if (prefix.empty()) return false;
  if (path == prefix) return true;
  return path.size() > prefix.size() && path.compare(0, prefix.size(), prefix) == 0 &&
         path[prefix.size()] == '/';
}

// Parent directory of the `internal` segment, or empty if this is not an
// internal package. Go: a package under X/internal/... is only importable
// from X and X/....
std::string InternalParentDir(const std::string& resolved) {
  const std::string p = NormalizePath(resolved);
  const std::string mid = "/internal/";
  size_t pos = p.find(mid);
  if (pos != std::string::npos) return p.substr(0, pos);
  if (p.size() >= 9 && p.compare(p.size() - 9, 9, "/internal") == 0) {
    return p.substr(0, p.size() - 9);
  }
  if (p == "internal" || (p.size() >= 9 && p.compare(0, 9, "internal/") == 0)) return "";
  return "";
}

bool InternalImportAllowed(const std::string& importer_resolved, const std::string& imported_resolved) {
  std::string parent = InternalParentDir(imported_resolved);
  if (parent.empty()) return true;
  std::string imp = NormalizePath(importer_resolved);
  return HasPrefixPath(imp, parent);
}

struct LoadState {
  const std::vector<std::string>& import_dirs;
  const FileReader& read_file;
  const DirLister& list_dir;
  std::unordered_set<std::string> merged;
  std::vector<std::string> loading;
  std::string module_root;
  GoMod gomod;
};

std::string CycleChainMessage(const std::vector<std::string>& loading,
                              const std::string& closing_path) {
  std::string msg = "import cycle: ";
  for (const std::string& p : loading) msg += p + " -> ";
  msg += closing_path;
  return msg;
}

struct Resolved {
  std::string path;
  bool is_dir = false;
  std::string content;
  std::vector<std::string> go_files;
};

std::optional<Resolved> TryRoot(const std::string& root, const std::string& import_path,
                                LoadState& state) {
  if (auto content = state.read_file(root)) {
    Resolved r;
    r.path = root;
    r.content = std::move(*content);
    return r;
  }
  if (!EndsWithGo(root)) {
    std::string as_file = root + ".go";
    if (auto content = state.read_file(as_file)) {
      Resolved r;
      r.path = as_file;
      r.content = std::move(*content);
      return r;
    }
  }
  if (state.list_dir && !EndsWithGo(import_path)) {
    auto files = state.list_dir(root);
    if (!files.empty()) {
      std::sort(files.begin(), files.end());
      Resolved r;
      r.path = root;
      r.is_dir = true;
      r.go_files = std::move(files);
      return r;
    }
  }
  return std::nullopt;
}

// Apply go.mod replace: relative `./`/`../` targets become filesystem roots;
// other targets rewrite the import path (caller re-resolves).
std::string ApplyReplace(const GoMod& gm, const std::string& import_path, const std::string& module_root,
                         bool* filesystem_target) {
  *filesystem_target = false;
  for (auto& kv : gm.replace) {
    const std::string& from = kv.first;
    const std::string& to = kv.second;
    if (!HasPrefixPath(import_path, from) && import_path != from) continue;
    std::string suffix =
        import_path.size() == from.size() ? std::string() : import_path.substr(from.size() + 1);
    bool rel = !to.empty() && to[0] == '.';
    if (rel) {
      *filesystem_target = true;
      std::string dest = NormalizePath(JoinPath(module_root, to));
      if (!suffix.empty()) dest = NormalizePath(JoinPath(dest, suffix));
      return dest;
    }
    return suffix.empty() ? to : to + "/" + suffix;
  }
  return "";
}

std::optional<Resolved> ResolveImport(const std::string& import_path,
                                      const std::string& importer_dir,
                                      LoadState& state) {
  std::vector<std::string> roots;
  bool fs_replace = false;
  std::string replaced = ApplyReplace(state.gomod, import_path, state.module_root, &fs_replace);
  if (fs_replace && !replaced.empty()) {
    roots.push_back(replaced);
  }

  // Relative import (./ or ../): importer directory only, like Go.
  bool relative = import_path.size() >= 2 && import_path[0] == '.' &&
                  (import_path[1] == '/' || import_path[1] == '.');
  if (relative) {
    roots.push_back(NormalizePath(JoinPath(importer_dir, import_path)));
  } else {
    // WASMVoodooCompile search order: importer dir, then --import-dir.
    // Then go.mod module-prefix (Go modules): import "example.com/app/shape"
    // is <module_root>/shape.
    roots.push_back(NormalizePath(JoinPath(importer_dir, import_path)));
    if (!replaced.empty() && !fs_replace) {
      roots.push_back(NormalizePath(JoinPath(importer_dir, replaced)));
    }
    if (!state.module_root.empty() && !state.gomod.module.empty()) {
      const std::string& mod = state.gomod.module;
      auto add_mod = [&](const std::string& ip) {
        if (ip == mod) {
          roots.push_back(state.module_root);
        } else if (HasPrefixPath(ip, mod)) {
          roots.push_back(NormalizePath(JoinPath(state.module_root, ip.substr(mod.size() + 1))));
        }
      };
      add_mod(import_path);
      if (!replaced.empty() && !fs_replace) add_mod(replaced);
    }
    for (const std::string& dir : state.import_dirs) {
      roots.push_back(NormalizePath(JoinPath(dir, import_path)));
      if (!replaced.empty() && !fs_replace) {
        roots.push_back(NormalizePath(JoinPath(dir, replaced)));
      }
    }
  }

  for (const std::string& root : roots) {
    if (auto r = TryRoot(root, import_path, state)) return r;
  }
  return std::nullopt;
}

void LoadResolved(const Resolved& r, LoadState& state, LoadedGraph& graph) {
  if (state.merged.count(r.path)) return;
  for (const std::string& loading_path : state.loading) {
    if (loading_path == r.path) throw LoadError(CycleChainMessage(state.loading, r.path));
  }
  state.loading.push_back(r.path);

  File file;
  std::string importer_dir;
  if (r.is_dir) {
    std::vector<File> parts;
    std::vector<std::string> paths;
    for (const std::string& gp : r.go_files) {
      auto content = state.read_file(gp);
      if (!content) throw LoadError("cannot read '" + gp + "'");
      parts.push_back(ParseGo(gp, *content));
      paths.push_back(gp);
    }
    file = MergePackageMove(parts, paths);
    importer_dir = r.path;
  } else {
    file = ParseGo(r.path, r.content);
    importer_dir = DirName(r.path);
  }

  std::vector<std::string> direct;
  for (const std::string& imp : file.imports) {
    if (IsBuiltinImport(imp)) continue;
    auto resolved = ResolveImport(imp, importer_dir, state);
    if (!resolved) {
      throw LoadError("cannot resolve import \"" + imp + "\" from '" + r.path + "'");
    }
    if (!InternalImportAllowed(r.path, resolved->path)) {
      throw LoadError("use of internal package \"" + imp + "\" not allowed from '" + r.path + "'");
    }
    direct.push_back(resolved->path);
    LoadResolved(*resolved, state, graph);
  }

  graph.push_back(LoadedFile{r.path, std::move(file), std::move(direct)});
  state.loading.pop_back();
  state.merged.insert(r.path);
}

void LoadFileInto(const std::string& path, const std::string& content, LoadState& state,
                  LoadedGraph& graph) {
  Resolved r;
  r.path = path;
  r.content = content;
  LoadResolved(r, state, graph);
}

}  // namespace

bool IsBuiltinImport(const std::string& path) {
  return path == "fmt" || path == "errors" || path == "os" || path == "reflect";
}

LoadedGraph LoadModuleGraph(const std::string& entry_path,
                            const std::vector<std::string>& import_dirs,
                            const FileReader& read_file, const DirLister& list_dir) {
  std::string normalized_entry = NormalizePath(entry_path);
  std::optional<std::string> entry_content = read_file(normalized_entry);
  if (!entry_content) {
    throw LoadError("cannot open '" + entry_path + "' for reading");
  }
  LoadedGraph graph;
  LoadState state{import_dirs, read_file, list_dir, {}, {}, "", {}};
  std::string entry_dir = DirName(normalized_entry);
  state.module_root = FindModuleRoot(entry_dir.empty() ? "." : entry_dir, read_file);
  if (!state.module_root.empty()) {
    if (auto gm = read_file(JoinPath(state.module_root, "go.mod"))) {
      state.gomod = ParseGoMod(*gm);
    }
  }
  LoadFileInto(normalized_entry, *entry_content, state, graph);
  return graph;
}

}  // namespace wasigo
