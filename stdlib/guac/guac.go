// Package guac defines the on-disk shape of a distributable, compiled
// Go++ wasm package: a directory holding one or more wasm artifacts
// alongside a manifest -- a "unil" bill-of-materials document (package
// unil) naming those files, their SHA-256/CRC-32 hashes, and any other
// guac packages this one depends on.
//
// This package only describes and verifies that shape. It does not
// create directories (Go++'s os builtin has no Mkdir), compile
// anything, or fetch a dependency over the network -- a future guac
// CLI does that, wrapping wasigoc/goclang++ builds and this manifest.
// What's here: hash files already on disk into a manifest, write/read
// that manifest, and verify a directory's contents still match one.
//
// The manifest is an ordinary unil.Document -- no new JSON fields, so
// it stays byte-compatible with WASMUniLoader's C++ core and every
// unil.* function (Canonical, DigestHex, SignDocument, VerifyDocument)
// applies to it unchanged. Two fields carry guac-specific meaning by
// convention rather than schema:
//
//   - Scope is "package" (unil itself only ever writes "sandbox" or
//     "bundle", so this can't collide).
//   - Name is "<import path>@<version>", e.g. "unil@0.1.0" -- split on
//     the last "@" to recover each half (PackageName/PackageVersion).
//   - Capabilities carries exactly one entry: the build target triple
//     (e.g. "wasm32-wasip1"), reusing the existing string-list field
//     rather than adding one.
//   - Runtime lists dependencies as unil.Component with Role
//     "depends": Name is the dependency's import path, Origin is
//     where to fetch it, Sha256 pins the dependency's own manifest
//     digest (see Depend / unil.DigestHex) -- the same
//     name+hash-pinning shape unil.Component already uses for
//     WASMUniLoader's runtime components, just with a different Role.
package guac

import (
	"crypto/sha256"
	"encoding/hex"
	"errors"
	"hash/crc32"
	"os"
	"strings"
	"unil"
)

// ManifestName is the file every guac package directory carries
// alongside its wasm artifacts.
const ManifestName = "guac.json"

// Package is a wasm package's identity before it becomes a manifest.
type Package struct {
	Name    string // Go++ import path this package provides, e.g. "unil"
	Version string
	Target  string           // build target triple, e.g. "wasm32-wasip1"
	Depends []unil.Component // build with Depend; Role should be "depends"
}

// Depend describes one dependency for a Package: name is the
// dependency's import path, origin is where to fetch it (a path,
// URL, or registry reference -- guac doesn't interpret it), and
// manifestSha256 pins the dependency's own manifest to a specific
// build via unil.DigestHex(depManifest).
func Depend(name string, origin string, manifestSha256 string) unil.Component {
	return unil.Component{Name: name, Role: "depends", Origin: origin, Sha256: manifestSha256}
}

func hexCrc32(v uint32) string {
	const hexDigits = "0123456789abcdef"
	b := make([]byte, 8)
	i := 7
	for i >= 0 {
		b[i] = hexDigits[v&15]
		v = v >> 4
		i = i - 1
	}
	return string(b)
}

// HashFile reads filename out of dir and describes it as a unil.File:
// size, CRC-32, and SHA-256 of its exact bytes on disk right now.
// Origin is set to pkgName so the file traces back to the package
// that provides it, the same way unil.SandboxDocument stamps its own
// Files with an Origin.
func HashFile(dir string, filename string, pkgName string) (unil.File, error) {
	data, err := os.ReadFile(dir + "/" + filename)
	if err != nil {
		return unil.File{}, err
	}
	sum := sha256.Sum(data)
	return unil.File{
		Name:   filename,
		Mime:   "application/wasm",
		Kind:   "wasm",
		Size:   len(data),
		Crc32:  hexCrc32(crc32.ChecksumIEEE(data)),
		Sha256: hex.EncodeToString(sum),
		Origin: pkgName,
	}, nil
}

// BuildManifest hashes each of filenames (already compiled and sitting
// in dir) and assembles the package's manifest document. It does not
// write anything -- pass the result to WriteManifest, or sign it first
// with unil.SignDocument.
func BuildManifest(dir string, pkg Package, filenames []string) (unil.Document, error) {
	d := unil.Document{BomFormat: "unil", SpecVersion: "1"}
	d.Name = pkg.Name + "@" + pkg.Version
	d.Scope = "package"
	d.Capabilities = []string{pkg.Target}
	d.Runtime = pkg.Depends
	for _, filename := range filenames {
		f, err := HashFile(dir, filename, pkg.Name)
		if err != nil {
			return unil.Document{}, err
		}
		d.Files = append(d.Files, f)
	}
	return d, nil
}

// PackageName splits a manifest's "<import path>@<version>" Name back
// into its two halves.
func PackageName(d unil.Document) (name string, version string) {
	i := strings.LastIndex(d.Name, "@")
	if i < 0 {
		return d.Name, ""
	}
	return d.Name[0:i], d.Name[i+1:]
}

// Target returns a manifest's build target triple, the sole entry
// BuildManifest put in Capabilities.
func Target(d unil.Document) string {
	if len(d.Capabilities) == 0 {
		return ""
	}
	return d.Capabilities[0]
}

func WriteManifest(dir string, d unil.Document) error {
	return os.WriteFile(dir+"/"+ManifestName, []byte(unil.Stringify(d, true)), 0644)
}

func ReadManifest(dir string) (unil.Document, error) {
	data, err := os.ReadFile(dir + "/" + ManifestName)
	if err != nil {
		return unil.Document{}, err
	}
	return unil.ParseDocument(string(data))
}

// Verify re-hashes every file d.Files names, in dir, and confirms
// each still matches the manifest's recorded size/CRC-32/SHA-256 --
// the directory's contents haven't drifted from what was manifested.
// It does not check d's signature; call unil.VerifyDocument(d) for
// that (Verify and signature checking are independent: a directory
// can fail integrity with no signature present at all, and checking
// a signature never requires touching disk).
func Verify(dir string, d unil.Document) error {
	for _, want := range d.Files {
		got, err := HashFile(dir, want.Name, want.Origin)
		if err != nil {
			return err
		}
		if got.Size != want.Size {
			return errors.New("guac: " + want.Name + ": size mismatch")
		}
		if got.Sha256 != want.Sha256 {
			return errors.New("guac: " + want.Name + ": sha256 mismatch")
		}
		if want.Crc32 != "" && got.Crc32 != want.Crc32 {
			return errors.New("guac: " + want.Name + ": crc32 mismatch")
		}
	}
	return nil
}
