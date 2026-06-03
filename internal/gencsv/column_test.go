package gencsv

import "testing"

func TestParseColumnSpec(t *testing.T) {
	tests := []struct {
		raw     string
		name    string
		typ     ColumnType
		nulls   bool
		wantErr bool
	}{
		{"a,int", "a", TypeInt, false, false},
		{"b,string", "b", TypeString, false, false},
		{"c,date,nulls", "c", TypeDate, true, false},
		{"c,date,some nulls here", "c", TypeDate, true, false},
		{"x,null", "x", TypeNull, false, false},
		{"y,bool", "y", TypeBoolean, false, false},
		{"z,float", "z", TypeFloat, false, false},
		{"", "", 0, false, true},
		{"onlyname", "", 0, false, true},
		{"n,unknown", "", 0, false, true},
	}
	for _, tt := range tests {
		spec, err := ParseColumnSpec(tt.raw)
		if tt.wantErr {
			if err == nil {
				t.Fatalf("ParseColumnSpec(%q): want error", tt.raw)
			}
			continue
		}
		if err != nil {
			t.Fatalf("ParseColumnSpec(%q): %v", tt.raw, err)
		}
		if spec.Name != tt.name || spec.Type != tt.typ || spec.Nulls != tt.nulls {
			t.Fatalf("ParseColumnSpec(%q) = %+v; want name=%s type=%d nulls=%v", tt.raw, spec, tt.name, tt.typ, tt.nulls)
		}
	}
}
