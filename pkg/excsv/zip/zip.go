package excsvzip

import (
	"archive/zip"
	"bytes"
	"compress/bzip2"
	"encoding/binary"
	"fmt"
	"io"
	"path/filepath"
	"strings"
	"time"
	"unicode/utf8"
)

type ErrorKind string

const (
	ErrPrimaryNotFirst        ErrorKind = "zip_primary_not_first"
	ErrEncrypted              ErrorKind = "zip_encrypted"
	ErrUnsupportedCompression ErrorKind = "zip_unsupported_compression"
	ErrCommentNotUTF8         ErrorKind = "zip_comment_not_utf8"
	ErrCommentNotExcsvPrefix  ErrorKind = "zip_comment_not_excsv_prefix"
	ErrPrimaryMissing         ErrorKind = "zip_primary_missing"
	ErrPrimaryBadName         ErrorKind = "zip_primary_bad_name"
)

type ZipError struct {
	Kind    ErrorKind
	Message string
}

func (e *ZipError) Error() string { return string(e.Kind) + ": " + e.Message }

type ExtractResult struct {
	Inner            []byte
	Comment          string
	PrimaryName      string
	UncompressedSize int64
	PrimaryIndex     int
}

var supportedMethods = map[uint16]bool{
	zip.Store:   true,
	zip.Deflate: true,
	12:          true, // bzip2
}

func Extract(archivePath string, data []byte) (*ExtractResult, error) {
	zr, err := zip.NewReader(bytes.NewReader(data), int64(len(data)))
	if err != nil {
		return nil, err
	}
	primary, idx, err := locatePrimary(archivePath, zr.File)
	if err != nil {
		return nil, err
	}
	if idx != 0 {
		return nil, &ZipError{Kind: ErrPrimaryNotFirst, Message: "primary entry is not first"}
	}
	if primary.Flags&0x1 != 0 {
		return nil, &ZipError{Kind: ErrEncrypted, Message: "encrypted entry"}
	}
	if !supportedMethods[primary.Method] {
		return nil, &ZipError{Kind: ErrUnsupportedCompression, Message: fmt.Sprintf("method %d", primary.Method)}
	}

	comment := zr.Comment
	if comment != "" {
		if !utf8.ValidString(comment) {
			return nil, &ZipError{Kind: ErrCommentNotUTF8, Message: "invalid UTF-8 in comment"}
		}
		if !strings.HasPrefix(comment, "#!excsv") {
			return nil, &ZipError{Kind: ErrCommentNotExcsvPrefix, Message: "comment must start with #!excsv"}
		}
	}

	inner, err := readEntry(data, primary)
	if err != nil {
		return nil, err
	}

	return &ExtractResult{
		Inner:            inner,
		Comment:          comment,
		PrimaryName:      primary.Name,
		UncompressedSize: int64(primary.UncompressedSize64),
		PrimaryIndex:     idx,
	}, nil
}

func readEntry(zipData []byte, f *zip.File) ([]byte, error) {
	if f.Method == zip.Store || f.Method == zip.Deflate {
		rc, err := f.Open()
		if err != nil {
			return nil, err
		}
		defer rc.Close()
		return io.ReadAll(rc)
	}
	if f.Method == 12 {
		comp, err := compressedData(zipData, f.Name)
		if err != nil {
			return nil, err
		}
		return io.ReadAll(bzip2.NewReader(bytes.NewReader(comp)))
	}
	return nil, fmt.Errorf("unsupported compression method %d", f.Method)
}

func compressedData(zipData []byte, name string) ([]byte, error) {
	off := 0
	for off+30 <= len(zipData) {
		if string(zipData[off:off+4]) != "PK\x03\x04" {
			break
		}
		nameLen := int(binary.LittleEndian.Uint16(zipData[off+26 : off+28]))
		extraLen := int(binary.LittleEndian.Uint16(zipData[off+28 : off+30]))
		entryName := string(zipData[off+30 : off+30+nameLen])
		compSize := binary.LittleEndian.Uint32(zipData[off+18 : off+22])
		dataStart := off + 30 + nameLen + extraLen
		dataEnd := dataStart + int(compSize)
		if dataEnd > len(zipData) {
			return nil, fmt.Errorf("truncated zip entry")
		}
		if entryName == name {
			return zipData[dataStart:dataEnd], nil
		}
		off = dataEnd
	}
	return nil, fmt.Errorf("zip entry %q not found", name)
}

func locatePrimary(archivePath string, files []*zip.File) (*zip.File, int, error) {
	if len(files) == 0 {
		return nil, -1, &ZipError{Kind: ErrPrimaryMissing, Message: "empty archive"}
	}
	base := strings.TrimSuffix(filepath.Base(archivePath), ".zip")
	base = strings.TrimSuffix(base, ".ZIP")
	want := base
	if !strings.HasSuffix(strings.ToLower(base), ".excsv") && !strings.HasSuffix(strings.ToLower(base), ".ecsv") {
		want = base + ".excsv"
	}

	var candidates []*zip.File
	var indices []int
	for i, f := range files {
		n := f.Name
		if strings.HasSuffix(strings.ToLower(n), ".excsv") || strings.HasSuffix(strings.ToLower(n), ".ecsv") {
			candidates = append(candidates, f)
			indices = append(indices, i)
		}
	}
	if len(candidates) == 0 {
		return nil, -1, &ZipError{Kind: ErrPrimaryMissing, Message: "no excsv entry"}
	}

	primary := candidates[0]
	pidx := indices[0]
	nameLower := strings.ToLower(filepath.Base(primary.Name))
	wantLower := strings.ToLower(filepath.Base(want))
	if nameLower == wantLower || nameLower == "data.excsv" || nameLower == "data.ecsv" {
		return primary, pidx, nil
	}
	return nil, -1, &ZipError{Kind: ErrPrimaryBadName, Message: primary.Name}
}

func Wrap(inner []byte, entryName string, comment string) ([]byte, error) {
	patched, err := patchOriginalSize(inner)
	if err != nil {
		return nil, err
	}
	var buf bytes.Buffer
	zw := zip.NewWriter(&buf)
	h := &zip.FileHeader{Name: entryName, Method: zip.Deflate}
	h.SetModTime(time.Date(2026, 1, 1, 0, 0, 0, 0, time.UTC))
	w, err := zw.CreateHeader(h)
	if err != nil {
		return nil, err
	}
	if _, err := w.Write(patched); err != nil {
		return nil, err
	}
	if comment == "" {
		comment = buildComment(string(patched))
	}
	if len(comment) > 65535 {
		comment = truncateComment(comment)
	}
	zw.SetComment(comment)
	if err := zw.Close(); err != nil {
		return nil, err
	}
	return buf.Bytes(), nil
}

func patchOriginalSize(inner []byte) ([]byte, error) {
	text := string(inner)
	trailingNL := strings.HasSuffix(text, "\n")
	text = strings.TrimSuffix(text, "\n")
	text = strings.TrimSuffix(text, "\r")
	lines := strings.Split(text, "\n")
	if len(lines) == 0 || !strings.HasPrefix(lines[0], "#!excsv") {
		return nil, fmt.Errorf("missing #!excsv header")
	}
	header := lines[0]
	tail := lines[1:]
	var n int64
	for i := 0; i < 3; i++ {
		h := stripOriginalSize(header)
		combined := h + " original-size=" + fmt.Sprintf("%d", n)
		body := combined
		if len(tail) > 0 {
			body = combined + "\n" + strings.Join(tail, "\n")
		}
		if trailingNL {
			body += "\n"
		}
		newN := int64(len([]byte(body)))
		if newN == n {
			return []byte(body), nil
		}
		n = newN
	}
	return nil, fmt.Errorf("original-size did not converge")
}

func stripOriginalSize(header string) string {
	rest := strings.TrimSpace(strings.TrimPrefix(header, "#!excsv"))
	if rest == "" {
		return "#!excsv"
	}
	var parts []string
	for _, p := range strings.Fields(rest) {
		if strings.HasPrefix(p, "original-size=") {
			continue
		}
		parts = append(parts, p)
	}
	if len(parts) == 0 {
		return "#!excsv"
	}
	return "#!excsv " + strings.Join(parts, " ")
}

func buildComment(inner string) string {
	var lines []string
	for _, line := range strings.Split(inner, "\n") {
		line = strings.TrimRight(line, "\r")
		if line == "" {
			continue
		}
		if strings.HasPrefix(line, "#") {
			lines = append(lines, line)
			continue
		}
		break
	}
	return strings.Join(lines, "\n")
}

func truncateComment(comment string) string {
	marker := "\n#@comment-truncated: 1"
	budget := 65535 - len(marker)
	cut := comment
	if len(cut) > budget {
		cut = cut[:budget]
		if i := strings.LastIndexByte(cut, '\n'); i >= 0 {
			cut = cut[:i]
		}
	}
	return cut + marker
}
