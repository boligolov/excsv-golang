package excsv

import (
	"fmt"
	"math/big"
	"sort"
	"strings"
)

// Formula language for #column formula= computed columns
// (implementation/columns.md#formula-language). Deliberately small and
// portable: no dialect selector, so any conforming tool can evaluate it.
//
// Grammar (case-insensitive keywords):
//
//	orExpr    := andExpr ('or' andExpr)*
//	andExpr   := notExpr ('and' notExpr)*
//	notExpr   := 'not' notExpr | cmpExpr
//	cmpExpr   := addExpr (('='|'<>'|'<'|'<='|'>'|'>=') addExpr)?
//	addExpr   := mulExpr (('+'|'-') mulExpr)*
//	mulExpr   := unary (('*'|'/'|'%') unary)*
//	unary     := '-' unary | primary
//	primary   := NUMBER | STRING | 'true' | 'false' | 'null'
//	           | 'case' ('when' orExpr 'then' orExpr)+ ['else' orExpr] 'end'
//	           | '(' orExpr ')' | IDENT ['(' (orExpr (',' orExpr)*)? ')']
//
// Function whitelist: abs round floor ceil coalesce nullif least greatest
// length lower upper trim substr concat.

// formulaFuncWhitelist is the closed set of callable functions.
var formulaFuncWhitelist = map[string]bool{
	"abs": true, "round": true, "floor": true, "ceil": true,
	"coalesce": true, "nullif": true, "least": true, "greatest": true,
	"length": true, "lower": true, "upper": true, "trim": true,
	"substr": true, "concat": true,
}

// ---- value model -----------------------------------------------------

type fvKind int

const (
	fvNull fvKind = iota
	fvBool
	fvNumber
	fvString
)

type formulaValue struct {
	Kind fvKind
	B    bool
	N    *big.Rat
	S    string
}

func fvNullVal() formulaValue             { return formulaValue{Kind: fvNull} }
func fvBoolVal(b bool) formulaValue       { return formulaValue{Kind: fvBool, B: b} }
func fvNumberVal(n *big.Rat) formulaValue { return formulaValue{Kind: fvNumber, N: n} }
func fvStringVal(s string) formulaValue   { return formulaValue{Kind: fvString, S: s} }

// FormulaValueFromCell converts a raw cell (already known non-null) into a
// formulaValue for evaluation, based on the stored column's declared type.
// Unknown/absent types are treated as strings, same as an ordinary column
// with no #column type= would read.
func formulaValueFromCell(v string, colType string) formulaValue {
	switch strings.ToLower(strings.TrimSpace(colType)) {
	case "int", "long", "float", "double", "decimal":
		if n, ok := new(big.Rat).SetString(strings.TrimSpace(v)); ok {
			return fvNumberVal(n)
		}
		return fvStringVal(v)
	case "boolean":
		switch strings.ToLower(strings.TrimSpace(v)) {
		case "true", "1":
			return fvBoolVal(true)
		case "false", "0":
			return fvBoolVal(false)
		default:
			return fvStringVal(v)
		}
	default:
		return fvStringVal(v)
	}
}

// ---- AST ---------------------------------------------------------------

type formulaNode interface {
	eval(env map[string]formulaValue) (formulaValue, error)
	collectRefs(set map[string]bool)
}

type litNode struct{ val formulaValue }

func (n litNode) eval(map[string]formulaValue) (formulaValue, error) { return n.val, nil }
func (n litNode) collectRefs(map[string]bool)                        {}

type colRefNode struct{ name string }

func (n colRefNode) eval(env map[string]formulaValue) (formulaValue, error) {
	v, ok := env[n.name]
	if !ok {
		return formulaValue{}, fmt.Errorf("unknown column reference: %s", n.name)
	}
	return v, nil
}
func (n colRefNode) collectRefs(set map[string]bool) { set[n.name] = true }

type unaryNode struct {
	op string
	x  formulaNode
}

func (n unaryNode) collectRefs(set map[string]bool) { n.x.collectRefs(set) }
func (n unaryNode) eval(env map[string]formulaValue) (formulaValue, error) {
	v, err := n.x.eval(env)
	if err != nil {
		return formulaValue{}, err
	}
	switch n.op {
	case "-":
		if v.Kind == fvNull {
			return fvNullVal(), nil
		}
		if v.Kind != fvNumber {
			return formulaValue{}, fmt.Errorf("unary - requires a number")
		}
		return fvNumberVal(new(big.Rat).Neg(v.N)), nil
	case "not":
		if v.Kind == fvNull {
			return fvNullVal(), nil
		}
		if v.Kind != fvBool {
			return formulaValue{}, fmt.Errorf("not requires a boolean")
		}
		return fvBoolVal(!v.B), nil
	}
	return formulaValue{}, fmt.Errorf("unknown unary operator %s", n.op)
}

type binNode struct {
	op   string
	l, r formulaNode
}

func (n binNode) collectRefs(set map[string]bool) {
	n.l.collectRefs(set)
	n.r.collectRefs(set)
}

func (n binNode) eval(env map[string]formulaValue) (formulaValue, error) {
	switch n.op {
	case "and":
		return evalAnd(env, n.l, n.r)
	case "or":
		return evalOr(env, n.l, n.r)
	}
	l, err := n.l.eval(env)
	if err != nil {
		return formulaValue{}, err
	}
	r, err := n.r.eval(env)
	if err != nil {
		return formulaValue{}, err
	}
	switch n.op {
	case "+", "-", "*", "/", "%":
		return evalArith(n.op, l, r)
	case "=", "<>", "<", "<=", ">", ">=":
		return evalCompare(n.op, l, r)
	}
	return formulaValue{}, fmt.Errorf("unknown binary operator %s", n.op)
}

// evalAnd / evalOr implement standard SQL three-valued logic: a null
// operand only makes the result null when it isn't already decided by the
// other (true OR x = true; false AND x = false, regardless of x).
func evalAnd(env map[string]formulaValue, ln, rn formulaNode) (formulaValue, error) {
	l, err := ln.eval(env)
	if err != nil {
		return formulaValue{}, err
	}
	if l.Kind == fvBool && !l.B {
		return fvBoolVal(false), nil
	}
	r, err := rn.eval(env)
	if err != nil {
		return formulaValue{}, err
	}
	if r.Kind == fvBool && !r.B {
		return fvBoolVal(false), nil
	}
	if l.Kind == fvNull || r.Kind == fvNull {
		return fvNullVal(), nil
	}
	if l.Kind != fvBool || r.Kind != fvBool {
		return formulaValue{}, fmt.Errorf("and requires boolean operands")
	}
	return fvBoolVal(l.B && r.B), nil
}

func evalOr(env map[string]formulaValue, ln, rn formulaNode) (formulaValue, error) {
	l, err := ln.eval(env)
	if err != nil {
		return formulaValue{}, err
	}
	if l.Kind == fvBool && l.B {
		return fvBoolVal(true), nil
	}
	r, err := rn.eval(env)
	if err != nil {
		return formulaValue{}, err
	}
	if r.Kind == fvBool && r.B {
		return fvBoolVal(true), nil
	}
	if l.Kind == fvNull || r.Kind == fvNull {
		return fvNullVal(), nil
	}
	if l.Kind != fvBool || r.Kind != fvBool {
		return formulaValue{}, fmt.Errorf("or requires boolean operands")
	}
	return fvBoolVal(l.B || r.B), nil
}

func evalArith(op string, l, r formulaValue) (formulaValue, error) {
	if l.Kind == fvNull || r.Kind == fvNull {
		return fvNullVal(), nil
	}
	if l.Kind != fvNumber || r.Kind != fvNumber {
		return formulaValue{}, fmt.Errorf("%s requires numeric operands", op)
	}
	out := new(big.Rat)
	switch op {
	case "+":
		out.Add(l.N, r.N)
	case "-":
		out.Sub(l.N, r.N)
	case "*":
		out.Mul(l.N, r.N)
	case "/":
		if r.N.Sign() == 0 {
			return formulaValue{}, fmt.Errorf("division by zero")
		}
		out.Quo(l.N, r.N)
	case "%":
		if r.N.Sign() == 0 {
			return formulaValue{}, fmt.Errorf("modulo by zero")
		}
		if !l.N.IsInt() || !r.N.IsInt() {
			return formulaValue{}, fmt.Errorf("%% requires integer operands")
		}
		var m big.Int
		m.Mod(l.N.Num(), r.N.Num())
		out.SetInt(&m)
	}
	return fvNumberVal(out), nil
}

func evalCompare(op string, l, r formulaValue) (formulaValue, error) {
	if l.Kind == fvNull || r.Kind == fvNull {
		return fvNullVal(), nil
	}
	cmp, err := compareFormulaValues(l, r)
	if err != nil {
		return formulaValue{}, err
	}
	switch op {
	case "=":
		return fvBoolVal(cmp == 0), nil
	case "<>":
		return fvBoolVal(cmp != 0), nil
	case "<":
		return fvBoolVal(cmp < 0), nil
	case "<=":
		return fvBoolVal(cmp <= 0), nil
	case ">":
		return fvBoolVal(cmp > 0), nil
	case ">=":
		return fvBoolVal(cmp >= 0), nil
	}
	return formulaValue{}, fmt.Errorf("unknown comparison %s", op)
}

func compareFormulaValues(l, r formulaValue) (int, error) {
	switch {
	case l.Kind == fvNumber && r.Kind == fvNumber:
		return l.N.Cmp(r.N), nil
	case l.Kind == fvString && r.Kind == fvString:
		return strings.Compare(l.S, r.S), nil
	case l.Kind == fvBool && r.Kind == fvBool:
		if l.B == r.B {
			return 0, nil
		}
		if !l.B {
			return -1, nil
		}
		return 1, nil
	default:
		return 0, fmt.Errorf("cannot compare mismatched types")
	}
}

type caseWhenClause struct {
	when, then formulaNode
}

type caseNode struct {
	whens []caseWhenClause
	els   formulaNode
}

func (n caseNode) collectRefs(set map[string]bool) {
	for _, w := range n.whens {
		w.when.collectRefs(set)
		w.then.collectRefs(set)
	}
	if n.els != nil {
		n.els.collectRefs(set)
	}
}

func (n caseNode) eval(env map[string]formulaValue) (formulaValue, error) {
	for _, w := range n.whens {
		cond, err := w.when.eval(env)
		if err != nil {
			return formulaValue{}, err
		}
		if cond.Kind == fvBool && cond.B {
			return w.then.eval(env)
		}
	}
	if n.els != nil {
		return n.els.eval(env)
	}
	return fvNullVal(), nil
}

type callNode struct {
	name string
	args []formulaNode
}

func (n callNode) collectRefs(set map[string]bool) {
	for _, a := range n.args {
		a.collectRefs(set)
	}
}

func (n callNode) eval(env map[string]formulaValue) (formulaValue, error) {
	args := make([]formulaValue, len(n.args))
	for i, a := range n.args {
		v, err := a.eval(env)
		if err != nil {
			return formulaValue{}, err
		}
		args[i] = v
	}
	return callFormulaFunc(n.name, args)
}

func callFormulaFunc(name string, args []formulaValue) (formulaValue, error) {
	switch name {
	case "abs":
		if err := arity(name, args, 1, 1); err != nil {
			return formulaValue{}, err
		}
		if args[0].Kind == fvNull {
			return fvNullVal(), nil
		}
		if args[0].Kind != fvNumber {
			return formulaValue{}, fmt.Errorf("abs() requires a number")
		}
		return fvNumberVal(new(big.Rat).Abs(args[0].N)), nil

	case "round":
		if err := arity(name, args, 1, 2); err != nil {
			return formulaValue{}, err
		}
		return roundFunc(args)

	case "floor", "ceil":
		if err := arity(name, args, 1, 1); err != nil {
			return formulaValue{}, err
		}
		if args[0].Kind == fvNull {
			return fvNullVal(), nil
		}
		if args[0].Kind != fvNumber {
			return formulaValue{}, fmt.Errorf("%s() requires a number", name)
		}
		return fvNumberVal(ratFloorCeil(args[0].N, name == "ceil")), nil

	case "coalesce":
		if err := arity(name, args, 1, -1); err != nil {
			return formulaValue{}, err
		}
		for _, a := range args {
			if a.Kind != fvNull {
				return a, nil
			}
		}
		return fvNullVal(), nil

	case "nullif":
		if err := arity(name, args, 2, 2); err != nil {
			return formulaValue{}, err
		}
		if args[0].Kind == fvNull || args[1].Kind == fvNull {
			return args[0], nil
		}
		cmp, err := compareFormulaValues(args[0], args[1])
		if err != nil {
			return formulaValue{}, err
		}
		if cmp == 0 {
			return fvNullVal(), nil
		}
		return args[0], nil

	case "least", "greatest":
		if err := arity(name, args, 1, -1); err != nil {
			return formulaValue{}, err
		}
		return leastGreatest(args, name == "greatest")

	case "length":
		if err := arity(name, args, 1, 1); err != nil {
			return formulaValue{}, err
		}
		if args[0].Kind == fvNull {
			return fvNullVal(), nil
		}
		if args[0].Kind != fvString {
			return formulaValue{}, fmt.Errorf("length() requires a string")
		}
		return fvNumberVal(new(big.Rat).SetInt64(int64(len([]rune(args[0].S))))), nil

	case "lower", "upper", "trim":
		if err := arity(name, args, 1, 1); err != nil {
			return formulaValue{}, err
		}
		if args[0].Kind == fvNull {
			return fvNullVal(), nil
		}
		if args[0].Kind != fvString {
			return formulaValue{}, fmt.Errorf("%s() requires a string", name)
		}
		switch name {
		case "lower":
			return fvStringVal(strings.ToLower(args[0].S)), nil
		case "upper":
			return fvStringVal(strings.ToUpper(args[0].S)), nil
		default:
			return fvStringVal(strings.TrimSpace(args[0].S)), nil
		}

	case "substr":
		if err := arity(name, args, 2, 3); err != nil {
			return formulaValue{}, err
		}
		return substrFunc(args)

	case "concat":
		if err := arity(name, args, 1, -1); err != nil {
			return formulaValue{}, err
		}
		var sb strings.Builder
		for _, a := range args {
			if a.Kind == fvNull {
				return fvNullVal(), nil
			}
			sb.WriteString(formulaValueToDisplayString(a))
		}
		return fvStringVal(sb.String()), nil
	}
	return formulaValue{}, fmt.Errorf("unknown function %s()", name)
}

func arity(name string, args []formulaValue, min, max int) error {
	if len(args) < min || (max >= 0 && len(args) > max) {
		return fmt.Errorf("%s() takes the wrong number of arguments", name)
	}
	return nil
}

func roundFunc(args []formulaValue) (formulaValue, error) {
	if args[0].Kind == fvNull {
		return fvNullVal(), nil
	}
	if args[0].Kind != fvNumber {
		return formulaValue{}, fmt.Errorf("round() requires a number")
	}
	prec := 0
	if len(args) == 2 {
		if args[1].Kind == fvNull {
			return fvNullVal(), nil
		}
		if args[1].Kind != fvNumber || !args[1].N.IsInt() {
			return formulaValue{}, fmt.Errorf("round() precision must be an integer")
		}
		prec = int(args[1].N.Num().Int64())
	}
	return fvNumberVal(ratRound(args[0].N, prec)), nil
}

// ratRound rounds n to prec decimal digits, half-away-from-zero.
func ratRound(n *big.Rat, prec int) *big.Rat {
	scale := new(big.Rat).SetInt(new(big.Int).Exp(big.NewInt(10), big.NewInt(int64(absInt(prec))), nil))
	scaled := new(big.Rat)
	if prec >= 0 {
		scaled.Mul(n, scale)
	} else {
		scaled.Quo(n, scale)
	}
	half := big.NewRat(1, 2)
	if scaled.Sign() >= 0 {
		scaled.Add(scaled, half)
	} else {
		scaled.Sub(scaled, half)
	}
	var rounded big.Int
	q := new(big.Int).Quo(scaled.Num(), scaled.Denom())
	rounded.Set(q)
	out := new(big.Rat).SetInt(&rounded)
	if prec >= 0 {
		out.Quo(out, scale)
	} else {
		out.Mul(out, scale)
	}
	return out
}

func ratFloorCeil(n *big.Rat, ceil bool) *big.Rat {
	q := new(big.Int)
	m := new(big.Int)
	q.QuoRem(n.Num(), n.Denom(), m)
	if m.Sign() == 0 {
		return new(big.Rat).SetInt(q)
	}
	if !ceil {
		if n.Sign() < 0 {
			q.Sub(q, big.NewInt(1))
		}
	} else {
		if n.Sign() > 0 {
			q.Add(q, big.NewInt(1))
		}
	}
	return new(big.Rat).SetInt(q)
}

func absInt(n int) int {
	if n < 0 {
		return -n
	}
	return n
}

func leastGreatest(args []formulaValue, greatest bool) (formulaValue, error) {
	best := formulaValue{}
	have := false
	for _, a := range args {
		if a.Kind == fvNull {
			return fvNullVal(), nil
		}
		if !have {
			best = a
			have = true
			continue
		}
		cmp, err := compareFormulaValues(a, best)
		if err != nil {
			return formulaValue{}, err
		}
		if (greatest && cmp > 0) || (!greatest && cmp < 0) {
			best = a
		}
	}
	return best, nil
}

func substrFunc(args []formulaValue) (formulaValue, error) {
	if args[0].Kind == fvNull {
		return fvNullVal(), nil
	}
	if args[0].Kind != fvString {
		return formulaValue{}, fmt.Errorf("substr() requires a string")
	}
	if args[1].Kind != fvNumber || !args[1].N.IsInt() {
		return formulaValue{}, fmt.Errorf("substr() start must be an integer")
	}
	runes := []rune(args[0].S)
	start := int(args[1].N.Num().Int64())
	if start < 1 {
		start = 1
	}
	if start > len(runes)+1 {
		return fvStringVal(""), nil
	}
	end := len(runes) + 1
	if len(args) == 3 {
		if args[2].Kind == fvNull {
			return fvNullVal(), nil
		}
		if args[2].Kind != fvNumber || !args[2].N.IsInt() {
			return formulaValue{}, fmt.Errorf("substr() length must be an integer")
		}
		length := int(args[2].N.Num().Int64())
		if length < 0 {
			length = 0
		}
		end = start + length
		if end > len(runes)+1 {
			end = len(runes) + 1
		}
	}
	return fvStringVal(string(runes[start-1 : end-1])), nil
}

func formulaValueToDisplayString(v formulaValue) string {
	switch v.Kind {
	case fvString:
		return v.S
	case fvBool:
		if v.B {
			return "true"
		}
		return "false"
	case fvNumber:
		return formatFormulaNumber(v.N, "")
	default:
		return ""
	}
}

// ---- formatting for materialize ---------------------------------------

// formatFormulaResult renders a computed value as the text cell that goes
// into the data section for the given column type/format.
func formatFormulaResult(colType, format string, v formulaValue, nullMarker string) (string, error) {
	if v.Kind == fvNull {
		return nullMarker, nil
	}
	ct := strings.ToLower(strings.TrimSpace(colType))
	switch ct {
	case "int", "long":
		if v.Kind != fvNumber || !v.N.IsInt() {
			return "", fmt.Errorf("formula result is not an integer for type=%s", ct)
		}
		return v.N.Num().String(), nil
	case "float", "double", "decimal", "":
		if v.Kind != fvNumber {
			return formulaValueToDisplayString(v), nil
		}
		return formatFormulaNumber(v.N, format), nil
	case "boolean":
		if v.Kind != fvBool {
			return "", fmt.Errorf("formula result is not a boolean for type=boolean")
		}
		return formulaValueToDisplayString(v), nil
	default:
		return formulaValueToDisplayString(v), nil
	}
}

// formatFormulaNumber renders a big.Rat without float rounding, trimming to
// a reasonable number of decimal digits (money-style: at least 2, no
// trailing zeros beyond that) unless format= pins an exact pattern such as
// "0.00".
func formatFormulaNumber(n *big.Rat, format string) string {
	if decimals, ok := decimalsFromFormat(format); ok {
		return n.FloatString(decimals)
	}
	s := n.FloatString(10)
	s = strings.TrimRight(s, "0")
	s = strings.TrimSuffix(s, ".")
	if !strings.Contains(s, ".") {
		return s + ".00"
	}
	parts := strings.SplitN(s, ".", 2)
	if len(parts[1]) == 1 {
		return s + "0"
	}
	return s
}

// decimalsFromFormat reads a "0.00"-style format hint for its decimal count.
func decimalsFromFormat(format string) (int, bool) {
	format = strings.TrimSpace(format)
	if format == "" || !strings.Contains(format, ".") {
		return 0, false
	}
	parts := strings.SplitN(format, ".", 2)
	for _, ch := range parts[1] {
		if ch != '0' && ch != '#' {
			return 0, false
		}
	}
	return len(parts[1]), true
}

// ---- lexer --------------------------------------------------------------

type ftokKind int

const (
	ftEOF ftokKind = iota
	ftNumber
	ftString
	ftIdent
	ftOp
	ftLParen
	ftRParen
	ftComma
)

type ftoken struct {
	kind ftokKind
	text string
}

func lexFormula(s string) ([]ftoken, error) {
	var out []ftoken
	r := []rune(s)
	i, n := 0, len(r)
	for i < n {
		c := r[i]
		switch {
		case c == ' ' || c == '\t' || c == '\n' || c == '\r':
			i++
		case c == '(':
			out = append(out, ftoken{ftLParen, "("})
			i++
		case c == ')':
			out = append(out, ftoken{ftRParen, ")"})
			i++
		case c == ',':
			out = append(out, ftoken{ftComma, ","})
			i++
		case c == '\'':
			j := i + 1
			var sb strings.Builder
			closed := false
			for j < n {
				if r[j] == '\'' {
					if j+1 < n && r[j+1] == '\'' {
						sb.WriteRune('\'')
						j += 2
						continue
					}
					closed = true
					j++
					break
				}
				sb.WriteRune(r[j])
				j++
			}
			if !closed {
				return nil, fmt.Errorf("unterminated string literal")
			}
			out = append(out, ftoken{ftString, sb.String()})
			i = j
		case c >= '0' && c <= '9':
			j := i
			for j < n && (r[j] >= '0' && r[j] <= '9' || r[j] == '.') {
				j++
			}
			out = append(out, ftoken{ftNumber, string(r[i:j])})
			i = j
		case isIdentStart(c):
			j := i
			for j < n && isIdentPart(r[j]) {
				j++
			}
			out = append(out, ftoken{ftIdent, string(r[i:j])})
			i = j
		case c == '<':
			if i+1 < n && (r[i+1] == '>' || r[i+1] == '=') {
				out = append(out, ftoken{ftOp, string(r[i : i+2])})
				i += 2
			} else {
				out = append(out, ftoken{ftOp, "<"})
				i++
			}
		case c == '>':
			if i+1 < n && r[i+1] == '=' {
				out = append(out, ftoken{ftOp, ">="})
				i += 2
			} else {
				out = append(out, ftoken{ftOp, ">"})
				i++
			}
		case c == '=' || c == '+' || c == '-' || c == '*' || c == '/' || c == '%':
			out = append(out, ftoken{ftOp, string(c)})
			i++
		case c == '|':
			return nil, fmt.Errorf("'||' is not supported; use concat(...)")
		default:
			return nil, fmt.Errorf("unexpected character %q", c)
		}
	}
	out = append(out, ftoken{ftEOF, ""})
	return out, nil
}

func isIdentStart(c rune) bool {
	return c == '_' || (c >= 'a' && c <= 'z') || (c >= 'A' && c <= 'Z')
}

func isIdentPart(c rune) bool {
	return isIdentStart(c) || (c >= '0' && c <= '9')
}

// ---- parser --------------------------------------------------------------

type formulaParser struct {
	toks []ftoken
	pos  int
}

// parseFormula parses a formula= payload into an evaluatable AST.
func parseFormula(expr string) (formulaNode, error) {
	toks, err := lexFormula(expr)
	if err != nil {
		return nil, fmt.Errorf("formula_parse_error: %w", err)
	}
	p := &formulaParser{toks: toks}
	node, err := p.parseOr()
	if err != nil {
		return nil, fmt.Errorf("formula_parse_error: %w", err)
	}
	if p.cur().kind != ftEOF {
		return nil, fmt.Errorf("formula_parse_error: unexpected trailing input %q", p.cur().text)
	}
	return node, nil
}

func (p *formulaParser) cur() ftoken { return p.toks[p.pos] }
func (p *formulaParser) advance() ftoken {
	t := p.toks[p.pos]
	if p.pos < len(p.toks)-1 {
		p.pos++
	}
	return t
}

func (p *formulaParser) isKeyword(kw string) bool {
	t := p.cur()
	return t.kind == ftIdent && strings.EqualFold(t.text, kw)
}

func (p *formulaParser) parseOr() (formulaNode, error) {
	left, err := p.parseAnd()
	if err != nil {
		return nil, err
	}
	for p.isKeyword("or") {
		p.advance()
		right, err := p.parseAnd()
		if err != nil {
			return nil, err
		}
		left = binNode{op: "or", l: left, r: right}
	}
	return left, nil
}

func (p *formulaParser) parseAnd() (formulaNode, error) {
	left, err := p.parseNot()
	if err != nil {
		return nil, err
	}
	for p.isKeyword("and") {
		p.advance()
		right, err := p.parseNot()
		if err != nil {
			return nil, err
		}
		left = binNode{op: "and", l: left, r: right}
	}
	return left, nil
}

func (p *formulaParser) parseNot() (formulaNode, error) {
	if p.isKeyword("not") {
		p.advance()
		x, err := p.parseNot()
		if err != nil {
			return nil, err
		}
		return unaryNode{op: "not", x: x}, nil
	}
	return p.parseCompare()
}

func (p *formulaParser) parseCompare() (formulaNode, error) {
	left, err := p.parseAdd()
	if err != nil {
		return nil, err
	}
	if t := p.cur(); t.kind == ftOp {
		switch t.text {
		case "=", "<>", "<", "<=", ">", ">=":
			p.advance()
			right, err := p.parseAdd()
			if err != nil {
				return nil, err
			}
			return binNode{op: t.text, l: left, r: right}, nil
		}
	}
	return left, nil
}

func (p *formulaParser) parseAdd() (formulaNode, error) {
	left, err := p.parseMul()
	if err != nil {
		return nil, err
	}
	for {
		t := p.cur()
		if t.kind == ftOp && (t.text == "+" || t.text == "-") {
			p.advance()
			right, err := p.parseMul()
			if err != nil {
				return nil, err
			}
			left = binNode{op: t.text, l: left, r: right}
			continue
		}
		break
	}
	return left, nil
}

func (p *formulaParser) parseMul() (formulaNode, error) {
	left, err := p.parseUnary()
	if err != nil {
		return nil, err
	}
	for {
		t := p.cur()
		if t.kind == ftOp && (t.text == "*" || t.text == "/" || t.text == "%") {
			p.advance()
			right, err := p.parseUnary()
			if err != nil {
				return nil, err
			}
			left = binNode{op: t.text, l: left, r: right}
			continue
		}
		break
	}
	return left, nil
}

func (p *formulaParser) parseUnary() (formulaNode, error) {
	if t := p.cur(); t.kind == ftOp && t.text == "-" {
		p.advance()
		x, err := p.parseUnary()
		if err != nil {
			return nil, err
		}
		return unaryNode{op: "-", x: x}, nil
	}
	return p.parsePrimary()
}

func (p *formulaParser) parsePrimary() (formulaNode, error) {
	t := p.cur()
	switch t.kind {
	case ftNumber:
		p.advance()
		n, ok := new(big.Rat).SetString(t.text)
		if !ok {
			return nil, fmt.Errorf("invalid number literal %q", t.text)
		}
		return litNode{val: fvNumberVal(n)}, nil
	case ftString:
		p.advance()
		return litNode{val: fvStringVal(t.text)}, nil
	case ftLParen:
		p.advance()
		inner, err := p.parseOr()
		if err != nil {
			return nil, err
		}
		if p.cur().kind != ftRParen {
			return nil, fmt.Errorf("expected )")
		}
		p.advance()
		return inner, nil
	case ftIdent:
		switch strings.ToLower(t.text) {
		case "true":
			p.advance()
			return litNode{val: fvBoolVal(true)}, nil
		case "false":
			p.advance()
			return litNode{val: fvBoolVal(false)}, nil
		case "null":
			p.advance()
			return litNode{val: fvNullVal()}, nil
		case "case":
			return p.parseCase()
		}
		p.advance()
		name := t.text
		if p.cur().kind == ftLParen {
			p.advance()
			var args []formulaNode
			if p.cur().kind != ftRParen {
				for {
					arg, err := p.parseOr()
					if err != nil {
						return nil, err
					}
					args = append(args, arg)
					if p.cur().kind == ftComma {
						p.advance()
						continue
					}
					break
				}
			}
			if p.cur().kind != ftRParen {
				return nil, fmt.Errorf("expected ) after arguments to %s(", name)
			}
			p.advance()
			lname := strings.ToLower(name)
			if !formulaFuncWhitelist[lname] {
				return nil, fmt.Errorf("function %s() is not in the formula whitelist", name)
			}
			return callNode{name: lname, args: args}, nil
		}
		return colRefNode{name: name}, nil
	}
	return nil, fmt.Errorf("unexpected token %q", t.text)
}

func (p *formulaParser) parseCase() (formulaNode, error) {
	p.advance() // 'case'
	var whens []caseWhenClause
	for p.isKeyword("when") {
		p.advance()
		cond, err := p.parseOr()
		if err != nil {
			return nil, err
		}
		if !p.isKeyword("then") {
			return nil, fmt.Errorf("expected then")
		}
		p.advance()
		then, err := p.parseOr()
		if err != nil {
			return nil, err
		}
		whens = append(whens, caseWhenClause{when: cond, then: then})
	}
	if len(whens) == 0 {
		return nil, fmt.Errorf("case requires at least one when clause")
	}
	var els formulaNode
	if p.isKeyword("else") {
		p.advance()
		e, err := p.parseOr()
		if err != nil {
			return nil, err
		}
		els = e
	}
	if !p.isKeyword("end") {
		return nil, fmt.Errorf("expected end")
	}
	p.advance()
	return caseNode{whens: whens, els: els}, nil
}

// formulaReferencedNames returns the sorted, de-duplicated set of bare
// column names a parsed formula reads.
func formulaReferencedNames(n formulaNode) []string {
	set := map[string]bool{}
	n.collectRefs(set)
	out := make([]string, 0, len(set))
	for name := range set {
		out = append(out, name)
	}
	sort.Strings(out)
	return out
}
