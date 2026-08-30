package excsv

import (
	"archive/zip"
	"bytes"
	"fmt"
	"strconv"
	"strings"
	"time"
)

func PackFromDocument(doc *Document, tableName string) *Pack {
	if tableName == "" {
		tableName = "table"
	}
	pt := documentToPackTable(doc, tableName)
	man := &Document{
		Form: FormPack,
		Header: Header{
			Fields: map[string]string{
				"version":      CurrentVersion,
				"layout":       "pack",
				"table-count":  "1",
				"single-table": tableName,
			},
			HasMagicLine: true,
			Version:      CurrentVersion,
		},
		Meta: MetaBlock{
			Tables: []TableDecl{pt.Decl},
		},
	}
	_ = applyHeaderDefaults(&man.Header)
	return &Pack{Manifest: man, Tables: []PackTable{pt}, FKs: nil}
}

func documentToPackTable(doc *Document, name string) PackTable {
	names := columnNamesFromHeader(doc)
	if len(names) == 0 && doc.Data.HasHeaderRow {
		names = append([]string{}, doc.Data.HeaderRow...)
	}
	width := len(names)
	if width == 0 && len(doc.Data.Rows) > 0 {
		width = len(doc.Data.Rows[0])
		names = make([]string, width)
		for i := range names {
			names[i] = fmt.Sprintf("col%d", i)
		}
	}
	cols := make([][]string, width)
	for _, row := range doc.Data.Rows {
		for c := 0; c < width; c++ {
			v := ""
			if c < len(row) {
				v = row[c]
			}
			cols[c] = append(cols[c], v)
		}
	}
	header := cloneDocumentMeta(doc)
	header.Form = FormPack
	header.Header.Fields["layout"] = "columnar"
	header.Header.Fields["rows"] = strconv.Itoa(len(doc.Data.Rows))
	n := len(doc.Data.Rows)
	header.Header.Rows = &n
	if header.Meta.Columns == nil && len(names) > 0 {
		for i, n := range names {
			header.Meta.Columns = append(header.Meta.Columns, ColumnDef{
				Attrs: map[string]string{"name": n, "index": strconv.Itoa(i)},
			})
		}
	}
	dir := name + "/"
	payload := 0
	for _, col := range cols {
		payload += len(colPayload(col))
	}
	pt := PackTable{
		Decl: TableDecl{
			Name:         name,
			Dir:          dir,
			Columns:      width,
			OriginalSize: int64(payload),
		},
		Header:    header,
		ColValues: cols,
		ColNames:  names,
	}
	materializePackTable(&pt)
	return pt
}

func cloneDocumentMeta(doc *Document) *Document {
	out := &Document{
		Form:   FormPack,
		Header: doc.Header,
		Meta:   doc.Meta,
		Source: doc.Source,
	}
	fields := map[string]string{}
	for k, v := range doc.Header.Fields {
		if k == "reference" || k == "checksum" || k == "original-size" {
			continue
		}
		fields[k] = v
	}
	out.Header.Fields = fields
	out.Header.Checksum = nil
	out.Header.OriginalSize = nil
	return out
}

func (p *Pack) AddTable(doc *Document, name string) {
	if p.Manifest != nil && len(p.Tables) >= 1 {
		delete(p.Manifest.Header.Fields, "single-table")
	}
	pt := documentToPackTable(doc, name)
	p.Tables = append(p.Tables, pt)
	p.syncManifest()
}

func (p *Pack) DropTable(name string) error {
	idx := -1
	for i := range p.Tables {
		if p.Tables[i].Decl.Name == name {
			idx = i
			break
		}
	}
	if idx < 0 {
		return fmt.Errorf("unknown table: %s", name)
	}
	p.Tables = append(p.Tables[:idx], p.Tables[idx+1:]...)
	var fks []ForeignKey
	for _, fk := range p.FKs {
		if strings.HasPrefix(fk.From, name+".") || strings.HasPrefix(fk.To, name+".") {
			continue
		}
		fks = append(fks, fk)
	}
	p.FKs = fks
	p.syncManifest()
	return nil
}

func (p *Pack) syncManifest() {
	if p.Manifest == nil {
		p.Manifest = &Document{Form: FormPack, Header: Header{Fields: map[string]string{"version": CurrentVersion, "layout": "pack"}, HasMagicLine: true, Version: CurrentVersion}}
		_ = applyHeaderDefaults(&p.Manifest.Header)
	}
	decls := make([]TableDecl, len(p.Tables))
	sum := int64(0)
	for i := range p.Tables {
		payload := int64(0)
		for _, col := range p.Tables[i].ColValues {
			payload += int64(len(colPayload(col)))
		}
		p.Tables[i].Decl.OriginalSize = payload
		p.Tables[i].Decl.Columns = len(p.Tables[i].ColValues)
		decls[i] = p.Tables[i].Decl
		sum += payload
	}
	p.Manifest.Meta.Tables = decls
	p.Manifest.Meta.FKs = p.FKs
	p.Manifest.Header.Fields["layout"] = "pack"
	p.Manifest.Header.Fields["version"] = CurrentVersion
	p.Manifest.Header.Fields["table-count"] = strconv.Itoa(len(p.Tables))
	p.Manifest.Header.Fields["original-size"] = strconv.FormatInt(sum, 10)
	p.Manifest.Header.OriginalSize = &sum
	if len(p.Tables) == 1 {
		if _, ok := p.Manifest.Header.Fields["single-table"]; !ok {
			p.Manifest.Header.Fields["single-table"] = p.Tables[0].Decl.Name
		}
	} else {
		delete(p.Manifest.Header.Fields, "single-table")
	}
}

func (p *Pack) Serialize() ([]byte, error) {
	p.syncManifest()
	var buf bytes.Buffer
	zw := zip.NewWriter(&buf)
	entries := []struct {
		name string
		body []byte
	}{}
	manBytes, err := p.Manifest.SerializeCanonical()
	if err != nil {
		return nil, err
	}
	entries = append(entries, struct {
		name string
		body []byte
	}{"_manifest.excsv", manBytes})
	for i := range p.Tables {
		t := &p.Tables[i]
		hb, err := t.Header.SerializeCanonical()
		if err != nil {
			return nil, err
		}
		entries = append(entries, struct {
			name string
			body []byte
		}{t.Decl.Dir + "_header.excsv", hb})
		header0 := false
		if t.Header.Header.Fields["header"] == "0" {
			header0 = true
		}
		rows := 0
		if len(t.ColValues) > 0 {
			rows = len(t.ColValues[0])
		}
		if t.Sectioned && t.SectionSize > 0 && rows > 0 {
			width := sectionPadWidth(rows)
			for ci, values := range t.ColValues {
				name := fmt.Sprintf("col%d", ci)
				if ci < len(t.ColNames) {
					name = t.ColNames[ci]
				}
				folder := t.Decl.Dir + strings.TrimSuffix(safeColFileName(ci, name, false), ".col") + "/"
				for _, start := range sectionStarts(rows, t.SectionSize) {
					end := start + t.SectionSize
					if end > rows {
						end = rows
					}
					rel := folder + fmt.Sprintf("%0*d.col", width, start)
					entries = append(entries, struct {
						name string
						body []byte
					}{rel, colPayload(values[start:end])})
				}
			}
		} else {
			for ci, values := range t.ColValues {
				name := fmt.Sprintf("col%d", ci)
				if ci < len(t.ColNames) {
					name = t.ColNames[ci]
				}
				rel := t.Decl.Dir + safeColFileName(ci, name, header0)
				entries = append(entries, struct {
					name string
					body []byte
				}{rel, colPayload(values)})
			}
		}
	}
	comment := packComment(manBytes)
	if err := zw.SetComment(string(comment)); err != nil {
		return nil, err
	}
	fixed := time.Date(2026, 1, 1, 0, 0, 0, 0, time.UTC)
	for _, e := range entries {
		h := &zip.FileHeader{Name: e.name, Method: zip.Deflate}
		h.SetModTime(fixed)
		w, err := zw.CreateHeader(h)
		if err != nil {
			return nil, err
		}
		if _, err := w.Write(e.body); err != nil {
			return nil, err
		}
	}
	if err := zw.Close(); err != nil {
		return nil, err
	}
	return buf.Bytes(), nil
}

func packComment(manifest []byte) []byte {
	var lines []string
	for _, line := range strings.Split(string(manifest), "\n") {
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
	c := strings.Join(lines, "\n")
	if len(c) > 65535 {
		marker := "\n#@comment-truncated: 1"
		keep := 65535 - len(marker)
		if keep < 0 {
			keep = 0
		}
		c = c[:keep] + marker
	}
	return []byte(c)
}

func (t *PackTable) SyncFromDocument(doc *Document) {
	if t == nil || doc == nil {
		return
	}
	t.Header = doc
	width := len(doc.Data.HeaderRow)
	if width == 0 && len(doc.Data.Rows) > 0 {
		width = len(doc.Data.Rows[0])
	}
	if n := len(columnNamesFromHeader(doc)); n > width {
		width = n
	}
	cols := make([][]string, width)
	for _, row := range doc.Data.Rows {
		for c := 0; c < width; c++ {
			v := ""
			if c < len(row) {
				v = row[c]
			}
			cols[c] = append(cols[c], v)
		}
	}
	t.ColValues = cols
	if len(doc.Data.HeaderRow) > 0 {
		t.ColNames = append([]string{}, doc.Data.HeaderRow...)
	} else {
		t.ColNames = columnNamesFromHeader(doc)
	}
	n := len(doc.Data.Rows)
	t.Header.Header.Rows = &n
	if t.Header.Header.Fields == nil {
		t.Header.Header.Fields = map[string]string{}
	}
	t.Header.Header.Fields["rows"] = strconv.Itoa(n)
	t.Header.Header.Fields["layout"] = "columnar"
}

func (t *PackTable) ExtractDocument() *Document {
	doc := cloneDocumentMeta(t.Header)
	delete(doc.Header.Fields, "layout")
	delete(doc.Header.Fields, "section-size")
	doc.Form = FormPlain
	doc.Data = t.Header.Data
	doc.Source.Profile = ProfileInline
	return doc
}
