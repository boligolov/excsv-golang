package excsv

import (
	"path/filepath"
	"strings"
)

func delimNameForPath(path string) string {
	switch strings.ToLower(filepath.Ext(path)) {
	case ".tsv":
		return "tab"
	case ".csv":
		return "comma"
	default:
		return ""
	}
}

func headerForDataPath(sidecarHeader Header, dataPath string) Header {
	h := sidecarHeader
	switch strings.ToLower(filepath.Ext(dataPath)) {
	case ".tsv":
		h.DelimName = "tab"
		h.Delim = '\t'
	case ".csv":
		if h.DelimName == "" {
			h.DelimName = "comma"
			h.Delim = ','
		}
	}
	return h
}
