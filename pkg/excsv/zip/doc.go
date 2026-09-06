// Package excsvzip implements the ExCSV row-ZIP container
// (".excsv.zip"/".ecsv.zip"): a single Deflate-wrapped ExCSV entry whose
// schema and summary stats are mirrored into the ZIP archive comment, so a
// tool can preview the header without decompressing the entry. It also
// supports the optional standard-ZIP AES-256 entry password.
//
// [Inspect] peeks the comment and locates the primary entry without
// extracting it; [Extract] and [ExtractWithPassword] decompress the inner
// document; [Wrap] and [WrapWithPassword] build an archive from inner bytes,
// recomputing original-size= and the comment to match.
//
// This package is consumed by github.com/boligolov/excsv-golang/pkg/excsv
// and the excsv CLI; most callers should use that package's higher-level
// [excsv.ParseFile]/[excsv.Document] API instead of this one directly.
package excsvzip
