package main

import (
	"fmt"
	"guac"
	"os"
	"unil"
)

func main() {
	dir := "."
	os.WriteFile(dir+"/guacpkg_demo.wasm", []byte("\x00asm fake module bytes"), 0644)

	pkg := guac.Package{
		Name:    "guacpkg-demo",
		Version: "0.1.0",
		Target:  "wasm32-wasip1",
		Depends: []unil.Component{guac.Depend("unil", "stdlib/unil", "deadbeef")},
	}

	manifest, err := guac.BuildManifest(dir, pkg, []string{"guacpkg_demo.wasm"})
	if err != nil {
		fmt.Println("build failed")
		return
	}
	fmt.Println(manifest.Scope)
	name, version := guac.PackageName(manifest)
	fmt.Println(name)
	fmt.Println(version)
	fmt.Println(guac.Target(manifest))
	fmt.Println(len(manifest.Files) == 1)
	fmt.Println(len(manifest.Runtime) == 1)

	if werr := guac.WriteManifest(dir, manifest); werr != nil {
		fmt.Println("write manifest failed")
		return
	}

	read, rerr := guac.ReadManifest(dir)
	if rerr != nil {
		fmt.Println("read manifest failed")
		return
	}
	fmt.Println(read.Name == manifest.Name)

	if verr := guac.Verify(dir, read); verr != nil {
		fmt.Println("verify failed")
	} else {
		fmt.Println("verify ok")
	}

	// Tamper the artifact and confirm Verify catches it.
	os.WriteFile(dir+"/guacpkg_demo.wasm", []byte("tampered bytes"), 0644)
	if verr := guac.Verify(dir, read); verr == nil {
		fmt.Println("BUG: tampered file verified")
	} else {
		fmt.Println("tamper rejected")
	}

	// Restore, then prove unil's signing composes with a guac manifest
	// unchanged -- it's an ordinary unil.Document.
	os.WriteFile(dir+"/guacpkg_demo.wasm", []byte("\x00asm fake module bytes"), 0644)
	_, priv, kerr := unil.GenerateKeypair()
	if kerr != nil {
		fmt.Println("keygen failed")
		return
	}
	if serr := unil.SignDocument(&manifest, priv); serr != nil {
		fmt.Println("sign failed")
		return
	}
	if verr := unil.VerifyDocument(manifest); verr != nil {
		fmt.Println("signature verify failed")
	} else {
		fmt.Println("signature verify ok")
	}
}
