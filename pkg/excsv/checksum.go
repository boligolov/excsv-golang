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

func (doc *Document) SerializeDataSection() string {
	var b strings.Builder
	d := doc.Header.Dialect()
	if doc.Data.HasHeaderRow {
		b.WriteString(joinCSVFields(doc.Data.HeaderRow, d))
		b.WriteByte('\n')
	}
	for _, row := range doc.Data.Rows {
		b.WriteString(joinCSVFields(row, d))
		b.WriteByte('\n')
	}
	return b.String()
}

func ComputeDataChecksum(dataSection string, algorithm string) (string, error) {
	return computeDataChecksum(dataSection, algorithm)
}

func (doc *Document) setChecksumFromSection(dataSection, algorithm string) error {
	hex, err := ComputeDataChecksum(dataSection, algorithm)
	if err != nil {
		return fail(ErrHeaderInvalidValue, 0, err.Error())
	}
	value := algorithm + ":" + hex
	doc.Header.Fields["checksum"] = value
	alg, digest, err := parseChecksumField(value)
	if err != nil {
		return fail(ErrHeaderInvalidValue, 0, "invalid checksum")
	}
	doc.Header.Checksum = &Checksum{Algorithm: alg, Hex: digest}
	return nil
}

func (doc *Document) SetDataChecksum(algorithm string) error {
	return doc.setChecksumFromSection(doc.SerializeDataSection(), algorithm)
}

func (doc *Document) SetDataChecksumFromSection(dataSection, algorithm string) error {
	return doc.setChecksumFromSection(dataSection, algorithm)
}
