// Tiny subset of go/token: the Token kind, keyword lookup, and Pos.
// TokenString(t) instead of a Token.String() method -- Token is `type
// Token int` (not a struct), and this compiler only supports methods on
// struct receivers (see README's Rosetta-not-parity notes).
package token

type Token int

const (
	ILLEGAL Token = iota
	EOF
	COMMENT

	IDENT
	INT
	FLOAT
	CHAR
	STRING

	ADD
	SUB
	MUL
	QUO
	REM

	AND
	OR
	XOR
	SHL
	SHR
	AND_NOT

	ADD_ASSIGN
	SUB_ASSIGN
	MUL_ASSIGN
	QUO_ASSIGN
	REM_ASSIGN

	AND_ASSIGN
	OR_ASSIGN
	XOR_ASSIGN
	SHL_ASSIGN
	SHR_ASSIGN
	AND_NOT_ASSIGN

	LAND
	LOR
	ARROW
	INC
	DEC

	EQL
	LSS
	GTR
	ASSIGN
	NOT

	NEQ
	LEQ
	GEQ
	DEFINE
	ELLIPSIS

	LPAREN
	LBRACK
	LBRACE
	COMMA
	PERIOD

	RPAREN
	RBRACK
	RBRACE
	SEMICOLON
	COLON

	keywordBeg
	BREAK
	CASE
	CHAN
	CONST
	CONTINUE

	DEFAULT
	DEFER
	ELSE
	FALLTHROUGH
	FOR

	FUNC
	GO
	GOTO
	IF
	IMPORT

	INTERFACE
	MAP
	PACKAGE
	RANGE
	RETURN

	SELECT
	STRUCT
	SWITCH
	TYPE
	VAR
	keywordEnd
)

// A keyed slice literal (`[]string{ILLEGAL: "ILLEGAL", ...}`) isn't
// supported here -- composite literals for a slice only take positional
// elements, not indices -- so this builds the table with ordinary index
// assignment instead.
func buildTokenNames() []string {
	n := make([]string, int(keywordEnd)+1)
	n[ILLEGAL] = "ILLEGAL"
	n[EOF] = "EOF"
	n[COMMENT] = "COMMENT"
	n[IDENT] = "IDENT"
	n[INT] = "INT"
	n[FLOAT] = "FLOAT"
	n[CHAR] = "CHAR"
	n[STRING] = "STRING"
	n[ADD] = "+"
	n[SUB] = "-"
	n[MUL] = "*"
	n[QUO] = "/"
	n[REM] = "%"
	n[AND] = "&"
	n[OR] = "|"
	n[XOR] = "^"
	n[SHL] = "<<"
	n[SHR] = ">>"
	n[AND_NOT] = "&^"
	n[ADD_ASSIGN] = "+="
	n[SUB_ASSIGN] = "-="
	n[MUL_ASSIGN] = "*="
	n[QUO_ASSIGN] = "/="
	n[REM_ASSIGN] = "%="
	n[AND_ASSIGN] = "&="
	n[OR_ASSIGN] = "|="
	n[XOR_ASSIGN] = "^="
	n[SHL_ASSIGN] = "<<="
	n[SHR_ASSIGN] = ">>="
	n[AND_NOT_ASSIGN] = "&^="
	n[LAND] = "&&"
	n[LOR] = "||"
	n[ARROW] = "<-"
	n[INC] = "++"
	n[DEC] = "--"
	n[EQL] = "=="
	n[LSS] = "<"
	n[GTR] = ">"
	n[ASSIGN] = "="
	n[NOT] = "!"
	n[NEQ] = "!="
	n[LEQ] = "<="
	n[GEQ] = ">="
	n[DEFINE] = ":="
	n[ELLIPSIS] = "..."
	n[LPAREN] = "("
	n[LBRACK] = "["
	n[LBRACE] = "{"
	n[COMMA] = ","
	n[PERIOD] = "."
	n[RPAREN] = ")"
	n[RBRACK] = "]"
	n[RBRACE] = "}"
	n[SEMICOLON] = ";"
	n[COLON] = ":"
	n[BREAK] = "break"
	n[CASE] = "case"
	n[CHAN] = "chan"
	n[CONST] = "const"
	n[CONTINUE] = "continue"
	n[DEFAULT] = "default"
	n[DEFER] = "defer"
	n[ELSE] = "else"
	n[FALLTHROUGH] = "fallthrough"
	n[FOR] = "for"
	n[FUNC] = "func"
	n[GO] = "go"
	n[GOTO] = "goto"
	n[IF] = "if"
	n[IMPORT] = "import"
	n[INTERFACE] = "interface"
	n[MAP] = "map"
	n[PACKAGE] = "package"
	n[RANGE] = "range"
	n[RETURN] = "return"
	n[SELECT] = "select"
	n[STRUCT] = "struct"
	n[SWITCH] = "switch"
	n[TYPE] = "type"
	n[VAR] = "var"
	return n
}

var tokenNames = buildTokenNames()

func TokenString(t Token) string {
	if int(t) >= 0 && int(t) < len(tokenNames) && tokenNames[t] != "" {
		return tokenNames[t]
	}
	return "token(" + itoa(int(t)) + ")"
}

func itoa(n int) string {
	if n == 0 {
		return "0"
	}
	neg := n < 0
	if neg {
		n = -n
	}
	var digits []byte
	for n > 0 {
		digits = append(digits, byte(48+n%10))
		n = n / 10
	}
	var buf []byte
	for i := len(digits) - 1; i >= 0; i-- {
		buf = append(buf, digits[i])
	}
	if neg {
		return "-" + string(buf)
	}
	return string(buf)
}

var keywords map[string]Token

func initKeywords() map[string]Token {
	m := make(map[string]Token)
	for i := keywordBeg + 1; i < keywordEnd; i++ {
		m[tokenNames[i]] = i
	}
	return m
}

var keywordsInit = false

// Lookup maps an identifier to its keyword token, or IDENT if it isn't
// one.
func Lookup(ident string) Token {
	if !keywordsInit {
		keywords = initKeywords()
		keywordsInit = true
	}
	tok, ok := keywords[ident]
	if ok {
		return tok
	}
	return IDENT
}

// Token is `type Token int`, not a struct -- these are free functions,
// not methods, for the same reason TokenString is.
func IsLiteral(t Token) bool  { return t >= IDENT && t <= STRING }
func IsOperator(t Token) bool { return t >= ADD && t <= RBRACE }
func IsKeyword(t Token) bool  { return t > keywordBeg && t < keywordEnd }

type Pos int

const NoPos Pos = 0

func PosIsValid(p Pos) bool { return p != NoPos }
