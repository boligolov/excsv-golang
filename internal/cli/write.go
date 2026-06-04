package cli

import (
	"os"

	"github.com/boligolov/excsv-golang/pkg/excsv"
	excsvzip "github.com/boligolov/excsv-golang/pkg/excsv/zip"
)

func loadDocForMutation(cfg *config, path string) (*excsv.Document, error) {
	return loadDocOnly(cfg, path, isRowZipPath(path))
}

func saveDocument(cfg *config, doc *excsv.Document, userPath string) error {
	serialized, err := doc.SerializeCanonical()
	if err != nil {
		return err
	}
	if _, err := excsv.ParseBytes(serialized, cfg.parseOpts()); err != nil {
		return err
	}
	if isRowZipPath(userPath) {
		data, err := os.ReadFile(userPath)
		if err != nil {
			return err
		}
		ext, err := excsvzip.Extract(userPath, data)
		if err != nil {
			return excsv.MapZipError(err)
		}
		zipped, err := excsv.WrapZip(serialized, ext.PrimaryName, "")
		if err != nil {
			return err
		}
		return os.WriteFile(userPath, zipped, 0o644)
	}
	writePath := doc.Source.Path
	if writePath == "" {
		writePath = userPath
	}
	return os.WriteFile(writePath, serialized, 0o644)
}
