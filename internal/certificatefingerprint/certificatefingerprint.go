package certificatefingerprint

import (
	"crypto/sha256"
	"fmt"
	"strings"
)

func CertificateFingerprint(rawCertificate []byte) string {
	digest := sha256.Sum256(rawCertificate)

	var pieces []string

	for _, piece := range digest[:] {
		pieces = append(pieces, fmt.Sprintf("%02X", piece))
	}

	return strings.Join(pieces, " ")
}
