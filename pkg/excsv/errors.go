package excsv

import "fmt"

type ErrorKind string

const (
	ErrHeaderMissingVersion      ErrorKind = "header_missing_version"
	ErrHeaderMalformedKV         ErrorKind = "header_malformed_kv"
	ErrHeaderMalformedMagic      ErrorKind = "header_malformed_magic"
	ErrHeaderUnclosedQuote       ErrorKind = "header_unclosed_quote"
	ErrHeaderInvalidValue        ErrorKind = "header_invalid_value"
	ErrUnknownVersion            ErrorKind = "unknown_version"
	ErrColumnMissingName         ErrorKind = "column_missing_name"
	ErrColumnMissingIndex        ErrorKind = "column_missing_index"
	ErrColumnTitleHeaderMismatch ErrorKind = "column_title_header_mismatch"
	ErrColumnNameHeaderMismatch  ErrorKind = "column_name_header_mismatch"
	ErrColumnMalformedAttribute  ErrorKind = "column_malformed_attribute"
	ErrColumnUnknownAttribute    ErrorKind = "column_unknown_attribute"
	ErrColumnValueInvalid        ErrorKind = "column_value_invalid"
	ErrColumnUnknownType         ErrorKind = "column_unknown_type"
	ErrColumnNotUnique           ErrorKind = "column_not_unique"
	ErrAggStale                  ErrorKind = "agg_stale"
	ErrUnknownMetaLine           ErrorKind = "unknown_meta_line"
	ErrDuplicateColumn           ErrorKind = "duplicate_column"
	ErrColumnCountMismatch       ErrorKind = "column_count_mismatch"
	ErrDefaultWithNulls          ErrorKind = "default_with_nulls"
	ErrAggArityMismatch          ErrorKind = "agg_arity_mismatch"
	ErrAggTypeIncompatible       ErrorKind = "agg_type_incompatible"
	ErrSQLMissingColon           ErrorKind = "sql_missing_colon"
	ErrSQLUnknownVerb            ErrorKind = "sql_unknown_verb"
	ErrSQLEmbeddedNewline        ErrorKind = "sql_embedded_newline"
	ErrSQLUnknownDialect         ErrorKind = "sql_unknown_dialect"
	ErrDataRowArityMismatch      ErrorKind = "data_row_arity_mismatch"
	ErrQuoteNoneDelimiterInValue ErrorKind = "quote_none_delimiter_in_value"
	ErrFirstFieldHashUnquoted    ErrorKind = "first_field_hash_unquoted"
	ErrQuotedValueRawNewline     ErrorKind = "quoted_value_raw_newline"
	ErrChecksumMismatch          ErrorKind = "checksum_mismatch"
	ErrChecksumUnknownAlgorithm  ErrorKind = "checksum_unknown_algorithm"
	ErrChecksumMalformed         ErrorKind = "checksum_malformed"
	ErrInvalidUTF8               ErrorKind = "invalid_utf8"
	ErrEncodingMismatch          ErrorKind = "encoding_mismatch"
	ErrEncodingNotASCIICompat    ErrorKind = "encoding_not_ascii_compatible"
	ErrEncodingUnsupported       ErrorKind = "encoding_unsupported"
	ErrZipMissingOriginalSize    ErrorKind = "zip_missing_original_size"
	ErrZipOriginalSizeMismatch   ErrorKind = "zip_original_size_mismatch"
	ErrZipPrimaryNotFirst        ErrorKind = "zip_primary_not_first"
	ErrZipPrimaryBadName         ErrorKind = "zip_primary_bad_name"
	ErrZipPrimaryMissing         ErrorKind = "zip_primary_missing"
	ErrZipCommentNotExcsvPrefix  ErrorKind = "zip_comment_not_excsv_prefix"
	ErrZipCommentNotUTF8         ErrorKind = "zip_comment_not_utf8"
	ErrZipCommentHeaderDisagree  ErrorKind = "zip_comment_header_disagree"
	ErrZipUnsupportedCompression ErrorKind = "zip_unsupported_compression"
	ErrZipEncrypted              ErrorKind = "zip_encrypted"
	ErrZipPasswordRequired       ErrorKind = "zip_password_required"
	ErrZipWrongPassword          ErrorKind = "zip_wrong_password"
	ErrRowParserGotPack          ErrorKind = "row_parser_got_pack"
	ErrPackKeyOnPlain            ErrorKind = "pack_key_on_plain"
	ErrPackManifestMissingLayout ErrorKind = "pack_manifest_missing_layout"
	ErrPackTableDirMissing       ErrorKind = "pack_table_dir_missing"
	ErrPackTableHeaderMissing    ErrorKind = "pack_table_header_missing"
	ErrPackColumnCountMismatch   ErrorKind = "pack_column_count_mismatch"
	ErrPackColLineCountMismatch  ErrorKind = "pack_col_line_count_mismatch"
	ErrPackSectionPartition      ErrorKind = "pack_section_partition_error"
	ErrPackSectionBoundary       ErrorKind = "pack_section_boundary_mismatch"
	ErrOriginalSizeOnPlain       ErrorKind = "original_size_on_plain"
	ErrRowsMismatch              ErrorKind = "rows_mismatch"
	ErrSidecarHasDataSection     ErrorKind = "sidecar_has_data_section"
	ErrSidecarMissingReference   ErrorKind = "sidecar_missing_reference"
	ErrSidecarReferenceNotFound  ErrorKind = "sidecar_reference_not_found"
	ErrSidecarReferenceEscapes   ErrorKind = "sidecar_reference_escapes_dir"
	ErrExtsvDelimMismatch        ErrorKind = "extsv_delim_mismatch"
	ErrSidecarChecksumMismatch   ErrorKind = "sidecar_checksum_mismatch"
	ErrReferenceOnInline         ErrorKind = "reference_on_inline"
)

// ErrSidecarDelimExtMismatch is the old name; fixtures use extsv_delim_mismatch.
const ErrSidecarDelimExtMismatch = ErrExtsvDelimMismatch

type Issue struct {
	Kind    ErrorKind `json:"kind"`
	Message string    `json:"message"`
	Line    int       `json:"line,omitempty"`
}

func (i Issue) Error() string {
	if i.Line > 0 {
		return fmt.Sprintf("%s at line %d: %s", i.Kind, i.Line, i.Message)
	}
	return fmt.Sprintf("%s: %s", i.Kind, i.Message)
}

type ParseError struct {
	Issue Issue
}

func (e *ParseError) Error() string { return e.Issue.Error() }

func newIssue(kind ErrorKind, line int, msg string) Issue {
	return Issue{Kind: kind, Message: msg, Line: line}
}

func fail(kind ErrorKind, line int, msg string) error {
	return &ParseError{Issue: newIssue(kind, line, msg)}
}
