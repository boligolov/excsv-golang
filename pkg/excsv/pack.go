package excsv

import (
	"fmt"
	"sort"
	"strconv"
	"strings"

	excsvzip "github.com/boligolov/excsv-golang/pkg/excsv/zip"
)

func parsePackPath(pathName string, data []byte, opts ParseOptions) (*ParseResult, error) {
	files, _, first, comment, err := excsvzip.ReadEntries(data, opts.ZipPassword)
	if err != nil {
		return nil, mapZipError(err)
	}

	pack := &Pack{}
	var res *ParseResult
	if first == "_manifest.excsv" {
		manOpts := opts
		manOpts.PackRole = "manifest"
		manOpts.ExpectZipInner = false
		manOpts.SourcePath = pathName
		res, err = ParseBytes(files["_manifest.excsv"], manOpts)
		if err != nil {
			return nil, err
		}
		if res.Doc.Header.Fields["layout"] != "pack" {
			return nil, fail(ErrPackManifestMissingLayout, 1, "_manifest.excsv lacks layout=pack")
		}
		res.Doc.Form = FormPack
		res.Doc.Source.Path = pathName
		res.Doc.Source.ZipPath = pathName
		res.Doc.Source.Comment = comment
		pack.Manifest = res.Doc
		pack.FKs = res.Doc.Meta.FKs
		if len(res.Doc.Meta.Tables) == 0 {
			pack.Discovered = true
			tables, err := discoverPackTables(files)
			if err != nil {
				return nil, err
			}
			pack.Tables = tables
		} else {
			for _, decl := range res.Doc.Meta.Tables {
				pt, err := loadPackTable(files, decl, opts)
				if err != nil {
					return nil, err
				}
				pack.Tables = append(pack.Tables, pt)
			}
		}
	} else {
		pack.Discovered = true
		doc := &Document{
			Form:   FormPack,
			Header: Header{Fields: map[string]string{"version": CurrentVersion, "layout": "pack"}, HasMagicLine: true, Version: CurrentVersion},
			Source: SourceInfo{Path: pathName, ZipPath: pathName, Comment: comment},
		}
		if err := applyHeaderDefaults(&doc.Header); err != nil {
			return nil, err
		}
		res = &ParseResult{Doc: doc}
		tables, err := discoverPackTables(files)
		if err != nil {
			return nil, err
		}
		pack.Tables = tables
		pack.Manifest = doc
	}

	for i := range pack.Tables {
		if err := validatePackTable(&pack.Tables[i], files); err != nil {
			return nil, err
		}
		materializePackTable(&pack.Tables[i])
	}

	res.Pack = pack
	res.Doc.Form = FormPack
	return res, nil
}

func discoverPackTables(files map[string][]byte) ([]PackTable, error) {
	dirs := map[string]struct{}{}
	for name := range files {
		if strings.HasSuffix(name, "/_header.excsv") {
			dirs[strings.TrimSuffix(name, "_header.excsv")] = struct{}{}
		}
	}
	names := make([]string, 0, len(dirs))
	for d := range dirs {
		names = append(names, d)
	}
	sort.Strings(names)
	var out []PackTable
	for _, dir := range names {
		tableName := strings.TrimSuffix(dir, "/")
		if i := strings.LastIndex(tableName, "/"); i >= 0 {
			tableName = tableName[i+1:]
		}
		decl := TableDecl{Name: tableName, Dir: dir}
		pt, err := loadPackTable(files, decl, ParseOptions{PackRole: "table", ResolveReference: true})
		if err != nil {
			return nil, err
		}
		out = append(out, pt)
	}
	return out, nil
}

func loadPackTable(files map[string][]byte, decl TableDecl, opts ParseOptions) (PackTable, error) {
	dir := decl.Dir
	if dir != "" && !strings.HasSuffix(dir, "/") {
		dir += "/"
		decl.Dir = dir
	}
	if dir != "" && tableDirMissing(files, dir) {
		return PackTable{}, fail(ErrPackTableDirMissing, decl.Line, "missing table dir "+dir)
	}
	headerName := dir + "_header.excsv"
	headerBytes, ok := files[headerName]
	if !ok {
		return PackTable{}, fail(ErrPackTableHeaderMissing, 0, "missing "+headerName)
	}
	tOpts := opts
	tOpts.PackRole = "table"
	tOpts.ExpectZipInner = false
	hres, err := ParseBytes(headerBytes, tOpts)
	if err != nil {
		return PackTable{}, err
	}
	pt := PackTable{Decl: decl, Header: hres.Doc}
	if ss := hres.Doc.Header.Fields["section-size"]; ss != "" && ss != "0" {
		n, err := parseIntField(ss)
		if err != nil {
			return PackTable{}, fail(ErrHeaderInvalidValue, 1, "invalid section-size="+ss)
		}
		pt.SectionSize = n
		pt.Sectioned = n > 0
	}
	names := columnNamesFromHeader(hres.Doc)
	pt.ColNames = names
	cols, paths, sectioned, err := readTableColumns(files, dir, names, pt.Sectioned)
	if err != nil {
		return PackTable{}, err
	}
	pt.ColValues = cols
	pt.ColPaths = paths
	if sectioned {
		pt.Sectioned = true
	}
	if pt.Decl.Columns == 0 {
		pt.Decl.Columns = len(cols)
	}
	return pt, nil
}

func columnNamesFromHeader(doc *Document) []string {
	var names []string
	for i, col := range doc.Meta.Columns {
		if isVirtualColumn(col) {
			continue
		}
		name := col.Attrs["name"]
		if name == "" {
			if idx, ok := col.Attrs["index"]; ok {
				name = idx
			} else {
				name = strconv.Itoa(i)
			}
		}
		names = append(names, name)
	}
	return names
}

func isVirtualColumn(col ColumnDef) bool {
	if col.Attrs["formula"] == "" {
		return false
	}
	return col.Attrs["materialized"] != "1"
}

func readTableColumns(files map[string][]byte, dir string, names []string, expectSectioned bool) ([][]string, []string, bool, error) {
	type sec struct {
		start int
		data  []byte
	}
	flat := map[string][]byte{}
	sections := map[string][]sec{}
	for name, body := range files {
		if !strings.HasPrefix(name, dir) {
			continue
		}
		rest := strings.TrimPrefix(name, dir)
		if rest == "_header.excsv" {
			continue
		}
		if !strings.HasSuffix(rest, ".col") {
			continue
		}
		parts := strings.Split(rest, "/")
		switch len(parts) {
		case 1:
			flat[parts[0]] = body
		case 2:
			start, err := strconv.Atoi(strings.TrimSuffix(parts[1], ".col"))
			if err != nil {
				continue
			}
			sections[parts[0]] = append(sections[parts[0]], sec{start: start, data: body})
		}
	}

	sectioned := len(sections) > 0
	keys := make([]string, 0)
	if sectioned {
		for k := range sections {
			keys = append(keys, k)
		}
	} else {
		for k := range flat {
			keys = append(keys, k)
		}
	}
	sort.Slice(keys, func(i, j int) bool {
		return colKeyIndex(keys[i]) < colKeyIndex(keys[j])
	})

	var values [][]string
	var paths []string
	if sectioned {
		for _, k := range keys {
			ss := sections[k]
			sort.Slice(ss, func(i, j int) bool { return ss[i].start < ss[j].start })
			var col []string
			for _, s := range ss {
				col = append(col, colLines(s.data)...)
			}
			values = append(values, col)
			paths = append(paths, dir+k+"/")
		}
	} else {
		for _, k := range keys {
			values = append(values, colLines(flat[k]))
			paths = append(paths, dir+k)
		}
	}
	_ = names
	_ = expectSectioned
	return values, paths, sectioned, nil
}

func colKeyIndex(name string) int {
	base := name
	if i := strings.IndexByte(base, '-'); i > 0 {
		base = base[:i]
	}
	base = strings.TrimSuffix(base, ".col")
	n, err := strconv.Atoi(base)
	if err != nil {
		return 1 << 30
	}
	return n
}

func colLines(b []byte) []string {
	s := string(b)
	s = strings.ReplaceAll(s, "\r\n", "\n")
	s = strings.ReplaceAll(s, "\r", "\n")
	if s == "" {
		return nil
	}
	if strings.HasSuffix(s, "\n") {
		s = s[:len(s)-1]
	}
	if s == "" {
		return []string{""}
	}
	return strings.Split(s, "\n")
}

func validatePackTable(pt *PackTable, files map[string][]byte) error {
	dir := pt.Decl.Dir
	if dir != "" {
		found := false
		for name := range files {
			if strings.HasPrefix(name, dir) {
				found = true
				break
			}
		}
		if !found {
			return fail(ErrPackTableDirMissing, pt.Decl.Line, "missing table dir "+dir)
		}
	}
	if _, ok := files[dir+"_header.excsv"]; !ok {
		return fail(ErrPackTableHeaderMissing, 0, "missing _header.excsv in "+dir)
	}

	physical := len(pt.ColValues)
	headerCols := 0
	for _, col := range pt.Header.Meta.Columns {
		if !isVirtualColumn(col) {
			headerCols++
		}
	}
	if pt.Decl.Columns > 0 && physical != pt.Decl.Columns {
		return fail(ErrPackColumnCountMismatch, pt.Decl.Line, "columns= does not match .col count")
	}
	if headerCols > 0 && physical != headerCols {
		return fail(ErrPackColumnCountMismatch, 0, "column file count does not match #column")
	}

	rows := 0
	if pt.Header.Header.Rows != nil {
		rows = *pt.Header.Header.Rows
	}

	if pt.Sectioned && pt.SectionSize > 0 {
		return validateSectionedTable(pt, files, rows)
	}

	for _, col := range pt.ColValues {
		if rows > 0 && len(col) != rows {
			return fail(ErrPackColLineCountMismatch, 0, "column line count does not match rows=")
		}
	}
	return nil
}

func validateSectionedTable(pt *PackTable, files map[string][]byte, rows int) error {
	ss := pt.SectionSize
	if ss <= 0 {
		return nil
	}
	type secMap map[int]int // start -> line count
	perCol := []secMap{}
	colKeys := []string{}
	for name := range files {
		if !strings.HasPrefix(name, pt.Decl.Dir) {
			continue
		}
		rest := strings.TrimPrefix(name, pt.Decl.Dir)
		parts := strings.Split(rest, "/")
		if len(parts) != 2 || !strings.HasSuffix(parts[1], ".col") {
			continue
		}
		start, err := strconv.Atoi(strings.TrimSuffix(parts[1], ".col"))
		if err != nil {
			continue
		}
		n := len(colLines(files[name]))
		idx := -1
		for i, k := range colKeys {
			if k == parts[0] {
				idx = i
				break
			}
		}
		if idx < 0 {
			colKeys = append(colKeys, parts[0])
			perCol = append(perCol, secMap{})
			idx = len(perCol) - 1
		}
		perCol[idx][start] = n
		remain := rows - start
		if remain < 0 {
			remain = 0
		}
		wantMax := ss
		if remain < wantMax {
			wantMax = remain
		}
		if n > wantMax {
			return fail(ErrPackSectionPartition, 0, "section line count exceeds remaining rows")
		}
	}

	if len(perCol) == 0 {
		return nil
	}
	ref := perCol[0]
	for i := 1; i < len(perCol); i++ {
		if !sameSecMap(ref, perCol[i]) {
			return fail(ErrPackSectionBoundary, 0, "section boundaries differ across columns")
		}
	}

	expected := map[int]int{}
	for start := 0; start < rows; start += ss {
		want := ss
		if rows-start < want {
			want = rows - start
		}
		expected[start] = want
	}
	if !sameSecMap(ref, expected) {
		return fail(ErrPackSectionPartition, 0, "section partitioning does not match section-size=")
	}
	return nil
}

func sameSecMap(a, b map[int]int) bool {
	if len(a) != len(b) {
		return false
	}
	for k, v := range a {
		if b[k] != v {
			return false
		}
	}
	return true
}

func materializePackTable(pt *PackTable) {
	if pt.Header == nil {
		return
	}
	names := pt.ColNames
	if len(names) == 0 {
		names = make([]string, len(pt.ColValues))
		for i := range names {
			names[i] = fmt.Sprintf("col%d", i)
		}
	}
	n := 0
	if pt.Header.Header.Rows != nil {
		n = *pt.Header.Header.Rows
	}
	for _, col := range pt.ColValues {
		if len(col) > n {
			n = len(col)
		}
	}
	rows := make([][]string, n)
	for r := 0; r < n; r++ {
		row := make([]string, len(pt.ColValues))
		for c, col := range pt.ColValues {
			if r < len(col) {
				row[c] = col[r]
			}
		}
		rows[r] = row
	}
	pt.Header.Data.HasHeaderRow = true
	pt.Header.Data.HeaderRow = names
	pt.Header.Data.Rows = rows
	pt.Header.Form = FormPack
}

func tableDirMissing(files map[string][]byte, dir string) bool {
	if dir == "" {
		return true
	}
	for name := range files {
		if strings.HasPrefix(name, dir) {
			return false
		}
	}
	return true
}

func formatPackTableLine(d TableDecl) string {
	dir := d.Dir
	if dir != "" && !strings.HasSuffix(dir, "/") {
		dir += "/"
	}
	return fmt.Sprintf("#table name=%s dir=%s columns=%d original-size=%d", d.Name, dir, d.Columns, d.OriginalSize)
}

func formatFKLine(fk ForeignKey) string {
	return "#fk from=" + fk.From + " to=" + fk.To
}

func safeColFileName(index int, name string, header0 bool) string {
	safe := strings.ToLower(name)
	var b strings.Builder
	for _, r := range safe {
		if (r >= 'a' && r <= 'z') || (r >= '0' && r <= '9') || r == '_' {
			b.WriteRune(r)
		} else {
			b.WriteByte('_')
		}
	}
	safe = strings.Trim(b.String(), "_")
	if safe == "" {
		safe = "col"
	}
	if header0 {
		return fmt.Sprintf("%02d.col", index)
	}
	return fmt.Sprintf("%02d-%s.col", index, safe)
}

func colPayload(values []string) []byte {
	if len(values) == 0 {
		return []byte{}
	}
	return []byte(strings.Join(values, "\n") + "\n")
}

func sectionStarts(rows, sectionSize int) []int {
	var out []int
	for start := 0; start < rows; start += sectionSize {
		out = append(out, start)
	}
	return out
}

func sectionPadWidth(rows int) int {
	n := rows - 1
	if n < 0 {
		n = 0
	}
	w := len(strconv.Itoa(n))
	if w < 1 {
		return 1
	}
	return w
}
