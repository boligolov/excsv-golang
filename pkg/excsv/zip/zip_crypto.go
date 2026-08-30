package excsvzip

import (
	"bytes"
	"encoding/binary"
	"fmt"
	"io"
	"strings"

	yekazip "github.com/yeka/zip"
)

// WrapWithPassword wraps inner bytes; non-empty password enables AES-256 entry encryption.
func WrapWithPassword(inner []byte, entryName, comment, password string) ([]byte, error) {
	patched, err := patchOriginalSize(inner)
	if err != nil {
		return nil, err
	}
	if password == "" {
		return wrapPlain(patched, entryName, comment)
	}
	return wrapEncrypted(patched, entryName, comment, password)
}

func wrapEncrypted(patched []byte, entryName, comment, password string) ([]byte, error) {
	var buf bytes.Buffer
	zw := yekazip.NewWriter(&buf)
	w, err := zw.Encrypt(entryName, password, yekazip.AES256Encryption)
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
	if err := zw.Close(); err != nil {
		return nil, err
	}
	return setArchiveComment(buf.Bytes(), comment)
}

func setArchiveComment(zipData []byte, comment string) ([]byte, error) {
	if len(comment) > 65535 {
		return nil, fmt.Errorf("zip comment too long")
	}
	sig := []byte{0x50, 0x4b, 0x05, 0x06}
	idx := bytes.LastIndex(zipData, sig)
	if idx < 0 {
		return nil, fmt.Errorf("zip EOCD not found")
	}
	commentLenOff := idx + 20
	if commentLenOff+2 > len(zipData) {
		return nil, fmt.Errorf("truncated zip EOCD")
	}
	existingLen := int(binary.LittleEndian.Uint16(zipData[commentLenOff:]))
	eocdEnd := commentLenOff + 2 + existingLen
	if eocdEnd != len(zipData) {
		return nil, fmt.Errorf("unexpected zip trailing data")
	}
	out := make([]byte, commentLenOff+2+len(comment))
	copy(out, zipData[:commentLenOff])
	binary.LittleEndian.PutUint16(out[commentLenOff:], uint16(len(comment)))
	copy(out[commentLenOff+2:], comment)
	return out, nil
}

// ReadEntries decompresses every non-directory entry. Encrypted archives require password.
func ReadEntries(zipData []byte, password string) (files map[string][]byte, order []string, firstName, comment string, err error) {
	zr, err := yekazip.NewReader(bytes.NewReader(zipData), int64(len(zipData)))
	if err != nil {
		return nil, nil, "", "", err
	}
	if len(zr.File) > 0 {
		firstName = strings.ReplaceAll(zr.File[0].Name, "\\", "/")
	}
	files = map[string][]byte{}
	for _, f := range zr.File {
		name := strings.ReplaceAll(f.Name, "\\", "/")
		if strings.HasSuffix(name, "/") {
			continue
		}
		if f.IsEncrypted() {
			if password == "" {
				return nil, nil, "", "", &ZipError{Kind: ErrEncrypted, Message: "encrypted pack requires a password"}
			}
			f.SetPassword(password)
		}
		rc, err := f.Open()
		if err != nil {
			if f.IsEncrypted() {
				return nil, nil, "", "", &ZipError{Kind: ErrWrongPassword, Message: "wrong password or corrupt entry"}
			}
			return nil, nil, "", "", err
		}
		body, err := io.ReadAll(rc)
		_ = rc.Close()
		if err != nil {
			if f.IsEncrypted() {
				return nil, nil, "", "", &ZipError{Kind: ErrWrongPassword, Message: "wrong password or corrupt entry"}
			}
			return nil, nil, "", "", err
		}
		files[name] = body
		order = append(order, name)
	}
	return files, order, firstName, zr.Comment, nil
}

func readEncryptedEntry(zipData []byte, entryName, password string) ([]byte, error) {
	if password == "" {
		return nil, &ZipError{Kind: ErrEncrypted, Message: "password required for encrypted entry"}
	}
	zr, err := yekazip.NewReader(bytes.NewReader(zipData), int64(len(zipData)))
	if err != nil {
		return nil, err
	}
	for _, f := range zr.File {
		if f.Name != entryName {
			continue
		}
		f.SetPassword(password)
		rc, err := f.Open()
		if err != nil {
			return nil, &ZipError{Kind: ErrWrongPassword, Message: "wrong password or corrupt entry"}
		}
		defer rc.Close()
		return io.ReadAll(rc)
	}
	return nil, &ZipError{Kind: ErrPrimaryMissing, Message: "encrypted entry not found"}
}

// ReWrap decrypts (when needed) and re-wraps with newPassword (empty removes encryption).
func ReWrap(archivePath string, data []byte, password, newPassword string) ([]byte, error) {
	ext, err := ExtractWithPassword(archivePath, data, password)
	if err != nil {
		return nil, err
	}
	return WrapWithPassword(ext.Inner, ext.PrimaryName, ext.Comment, newPassword)
}
