package excsv

import (
	"os"
	"path/filepath"
	"strings"

	excsvzip "github.com/boligolov/excsv-golang/pkg/excsv/zip"
)

func ParseFile(path string, opts ParseOptions) (*ParseResult, error) {
	path = filepath.Clean(path)
	data, err := os.ReadFile(path)
	if err != nil {
		return nil, err
	}
	if sidecarPath, sideData, ok, err := discoverSidecarForData(path); err != nil {
		return nil, err
	} else if ok {
		return parseResolvedPath(sidecarPath, sideData, opts)
	}
	return ParsePath(path, data, opts)
}

func ParsePath(path string, data []byte, opts ParseOptions) (*ParseResult, error) {
	path = filepath.Clean(path)
	opts.SourcePath = path
	ext := strings.ToLower(filepath.Ext(path))
	base := strings.ToLower(strings.TrimSuffix(filepath.Base(path), ext))
	isRowZip := ext == ".zip" && (strings.HasSuffix(base, ".excsv") || strings.HasSuffix(base, ".ecsv") || strings.HasSuffix(base, ".extsv"))
	isZipMagic := len(data) >= 4 && data[0] == 'P' && data[1] == 'K' && data[2] == 3 && data[3] == 4

	if IsPackPath(path) {
		if isZipMagic {
			return parsePackPath(path, data, opts)
		}
		return nil, fail(ErrRowParserGotPack, 0, "pack container routed to row parser")
	}

	if isRowZip || isZipMagic {
		return parseZipPath(path, data, opts)
	}

	return parseResolvedPath(path, data, opts)
}

func parseResolvedPath(path string, data []byte, opts ParseOptions) (*ParseResult, error) {
	opts.SourcePath = path
	res, err := ParseBytes(data, opts)
	if err != nil {
		return nil, err
	}
	if res.Doc != nil {
		res.Doc.Source.Path = path
		if res.Doc.Source.SidecarPath == "" && headerReference(res.Doc.Header) != "" {
			res.Doc.Source.SidecarPath = path
		}
	}
	return res, nil
}

func parseZipPath(path string, data []byte, opts ParseOptions) (*ParseResult, error) {
	ins, err := excsvzip.Inspect(path, data)
	if err != nil {
		return nil, mapZipError(err)
	}
	if opts.ZipLoadData {
		return parseZipInner(path, data, ins, opts)
	}
	return parseZipComment(path, ins, opts)
}

func parseZipComment(path string, ins *excsvzip.InspectResult, opts ParseOptions) (*ParseResult, error) {
	if ins.Comment == "" {
		return nil, fail(ErrZipCommentNotExcsvPrefix, 0, "zip archive has no #!excsv comment for metadata-only read")
	}
	// Invalid UTF-8 in the comment is a warning, not a hard parse failure —
	// metadata-only reads never touch the inner entry bytes.
	if ins.CommentNotUTF8 {
		doc := &Document{
			Form:   FormZipInner,
			Header: Header{Fields: map[string]string{}, HasMagicLine: false},
		}
		if err := applyHeaderDefaults(&doc.Header); err != nil {
			return nil, err
		}
		res := &ParseResult{Doc: doc}
		applyZipSource(res.Doc, path, ins)
		applyZipCommentWarnings(res, ins)
		return res, nil
	}
	opts.ExpectZipInner = true
	opts.ZipUncompressedSize = ins.UncompressedSize
	res, err := ParseBytes([]byte(ins.Comment), opts)
	if err != nil {
		return nil, err
	}
	applyZipSource(res.Doc, path, ins)
	applyZipCommentWarnings(res, ins)
	return res, nil
}

func parseZipInner(path string, data []byte, ins *excsvzip.InspectResult, opts ParseOptions) (*ParseResult, error) {
	inner, err := excsvzip.ExtractPrimaryWithPassword(data, ins, opts.ZipPassword)
	if err != nil {
		return nil, mapZipError(err)
	}
	opts.ExpectZipInner = true
	opts.ZipUncompressedSize = ins.UncompressedSize
	res, err := ParseBytes(inner, opts)
	if err != nil {
		return nil, err
	}
	applyZipSource(res.Doc, path, ins)
	applyZipCommentWarnings(res, ins)
	if zipCommentHeaderDisagree(ins.Comment, res.Doc.Header) {
		res.warn(ErrZipCommentHeaderDisagree, 1, "ZIP comment disagrees with inner header")
	}
	return res, nil
}

func applyZipCommentWarnings(res *ParseResult, ins *excsvzip.InspectResult) {
	if ins == nil || res == nil {
		return
	}
	if ins.CommentNotUTF8 {
		res.warn(ErrZipCommentNotUTF8, 0, "invalid UTF-8 in ZIP comment")
	}
	if ins.CommentNotExcsvPrefix {
		res.warn(ErrZipCommentNotExcsvPrefix, 0, "ZIP comment does not start with #!excsv")
	}
}

func zipCommentHeaderDisagree(comment string, inner Header) bool {
	if comment == "" || !strings.HasPrefix(comment, "#!excsv") {
		return false
	}
	first := firstLineBytes([]byte(comment))
	cf, err := parseHeaderLine(first)
	if err != nil {
		return true
	}
	for k, v := range cf {
		if iv, ok := inner.Fields[k]; ok && iv != v {
			return true
		}
	}
	return false
}

func applyZipSource(doc *Document, path string, ins *excsvzip.InspectResult) {
	if doc == nil || ins == nil {
		return
	}
	doc.Form = FormZipInner
	doc.Source.Path = path
	doc.Source.ZipPath = path
	doc.Source.Comment = ins.Comment
	doc.Source.PrimaryName = ins.PrimaryName
}

func MapZipError(err error) error {
	return mapZipError(err)
}

func mapZipError(err error) error {
	if ze, ok := err.(*excsvzip.ZipError); ok {
		switch ze.Kind {
		case excsvzip.ErrPrimaryNotFirst:
			return fail(ErrZipPrimaryNotFirst, 0, ze.Message)
		case excsvzip.ErrEncrypted:
			return fail(ErrZipEncrypted, 0, ze.Message)
		case excsvzip.ErrPasswordRequired:
			return fail(ErrZipPasswordRequired, 0, ze.Message)
		case excsvzip.ErrWrongPassword:
			return fail(ErrZipWrongPassword, 0, ze.Message)
		case excsvzip.ErrUnsupportedCompression:
			return fail(ErrZipUnsupportedCompression, 0, ze.Message)
		case excsvzip.ErrCommentNotUTF8:
			return fail(ErrZipCommentNotUTF8, 0, ze.Message)
		case excsvzip.ErrCommentNotExcsvPrefix:
			return fail(ErrZipCommentNotExcsvPrefix, 0, ze.Message)
		case excsvzip.ErrPrimaryMissing:
			return fail(ErrZipPrimaryMissing, 0, ze.Message)
		case excsvzip.ErrPrimaryBadName:
			return fail(ErrZipPrimaryBadName, 0, ze.Message)
		}
	}
	return err
}

func WrapZip(inner []byte, entryName, comment string) ([]byte, error) {
	return excsvzip.Wrap(inner, entryName, comment)
}

func WrapZipWithPassword(inner []byte, entryName, comment, password string) ([]byte, error) {
	return excsvzip.WrapWithPassword(inner, entryName, comment, password)
}
