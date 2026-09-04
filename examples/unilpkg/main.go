package main

import (
	"fmt"
	"unil"
)

func main() {
	guikit := unil.File{Name: "guikit.wasm", Mime: "application/wasm", Kind: "wasm",
		Crc32: "072aac36", Sha256: "4c16f6e3", Origin: "guikit/cpp", Size: 1050771}
	wkv := unil.File{Name: "wkv.wasm", Mime: "application/wasm", Kind: "wasm",
		Crc32: "2c72c0d2", Sha256: "dbe13ff5", Origin: "WASMKV", Size: 3463986}

	sandbox := unil.SandboxDocument(guikit, wkv)
	fmt.Println(sandbox.Name)
	fmt.Println(sandbox.Scope)
	fmt.Println(len(sandbox.Runtime) == 5)

	digest := unil.DigestHex(sandbox)
	fmt.Println(len(digest) == 64)

	bundle := unil.BundleDocument("demo-bundle", []unil.File{guikit}, sandbox)
	fmt.Println(bundle.Sandbox == digest)

	pub, priv, err := unil.GenerateKeypair()
	if err != nil {
		fmt.Println("keygen failed")
		return
	}
	fmt.Println(len(pub) == 32)
	fmt.Println(len(priv) == 64)

	if serr := unil.SignDocument(&bundle, priv); serr != nil {
		fmt.Println("sign failed")
		return
	}
	fmt.Println(bundle.HasSignature)

	if verr := unil.VerifyDocument(bundle); verr != nil {
		fmt.Println("verify failed")
	} else {
		fmt.Println("verify ok")
	}

	rendered := unil.Stringify(bundle, true)
	round, perr := unil.ParseDocument(rendered)
	if perr != nil {
		fmt.Println("parse failed")
		return
	}
	if rerr := unil.VerifyDocument(round); rerr != nil {
		fmt.Println("roundtrip verify failed")
	} else {
		fmt.Println("roundtrip verify ok")
	}
	fmt.Println(round.Name == bundle.Name)

	det, berr := unil.SignBytes([]byte("hello sbom"), priv)
	if berr != nil {
		fmt.Println("signbytes failed")
		return
	}
	if verr := unil.VerifyBytes([]byte("hello sbom"), det); verr != nil {
		fmt.Println("verifybytes failed")
	} else {
		fmt.Println("verifybytes ok")
	}

	if verr := unil.VerifyBytes([]byte("tampered"), det); verr == nil {
		fmt.Println("BUG: tampered data verified")
	} else {
		fmt.Println("tamper rejected")
	}

	cmdOut, cerr := unil.Execute("{\"op\":\"keygen\"}")
	if cerr != nil {
		fmt.Println("execute keygen failed")
	} else {
		fmt.Println(len(cmdOut) > 0)
	}
}
