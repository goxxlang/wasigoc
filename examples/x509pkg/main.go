package main

import (
	"crypto/x509"
	"crypto/x509/pkix"
	"fmt"
)

func main() {
	der := []byte{0x30, 0x05, 0x30, 0x03, 0x02, 0x01, 0x2a}
	c, err := x509.ParseCertificate(der)
	fmt.Println(err == nil)
	fmt.Println(c.SerialNumber.String() == "42")
	fmt.Println(len(c.Raw) == 7)
	n := pkix.Name{CommonName: "test"}
	fmt.Println(n.String() == "test")
	_, err2 := x509.ParseCertificate([]byte{0x00})
	fmt.Println(err2 != nil)
}
