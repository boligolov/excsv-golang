package excsv

import (
	"crypto/sha256"
	"encoding/hex"
	"fmt"
	"strings"
)

func computeDataChecksum(dataSection string, alg string) (string, error) {
	normalized := strings.ReplaceAll(dataSection, "\r\n", "\n")
	normalized = strings.ReplaceAll(normalized, "\r", "\n")
	switch alg {
	case "sha256":
		sum := sha256.Sum256([]byte(normalized))
		return hex.EncodeToString(sum[:]), nil
	default:
		return "", fmt.Errorf("unsupported checksum algorithm: %s", alg)
	}
}

func verifyChecksum(dataSection string, cs *Checksum) error {
	if cs == nil {
		return nil
	}
	got, err := computeDataChecksum(dataSection, cs.Algorithm)
	if err != nil {
		return fail(ErrHeaderInvalidValue, 0, err.Error())
	}
	if !strings.EqualFold(got, cs.Hex) {
		return fail(ErrChecksumMismatch, 0, "checksum mismatch")
	}
	return nil
}
