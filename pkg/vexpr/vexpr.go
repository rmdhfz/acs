// Package vexpr = bahasa ekspresi kecil untuk virtual parameter: nilai turunan yang dihitung dari parameter perangkat
// (mis. redaman dari satuan 0,001 dBm, uptime dalam hari, atau "parameter pertama yang terisi" lintas TR-098/TR-181).
//
// Sengaja KECIL dan tertutup, bukan bahasa pemrograman: tanpa loop, rekursi, variabel, atau akses ke luar ekspresi. Setiap
// ekspresi pasti berhenti dan biayanya dibatasi (panjang, jumlah node, kedalaman, panjang string hasil), sehingga aman dijalankan
// pada data tenant mana pun. Nilai parameter diambil lewat callback; paket ini tidak mengenal database.
//
// Sintaks:
//
//	{Nama.Parameter.TR069}            nilai parameter (null bila belum dilaporkan)
//	123  -1.5  'teks'  "teks"         literal
//	+ - * / %   == != < <= > >=   and or not   ( )
//	fungsi(arg, ...)                  lihat functions di bawah
//
// Nilai: angka, teks, atau null. Aritmetika atas nilai non-angka/null menghasilkan null (pembagian nol juga null). Perbandingan
// memakai angka bila kedua sisi angka, selain itu teks; perbandingan dengan null salah, kecuali != (benar bila hanya satu sisi
// null). Hasil boolean = 1 atau 0.
package vexpr

import (
	"fmt"
	"math"
	"strconv"
	"strings"
	"unicode"
	"unicode/utf8"
)

// Batas biaya ekspresi.
const (
	MaxExprLen   = 1000 // karakter sumber
	MaxNodes     = 200
	MaxDepth     = 20
	MaxRefs      = 20
	MaxResultLen = 1024 // rune; hasil/antara yang lebih panjang dipotong
	maxStrLen    = 4096 // batas string antara (concat/replace) agar memori terbatas
)

// ParseError = kesalahan sintaks dengan posisi (rune ke-Pos, mulai 0) agar UI dapat menandai.
type ParseError struct {
	Pos int
	Msg string
}

func (e *ParseError) Error() string { return fmt.Sprintf("posisi %d: %s", e.Pos, e.Msg) }

// Value = hasil evaluasi.
type Value struct {
	kind byte // 0 null, 'n' angka, 's' teks
	num  float64
	str  string
}

func Null() Value              { return Value{} }
func Num(f float64) Value      { return Value{kind: 'n', num: f} }
func Str(s string) Value       { return Value{kind: 's', str: s} }
func (v Value) IsNull() bool   { return v.kind == 0 }
func (v Value) IsNum() bool    { return v.kind == 'n' }
func (v Value) IsString() bool { return v.kind == 's' }

// String = representasi untuk disimpan/ditampilkan; null = "".
func (v Value) String() string {
	switch v.kind {
	case 'n':
		return formatNum(v.num)
	case 's':
		return v.str
	}
	return ""
}

func formatNum(f float64) string {
	if math.IsNaN(f) || math.IsInf(f, 0) {
		return ""
	}
	// buang derau floating point (0.1+0.2) tanpa menghilangkan presisi bermakna
	f = math.Round(f*1e9) / 1e9
	return strconv.FormatFloat(f, 'f', -1, 64)
}

// asNum: angka murni, atau teks yang berbentuk angka.
func (v Value) asNum() (float64, bool) {
	switch v.kind {
	case 'n':
		return v.num, !math.IsNaN(v.num) && !math.IsInf(v.num, 0)
	case 's':
		f, err := strconv.ParseFloat(strings.TrimSpace(v.str), 64)
		if err != nil || math.IsNaN(f) || math.IsInf(f, 0) {
			return 0, false
		}
		return f, true
	}
	return 0, false
}

// Truthy: null, 0, "", "0", "false" = salah; selain itu benar (dipakai sebagai hasil kondisi).
func (v Value) Truthy() bool { return v.truthy() }

func (v Value) truthy() bool {
	switch v.kind {
	case 'n':
		return v.num != 0
	case 's':
		s := strings.ToLower(strings.TrimSpace(v.str))
		return s != "" && s != "0" && s != "false"
	}
	return false
}

// finite: hasil +Inf/-Inf/NaN (mis. 1e200*1e200) bukan angka yang bermakna; jadikan null supaya tidak ditulis ke CPE sebagai "+Inf".
func finite(f float64) Value {
	if math.IsNaN(f) || math.IsInf(f, 0) {
		return Null()
	}
	return Num(f)
}

func boolVal(b bool) Value {
	if b {
		return Num(1)
	}
	return Num(0)
}

// ---- AST ----

type node interface{ eval(c *ctx) Value }

type (
	numLit  struct{ v float64 }
	strLit  struct{ v string }
	refNode struct{ name string }
	unary   struct {
		op string
		x  node
	}
	binary struct {
		op   string
		l, r node
	}
	call struct {
		fn   string
		args []node
	}
)

// Expr = ekspresi hasil parse (aman dipakai bersamaan; tidak berubah setelah Parse).
type Expr struct {
	root node
	refs []string
}

// Refs: nama parameter yang dirujuk ekspresi (unik, urut kemunculan).
func (e *Expr) Refs() []string { return append([]string(nil), e.refs...) }

// Eval menjalankan ekspresi; get mengembalikan (nilai, true) bila parameter sudah dilaporkan.
func (e *Expr) Eval(get func(name string) (string, bool)) Value {
	v := e.root.eval(&ctx{get: get})
	if v.kind == 's' {
		v.str = truncate(v.str, MaxResultLen)
	}
	return v
}

type ctx struct{ get func(string) (string, bool) }

// ---- Lexer ----

type tokKind int

const (
	tEOF tokKind = iota
	tNum
	tStr
	tRef
	tIdent
	tOp // operator & tanda baca
)

type token struct {
	kind tokKind
	text string
	num  float64
	pos  int
}

func lex(src string) ([]token, error) {
	rs := []rune(src)
	var toks []token
	for i := 0; i < len(rs); {
		r := rs[i]
		switch {
		case unicode.IsSpace(r):
			i++
		case r == '{':
			j := i + 1
			for j < len(rs) && rs[j] != '}' {
				j++
			}
			if j >= len(rs) {
				return nil, &ParseError{i, "kurung '{' tidak ditutup"}
			}
			name := strings.TrimSpace(string(rs[i+1 : j]))
			if !validRefName(name) {
				return nil, &ParseError{i, "nama parameter tidak valid (huruf, angka, '.', '_', '-'; maks 256 karakter)"}
			}
			toks = append(toks, token{kind: tRef, text: name, pos: i})
			i = j + 1
		case r == '\'' || r == '"':
			j := i + 1
			var sb strings.Builder
			for j < len(rs) && rs[j] != r {
				if rs[j] == '\\' && j+1 < len(rs) {
					j++
					switch rs[j] {
					case 'n':
						sb.WriteRune('\n')
					case 't':
						sb.WriteRune('\t')
					default:
						sb.WriteRune(rs[j])
					}
				} else {
					sb.WriteRune(rs[j])
				}
				j++
			}
			if j >= len(rs) {
				return nil, &ParseError{i, "teks tidak ditutup"}
			}
			toks = append(toks, token{kind: tStr, text: sb.String(), pos: i})
			i = j + 1
		case r >= '0' && r <= '9' || (r == '.' && i+1 < len(rs) && rs[i+1] >= '0' && rs[i+1] <= '9'):
			j := i
			for j < len(rs) && (rs[j] >= '0' && rs[j] <= '9' || rs[j] == '.') {
				j++
			}
			f, err := strconv.ParseFloat(string(rs[i:j]), 64)
			if err != nil {
				return nil, &ParseError{i, "angka tidak valid"}
			}
			toks = append(toks, token{kind: tNum, num: f, pos: i})
			i = j
		case unicode.IsLetter(r) || r == '_':
			j := i
			for j < len(rs) && (unicode.IsLetter(rs[j]) || unicode.IsDigit(rs[j]) || rs[j] == '_') {
				j++
			}
			toks = append(toks, token{kind: tIdent, text: strings.ToLower(string(rs[i:j])), pos: i})
			i = j
		default:
			two := ""
			if i+1 < len(rs) {
				two = string(rs[i : i+2])
			}
			switch two {
			case "==", "!=", "<=", ">=":
				toks = append(toks, token{kind: tOp, text: two, pos: i})
				i += 2
				continue
			}
			if strings.ContainsRune("+-*/%<>(),", r) {
				toks = append(toks, token{kind: tOp, text: string(r), pos: i})
				i++
				continue
			}
			return nil, &ParseError{i, fmt.Sprintf("karakter tak dikenal %q", r)}
		}
	}
	toks = append(toks, token{kind: tEOF, pos: len(rs)})
	return toks, nil
}

// IsContextRef: rujukan berawalan '@' (mis. {@serial_number}) = variabel konteks yang disuplai pemanggil (atribut perangkat,
// event sesi), BUKAN parameter TR-069. Pemanggil yang tidak menyuplainya (virtual parameter) memperlakukannya null.
func IsContextRef(name string) bool { return strings.HasPrefix(name, "@") }

func validRefName(s string) bool {
	if s == "" || len(s) > 256 {
		return false
	}
	if IsContextRef(s) { // @nama: huruf kecil, angka, underscore
		s = s[1:]
		if s == "" || s[0] >= '0' && s[0] <= '9' {
			return false
		}
		for _, r := range s {
			if !(r >= 'a' && r <= 'z' || r >= '0' && r <= '9' || r == '_') {
				return false
			}
		}
		return true
	}
	for _, r := range s {
		if !(r >= 'a' && r <= 'z' || r >= 'A' && r <= 'Z' || r >= '0' && r <= '9' || r == '.' || r == '_' || r == '-') {
			return false
		}
	}
	return true
}

// ---- Parser (recursive descent) ----

type parser struct {
	toks  []token
	p     int
	nodes int
	depth int
	refs  []string
	seen  map[string]bool
}

// Parse mengurai ekspresi; kesalahan bertipe *ParseError.
func Parse(src string) (*Expr, error) {
	if strings.TrimSpace(src) == "" {
		return nil, &ParseError{0, "ekspresi kosong"}
	}
	if utf8.RuneCountInString(src) > MaxExprLen {
		return nil, &ParseError{0, fmt.Sprintf("ekspresi maksimal %d karakter", MaxExprLen)}
	}
	toks, err := lex(src)
	if err != nil {
		return nil, err
	}
	ps := &parser{toks: toks, seen: map[string]bool{}}
	root, err := ps.parseOr()
	if err != nil {
		return nil, err
	}
	if t := ps.peek(); t.kind != tEOF {
		return nil, &ParseError{t.pos, "token tak terduga setelah akhir ekspresi"}
	}
	if len(ps.refs) > MaxRefs {
		return nil, &ParseError{0, fmt.Sprintf("maksimal %d rujukan parameter", MaxRefs)}
	}
	return &Expr{root: root, refs: ps.refs}, nil
}

func (ps *parser) peek() token { return ps.toks[ps.p] }
func (ps *parser) next() token { t := ps.toks[ps.p]; ps.p++; return t }

func (ps *parser) isOp(s string) bool {
	t := ps.peek()
	return t.kind == tOp && t.text == s
}
func (ps *parser) isKw(s string) bool {
	t := ps.peek()
	return t.kind == tIdent && t.text == s
}

func (ps *parser) count() error {
	ps.nodes++
	if ps.nodes > MaxNodes {
		return &ParseError{ps.peek().pos, fmt.Sprintf("ekspresi terlalu kompleks (maks %d node)", MaxNodes)}
	}
	return nil
}

func (ps *parser) enter() error {
	ps.depth++
	if ps.depth > MaxDepth {
		return &ParseError{ps.peek().pos, fmt.Sprintf("ekspresi terlalu bersarang (maks %d tingkat)", MaxDepth)}
	}
	return nil
}
func (ps *parser) leave() { ps.depth-- }

func (ps *parser) parseOr() (node, error) {
	l, err := ps.parseAnd()
	if err != nil {
		return nil, err
	}
	for ps.isKw("or") {
		ps.next()
		r, err := ps.parseAnd()
		if err != nil {
			return nil, err
		}
		if err := ps.count(); err != nil {
			return nil, err
		}
		l = &binary{"or", l, r}
	}
	return l, nil
}

func (ps *parser) parseAnd() (node, error) {
	l, err := ps.parseNot()
	if err != nil {
		return nil, err
	}
	for ps.isKw("and") {
		ps.next()
		r, err := ps.parseNot()
		if err != nil {
			return nil, err
		}
		if err := ps.count(); err != nil {
			return nil, err
		}
		l = &binary{"and", l, r}
	}
	return l, nil
}

func (ps *parser) parseNot() (node, error) {
	if ps.isKw("not") {
		ps.next()
		if err := ps.enter(); err != nil {
			return nil, err
		}
		x, err := ps.parseNot()
		ps.leave()
		if err != nil {
			return nil, err
		}
		if err := ps.count(); err != nil {
			return nil, err
		}
		return &unary{"not", x}, nil
	}
	return ps.parseCmp()
}

func (ps *parser) parseCmp() (node, error) {
	l, err := ps.parseAdd()
	if err != nil {
		return nil, err
	}
	for _, op := range []string{"==", "!=", "<=", ">=", "<", ">"} {
		if ps.isOp(op) {
			ps.next()
			r, err := ps.parseAdd()
			if err != nil {
				return nil, err
			}
			if err := ps.count(); err != nil {
				return nil, err
			}
			return &binary{op, l, r}, nil
		}
	}
	return l, nil
}

func (ps *parser) parseAdd() (node, error) {
	l, err := ps.parseMul()
	if err != nil {
		return nil, err
	}
	for ps.isOp("+") || ps.isOp("-") {
		op := ps.next().text
		r, err := ps.parseMul()
		if err != nil {
			return nil, err
		}
		if err := ps.count(); err != nil {
			return nil, err
		}
		l = &binary{op, l, r}
	}
	return l, nil
}

func (ps *parser) parseMul() (node, error) {
	l, err := ps.parseUnary()
	if err != nil {
		return nil, err
	}
	for ps.isOp("*") || ps.isOp("/") || ps.isOp("%") {
		op := ps.next().text
		r, err := ps.parseUnary()
		if err != nil {
			return nil, err
		}
		if err := ps.count(); err != nil {
			return nil, err
		}
		l = &binary{op, l, r}
	}
	return l, nil
}

func (ps *parser) parseUnary() (node, error) {
	if ps.isOp("-") {
		ps.next()
		if err := ps.enter(); err != nil {
			return nil, err
		}
		x, err := ps.parseUnary()
		ps.leave()
		if err != nil {
			return nil, err
		}
		if err := ps.count(); err != nil {
			return nil, err
		}
		return &unary{"-", x}, nil
	}
	return ps.parsePrimary()
}

// functions = nama → (min arg, maks arg; -1 = tak terbatas).
var functions = map[string][2]int{
	"first": {1, -1}, "coalesce": {1, -1}, "if": {3, 3},
	"round": {1, 2}, "floor": {1, 1}, "ceil": {1, 1}, "abs": {1, 1}, "min": {1, -1}, "max": {1, -1},
	"num": {1, 1}, "str": {1, 1}, "concat": {1, -1}, "upper": {1, 1}, "lower": {1, 1}, "trim": {1, 1}, "len": {1, 1},
	"contains": {2, 2}, "startswith": {2, 2}, "endswith": {2, 2}, "replace": {3, 3}, "substr": {2, 3},
	"duration": {1, 1}, "isnull": {1, 1},
}

// FunctionNames: daftar fungsi yang tersedia (untuk bantuan di UI/dokumen).
func FunctionNames() []string {
	out := make([]string, 0, len(functions))
	for k := range functions {
		out = append(out, k)
	}
	return out
}

func (ps *parser) parsePrimary() (node, error) {
	t := ps.next()
	if err := ps.count(); err != nil {
		return nil, err
	}
	switch t.kind {
	case tNum:
		return &numLit{t.num}, nil
	case tStr:
		return &strLit{t.text}, nil
	case tRef:
		if !ps.seen[t.text] {
			ps.seen[t.text] = true
			ps.refs = append(ps.refs, t.text)
		}
		return &refNode{t.text}, nil
	case tIdent:
		if t.text == "true" {
			return &numLit{1}, nil
		}
		if t.text == "false" {
			return &numLit{0}, nil
		}
		if t.text == "null" {
			return &call{fn: "null"}, nil
		}
		ar, ok := functions[t.text]
		if !ok {
			return nil, &ParseError{t.pos, fmt.Sprintf("fungsi %q tidak dikenal", t.text)}
		}
		if !ps.isOp("(") {
			return nil, &ParseError{ps.peek().pos, fmt.Sprintf("fungsi %s harus dipanggil dengan (...)", t.text)}
		}
		ps.next()
		if err := ps.enter(); err != nil {
			return nil, err
		}
		var args []node
		if !ps.isOp(")") {
			for {
				a, err := ps.parseOr()
				if err != nil {
					return nil, err
				}
				args = append(args, a)
				if ps.isOp(",") {
					ps.next()
					continue
				}
				break
			}
		}
		ps.leave()
		if !ps.isOp(")") {
			return nil, &ParseError{ps.peek().pos, "kurung ')' diharapkan"}
		}
		ps.next()
		if len(args) < ar[0] || (ar[1] >= 0 && len(args) > ar[1]) {
			return nil, &ParseError{t.pos, fmt.Sprintf("fungsi %s menerima %s argumen, dapat %d", t.text, arity(ar), len(args))}
		}
		return &call{fn: t.text, args: args}, nil
	case tOp:
		if t.text == "(" {
			if err := ps.enter(); err != nil {
				return nil, err
			}
			x, err := ps.parseOr()
			ps.leave()
			if err != nil {
				return nil, err
			}
			if !ps.isOp(")") {
				return nil, &ParseError{ps.peek().pos, "kurung ')' diharapkan"}
			}
			ps.next()
			return x, nil
		}
	}
	if t.kind == tEOF {
		return nil, &ParseError{t.pos, "ekspresi berakhir terlalu cepat"}
	}
	return nil, &ParseError{t.pos, "token tak terduga"}
}

func arity(a [2]int) string {
	switch {
	case a[1] < 0:
		return fmt.Sprintf("minimal %d", a[0])
	case a[0] == a[1]:
		return strconv.Itoa(a[0])
	}
	return fmt.Sprintf("%d-%d", a[0], a[1])
}

// ---- Evaluasi ----

func (n *numLit) eval(*ctx) Value { return Num(n.v) }
func (n *strLit) eval(*ctx) Value { return Str(truncate(n.v, maxStrLen)) }

func (n *refNode) eval(c *ctx) Value {
	if c.get == nil {
		return Null()
	}
	s, ok := c.get(n.name)
	if !ok {
		return Null()
	}
	return Str(truncate(s, maxStrLen))
}

func (n *unary) eval(c *ctx) Value {
	x := n.x.eval(c)
	switch n.op {
	case "not":
		return boolVal(!x.truthy())
	case "-":
		if f, ok := x.asNum(); ok {
			return Num(-f)
		}
	}
	return Null()
}

func (n *binary) eval(c *ctx) Value {
	switch n.op {
	case "and":
		l := n.l.eval(c)
		if !l.truthy() {
			return boolVal(false)
		}
		return boolVal(n.r.eval(c).truthy())
	case "or":
		if n.l.eval(c).truthy() {
			return boolVal(true)
		}
		return boolVal(n.r.eval(c).truthy())
	}
	l, r := n.l.eval(c), n.r.eval(c)
	switch n.op {
	case "==", "!=", "<", "<=", ">", ">=":
		return compare(n.op, l, r)
	}
	a, ok1 := l.asNum()
	b, ok2 := r.asNum()
	if !ok1 || !ok2 {
		return Null()
	}
	switch n.op {
	case "+":
		return finite(a + b)
	case "-":
		return finite(a - b)
	case "*":
		return finite(a * b)
	case "/":
		if b == 0 {
			return Null()
		}
		return finite(a / b)
	case "%":
		if b == 0 {
			return Null()
		}
		return finite(math.Mod(a, b))
	}
	return Null()
}

func compare(op string, l, r Value) Value {
	if l.IsNull() || r.IsNull() {
		switch op {
		case "==":
			return boolVal(l.IsNull() && r.IsNull())
		case "!=":
			return boolVal(l.IsNull() != r.IsNull())
		}
		return boolVal(false)
	}
	var c int
	a, ok1 := l.asNum()
	b, ok2 := r.asNum()
	if ok1 && ok2 {
		switch {
		case a < b:
			c = -1
		case a > b:
			c = 1
		}
	} else {
		c = strings.Compare(strings.ToLower(l.String()), strings.ToLower(r.String()))
	}
	switch op {
	case "==":
		return boolVal(c == 0)
	case "!=":
		return boolVal(c != 0)
	case "<":
		return boolVal(c < 0)
	case "<=":
		return boolVal(c <= 0)
	case ">":
		return boolVal(c > 0)
	}
	return boolVal(c >= 0)
}

func truncate(s string, max int) string {
	if utf8.RuneCountInString(s) <= max {
		return s
	}
	return string([]rune(s)[:max])
}

func (n *call) eval(c *ctx) Value {
	// if() mengevaluasi hanya cabang terpilih
	switch n.fn {
	case "null":
		return Null()
	case "if":
		if n.args[0].eval(c).truthy() {
			return n.args[1].eval(c)
		}
		return n.args[2].eval(c)
	case "first", "coalesce":
		for _, a := range n.args {
			if v := a.eval(c); !v.IsNull() && !(v.IsString() && strings.TrimSpace(v.str) == "") {
				return v
			}
		}
		return Null()
	}
	vs := make([]Value, len(n.args))
	for i, a := range n.args {
		vs[i] = a.eval(c)
	}
	return callFn(n.fn, vs)
}

func callFn(fn string, vs []Value) Value {
	num := func(i int) (float64, bool) { return vs[i].asNum() }
	switch fn {
	case "isnull":
		return boolVal(vs[0].IsNull() || (vs[0].IsString() && strings.TrimSpace(vs[0].str) == ""))
	case "round":
		x, ok := num(0)
		if !ok {
			return Null()
		}
		digits := 0.0
		if len(vs) == 2 {
			d, ok := num(1)
			if !ok || d < 0 || d > 9 {
				return Null()
			}
			digits = math.Floor(d)
		}
		p := math.Pow(10, digits)
		return Num(math.Round(x*p) / p)
	case "floor", "ceil", "abs":
		x, ok := num(0)
		if !ok {
			return Null()
		}
		switch fn {
		case "floor":
			return Num(math.Floor(x))
		case "ceil":
			return Num(math.Ceil(x))
		}
		return Num(math.Abs(x))
	case "min", "max":
		var best float64
		for i := range vs {
			x, ok := num(i)
			if !ok {
				return Null()
			}
			if i == 0 || (fn == "min" && x < best) || (fn == "max" && x > best) {
				best = x
			}
		}
		return Num(best)
	case "num":
		if x, ok := num(0); ok {
			return Num(x)
		}
		return Null()
	case "str":
		if vs[0].IsNull() {
			return Null()
		}
		return Str(vs[0].String())
	case "concat":
		var sb strings.Builder
		any := false
		for _, v := range vs {
			if !v.IsNull() {
				any = true
				sb.WriteString(v.String())
			}
		}
		if !any {
			return Null()
		}
		return Str(truncate(sb.String(), maxStrLen))
	case "upper", "lower", "trim":
		if vs[0].IsNull() {
			return Null()
		}
		s := vs[0].String()
		switch fn {
		case "upper":
			return Str(strings.ToUpper(s))
		case "lower":
			return Str(strings.ToLower(s))
		}
		return Str(strings.TrimSpace(s))
	case "len":
		if vs[0].IsNull() {
			return Null()
		}
		return Num(float64(utf8.RuneCountInString(vs[0].String())))
	case "contains", "startswith", "endswith":
		if vs[0].IsNull() || vs[1].IsNull() {
			return boolVal(false)
		}
		a, b := strings.ToLower(vs[0].String()), strings.ToLower(vs[1].String())
		switch fn {
		case "contains":
			return boolVal(strings.Contains(a, b))
		case "startswith":
			return boolVal(strings.HasPrefix(a, b))
		}
		return boolVal(strings.HasSuffix(a, b))
	case "replace":
		if vs[0].IsNull() || vs[1].IsNull() || vs[1].String() == "" {
			return vs[0]
		}
		return Str(truncate(replaceBounded(vs[0].String(), vs[1].String(), vs[2].String(), maxStrLen*utf8.UTFMax), maxStrLen))
	case "substr":
		if vs[0].IsNull() {
			return Null()
		}
		start, ok := num(1)
		if !ok || start < 0 {
			return Null()
		}
		rs := []rune(vs[0].String())
		// Bandingkan sebagai float SEBELUM konversi ke int: angka sangat besar tidak boleh meluap (int(1e22) tak terdefinisi).
		if start > float64(len(rs)) {
			return Str("")
		}
		s := int(start)
		end := len(rs)
		if len(vs) == 3 {
			l, ok := num(2)
			if !ok || l < 0 {
				return Null()
			}
			if l < float64(end-s) {
				end = s + int(l)
			}
		}
		return Str(string(rs[s:end]))
	case "duration":
		x, ok := num(0)
		if !ok || x < 0 || x > 1e11 { // ~3000 tahun; di atasnya bukan durasi nyata dan int64() bisa meluap
			return Null()
		}
		return Str(formatDuration(int64(x)))
	}
	return Null()
}

// replaceBounded = strings.ReplaceAll yang berhenti menambah hasil begitu panjangnya mencapai limit byte. strings.ReplaceAll
// mengalokasikan hasil PENUH dulu (len(s) + n*(len(new)-len(old))) baru dipotong pemanggil: dengan s dan new masing-masing ~4 KB
// dan beberapa replace bersarang itu mencapai ribuan MB per Eval — celah DoS yang ditemukan audit keamanan independen.
func replaceBounded(s, old, new string, limit int) string {
	var sb strings.Builder
	sb.Grow(min(len(s), limit))
	for len(s) > 0 && sb.Len() < limit {
		i := strings.Index(s, old)
		if i < 0 {
			sb.WriteString(s)
			break
		}
		sb.WriteString(s[:i])
		sb.WriteString(new)
		s = s[i+len(old):]
	}
	out := sb.String()
	if len(out) > limit {
		out = out[:limit]
	}
	return out
}

// formatDuration: detik → "1d 2h 3m 4s" (satuan nol dilewati; 0 → "0s").
func formatDuration(sec int64) string {
	d, h, m, s := sec/86400, sec%86400/3600, sec%3600/60, sec%60
	var parts []string
	if d > 0 {
		parts = append(parts, fmt.Sprintf("%dd", d))
	}
	if h > 0 {
		parts = append(parts, fmt.Sprintf("%dh", h))
	}
	if m > 0 {
		parts = append(parts, fmt.Sprintf("%dm", m))
	}
	if s > 0 || len(parts) == 0 {
		parts = append(parts, fmt.Sprintf("%ds", s))
	}
	return strings.Join(parts, " ")
}
