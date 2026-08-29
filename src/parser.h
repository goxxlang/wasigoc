// Recursive-descent parser: token stream (see lexer.h) -> ast.h::File.
#pragma once

#include <stdexcept>
#include <vector>

#include "ast.h"
#include "lexer.h"

namespace wasigo {

class ParseError : public std::runtime_error {
 public:
  explicit ParseError(const std::string& msg) : std::runtime_error(msg) {}
};

File Parse(const std::vector<Token>& tokens);

}  // namespace wasigo
