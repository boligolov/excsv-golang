package excsv

import "fmt"

type ErrorKind string

const (
	ErrHeaderMissingVersion      ErrorKind = "header_missing_version"
	ErrHeaderMalformedKV         ErrorKind = "header_malformed_kv"
	ErrHeaderMalformedMagic      ErrorKind = "header_malformed_magic"
	ErrHeaderUnclosedQuote       ErrorKind = "header_unclosed_quote"
	ErrHeaderInvalidValue        ErrorKind = "header_invalid_value"
	ErrColumnMissingName         ErrorKind = "column_missing_name"
	ErrColumnMissingIndex        ErrorKind = "column_missing_index"
	ErrColumnTitleHeaderMismatch ErrorKind = "column_title_header_mismatch"
	ErrColumnNameHeaderMismatch  ErrorKind = "column_name_header_mismatch"
	ErrColumnMalformedAttribute  ErrorKind = "column_malformed_attribute"
	ErrAggArityMismatch          ErrorKind = "agg_arity_mismatch"
	ErrSQLMissingColon           ErrorKind = "sql_missing_colon"
	ErrSQLUnknownVerb            ErrorKind = "sql_unknown_verb"
	ErrSQLEmbeddedNewline        ErrorKind = "sql_embedded_newline"
	ErrDataRowArityMismatch      ErrorKind = "data_row_arity_mismatch"
	ErrQuoteNoneDelimiterInValue ErrorKind = "quote_none_delimiter_in_value"
	ErrFirstFieldHashUnquoted    ErrorKind = "first_field_hash_unquoted"
	ErrQuotedValueRawNewline     ErrorKind = "quoted_value_raw_newline"
	ErrChecksumMismatch          ErrorKind = "checksum_mismatch"
	ErrInvalidUTF8               ErrorKind = "invalid_utf8"
	ErrEncodingMismatch          ErrorKind = "encoding_mismatch"
	ErrZipMissingOriginalSize    ErrorKind = "zip_missing_original_size"
	ErrZipOriginalSizeMismatch   ErrorKind = "zip_original_size_mismatch"
	ErrZipPrimaryNotFirst        ErrorKind = "zip_primary_not_first"
	ErrZipPrimaryBadName         ErrorKind = "zip_primary_bad_name"
	ErrZipPrimaryMissing         ErrorKind = "zip_primary_missing"
	ErrZipCommentNotExcsvPrefix  ErrorKind = "zip_comment_not_excsv_prefix"
	ErrZipCommentNotUTF8         ErrorKind = "zip_comment_not_utf8"
	ErrZipUnsupportedCompression ErrorKind = "zip_unsupported_compression"
	ErrZipEncrypted              ErrorKind = "zip_encrypted"
)

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
