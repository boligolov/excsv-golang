package excsv

import (
	"os"
	"path/filepath"
	"strings"

	excsvzip "github.com/boligolov/excsv-golang/pkg/excsv/zip"
)

func ParseFile(path string, opts ParseOptions) (*ParseResult, error) {
	data, err := os.ReadFile(path)
	if err != nil {
		return nil, err
	}
	return ParsePath(path, data, opts)
}

func ParsePath(path string, data []byte, opts ParseOptions) (*ParseResult, error) {
	ext := strings.ToLower(filepath.Ext(path))
	base := strings.ToLower(strings.TrimSuffix(filepath.Base(path), ext))
	isRowZip := ext == ".zip" && (strings.HasSuffix(base, ".excsv") || strings.HasSuffix(base, ".ecsv"))
	isZipMagic := len(data) >= 4 && data[0] == 'P' && data[1] == 'K' && data[2] == 3 && data[3] == 4

	if strings.Contains(strings.ToLower(path), ".pack.") && (isRowZip || isZipMagic) {
		return nil, fail(ErrZipPrimaryMissing, 0, "pack format not supported")
	}

	if isRowZip || isZipMagic {
		return parseZipPath(path, data, opts)
	}

	res, err := ParseBytes(data, opts)
	if err != nil {
		return nil, err
	}
	if res.Doc != nil {
		res.Doc.Source.Path = path
	}
	return res, nil
}

func parseZipPath(path string, data []byte, opts ParseOptions) (*ParseResult, error) {
	ext, err := excsvzip.Extract(path, data)
	if err != nil {
		return nil, mapZipError(err)
	}
	opts.ExpectZipInner = true
	opts.ZipUncompressedSize = ext.UncompressedSize
	res, err := ParseBytes(ext.Inner, opts)
	if err != nil {
		return nil, err
	}
	if res.Doc != nil {
		res.Doc.Form = FormZipInner
		res.Doc.Source.Path = path
		res.Doc.Source.ZipPath = path
		res.Doc.Source.Comment = ext.Comment
		res.Doc.Source.PrimaryName = ext.PrimaryName
	}
	return res, nil
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
