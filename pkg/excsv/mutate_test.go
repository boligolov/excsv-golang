package excsv_test

import (
	"strings"
	"testing"

	"github.com/boligolov/excsv-golang/pkg/excsv"
)

func TestSetFileMeta(t *testing.T) {
	res, err := excsv.ParseBytes([]byte("#!excsv version=0.2\n#@author: old\nid\n1\n"), excsv.StrictOptions())
	if err != nil {
		t.Fatal(err)
	}
	res.Doc.SetFileMeta("author", "new@example.com")
	if res.Doc.MetaMap()["author"] != "new@example.com" {
		t.Fatalf("author=%q", res.Doc.MetaMap()["author"])
	}
	out, err := res.Doc.SerializeCanonical()
	if err != nil {
		t.Fatal(err)
	}
	if !strings.Contains(string(out), "#@author: new@example.com") {
		t.Fatalf("serialize:\n%s", out)
	}
}

func TestHumanComments(t *testing.T) {
	res, err := excsv.ParseBytes([]byte("#!excsv version=0.2\nid\n1\n"), excsv.StrictOptions())
	if err != nil {
		t.Fatal(err)
	}
	res.Doc.AddHumanComment("note one")
	res.Doc.AddHumanComment("## already prefixed")
	out, err := res.Doc.SerializeCanonical()
	if err != nil {
		t.Fatal(err)
	}
	text := string(out)
	if !strings.Contains(text, "## note one") {
		t.Fatalf("missing added comment:\n%s", text)
	}
	if !strings.Contains(text, "## already prefixed") {
		t.Fatalf("missing prefixed comment:\n%s", text)
	}
	if !res.Doc.RemoveHumanComment(0) {
		t.Fatal("remove index 0")
	}
	out, err = res.Doc.SerializeCanonical()
	if err != nil {
		t.Fatal(err)
	}
	if strings.Contains(string(out), "## note one") {
		t.Fatalf("comment should be removed:\n%s", out)
	}
}

func TestSetSQL(t *testing.T) {
	src := "#!excsv version=0.2\n#$ddl: OLD\nid,amount\n1,10\n"
	res, err := excsv.ParseBytes([]byte(src), excsv.StrictOptions())
	if err != nil {
		t.Fatal(err)
	}
	if err := res.Doc.SetSQL("ddl", "CREATE TABLE t (id INT)"); err != nil {
		t.Fatal(err)
	}
	if len(res.Doc.Meta.SQL) != 1 || res.Doc.Meta.SQL[0].Payload != "CREATE TABLE t (id INT)" {
		t.Fatalf("sql=%+v", res.Doc.Meta.SQL)
	}
	if err := res.Doc.SetSQL("dql", "SELECT 1"); err != nil {
		t.Fatal(err)
	}
	if len(res.Doc.Meta.SQL) != 2 {
		t.Fatalf("want 2 statements, got %d", len(res.Doc.Meta.SQL))
	}
}
