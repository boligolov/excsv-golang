package excsv_test

import (
	"encoding/json"
	"strings"
	"testing"

	"github.com/boligolov/excsv-golang/pkg/excsv"
	excsvzip "github.com/boligolov/excsv-golang/pkg/excsv/zip"
)

const notesDoc = "#!excsv version=0.6 header=1 rows=3\n" +
	"#column name=id type=int role=id link=\"https://crm.example.com/o/{$}\"\n" +
	"#column name=amount type=decimal\n" +
	"#note row=2 col=amount author=alex@example.com text=\"Refund \"\"pending\"\"\" x-color=red\n" +
	"#note key=1 text=\"First order\"\n" +
	"#note col=amount text=\"Includes VAT\"\n" +
	"#link row=0 col=amount href=\"https://billing.example.com/inv/1\"\n" +
	"id,amount\n" +
	"3,30.00\n" +
	"1,10.00\n" +
	"2,20.00\n"

func TestNotesRoundTrip(t *testing.T) {
	doc := parseDoc(t, notesDoc)
	if len(doc.Meta.Notes) != 3 || len(doc.Meta.Links) != 1 {
		t.Fatalf("notes=%d links=%d", len(doc.Meta.Notes), len(doc.Meta.Links))
	}
	if got := doc.Meta.Notes[0].Text(); got != `Refund "pending"` {
		t.Fatalf("text=%q", got)
	}
	out, err := doc.SerializeCanonical()
	if err != nil {
		t.Fatal(err)
	}
	s := string(out)
	for _, want := range []string{
		`#note row=2 col=amount author=alex@example.com text="Refund ""pending""" x-color=red` + "\n",
		`#link row=0 col=amount href="https://billing.example.com/inv/1"` + "\n",
		`link=https://crm.example.com/o/{$}`,
	} {
		if !strings.Contains(s, want) {
			t.Fatalf("serialized output lacks %q:\n%s", want, s)
		}
	}
	// Recommended order: #column, #chart, then #note/#link, then the data.
	if strings.Index(s, "#note") < strings.LastIndex(s, "#column") || strings.Index(s, "#link") > strings.Index(s, "id,amount") {
		t.Fatalf("#note/#link out of the recommended order:\n%s", s)
	}
	again := parseDoc(t, s)
	if len(again.Meta.Notes) != 3 || len(again.Meta.Links) != 1 {
		t.Fatalf("round-trip lost lines:\n%s", s)
	}
}

func TestSortKeepsRowAnchors(t *testing.T) {
	doc := parseDoc(t, notesDoc)
	idx, err := doc.ColumnIndex("id")
	if err != nil {
		t.Fatal(err)
	}
	if err := doc.SortRows([]excsv.SortKey{{Index: idx}}); err != nil {
		t.Fatal(err)
	}
	// Before: rows 3,1,2. After: 1,2,3. The row=2 note was on id=2 (now row 1);
	// the row=0 link was on id=3 (now row 2); key=1 needs no change.
	if v, _ := doc.Meta.Notes[0].Attr("row"); v != "1" {
		t.Fatalf("note row=%s, want 1", v)
	}
	if v, _ := doc.Meta.Links[0].Attr("row"); v != "2" {
		t.Fatalf("link row=%s, want 2", v)
	}
	if v, _ := doc.Meta.Notes[1].Attr("key"); v != "1" {
		t.Fatalf("key-anchored note changed: %v", doc.Meta.Notes[1].Attrs)
	}
	notes := doc.ResolveNotes()
	if notes[0].Row != 1 || notes[1].Row != 0 {
		t.Fatalf("resolved rows = %d, %d", notes[0].Row, notes[1].Row)
	}
	if href, ok := doc.ResolveLinks().CellLink(2, "amount"); !ok || href != "https://billing.example.com/inv/1" {
		t.Fatalf("CellLink(2, amount) = %q, %v", href, ok)
	}
}

func TestNotesJSONMirror(t *testing.T) {
	doc := parseDoc(t, notesDoc)
	res, err := doc.ExportJSON(excsv.JSONExportOptions{})
	if err != nil {
		t.Fatal(err)
	}
	var got struct {
		Excsv   string           `json:"excsv"`
		Columns []map[string]any `json:"columns"`
		Notes   []map[string]any `json:"notes"`
		Links   []map[string]any `json:"links"`
	}
	if err := json.Unmarshal(res.Data, &got); err != nil {
		t.Fatal(err)
	}
	if got.Excsv != "0.6" {
		t.Fatalf("excsv=%q", got.Excsv)
	}
	if got.Columns[0]["link"] != "https://crm.example.com/o/{$}" {
		t.Fatalf("column link = %v", got.Columns[0]["link"])
	}
	if len(got.Notes) != 3 || len(got.Links) != 1 {
		t.Fatalf("notes=%v links=%v", got.Notes, got.Links)
	}
	first := got.Notes[0]
	if first["row"] != float64(2) || first["col"] != "amount" || first["x-color"] != "red" {
		t.Fatalf("note[0] = %v", first)
	}
	if got.Notes[1]["key"] != "1" {
		t.Fatalf("key must stay a string: %#v", got.Notes[1]["key"])
	}
	if got.Links[0]["href"] != "https://billing.example.com/inv/1" {
		t.Fatalf("link[0] = %v", got.Links[0])
	}
}

func TestNotesHeader0JSONIndex(t *testing.T) {
	doc := parseDoc(t, "#!excsv version=0.6 header=0 rows=1\n#column index=0 type=int\n#note row=0 col=0 text=x\n7\n")
	res, err := doc.ExportJSON(excsv.JSONExportOptions{})
	if err != nil {
		t.Fatal(err)
	}
	if !strings.Contains(string(res.Data), `"col":0`) {
		t.Fatalf("header=0 col must be an integer: %s", res.Data)
	}
}

func TestAddNoteRaisesVersion(t *testing.T) {
	doc := parseDoc(t, "#!excsv version=0.5 header=1 rows=1\n#column name=id type=int role=id\nid\n42\n")
	anchor, err := doc.RowAnchor(0)
	if err != nil {
		t.Fatal(err)
	}
	if anchor != (excsv.KV{Key: "key", Value: "42"}) {
		t.Fatalf("RowAnchor(0) = %+v, want key=42 (the table has an id column)", anchor)
	}
	if err := doc.AddNote([]excsv.KV{anchor, {Key: "text", Value: "hi"}}); err != nil {
		t.Fatal(err)
	}
	if doc.Header.Version != "0.6" || doc.Header.Fields["version"] != "0.6" {
		t.Fatalf("version=%q, a 0.5 file using #note must declare 0.6", doc.Header.Version)
	}
	if err := doc.AddNote([]excsv.KV{{Key: "col", Value: "id"}}); err == nil {
		t.Fatal("AddNote without text= must fail")
	}
}

func TestSetLinkReplacesSameCell(t *testing.T) {
	doc := parseDoc(t, notesDoc)
	attrs := []excsv.KV{{Key: "row", Value: "0"}, {Key: "col", Value: "amount"}, {Key: "href", Value: "https://new.example.com"}}
	if err := doc.SetLink(attrs); err != nil {
		t.Fatal(err)
	}
	if len(doc.Meta.Links) != 1 || doc.Meta.Links[0].Href() != "https://new.example.com" {
		t.Fatalf("links = %+v", doc.Meta.Links)
	}
	if err := doc.SetLink([]excsv.KV{{Key: "col", Value: "amount"}, {Key: "href", Value: "x"}}); err == nil {
		t.Fatal("SetLink without row=/key= must fail")
	}
}

func TestVersionCompatibility(t *testing.T) {
	for _, v := range []string{"0.2", "0.5", "0.6", "0.1"} {
		res, err := excsv.ParseBytes([]byte("#!excsv version="+v+" header=1 rows=1\nid\n1\n"), excsv.StrictOptions())
		if err != nil {
			t.Fatal(err)
		}
		for _, w := range res.Warnings {
			if w.Kind == excsv.ErrUnknownVersion {
				t.Fatalf("version=%s: an earlier version must read without unknown_version", v)
			}
		}
	}
	res, err := excsv.ParseBytes([]byte("#!excsv version=0.7 header=1 rows=1\nid\n1\n"), excsv.StrictOptions())
	if err != nil {
		t.Fatal(err)
	}
	if len(res.Warnings) != 1 || res.Warnings[0].Kind != excsv.ErrUnknownVersion {
		t.Fatalf("version=0.7: warnings=%v, want unknown_version", res.Warnings)
	}
}

func TestLinkSchemeAndEncoding(t *testing.T) {
	for u, want := range map[string]bool{
		"https://x": true, "HTTP://x": true, "mailto:a@b": true,
		"javascript:alert(1)": false, "file:///etc": false, "/relative": false, "": false, "1http://x": false,
	} {
		if got := excsv.IsSafeLinkScheme(u); got != want {
			t.Errorf("IsSafeLinkScheme(%q) = %v, want %v", u, got, want)
		}
	}
	if got := excsv.PercentEncodeLinkValue("a b/é~"); got != "a%20b%2F%C3%A9~" {
		t.Fatalf("PercentEncodeLinkValue = %q", got)
	}
}

func TestZipCommentPriority(t *testing.T) {
	inner := "#!excsv version=0.6 header=1 rows=1\n" +
		"#@note-to-self: misc\n" +
		"#column name=id type=int\n" +
		"#chart type=bar name=c x=id y=id\n" +
		"#note text=\"table note\"\n" +
		"#$dql: SELECT 1\n" +
		"#@source: unit-test\n" +
		"#%sum: 1\n" +
		"id\n1\n"
	z, err := excsvzip.Wrap([]byte(inner), "t.excsv", "")
	if err != nil {
		t.Fatal(err)
	}
	ins, err := excsvzip.Inspect("t.excsv.zip", z)
	if err != nil {
		t.Fatal(err)
	}
	lines := strings.Split(ins.Comment, "\n")
	var prefixes []string
	for _, l := range lines[1:] {
		prefixes = append(prefixes, strings.SplitN(l, " ", 2)[0])
	}
	want := "#@source:,#column,#%sum:,#@note-to-self:,#$dql:,#chart,#note"
	if got := strings.Join(prefixes, ","); got != want {
		t.Fatalf("comment order:\n got %s\nwant %s", got, want)
	}
}
