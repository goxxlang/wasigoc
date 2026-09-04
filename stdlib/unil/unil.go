// Package unil is the "unil" bill-of-materials format used by
// WASMUniLoader's sandbox: files, runtime components, capabilities,
// canonical JSON, a SHA-256 digest, and detached or embedded Ed25519
// signatures.
//
// This is a Go++ port of WASMUniLoader/cpp/src/sbom.cc (the "unil"
// bomFormat) so a Go++ program can produce and verify byte-identical
// documents without linking the C++ core. Canonical() and Stringify()
// match that C++ writer field-for-field, including its indent quirks,
// so DigestHex() and signatures interop across both implementations.
//
// Private keys are 64 bytes everywhere in this package: a 32-byte
// Ed25519 seed followed by its 32-byte public key, the same layout
// WASMUniLoader's CLI and Go's own crypto/ed25519.PrivateKey use. Only
// the first 32 bytes (the seed) are fed to the signing primitive.
package unil

import (
	"crypto/ed25519"
	"crypto/rand"
	"crypto/sha256"
	"encoding/base64"
	"encoding/hex"
	"encoding/json"
	"errors"
	"strconv"
)

type File struct {
	Name, Mime, Kind, Crc32, Sha256, Origin string
	Size                                    int
}

type Component struct {
	Name, Role, Origin, Sha256, Engine string
	Size                                int
}

type Signature struct {
	Algorithm, PublicKey, Value string
}

type Document struct {
	BomFormat    string
	SpecVersion  string
	Name         string
	Scope        string
	Created      string
	Sandbox      string
	Files        []File
	Runtime      []Component
	Capabilities []string
	Signature    Signature
	HasSignature bool
}

type Detached struct {
	Algorithm, PublicKey, Sha256, Signature string
}

const hexDigits = "0123456789abcdef"

// --- canonical JSON writer (matches sbom.cc's esc/field/fieldInt) ---

func escString(b []byte, s string) []byte {
	b = append(b, '"')
	for i := 0; i < len(s); i++ {
		c := s[i]
		if c == '"' {
			b = append(b, '\\', '"')
		} else if c == '\\' {
			b = append(b, '\\', '\\')
		} else if c == '\n' {
			b = append(b, '\\', 'n')
		} else if c == '\r' {
			b = append(b, '\\', 'r')
		} else if c == '\t' {
			b = append(b, '\\', 't')
		} else if c < 32 {
			b = append(b, '\\', 'u', '0', '0')
			b = append(b, hexDigits[c>>4], hexDigits[c&15])
		} else {
			b = append(b, c)
		}
	}
	b = append(b, '"')
	return b
}

func appendComma(b []byte, first *bool) []byte {
	if !*first {
		b = append(b, ',')
	}
	*first = false
	return b
}

func appendField(b []byte, first *bool, key string, val string, omitEmpty bool) []byte {
	if omitEmpty && val == "" {
		return b
	}
	b = appendComma(b, first)
	b = escString(b, key)
	b = append(b, ':')
	b = escString(b, val)
	return b
}

func appendFieldInt(b []byte, first *bool, key string, val int, omitZero bool) []byte {
	if omitZero && val == 0 {
		return b
	}
	b = appendComma(b, first)
	b = escString(b, key)
	b = append(b, ':')
	b = append(b, []byte(strconv.Itoa(val))...)
	return b
}

func appendFile(b []byte, f File) []byte {
	b = append(b, '{')
	first := true
	b = appendField(b, &first, "name", f.Name, false)
	b = appendField(b, &first, "mime", f.Mime, false)
	b = appendField(b, &first, "kind", f.Kind, false)
	b = appendFieldInt(b, &first, "size", f.Size, true)
	b = appendField(b, &first, "crc32", f.Crc32, false)
	b = appendField(b, &first, "sha256", f.Sha256, false)
	b = appendField(b, &first, "origin", f.Origin, true)
	b = append(b, '}')
	return b
}

func appendComponent(b []byte, c Component) []byte {
	b = append(b, '{')
	first := true
	b = appendField(b, &first, "name", c.Name, false)
	b = appendField(b, &first, "role", c.Role, false)
	b = appendField(b, &first, "origin", c.Origin, false)
	b = appendField(b, &first, "sha256", c.Sha256, true)
	b = appendFieldInt(b, &first, "size", c.Size, true)
	b = appendField(b, &first, "engine", c.Engine, true)
	b = append(b, '}')
	return b
}

// Stringify renders d as JSON. With indent it matches sbom.cc's
// partial-indent style exactly (top-level scalars stay on one line;
// "files"/"runtime"/"capabilities"/"signature" keys and file/runtime
// array elements get their own line; capabilities stay inline) --
// deliberately, so output byte-matches the C++ writer.
func Stringify(d Document, indent bool) string {
	var b []byte
	b = append(b, '{')
	first := true
	b = appendField(b, &first, "bomFormat", d.BomFormat, false)
	b = appendField(b, &first, "specVersion", d.SpecVersion, false)
	b = appendField(b, &first, "name", d.Name, true)
	b = appendField(b, &first, "scope", d.Scope, true)
	b = appendField(b, &first, "created", d.Created, true)

	b = appendComma(b, &first)
	if indent {
		b = append(b, '\n', ' ', ' ')
	}
	b = append(b, "\"files\":["...)
	for i := 0; i < len(d.Files); i++ {
		if i > 0 {
			b = append(b, ',')
		}
		if indent {
			b = append(b, '\n', ' ', ' ', ' ', ' ')
		}
		b = appendFile(b, d.Files[i])
	}
	b = append(b, ']')

	b = appendComma(b, &first)
	if indent {
		b = append(b, '\n', ' ', ' ')
	}
	b = append(b, "\"runtime\":["...)
	for i := 0; i < len(d.Runtime); i++ {
		if i > 0 {
			b = append(b, ',')
		}
		if indent {
			b = append(b, '\n', ' ', ' ', ' ', ' ')
		}
		b = appendComponent(b, d.Runtime[i])
	}
	b = append(b, ']')

	b = appendComma(b, &first)
	if indent {
		b = append(b, '\n', ' ', ' ')
	}
	b = append(b, "\"capabilities\":["...)
	for i := 0; i < len(d.Capabilities); i++ {
		if i > 0 {
			b = append(b, ',')
		}
		b = escString(b, d.Capabilities[i])
	}
	b = append(b, ']')

	b = appendField(b, &first, "sandbox", d.Sandbox, true)

	if d.HasSignature {
		b = appendComma(b, &first)
		if indent {
			b = append(b, '\n', ' ', ' ')
		}
		b = append(b, "\"signature\":{"...)
		sf := true
		b = appendField(b, &sf, "algorithm", d.Signature.Algorithm, false)
		b = appendField(b, &sf, "publicKey", d.Signature.PublicKey, false)
		b = appendField(b, &sf, "value", d.Signature.Value, false)
		b = append(b, '}')
	}
	if indent {
		b = append(b, '\n')
	}
	b = append(b, '}')
	return string(b)
}

// Canonical is Stringify(d, false) with the signature stripped -- the
// exact bytes that are hashed (DigestHex) and signed (SignDocument).
func Canonical(d Document) string {
	c := d
	c.HasSignature = false
	c.Signature = Signature{}
	return Stringify(c, false)
}

func DigestHex(d Document) string {
	c := Canonical(d)
	sum := sha256.Sum([]byte(c))
	return hex.EncodeToString(sum)
}

// --- document builders (match sbom.cc) ---

func DefaultCapabilities() []string {
	return []string{"compileGML", "invokeEvent", "db", "wasmCall", "console", "url", "document", "display"}
}

// SandboxDocument describes the unil sandbox runtime given its two
// guest modules. There is no embedded-hash fallback here (unlike the
// C++ core's embeddedSandbox, which bakes in WASMUniLoader's own
// guikit.wasm/wkv.wasm hashes) -- callers always supply guikit and wkv.
func SandboxDocument(guikit File, wkv File) Document {
	d := Document{BomFormat: "unil", SpecVersion: "1"}
	d.Name = "unil-sandbox"
	d.Scope = "sandbox"
	d.Files = []File{guikit, wkv}
	d.Runtime = []Component{
		{Name: "guikit", Role: "language", Origin: "guikit/cpp", Sha256: guikit.Sha256, Size: guikit.Size},
		{Name: "wkv", Role: "db", Origin: "WASMKV", Sha256: wkv.Sha256, Size: wkv.Size},
		{Name: "wasmv16", Role: "console", Origin: "WASMv16", Engine: "quickjs-ng"},
		{Name: "unil_display", Role: "display", Origin: "WASMUniLoader/cpp/display", Engine: "wasmv16"},
		{Name: "sbom", Role: "sbom", Origin: "WASMUniLoader/cpp"},
	}
	d.Capabilities = DefaultCapabilities()
	return d
}

func BundleDocument(name string, files []File, sandbox Document) Document {
	d := Document{BomFormat: "unil", SpecVersion: "1"}
	d.Name = name
	d.Scope = "bundle"
	d.Files = files
	d.Runtime = sandbox.Runtime
	d.Capabilities = sandbox.Capabilities
	d.Sandbox = DigestHex(sandbox)
	return d
}

// --- parsing (encoding/json's generic map[string]any decode; its
// struct-Unmarshal path does not walk slice fields, so Document is
// assembled by hand from the generic tree, same shape as sbom.cc's J) ---

func strField(m map[string]any, k string) string {
	if v, ok := m[k]; ok {
		if s, ok := v.(string); ok {
			return s
		}
	}
	return ""
}

func intField(m map[string]any, k string) int {
	if v, ok := m[k]; ok {
		if f, ok := v.(float64); ok {
			return int(f)
		}
	}
	return 0
}

func fileFromMap(m map[string]any) File {
	return File{
		Name:   strField(m, "name"),
		Mime:   strField(m, "mime"),
		Kind:   strField(m, "kind"),
		Size:   intField(m, "size"),
		Crc32:  strField(m, "crc32"),
		Sha256: strField(m, "sha256"),
		Origin: strField(m, "origin"),
	}
}

func componentFromMap(m map[string]any) Component {
	return Component{
		Name:   strField(m, "name"),
		Role:   strField(m, "role"),
		Origin: strField(m, "origin"),
		Sha256: strField(m, "sha256"),
		Size:   intField(m, "size"),
		Engine: strField(m, "engine"),
	}
}

func documentFromMap(m map[string]any) Document {
	d := Document{}
	d.BomFormat = strField(m, "bomFormat")
	if d.BomFormat == "" {
		d.BomFormat = "unil"
	}
	d.SpecVersion = strField(m, "specVersion")
	if d.SpecVersion == "" {
		d.SpecVersion = "1"
	}
	d.Name = strField(m, "name")
	d.Scope = strField(m, "scope")
	d.Created = strField(m, "created")
	d.Sandbox = strField(m, "sandbox")
	if arr, ok := m["files"].([]any); ok {
		for _, el := range arr {
			if fm, ok := el.(map[string]any); ok {
				d.Files = append(d.Files, fileFromMap(fm))
			}
		}
	}
	if arr, ok := m["runtime"].([]any); ok {
		for _, el := range arr {
			if cm, ok := el.(map[string]any); ok {
				d.Runtime = append(d.Runtime, componentFromMap(cm))
			}
		}
	}
	if arr, ok := m["capabilities"].([]any); ok {
		for _, el := range arr {
			if s, ok := el.(string); ok {
				d.Capabilities = append(d.Capabilities, s)
			}
		}
	}
	if sm, ok := m["signature"].(map[string]any); ok {
		d.HasSignature = true
		d.Signature.Algorithm = strField(sm, "algorithm")
		d.Signature.PublicKey = strField(sm, "publicKey")
		d.Signature.Value = strField(sm, "value")
	}
	return d
}

func ParseDocument(data string) (Document, error) {
	var raw map[string]any
	if err := json.Unmarshal([]byte(data), &raw); err != nil {
		return Document{}, err
	}
	return documentFromMap(raw), nil
}

// --- signing (Ed25519, RFC 8032; priv is 32-byte seed || 32-byte public) ---

func GenerateKeypair() (pub []byte, priv []byte, err error) {
	seed := make([]byte, 32)
	if _, err := rand.Read(seed); err != nil {
		return nil, nil, err
	}
	pub = ed25519.PublicKey(seed)
	priv = append(append([]byte{}, seed...), pub...)
	return pub, priv, nil
}

func SignDocument(d *Document, priv []byte) error {
	if len(priv) != 64 {
		return errors.New("unil: private key must be 64 bytes (seed||public)")
	}
	c := Canonical(*d)
	sig := ed25519.Sign(priv[0:32], []byte(c))
	pub := priv[32:64]
	d.HasSignature = true
	d.Signature.Algorithm = "ed25519"
	d.Signature.PublicKey = base64.StdEncoding.EncodeToString(pub)
	d.Signature.Value = base64.StdEncoding.EncodeToString(sig)
	return nil
}

func VerifyDocument(d Document) error {
	if !d.HasSignature {
		return errors.New("unil: not signed")
	}
	if d.Signature.Algorithm != "ed25519" {
		return errors.New("unil: unsupported algorithm")
	}
	pub, err := base64.StdEncoding.DecodeString(d.Signature.PublicKey)
	if err != nil || len(pub) != 32 {
		return errors.New("unil: bad signature encoding")
	}
	sig, err := base64.StdEncoding.DecodeString(d.Signature.Value)
	if err != nil || len(sig) != 64 {
		return errors.New("unil: bad signature encoding")
	}
	c := Canonical(d)
	if !ed25519.Verify(pub, []byte(c), sig) {
		return errors.New("unil: signature mismatch")
	}
	return nil
}

func SignBytes(data []byte, priv []byte) (Detached, error) {
	if len(priv) != 64 {
		return Detached{}, errors.New("unil: private key must be 64 bytes (seed||public)")
	}
	sig := ed25519.Sign(priv[0:32], data)
	pub := priv[32:64]
	sum := sha256.Sum(data)
	return Detached{
		Algorithm: "ed25519",
		PublicKey: base64.StdEncoding.EncodeToString(pub),
		Sha256:    hex.EncodeToString(sum),
		Signature: base64.StdEncoding.EncodeToString(sig),
	}, nil
}

func VerifyBytes(data []byte, det Detached) error {
	if det.Algorithm != "ed25519" {
		return errors.New("unil: unsupported algorithm")
	}
	if det.Sha256 != "" {
		sum := hex.EncodeToString(sha256.Sum(data))
		if sum != det.Sha256 {
			return errors.New("unil: bundle sha256 mismatch")
		}
	}
	pub, err := base64.StdEncoding.DecodeString(det.PublicKey)
	if err != nil || len(pub) != 32 {
		return errors.New("unil: bad signature encoding")
	}
	sig, err := base64.StdEncoding.DecodeString(det.Signature)
	if err != nil || len(sig) != 64 {
		return errors.New("unil: bad signature encoding")
	}
	if !ed25519.Verify(pub, data, sig) {
		return errors.New("unil: signature mismatch")
	}
	return nil
}

// --- one JSON command in, JSON (or "ok") result out; mirrors sbom.cc's execute() ---

func guestFile(m map[string]any, name string, origin string) File {
	f := fileFromMap(m)
	if f.Name == "" {
		f.Name = name
	}
	if f.Mime == "" {
		f.Mime = "application/wasm"
	}
	if f.Kind == "" {
		f.Kind = "wasm"
	}
	if f.Origin == "" {
		f.Origin = origin
	}
	return f
}

func docFromOp(root map[string]any) (Document, error) {
	if s := strField(root, "doc"); s != "" {
		return ParseDocument(s)
	}
	dm, ok := root["doc"].(map[string]any)
	if !ok {
		return Document{}, errors.New("unil: missing doc")
	}
	return documentFromMap(dm), nil
}

func Execute(cmd string) (string, error) {
	var root map[string]any
	if err := json.Unmarshal([]byte(cmd), &root); err != nil {
		return "", errors.New("unil: command must be a JSON object")
	}
	op := strField(root, "op")

	if op == "sandbox" {
		g, gok := root["guikit"].(map[string]any)
		w, wok := root["wkv"].(map[string]any)
		if !gok || !wok {
			return "", errors.New("unil: sandbox needs guikit and wkv (no embedded fallback in this package)")
		}
		d := SandboxDocument(guestFile(g, "guikit.wasm", "guikit/cpp"), guestFile(w, "wkv.wasm", "WASMKV"))
		return Stringify(d, true), nil
	}

	if op == "bundle" {
		filesRaw, fok := root["files"].([]any)
		g, gok := root["guikit"].(map[string]any)
		w, wok := root["wkv"].(map[string]any)
		if !fok || !gok || !wok {
			return "", errors.New("unil: bundle needs files[], guikit, wkv")
		}
		var fs []File
		for _, el := range filesRaw {
			if m, ok := el.(map[string]any); ok {
				fs = append(fs, fileFromMap(m))
			}
		}
		sand := SandboxDocument(guestFile(g, "guikit.wasm", "guikit/cpp"), guestFile(w, "wkv.wasm", "WASMKV"))
		d := BundleDocument(strField(root, "name"), fs, sand)
		key := strField(root, "key")
		if key != "" {
			priv, err := base64.StdEncoding.DecodeString(key)
			if err != nil || len(priv) != 64 {
				return "", errors.New("unil: bad key")
			}
			if err := SignDocument(&d, priv); err != nil {
				return "", err
			}
		}
		return Stringify(d, true), nil
	}

	if op == "canonical" || op == "digest" || op == "sign" || op == "verify" {
		d, err := docFromOp(root)
		if err != nil {
			return "", err
		}
		if op == "canonical" {
			return Canonical(d), nil
		}
		if op == "digest" {
			return DigestHex(d), nil
		}
		if op == "sign" {
			priv, err := base64.StdEncoding.DecodeString(strField(root, "key"))
			if err != nil || len(priv) != 64 {
				return "", errors.New("unil: bad key")
			}
			if err := SignDocument(&d, priv); err != nil {
				return "", err
			}
			return Stringify(d, true), nil
		}
		if err := VerifyDocument(d); err != nil {
			return "", err
		}
		return "ok", nil
	}

	if op == "keygen" {
		pub, priv, err := GenerateKeypair()
		if err != nil {
			return "", err
		}
		return "{\"publicKey\":\"" + base64.StdEncoding.EncodeToString(pub) +
			"\",\"privateKey\":\"" + base64.StdEncoding.EncodeToString(priv) + "\"}", nil
	}

	if op == "signbytes" {
		data, err := base64.StdEncoding.DecodeString(strField(root, "data"))
		if err != nil {
			return "", errors.New("unil: bad data")
		}
		priv, err := base64.StdEncoding.DecodeString(strField(root, "key"))
		if err != nil || len(priv) != 64 {
			return "", errors.New("unil: bad key")
		}
		det, err := SignBytes(data, priv)
		if err != nil {
			return "", err
		}
		return "{\"algorithm\":\"ed25519\",\"publicKey\":\"" + det.PublicKey +
			"\",\"sha256\":\"" + det.Sha256 + "\",\"signature\":\"" + det.Signature + "\"}", nil
	}

	if op == "verifybytes" {
		data, err := base64.StdEncoding.DecodeString(strField(root, "data"))
		if err != nil {
			return "", errors.New("unil: bad data")
		}
		det := Detached{Algorithm: "ed25519"}
		det.PublicKey = strField(root, "pub")
		if det.PublicKey == "" {
			det.PublicKey = strField(root, "publicKey")
		}
		det.Signature = strField(root, "sig")
		if det.Signature == "" {
			det.Signature = strField(root, "signature")
		}
		det.Sha256 = strField(root, "sha256")
		if err := VerifyBytes(data, det); err != nil {
			return "", err
		}
		return "ok", nil
	}

	return "", errors.New("unil: unknown op: " + op)
}
