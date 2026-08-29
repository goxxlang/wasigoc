// Bounded crypto/x509/pkix: Name is the distinguished-name struct
// ParseCertificate fills (CommonName only here). No RDNSequence
// marshaling, no ExtraNames. Name.String is the CommonName, not real
// Go's full RFC 2253 formatting.
package pkix

type Name struct {
	Country            []string
	Organization       []string
	OrganizationalUnit []string
	Locality           []string
	Province           []string
	StreetAddress      []string
	PostalCode         []string
	SerialNumber       string
	CommonName         string
}

func (n Name) String() string {
	return n.CommonName
}
