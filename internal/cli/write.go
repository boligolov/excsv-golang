package cli

import (
	"os"

	"github.com/boligolov/excsv-golang/pkg/excsv"
	excsvzip "github.com/boligolov/excsv-golang/pkg/excsv/zip"
)

func loadDocForMutation(cfg *config, path string) (*excsv.Document, error) {
	if excsv.IsPackPath(path) {
		return loadTableDoc(cfg, path, true)
	}
	return loadDocOnly(cfg, path, isRowZipPath(path))
}

func loadPackScopedDoc(cfg *config, path string) (*excsv.Document, error) {
	if excsv.IsPackPath(path) {
		return loadDocOnly(cfg, path, true)
	}
	return loadDocForMutation(cfg, path)
}

func saveDocumentTo(cfg *config, doc *excsv.Document, srcPath, dest string) error {
	if dest == "" {
		dest = srcPath
	}
	if dest != srcPath && !isRowZipPath(dest) && !excsv.IsPackPath(dest) {
		serialized, err := doc.SerializeCanonical()
		if err != nil {
			return err
		}
		return os.WriteFile(dest, serialized, 0o644)
	}
	return saveDocument(cfg, doc, dest)
}

func saveDocument(cfg *config, doc *excsv.Document, userPath string) error {
	if cfg.pack != nil {
		if cfg.packTable != nil {
			cfg.packTable.SyncFromDocument(doc)
		}
		zipped, err := cfg.pack.Serialize()
		if err != nil {
			return err
		}
		return os.WriteFile(userPath, zipped, 0o644)
	}
	serialized, err := doc.SerializeCanonical()
	if err != nil {
		return err
	}
	opts := cfg.parseOpts()
	opts.PackRole = ""
	if _, err := excsv.ParseBytes(serialized, opts); err != nil {
		return err
	}
	if doc.Source.Profile == excsv.ProfileSidecar && doc.Source.ReferencePath != "" {
		if doc.Data.HasHeaderRow || len(doc.Data.Rows) > 0 {
			if err := os.WriteFile(doc.Source.ReferencePath, []byte(doc.SerializeDataSection()), 0o644); err != nil {
				return err
			}
		}
	}
	if isRowZipPath(userPath) {
		data, err := os.ReadFile(userPath)
		if err != nil {
			return err
		}
		ins, err := excsvzip.Inspect(userPath, data)
		if err != nil {
			return excsv.MapZipError(err)
		}
		password := cfg.zipPassword
		if ins.Encrypted && password == "" {
			return excsv.MapZipError(&excsvzip.ZipError{
				Kind: excsvzip.ErrPasswordRequired, Message: "encrypted zip requires --zip-password",
			})
		}
		ext, err := excsvzip.ExtractWithPassword(userPath, data, password)
		if err != nil {
			return excsv.MapZipError(err)
		}
		wrapPassword := ""
		if ins.Encrypted {
			wrapPassword = password
		}
		zipped, err := excsv.WrapZipWithPassword(serialized, ext.PrimaryName, ext.Comment, wrapPassword)
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
