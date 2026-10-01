package excsv

import (
	"fmt"
	"strconv"
	"strings"
)

// NotesVersion is the first spec version that defines #note, link= and #link.
const NotesVersion = "0.6"

// NoteTarget is what a #note is attached to, decided by which address fields
// it carries (implementation/notes.md § #note).
type NoteTarget string

// Note targets.
const (
	NoteTargetCell   NoteTarget = "cell"
	NoteTargetColumn NoteTarget = "column"
	NoteTargetRow    NoteTarget = "row"
	NoteTargetTable  NoteTarget = "table"
)

// Note is one #note line (implementation/notes.md): an advisory remark on a
// cell, a row, a column, or the table. Attrs keeps every attribute in file
// order, unknown ones included, so the line round-trips unchanged.
type Note struct {
	Attrs []KV
	Line  int
}

// Link is one #link line: a URL pinned to a single cell, overriding the
// column's link= template.
type Link struct {
	Attrs []KV
	Line  int
}

// Attr returns the value of a #note attribute.
func (n Note) Attr(key string) (string, bool) { return kvGet(n.Attrs, key) }

// Text returns text=.
func (n Note) Text() string { v, _ := n.Attr("text"); return v }

// Target reports what the note is attached to.
func (n Note) Target() NoteTarget {
	_, hasCol := n.Attr("col")
	_, hasRow := n.Attr("row")
	_, hasKey := n.Attr("key")
	switch {
	case (hasRow || hasKey) && hasCol:
		return NoteTargetCell
	case hasCol:
		return NoteTargetColumn
	case hasRow || hasKey:
		return NoteTargetRow
	}
	return NoteTargetTable
}

// MetaLine renders the note back to its #note form.
func (n Note) MetaLine() string { return formatAnchoredLine("#note", n.Attrs, "text") }

// Attr returns the value of a #link attribute.
func (l Link) Attr(key string) (string, bool) { return kvGet(l.Attrs, key) }

// Href returns href=.
func (l Link) Href() string { v, _ := l.Attr("href"); return v }

// MetaLine renders the link back to its #link form.
func (l Link) MetaLine() string { return formatAnchoredLine("#link", l.Attrs, "href") }

func kvGet(list []KV, key string) (string, bool) {
	for _, kv := range list {
		if kv.Key == key {
			return kv.Value, true
		}
	}
	return "", false
}

// formatAnchoredLine writes a #note / #link line. The free-text attribute
// (text= / href=) is always quoted, as the spec's examples do; the rest only
// when they contain whitespace or a quote.
func formatAnchoredLine(prefix string, attrs []KV, quoted string) string {
	var b strings.Builder
	b.WriteString(prefix)
	for _, kv := range attrs {
		b.WriteByte(' ')
		if kv.Key == quoted || strings.ContainsAny(kv.Value, " \t\"") || kv.Value == "" {
			b.WriteString(kv.Key + "=\"" + strings.ReplaceAll(kv.Value, "\"", "\"\"") + "\"")
		} else {
			b.WriteString(kv.Key + "=" + kv.Value)
		}
	}
	return b.String()
}

// parseAnchoredAttrs tokenizes the attributes of a #note / #link line with the
// #column tokenizer. Any token that is not key=value, or an unterminated
// quote, is the line's malformed code.
func parseAnchoredAttrs(rest string, lineNo int, malformed ErrorKind, what string) ([]KV, error) {
	pairs, err := splitHeaderPairs(strings.TrimSpace(rest))
	if err != nil {
		return nil, fail(malformed, lineNo, what+" line does not tokenize")
	}
	var attrs []KV
	for _, p := range pairs {
		eq := strings.IndexByte(p, '=')
		if eq <= 0 {
			return nil, fail(malformed, lineNo, what+" token "+strconv.Quote(p)+" is not key=value")
		}
		attrs = upsertKV(attrs, p[:eq], p[eq+1:])
	}
	return attrs, nil
}

// isMetaKeyword reports whether line is the meta keyword kw followed by end of
// line or whitespace ("#note" but not "#notes").
func isMetaKeyword(line, kw string) bool {
	if !strings.HasPrefix(line, kw) {
		return false
	}
	rest := line[len(kw):]
	return rest == "" || rest[0] == ' ' || rest[0] == '\t'
}

func parseNoteLine(line string, lineNo int) (Note, error) {
	attrs, err := parseAnchoredAttrs(strings.TrimPrefix(line, "#note"), lineNo, ErrNoteMalformed, "#note")
	if err != nil {
		return Note{}, err
	}
	n := Note{Attrs: attrs, Line: lineNo}
	return n, n.check()
}

func parseLinkLine(line string, lineNo int) (Link, error) {
	attrs, err := parseAnchoredAttrs(strings.TrimPrefix(line, "#link"), lineNo, ErrLinkMalformed, "#link")
	if err != nil {
		return Link{}, err
	}
	l := Link{Attrs: attrs, Line: lineNo}
	return l, l.check()
}

// check applies the #note FAIL rules that need nothing but the line itself.
func (n Note) check() error {
	if _, ok := n.Attr("text"); !ok {
		return fail(ErrNoteMissingText, n.Line, "#note lacks text=")
	}
	_, hasRow := n.Attr("row")
	_, hasKey := n.Attr("key")
	if hasRow && hasKey {
		return fail(ErrNoteRowAndKey, n.Line, "#note sets both row= and key=")
	}
	return nil
}

// check applies the #link FAIL rules that need nothing but the line itself.
func (l Link) check() error {
	if _, ok := l.Attr("href"); !ok {
		return fail(ErrLinkMissingHref, l.Line, "#link lacks href=")
	}
	_, hasRow := l.Attr("row")
	_, hasKey := l.Attr("key")
	if hasRow && hasKey {
		return fail(ErrLinkRowAndKey, l.Line, "#link sets both row= and key=")
	}
	if _, hasCol := l.Attr("col"); !hasCol || (!hasRow && !hasKey) {
		return fail(ErrLinkMissingAddress, l.Line, "#link needs col= and one of row= / key=")
	}
	return nil
}

// ---------------------------------------------------------------------------
// Resolution
// ---------------------------------------------------------------------------

// ResolvedNote is a #note with its address resolved against the data. Row and
// Col are -1 when the note is not attached to a row / a column, or when that
// part of its address does not resolve. Col is also -1 for a virtual computed
// column, which has no physical position; ColName still names it.
type ResolvedNote struct {
	Note
	Target   NoteTarget
	Row      int
	Col      int
	ColName  string
	Resolved bool
}

// anchorResolver resolves col= / row= / key= addresses, and link= placeholders,
// against one table.
type anchorResolver struct {
	doc     *Document
	width   int
	rows    int
	idCol   int
	idWhy   string
	keyRows map[string]int
}

type colRef struct {
	phys    int // physical index; -1 for a virtual computed column
	name    string
	virtual bool
}

func newAnchorResolver(doc *Document) *anchorResolver {
	r := &anchorResolver{doc: doc, width: doc.PhysicalColumnCount(), rows: len(doc.Data.Rows), idCol: -1}
	r.findIDColumn()
	return r
}

// findIDColumn picks the table's id column (notes.md § The id column): the
// single role=id column, else the single unique=1 column.
func (r *anchorResolver) findIDColumn() {
	pick := func(match func(ColumnDef) bool, label string) bool {
		var found []ColumnDef
		for _, col := range r.doc.Meta.Columns {
			if match(col) {
				found = append(found, col)
			}
		}
		switch len(found) {
		case 0:
			return false
		case 1:
			if isVirtualColumn(found[0]) {
				r.idWhy = "the " + label + " column is virtual and has no raw values"
				return true
			}
			r.idCol = r.physIndexOfDecl(found[0])
			if r.idCol < 0 {
				r.idWhy = "the " + label + " column has no physical position"
			}
			return true
		default:
			r.idWhy = fmt.Sprintf("%d columns have %s, so the table has no id column", len(found), label)
			return true
		}
	}
	if !pick(func(c ColumnDef) bool { return c.Attrs["role"] == "id" }, "role=id") &&
		!pick(func(c ColumnDef) bool { return c.Attrs["unique"] == "1" }, "unique=1") {
		r.idWhy = "the table has no id column (role=id or unique=1)"
	}
	if r.idCol < 0 {
		return
	}
	r.keyRows = map[string]int{}
	for i, row := range r.doc.Data.Rows {
		v := cellAt(r.doc, row, r.idCol)
		if isNullCell(r.doc, v) {
			continue
		}
		if _, seen := r.keyRows[v]; !seen {
			r.keyRows[v] = i
		}
	}
}

// physIndexOfDecl maps a non-virtual #column to its physical position: index=
// when set, else (header=1) its header cell, else declaration order over
// non-virtual columns.
func (r *anchorResolver) physIndexOfDecl(col ColumnDef) int {
	if v, ok := col.Attrs["index"]; ok {
		if n, err := strconv.Atoi(v); err == nil {
			return n
		}
		return -1
	}
	name := col.Attrs["name"]
	if r.doc.Header.HeaderRow && name != "" {
		for i, cell := range r.doc.Data.HeaderRow {
			if cell == name {
				return i
			}
		}
		if title := col.Attrs["title"]; title != "" {
			for i, cell := range r.doc.Data.HeaderRow {
				if cell == title {
					return i
				}
			}
		}
	}
	phys := 0
	for _, c := range r.doc.Meta.Columns {
		if isVirtualColumn(c) {
			continue
		}
		if c.Attrs["name"] == name && c.Line == col.Line {
			return phys
		}
		phys++
	}
	return -1
}

// column resolves a col= value or a link= placeholder: a name with header=1;
// with header=0 a declared name, else a zero-based index.
func (r *anchorResolver) column(ref string) (colRef, bool) {
	if ref == "" {
		return colRef{}, false
	}
	for _, col := range r.doc.Meta.Columns {
		if col.Attrs["name"] != ref {
			continue
		}
		if isVirtualColumn(col) {
			return colRef{phys: -1, name: ref, virtual: true}, true
		}
		if i := r.physIndexOfDecl(col); i >= 0 && (r.width == 0 || i < r.width) {
			return colRef{phys: i, name: ref}, true
		}
		return colRef{}, false
	}
	if r.doc.Header.HeaderRow {
		for i, cell := range r.doc.Data.HeaderRow {
			if cell == ref {
				return colRef{phys: i, name: ref}, true
			}
		}
		return colRef{}, false
	}
	n, err := strconv.Atoi(ref)
	if err != nil || n < 0 || n >= r.width || strings.TrimSpace(ref) != ref {
		return colRef{}, false
	}
	return colRef{phys: n, name: ref}, true
}

// row resolves row= / key=. present is false when the line carries neither.
func (r *anchorResolver) row(attrs []KV) (row int, present, ok bool, why string) {
	if v, has := kvGet(attrs, "row"); has {
		n, err := strconv.Atoi(v)
		if err != nil || n < 0 {
			return -1, true, false, "row=" + v + " is not a non-negative integer"
		}
		if n >= r.rows {
			return -1, true, false, fmt.Sprintf("row=%d is past the last data row", n)
		}
		return n, true, true, ""
	}
	if v, has := kvGet(attrs, "key"); has {
		if r.idCol < 0 {
			return -1, true, false, "key=" + v + ": " + r.idWhy
		}
		n, found := r.keyRows[v]
		if !found {
			return -1, true, false, "key=" + v + " matches no row"
		}
		return n, true, true, ""
	}
	return -1, false, false, ""
}

// ResolveNotes resolves every #note against the data, in file order.
// Unresolved notes are kept, with Resolved false.
func (doc *Document) ResolveNotes() []ResolvedNote {
	out, _ := doc.resolveNotes(newAnchorResolver(doc))
	return out
}

func (doc *Document) resolveNotes(r *anchorResolver) ([]ResolvedNote, []Issue) {
	var out []ResolvedNote
	var issues []Issue
	for _, n := range doc.Meta.Notes {
		rn := ResolvedNote{Note: n, Target: n.Target(), Row: -1, Col: -1, Resolved: true}
		var why []string
		if v, has := n.Attr("col"); has {
			if c, ok := r.column(v); ok {
				rn.Col, rn.ColName = c.phys, c.name
			} else {
				rn.Resolved = false
				why = append(why, "col="+v+" names no column")
			}
		}
		if row, present, ok, msg := r.row(n.Attrs); present {
			if ok {
				rn.Row = row
			} else {
				rn.Resolved = false
				why = append(why, msg)
			}
		}
		if !rn.Resolved {
			rn.Row, rn.Col = -1, -1
			issues = append(issues, newIssue(ErrNoteUnresolved, n.Line,
				"#note does not resolve ("+strings.Join(why, "; ")+"); kept"))
		}
		out = append(out, rn)
	}
	return out, issues
}

// cellKey addresses one cell: a data row and a physical column, or a virtual
// column by name (vname set, col -1).
type cellKey struct {
	row   int
	col   int
	vname string
}

// LinkSet is every link of a table, resolved: link= templates substituted per
// cell, #link overrides applied, and the scheme check done. Only links that
// passed the check are present.
type LinkSet struct {
	doc   *Document
	r     *anchorResolver
	cells map[cellKey]string
}

// CellLink returns the URL the cell at data row row, column col (a name, or an
// index with header=0) links to. ok is false when the cell has no link, or its
// link failed the scheme check and is shown as plain text.
func (s *LinkSet) CellLink(row int, col string) (string, bool) {
	if s == nil {
		return "", false
	}
	c, ok := s.r.column(col)
	if !ok {
		return "", false
	}
	href, ok := s.cells[c.key(row)]
	return href, ok
}

// Count is the number of cells that render as links.
func (s *LinkSet) Count() int {
	if s == nil {
		return 0
	}
	return len(s.cells)
}

func (c colRef) key(row int) cellKey {
	if c.virtual {
		return cellKey{row: row, col: -1, vname: c.name}
	}
	return cellKey{row: row, col: c.phys}
}

// ResolveLinks resolves every link= template and #link line against the data.
func (doc *Document) ResolveLinks() *LinkSet {
	s, _ := doc.resolveLinks(newAnchorResolver(doc))
	return s
}

func (doc *Document) resolveLinks(r *anchorResolver) (*LinkSet, []Issue) {
	s := &LinkSet{doc: doc, r: r, cells: map[cellKey]string{}}
	var issues []Issue
	unsafe := map[int]int{} // #column line -> unsafe cells

	for _, col := range doc.Meta.Columns {
		tmpl, ok := col.Attrs["link"]
		if !ok || isVirtualColumn(col) {
			continue
		}
		label := "column " + columnLabel(col) + ": "
		parts, err := parseLinkTemplate(tmpl)
		if err != nil {
			issues = append(issues, newIssue(ErrLinkTemplateMalformed, col.Line,
				label+err.Error()+"; shown without links"))
			continue
		}
		self := r.physIndexOfDecl(col)
		if self < 0 {
			continue
		}
		refs, bad := r.templateColumns(parts, self)
		if bad != "" {
			issues = append(issues, newIssue(ErrLinkUnknownColumn, col.Line,
				label+"link= placeholder {"+bad+"} names no stored or materialized column; shown without links"))
			continue
		}
		single := len(parts) == 1 && parts[0].ref != ""
		for i, row := range doc.Data.Rows {
			href, ok := expandLinkTemplate(doc, parts, refs, row, single)
			if !ok {
				continue
			}
			if !IsSafeLinkScheme(href) {
				unsafe[col.Line]++
				continue
			}
			s.cells[cellKey{row: i, col: self}] = href
		}
	}
	for _, col := range doc.Meta.Columns {
		if n := unsafe[col.Line]; n > 0 {
			issues = append(issues, newIssue(ErrLinkUnsafeScheme, col.Line, fmt.Sprintf(
				"column %s: %d link(s) not http/https/mailto; shown as plain text", columnLabel(col), n)))
		}
	}

	seen := map[cellKey]int{}
	for _, l := range doc.Meta.Links {
		colVal, _ := l.Attr("col")
		c, okCol := r.column(colVal)
		row, _, okRow, why := r.row(l.Attrs)
		if !okCol || !okRow {
			if !okCol {
				why = "col=" + colVal + " names no column"
			}
			issues = append(issues, newIssue(ErrLinkUnresolved, l.Line, "#link does not resolve ("+why+"); kept"))
			continue
		}
		key := c.key(row)
		if prev, dup := seen[key]; dup {
			issues = append(issues, newIssue(ErrLinkDuplicate, l.Line,
				fmt.Sprintf("#link targets the same cell as line %d; last one wins", prev)))
		}
		seen[key] = l.Line
		href := l.Href()
		if !IsSafeLinkScheme(href) {
			delete(s.cells, key)
			issues = append(issues, newIssue(ErrLinkUnsafeScheme, l.Line,
				"#link href is not http/https/mailto; shown as plain text"))
			continue
		}
		s.cells[key] = href
	}
	return s, issues
}

func columnLabel(col ColumnDef) string {
	if n := col.Attrs["name"]; n != "" {
		return n
	}
	return col.Attrs["index"]
}

// templatePart is a literal run or a placeholder (ref set) of a link= template.
type templatePart struct {
	lit string
	ref string
}

// parseLinkTemplate splits a link= template (notes.md § Template grammar).
func parseLinkTemplate(t string) ([]templatePart, error) {
	var parts []templatePart
	var lit strings.Builder
	for i := 0; i < len(t); i++ {
		if t[i] != '{' {
			lit.WriteByte(t[i])
			continue
		}
		if i+1 < len(t) && t[i+1] == '{' {
			lit.WriteByte('{')
			i++
			continue
		}
		end := strings.IndexByte(t[i+1:], '}')
		if end < 0 {
			return nil, fmt.Errorf("link= template has an unterminated {")
		}
		if lit.Len() > 0 {
			parts = append(parts, templatePart{lit: lit.String()})
			lit.Reset()
		}
		parts = append(parts, templatePart{ref: t[i+1 : i+1+end]})
		i += end + 1
	}
	if lit.Len() > 0 {
		parts = append(parts, templatePart{lit: lit.String()})
	}
	return parts, nil
}

// templateColumns resolves each placeholder to a physical column. A
// placeholder must name a stored or materialized column; bad is the first one
// that does not.
func (r *anchorResolver) templateColumns(parts []templatePart, self int) (refs []int, bad string) {
	refs = make([]int, len(parts))
	for i, p := range parts {
		refs[i] = -1
		switch {
		case p.ref == "" && p.lit != "":
		case p.ref == "$":
			refs[i] = self
		default:
			c, ok := r.column(p.ref)
			if !ok || c.virtual {
				return nil, p.ref
			}
			refs[i] = c.phys
		}
	}
	return refs, ""
}

// expandLinkTemplate substitutes one row. ok is false when a placeholder's
// value is null (that cell has no link).
func expandLinkTemplate(doc *Document, parts []templatePart, refs []int, row []string, single bool) (string, bool) {
	var b strings.Builder
	for i, p := range parts {
		if refs[i] < 0 {
			b.WriteString(p.lit)
			continue
		}
		v := cellAt(doc, row, refs[i])
		if isNullCell(doc, v) {
			return "", false
		}
		if single {
			b.WriteString(v)
		} else {
			b.WriteString(PercentEncodeLinkValue(v))
		}
	}
	return b.String(), true
}

// PercentEncodeLinkValue percent-encodes a substituted link= value: UTF-8
// bytes, uppercase hex, only RFC 3986 unreserved characters left as is.
func PercentEncodeLinkValue(v string) string {
	const hex = "0123456789ABCDEF"
	var b strings.Builder
	for i := 0; i < len(v); i++ {
		c := v[i]
		if (c >= 'A' && c <= 'Z') || (c >= 'a' && c <= 'z') || (c >= '0' && c <= '9') ||
			c == '-' || c == '.' || c == '_' || c == '~' {
			b.WriteByte(c)
			continue
		}
		b.WriteByte('%')
		b.WriteByte(hex[c>>4])
		b.WriteByte(hex[c&0x0f])
	}
	return b.String()
}

// IsSafeLinkScheme reports whether a final URL may be rendered as a link:
// its scheme is http, https or mailto (case-insensitive). A URL without a
// scheme is not safe.
func IsSafeLinkScheme(u string) bool {
	colon := strings.IndexByte(u, ':')
	if colon <= 0 {
		return false
	}
	scheme := u[:colon]
	for i := 0; i < len(scheme); i++ {
		c := scheme[i]
		isAlpha := (c >= 'a' && c <= 'z') || (c >= 'A' && c <= 'Z')
		if i == 0 && !isAlpha {
			return false
		}
		if !isAlpha && !(c >= '0' && c <= '9') && c != '+' && c != '-' && c != '.' {
			return false
		}
	}
	switch strings.ToLower(scheme) {
	case "http", "https", "mailto":
		return true
	}
	return false
}

// CheckNotesLinks runs the #note / #link / link= resolution rules (all WARN)
// against the data: unresolved addresses, duplicate #link cells, unknown or
// malformed template placeholders, and links failing the scheme check.
func (doc *Document) CheckNotesLinks() []Issue {
	if len(doc.Meta.Notes) == 0 && len(doc.Meta.Links) == 0 && !doc.hasLinkTemplates() {
		return nil
	}
	r := newAnchorResolver(doc)
	_, issues := doc.resolveNotes(r)
	_, linkIssues := doc.resolveLinks(r)
	return append(issues, linkIssues...)
}

func (doc *Document) hasLinkTemplates() bool {
	for _, col := range doc.Meta.Columns {
		if _, ok := col.Attrs["link"]; ok {
			return true
		}
	}
	return false
}

// ---------------------------------------------------------------------------
// Editing
// ---------------------------------------------------------------------------

// RowAnchor returns the address a writer should use for data row row: key=
// with the row's id-column value when the table has an id column and that
// value leads back to this row, else row= (notes.md § Positional vs. key
// anchors).
func (doc *Document) RowAnchor(row int) (KV, error) {
	if row < 0 || row >= len(doc.Data.Rows) {
		return KV{}, fmt.Errorf("row %d is out of range (the table has %d data rows)", row, len(doc.Data.Rows))
	}
	r := newAnchorResolver(doc)
	if r.idCol >= 0 {
		v := cellAt(doc, doc.Data.Rows[row], r.idCol)
		if at, ok := r.keyRows[v]; ok && at == row {
			return KV{Key: "key", Value: v}, nil
		}
	}
	return KV{Key: "row", Value: strconv.Itoa(row)}, nil
}

// ResolveColumn reports whether col= value v names a column of the table.
func (doc *Document) ResolveColumn(v string) bool {
	_, ok := newAnchorResolver(doc).column(v)
	return ok
}

// AddNote appends a #note built from attrs (in the given order).
func (doc *Document) AddNote(attrs []KV) error {
	n := Note{Attrs: append([]KV(nil), attrs...)}
	if err := n.check(); err != nil {
		return err
	}
	doc.Meta.Notes = append(doc.Meta.Notes, n)
	doc.requireVersion(NotesVersion)
	return nil
}

// RemoveNote removes the note at index i of Meta.Notes.
func (doc *Document) RemoveNote(i int) bool {
	if i < 0 || i >= len(doc.Meta.Notes) {
		return false
	}
	doc.Meta.Notes = append(doc.Meta.Notes[:i:i], doc.Meta.Notes[i+1:]...)
	return true
}

// SetLink adds a #link, or replaces the href of an existing #link with the
// same address (col= plus row= or key=).
func (doc *Document) SetLink(attrs []KV) error {
	l := Link{Attrs: append([]KV(nil), attrs...)}
	if err := l.check(); err != nil {
		return err
	}
	doc.requireVersion(NotesVersion)
	for i, existing := range doc.Meta.Links {
		if sameAnchor(existing.Attrs, l.Attrs) {
			doc.Meta.Links[i] = l
			return nil
		}
	}
	doc.Meta.Links = append(doc.Meta.Links, l)
	return nil
}

// RemoveLink removes the #link at index i of Meta.Links.
func (doc *Document) RemoveLink(i int) bool {
	if i < 0 || i >= len(doc.Meta.Links) {
		return false
	}
	doc.Meta.Links = append(doc.Meta.Links[:i:i], doc.Meta.Links[i+1:]...)
	return true
}

func sameAnchor(a, b []KV) bool {
	for _, k := range []string{"col", "row", "key"} {
		va, oka := kvGet(a, k)
		vb, okb := kvGet(b, k)
		if oka != okb || va != vb {
			return false
		}
	}
	return true
}

// requireVersion raises a declared version= that predates v, once the
// document uses a construct v introduced. A version this package does not
// know is left alone.
func (doc *Document) requireVersion(v string) {
	h := &doc.Header
	if !h.HasMagicLine || h.Version == "" || !isKnownVersion(h.Version) || compareVersions(h.Version, v) >= 0 {
		return
	}
	h.Version = v
	if h.Fields != nil {
		h.Fields["version"] = v
	}
}

// remapRowAnchors keeps row= on notes and links pointing at the same row after
// rows were reordered or removed (notes.md § Writer obligations). newIndex
// maps an old row position to its new one, -1 when the row was removed; lines
// whose row was removed are dropped, whether anchored by row= or key=.
// Unresolved lines are kept unchanged.
func (doc *Document) remapRowAnchors(r *anchorResolver, newIndex []int) {
	remap := func(attrs []KV) ([]KV, bool) {
		row, present, ok, _ := r.row(attrs)
		if !present || !ok || row >= len(newIndex) {
			return attrs, true
		}
		to := newIndex[row]
		if to < 0 {
			return nil, false
		}
		if _, hasRow := kvGet(attrs, "row"); hasRow {
			attrs = upsertKV(append([]KV(nil), attrs...), "row", strconv.Itoa(to))
		}
		return attrs, true
	}
	var notes []Note
	for _, n := range doc.Meta.Notes {
		if attrs, keep := remap(n.Attrs); keep {
			n.Attrs = attrs
			notes = append(notes, n)
		}
	}
	doc.Meta.Notes = notes
	var links []Link
	for _, l := range doc.Meta.Links {
		if attrs, keep := remap(l.Attrs); keep {
			l.Attrs = attrs
			links = append(links, l)
		}
	}
	doc.Meta.Links = links
}
