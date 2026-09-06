package excsv

import (
	"math/big"
	"testing"
)

func bigFromInt(n int64) *big.Rat { return new(big.Rat).SetInt64(n) }

func evalFormulaStr(t *testing.T, expr string, env map[string]formulaValue) formulaValue {
	t.Helper()
	node, err := parseFormula(expr)
	if err != nil {
		t.Fatalf("parseFormula(%q): %v", expr, err)
	}
	v, err := node.eval(env)
	if err != nil {
		t.Fatalf("eval(%q): %v", expr, err)
	}
	return v
}

func numEnv(vals map[string]string) map[string]formulaValue {
	env := map[string]formulaValue{}
	for k, v := range vals {
		env[k] = formulaValueFromCell(v, "decimal")
	}
	return env
}

func TestFormulaArithmetic(t *testing.T) {
	env := numEnv(map[string]string{"price": "10.00", "quantity": "3"})
	v := evalFormulaStr(t, "price * quantity", env)
	if got := formatFormulaNumber(v.N, ""); got != "30.00" {
		t.Fatalf("price*quantity = %s, want 30.00", got)
	}

	env2 := numEnv(map[string]string{"price": "10.00", "cost": "6.00"})
	v2 := evalFormulaStr(t, "(price - cost) / price", env2)
	if got := formatFormulaNumber(v2.N, ""); got != "0.40" {
		t.Fatalf("margin = %s, want 0.40", got)
	}
}

func TestFormulaConcat(t *testing.T) {
	env := map[string]formulaValue{
		"first_name": fvStringVal("Ada"),
		"last_name":  fvStringVal("Lovelace"),
	}
	v := evalFormulaStr(t, "concat(first_name, ' ', last_name)", env)
	if v.Kind != fvString || v.S != "Ada Lovelace" {
		t.Fatalf("concat = %+v", v)
	}
}

func TestFormulaCaseWhen(t *testing.T) {
	env := numEnv(map[string]string{"amount": "150"})
	v := evalFormulaStr(t, "case when amount > 100 then 'high' else 'low' end", env)
	if v.Kind != fvString || v.S != "high" {
		t.Fatalf("case = %+v", v)
	}
}

func TestFormulaCoalesceRoundFloorCeil(t *testing.T) {
	env := map[string]formulaValue{"a": fvNullVal()}
	v := evalFormulaStr(t, "coalesce(a, 5)", env)
	if v.Kind != fvNumber || v.N.Cmp(bigFromInt(5)) != 0 {
		t.Fatalf("coalesce = %+v", v)
	}

	v2 := evalFormulaStr(t, "round(2.005, 2)", map[string]formulaValue{})
	if got := formatFormulaNumber(v2.N, ""); got != "2.01" {
		t.Fatalf("round(2.005,2) = %s, want 2.01", got)
	}

	v3 := evalFormulaStr(t, "floor(2.9)", map[string]formulaValue{})
	if got := formatFormulaNumber(v3.N, ""); got != "2.00" {
		t.Fatalf("floor(2.9) = %s, want 2.00", got)
	}

	v4 := evalFormulaStr(t, "ceil(2.1)", map[string]formulaValue{})
	if got := formatFormulaNumber(v4.N, ""); got != "3.00" {
		t.Fatalf("ceil(2.1) = %s, want 3.00", got)
	}
}

func TestFormulaUnknownFunctionRejected(t *testing.T) {
	if _, err := parseFormula("now()"); err == nil {
		t.Fatal("expected error for non-whitelisted function")
	}
}

func TestFormulaDoublePipeRejected(t *testing.T) {
	if _, err := parseFormula("a || b"); err == nil {
		t.Fatal("expected error for ||")
	}
}

func TestFormulaReferencedNames(t *testing.T) {
	node, err := parseFormula("(price - cost) / price + tax_rate")
	if err != nil {
		t.Fatal(err)
	}
	names := formulaReferencedNames(node)
	want := []string{"cost", "price", "tax_rate"}
	if len(names) != len(want) {
		t.Fatalf("names=%v", names)
	}
	for i := range want {
		if names[i] != want[i] {
			t.Fatalf("names=%v want=%v", names, want)
		}
	}
}
