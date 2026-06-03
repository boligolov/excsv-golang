package excsv

import "bytes"

var utf8BOM = []byte{0xEF, 0xBB, 0xBF}

// stripUTF8BOM removes a leading UTF-8 BOM (U+FEFF) when present.
// Returns a sub-slice of data; does not allocate.
func stripUTF8BOM(data []byte) []byte {
	data, _ = bytes.CutPrefix(data, utf8BOM)
	return data
}
