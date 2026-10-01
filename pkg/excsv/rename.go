package excsv

import (
	"fmt"
	"regexp"
	"strings"
)

// formulaKeywords are the identifiers the formula grammar reserves; a column
// named like one could not be referenced from formula=.
var formulaKeywords = stringSet([]string{
	"and", "or", "not", "case", "when", "then", "else", "end", "true", "false", "null",
})

// RenameColumn renames column old to new and updates every reference to it
// by name: the #column declaration, the header row cell, formula= of computed
// columns, #chart channels, #note / #link col=, and {old} placeholders in
// link= templates (notes.md § Writer obligations). It returns notices about
// references it cannot rewrite safely (#$ SQL, #chart-<engine>: payloads).
//
// A sidecar's referenced data file is never rewritten: when its header cell
// carries the old name, that text is kept as title= so the declaration still
// maps onto it.
func (doc *Document) RenameColumn(old, new string) ([]string, error) {
	new = strings.TrimSpace(new)
	if new == "" {
		return nil, fmt.Errorf("new column name is required")
	}
	if strings.ContainsAny(new, " \t") {
		return nil, fail(ErrColumnMalformedAttribute, 0, "column name must not contain spaces")
	}
	if old == new {
		return nil, fmt.Errorf("column is already named %s", new)
	}
	declIdx, def, declared := doc.columnByName(old)
	headerIdx := -1
	if doc.Data.HasHeaderRow && !(declared && isVirtualColumn(def)) {
		r := newAnchorResolver(doc)
		if declared {
			headerIdx = r.physIndexOfDecl(def)
		} else {
			for i, cell := range doc.Data.HeaderRow {
				if cell == old {
					headerIdx = i
					break
				}
			}
		}
		if headerIdx >= len(doc.Data.HeaderRow) {
			headerIdx = -1
		}
	}
	if !declared && headerIdx < 0 {
		return nil, fmt.Errorf("unknown column: %s", old)
	}
	if !declared && doc.Source.Profile == ProfileSidecar {
		return nil, fmt.Errorf("column %s has no #column declaration; a sidecar cannot rename its referenced file's header — declare it first (column set %s)", old, old)
	}
	if _, _, taken := doc.columnByName(new); taken {
		return nil, fmt.Errorf("column %s already exists", new)
	}
	for i, cell := range doc.Data.HeaderRow {
		if cell == new && i != headerIdx {
			return nil, fmt.Errorf("column %s already exists", new)
		}
	}

	// Formulas are checked before anything changes: a name the formula
	// grammar cannot reference would leave them dangling.
	formulas := map[int]string{}
	for i, col := range doc.Meta.Columns {
		f := col.Attrs["formula"]
		if f == "" {
			continue
		}
		if renamed, changed := renameFormulaRef(f, old, new); changed {
			if !isFormulaIdent(new) {
				return nil, fmt.Errorf("column %s is used in formula= of %s; %q is not a valid formula identifier",
					old, columnLabel(col), new)
			}
			formulas[i] = renamed
		}
	}

	if declared {
		def.Attrs["name"] = new
		doc.Meta.Columns[declIdx] = def
	}
	for i, f := range formulas {
		doc.Meta.Columns[i].Attrs["formula"] = f
	}
	var notices []string
	if headerIdx >= 0 && doc.Data.HeaderRow[headerIdx] == old {
		if doc.Source.Profile == ProfileSidecar {
			if declared && def.Attrs["title"] == "" {
				def.Attrs["title"] = old
				notices = append(notices, "sidecar: the referenced data file is not rewritten; its header cell "+
					old+" is kept as title="+old)
			}
		} else {
			doc.Data.HeaderRow[headerIdx] = new
			// The header row is part of the data section, which checksum= covers.
			if err := doc.SyncDerived(); err != nil {
				return nil, err
			}
		}
	}

	doc.renameColumnInCharts(old, new, &notices)
	doc.renameColumnInNotesLinks(old, new)
	doc.renameColumnInSQL(old, &notices)
	return notices, nil
}

func (doc *Document) renameColumnInCharts(old, new string, notices *[]string) {
	for ci, c := range doc.Meta.Charts {
		if c.IsEscape() {
			if strings.Contains(c.Payload, `"`+old+`"`) {
				*notices = append(*notices, fmt.Sprintf("#chart-%s: payload at line %d mentions %q; update it by hand", c.Engine, c.Line, old))
			}
			continue
		}
		for ai, kv := range c.Attrs {
			if !knownChartChannels[kv.Key] {
				continue
			}
			refs := chartChannelValues(kv)
			changed := false
			for i, ref := range refs {
				if ref == old {
					refs[i] = new
					changed = true
				}
			}
			if changed {
				doc.Meta.Charts[ci].Attrs[ai].Value = strings.Join(refs, ",")
			}
		}
	}
}

func (doc *Document) renameColumnInNotesLinks(old, new string) {
	// With header=0, col= and {N} may be bare indexes; only names are renamed.
	for i := range doc.Meta.Notes {
		if v, ok := doc.Meta.Notes[i].Attr("col"); ok && v == old {
			doc.Meta.Notes[i].Attrs = upsertKV(doc.Meta.Notes[i].Attrs, "col", new)
		}
	}
	for i := range doc.Meta.Links {
		if v, ok := doc.Meta.Links[i].Attr("col"); ok && v == old {
			doc.Meta.Links[i].Attrs = upsertKV(doc.Meta.Links[i].Attrs, "col", new)
		}
	}
	for _, col := range doc.Meta.Columns {
		tmpl, ok := col.Attrs["link"]
		if !ok {
			continue
		}
		parts, err := parseLinkTemplate(tmpl)
		if err != nil {
			continue // link_template_malformed: leave it as written
		}
		changed := false
		for i := range parts {
			if parts[i].ref == old {
				parts[i].ref = new
				changed = true
			}
		}
		if changed {
			col.Attrs["link"] = formatLinkTemplate(parts)
		}
	}
}

func (doc *Document) renameColumnInSQL(old string, notices *[]string) {
	word := regexp.MustCompile(`\b` + regexp.QuoteMeta(old) + `\b`)
	for _, s := range doc.Meta.SQL {
		if word.MatchString(s.Payload) {
			*notices = append(*notices, fmt.Sprintf("#$%s at line %d mentions %s; SQL is not rewritten, update it by hand",
				s.Verb, s.Line, old))
		}
	}
}

// formatLinkTemplate is the inverse of parseLinkTemplate: a literal { is
// written back as {{.
func formatLinkTemplate(parts []templatePart) string {
	var b strings.Builder
	for _, p := range parts {
		if p.ref != "" {
			b.WriteString("{" + p.ref + "}")
			continue
		}
		b.WriteString(strings.ReplaceAll(p.lit, "{", "{{"))
	}
	return b.String()
}

func isFormulaIdent(s string) bool {
	if s == "" || formulaKeywords[strings.ToLower(s)] {
		return false
	}
	for i, c := range s {
		if (i == 0 && !isIdentStart(c)) || !isIdentPart(c) {
			return false
		}
	}
	return true
}

// renameFormulaRef replaces column references to old in a formula= payload,
// with the formula lexer's rules: string literals and function names (an
// identifier followed by "(") are left alone.
func renameFormulaRef(expr, old, new string) (string, bool) {
	r := []rune(expr)
	var b strings.Builder
	changed := false
	for i := 0; i < len(r); {
		c := r[i]
		switch {
		case c == '\'':
			j := i + 1
			for j < len(r) {
				if r[j] == '\'' {
					if j+1 < len(r) && r[j+1] == '\'' {
						j += 2
						continue
					}
					j++
					break
				}
				j++
			}
			b.WriteString(string(r[i:j]))
			i = j
		case isIdentStart(c):
			j := i
			for j < len(r) && isIdentPart(r[j]) {
				j++
			}
			ident := string(r[i:j])
			k := j
			for k < len(r) && (r[k] == ' ' || r[k] == '\t') {
				k++
			}
			if ident == old && (k >= len(r) || r[k] != '(') {
				b.WriteString(new)
				changed = true
			} else {
				b.WriteString(ident)
			}
			i = j
		case c >= '0' && c <= '9':
			// A number: digits and dots, so "1e5"-like runs never start an identifier.
			j := i
			for j < len(r) && (r[j] >= '0' && r[j] <= '9' || r[j] == '.') {
				j++
			}
			b.WriteString(string(r[i:j]))
			i = j
		default:
			b.WriteRune(c)
			i++
		}
	}
	return b.String(), changed
}

// RenameColumnFKs updates #fk from= / to= references to table.old after a
// column of that table was renamed.
func (p *Pack) RenameColumnFKs(table, old, new string) {
	if p == nil {
		return
	}
	ref, repl := table+"."+old, table+"."+new
	for i := range p.FKs {
		if p.FKs[i].From == ref {
			p.FKs[i].From = repl
		}
		if p.FKs[i].To == ref {
			p.FKs[i].To = repl
		}
	}
}
