// Extension-to-MIME-type lookup, bounded to TypeByExtension/
// AddExtensionType (no ParseMediaType/FormatMediaType, no reading
// /etc/mime.types -- no filesystem directory listing here at all).
// Seeded with real Go's own built-in default table (mime/type.go's
// builtinTypesLower), so this is a faithful subset, not invented values.
package mime

import "strings"

var types = map[string]string{
	".css":  "text/css; charset=utf-8",
	".gif":  "image/gif",
	".htm":  "text/html; charset=utf-8",
	".html": "text/html; charset=utf-8",
	".jpeg": "image/jpeg",
	".jpg":  "image/jpeg",
	".js":   "text/javascript; charset=utf-8",
	".json": "application/json",
	".mjs":  "text/javascript; charset=utf-8",
	".pdf":  "application/pdf",
	".png":  "image/png",
	".svg":  "image/svg+xml",
	".txt":  "text/plain; charset=utf-8",
	".wasm": "application/wasm",
	".webp": "image/webp",
	".xml":  "text/xml; charset=utf-8",
	".zip":  "application/zip",
}

// TypeByExtension returns the MIME type for ext (which should start with
// a leading dot, as in ".html"), or "" if unknown.
func TypeByExtension(ext string) string {
	v, ok := types[strings.ToLower(ext)]
	if !ok {
		return ""
	}
	return v
}

// AddExtensionType associates ext with the MIME type typ.
func AddExtensionType(ext string, typ string) error {
	types[strings.ToLower(ext)] = typ
	return nil
}
