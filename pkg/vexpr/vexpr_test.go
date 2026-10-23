package vexpr

import (
	"errors"
	"strings"
	"testing"
	"time"
)

func eval(t *testing.T, src string, params map[string]string) Value {
	t.Helper()
	e, err := Parse(src)
	if err != nil {
		t.Fatalf("Parse(%q): %v", src, err)
	}
	return e.Eval(func(n string) (string, bool) { v, ok := params[n]; return v, ok })
}

func TestEval(t *testing.T) {
	p := map[string]string{
		"A": "21500", "B": "-23500", "Up": "93784", "Name": "  Rumah Budi ", "Empty": "", "Txt": "abc",
		"Device.UpTime": "100",
	}
	cases := []struct{ src, want string }{
		// aritmetika & presisi
		{"{A} / 1000", "21.5"},
		{"{B} / 1000", "-23.5"},
		{"0.1 + 0.2", "0.3"},
		{"round({A} / 7000, 2)", "3.07"},
		{"round(2.5)", "3"},
		{"7 % 4 + 2 * 3 - 1", "8"},
		{"-(2 + 3) * 2", "-10"},
		{"abs({B})", "23500"},
		{"floor(2.9) + ceil(2.1)", "5"},
		{"min(3, {A}, 1) + max(1, 2)", "3"},
		// null menjalar; pembagian nol = null
		{"{Hilang} + 1", ""},
		{"1 / 0", ""},
		{"5 % 0", ""},
		{"{Txt} * 2", ""}, // teks non-angka
		{"num({Txt})", ""},
		{"num('  42 ')", "42"},
		// first / coalesce (lintas TR-098 vs TR-181)
		{"first({InternetGatewayDevice.UpTime}, {Device.UpTime})", "100"},
		{"first({Empty}, {Hilang}, 'default')", "default"},
		{"coalesce({Empty}, 'x')", "x"},
		{"first({Hilang})", ""},
		// if lazily
		{"if({A} > 20000, 'tinggi', 'rendah')", "tinggi"},
		{"if(0, 1 / 0, 'aman')", "aman"},
		// perbandingan
		{"{A} == 21500", "1"},
		{"{A} == '21500.0'", "1"}, // numerik bila keduanya angka
		{"{Txt} == 'ABC'", "1"},   // teks tak peka huruf besar/kecil
		{"{Txt} < 'abd'", "1"},
		{"{Hilang} == 1", "0"},
		{"{Hilang} != 1", "1"},
		{"{Hilang} == {Hilang2}", "1"},
		{"{Hilang} < 1", "0"},
		{"10 > 9", "1"},
		{"'10' > '9'", "1"}, // angka, bukan leksikografis
		// logika
		{"{A} > 1 and {B} < 0", "1"},
		{"{A} < 1 or {B} < 0", "1"},
		{"not ({A} > 1)", "0"},
		{"not {Hilang}", "1"},
		{"true and false", "0"},
		{"'false' or 0", "0"},
		// teks
		{"concat({Txt}, '-', {A})", "abc-21500"},
		{"concat({Hilang}, {Hilang2})", ""},
		{"upper(trim({Name}))", "RUMAH BUDI"},
		{"lower('ABC')", "abc"},
		{"len('héllo')", "5"},
		{"contains({Name}, 'BUDI')", "1"},
		{"startswith({Txt}, 'AB')", "1"},
		{"endswith({Txt}, 'x')", "0"},
		{"replace({Name}, 'Budi', 'Siti')", "  Rumah Siti "},
		{"substr('abcdef', 2, 3)", "cde"},
		{"substr('abcdef', 4)", "ef"},
		{"substr('abc', 10)", ""},
		{"str({A})", "21500"},
		{"duration({Up})", "1d 2h 3m 4s"},
		{"duration(0)", "0s"},
		{"duration(3600)", "1h"},
		{"isnull({Empty})", "1"},
		{"isnull({A})", "0"},
		{"'a\\'b'", "a'b"},
	}
	for _, c := range cases {
		if got := eval(t, c.src, p).String(); got != c.want {
			t.Errorf("%s = %q, want %q", c.src, got, c.want)
		}
	}
}

func TestRefsUniqueInOrder(t *testing.T) {
	e, err := Parse("{A} + {B} + {A} + first({C}, {B})")
	if err != nil {
		t.Fatal(err)
	}
	if got := strings.Join(e.Refs(), ","); got != "A,B,C" {
		t.Errorf("refs = %s", got)
	}
}

func TestParseErrors(t *testing.T) {
	bad := []string{
		"", "   ", "1 +", "(1", "1)", "foo(1)", "round(1,2,3)", "if(1,2)", "first()", "round",
		"{}", "{bad name}", "{A", "'abc", "1 $ 2", "1 2", "{A} {B}", "and", "1 and", "not", "@",
		"round(1, 2",
	}
	for _, src := range bad {
		_, err := Parse(src)
		var pe *ParseError
		if !errors.As(err, &pe) {
			t.Errorf("%q harus ParseError, got %v", src, err)
		}
	}
}

func TestParseLimits(t *testing.T) {
	// panjang
	if _, err := Parse(strings.Repeat("1+", MaxExprLen/2+1) + "1"); err == nil {
		t.Error("ekspresi terlalu panjang harus ditolak")
	}
	// node
	var sb strings.Builder
	sb.WriteString("1")
	for i := 0; i < MaxNodes; i++ {
		sb.WriteString("+1")
	}
	if len(sb.String()) <= MaxExprLen {
		if _, err := Parse(sb.String()); err == nil {
			t.Error("terlalu banyak node harus ditolak")
		}
	}
	// kedalaman: kurung bersarang
	deep := strings.Repeat("(", MaxDepth+2) + "1" + strings.Repeat(")", MaxDepth+2)
	if _, err := Parse(deep); err == nil {
		t.Error("terlalu bersarang harus ditolak")
	}
	if _, err := Parse(strings.Repeat("not ", MaxDepth+2) + "1"); err == nil {
		t.Error("not bersarang terlalu dalam harus ditolak")
	}
	if _, err := Parse(strings.Repeat("-", MaxDepth+2) + "1"); err == nil {
		t.Error("minus bersarang terlalu dalam harus ditolak")
	}
	// rujukan
	var refs []string
	for i := 0; i <= MaxRefs; i++ {
		refs = append(refs, "{P"+strings.Repeat("x", i)+"}")
	}
	if _, err := Parse("first(" + strings.Join(refs, ",") + ")"); err == nil {
		t.Error("terlalu banyak rujukan harus ditolak")
	}
}

// Hasil/antara string dibatasi: tidak ada ekspresi yang bisa menghasilkan string besar tak terbatas.
func TestResultLengthBounded(t *testing.T) {
	big := strings.Repeat("x", 60000)
	got := eval(t, "concat({B}, {B}, {B}, {B}, {B}, {B}, {B}, {B})", map[string]string{"B": big}).String()
	if n := len([]rune(got)); n > MaxResultLen {
		t.Errorf("hasil %d rune melebihi batas %d", n, MaxResultLen)
	}
	got = eval(t, "replace({B}, 'x', 'yyyyyyyyyyyyyyyyyyyy')", map[string]string{"B": big}).String()
	if n := len([]rune(got)); n > MaxResultLen {
		t.Errorf("replace %d rune melebihi batas %d", n, MaxResultLen)
	}
}

// Tidak panik pada masukan sembarang, dan evaluasi selalu cepat (tanpa loop).
func TestNoPanicOnGarbage(t *testing.T) {
	inputs := []string{
		"{A}/{A}", "{A} % {A}", "round({A}, {A})", "substr({A}, {A}, {A})", "substr('abc', -1)", "min()", "-{Hilang}",
		"duration(-5)", "duration({Hilang})", "round(1, 99)", "replace('abc', '', 'x')", "'\\", "\"", "{{A}}", "1..2", ".5 + .5",
		"((((((((((1))))))))))", "if(1,2,3,4)", "len()", "first(,)",
	}
	start := time.Now()
	for _, in := range inputs {
		e, err := Parse(in)
		if err != nil {
			continue
		}
		_ = e.Eval(func(string) (string, bool) { return "9999999999999999999999", true })
		_ = e.Eval(nil)
	}
	if time.Since(start) > 2*time.Second {
		t.Error("evaluasi terlalu lambat")
	}
}

func TestEvalNilGetter(t *testing.T) {
	e, _ := Parse("first({A}, 'x')")
	if got := e.Eval(nil).String(); got != "x" {
		t.Errorf("getter nil harus memperlakukan semua parameter sebagai null, got %q", got)
	}
}

// Regresi: angka sangat besar pada substr/duration/round meluapkan konversi int dan membuat panic.
func TestHugeNumbersDoNotOverflow(t *testing.T) {
	for _, src := range []string{
		"substr('abc', 9999999999999999999999)", "substr('abc', 1, 9999999999999999999999)",
		"duration(9999999999999999999999)", "round(1, 9999999999999999999999)",
	} {
		e, err := Parse(src)
		if err != nil {
			t.Fatal(err)
		}
		_ = e.Eval(nil) // tidak boleh panic
	}
	if got := eval(t, "substr('abcdef', 1, 9999999999999999999999)", nil).String(); got != "bcdef" {
		t.Errorf("substr dengan panjang raksasa = %q, want bcdef", got)
	}
}

// Variabel konteks {@nama}: disuplai pemanggil lewat getter yang sama; bukan parameter TR-069.
func TestContextRefs(t *testing.T) {
	p := map[string]string{"@serial_number": "ZTEGABC123", "@events": "1 BOOT,2 PERIODIC", "Device.UpTime": "10"}
	cases := []struct{ src, want string }{
		{"{@serial_number}", "ZTEGABC123"},
		{"startswith({@serial_number}, 'zte')", "1"},
		{"contains({@events}, '1 BOOT') and {Device.UpTime} < 60", "1"},
		{"{@tidak_ada} == 1", "0"},
	}
	for _, c := range cases {
		if got := eval(t, c.src, p).String(); got != c.want {
			t.Errorf("%s = %q, want %q", c.src, got, c.want)
		}
	}
	e, err := Parse("{@serial_number} + {Device.UpTime}")
	if err != nil {
		t.Fatal(err)
	}
	if !IsContextRef(e.Refs()[0]) || IsContextRef(e.Refs()[1]) {
		t.Errorf("IsContextRef salah: %v", e.Refs())
	}
	for _, bad := range []string{"{@}", "{@1x}", "{@Besar}", "{@a-b}", "{@a.b}"} {
		if _, err := Parse(bad); err == nil {
			t.Errorf("%q harus ditolak", bad)
		}
	}
}

// Regresi audit keamanan: replace bersarang dengan string besar tidak boleh mengalokasikan hasil penuh (dulu ~3,3 GB per Eval).
func TestReplaceTidakMengalokasiHasilPenuh(t *testing.T) {
	big := strings.Repeat("a", 4096)
	var expr strings.Builder
	expr.WriteString("len(concat(")
	for i := 0; i < 20; i++ {
		if i > 0 {
			expr.WriteString(",")
		}
		expr.WriteString("replace({P},'a',{P})")
	}
	expr.WriteString("))")
	e, err := Parse(expr.String())
	if err != nil {
		t.Fatal(err)
	}
	get := func(string) (string, bool) { return big, true }
	allocs := testing.AllocsPerRun(1, func() { _ = e.Eval(get) })
	_ = allocs
	var m1, m2 runtimeMemStats
	m1.read()
	start := time.Now()
	v := e.Eval(get)
	m2.read()
	if d := time.Since(start); d > 500*time.Millisecond {
		t.Errorf("Eval terlalu lama: %v", d)
	}
	if grew := m2.total - m1.total; grew > 64<<20 {
		t.Errorf("Eval mengalokasikan %d MiB (batas 64)", grew>>20)
	}
	if n, ok := v.asNum(); !ok || n > float64(MaxResultLen*20) {
		t.Errorf("hasil len tidak terbatas: %v", v.String())
	}
	// semantik tetap benar untuk kasus kecil
	if got := eval(t, "replace('a-b-c','-','+')", nil).String(); got != "a+b+c" {
		t.Errorf("replace biasa = %q", got)
	}
	if got := eval(t, "replace('aaa','a','')", nil).String(); got != "" {
		t.Errorf("replace dengan pengganti kosong = %q", got)
	}
}

// Hasil +Inf/NaN dari aritmetika menjadi null (dulu "+Inf" ikut ditulis ke CPE oleh set_parameter).
func TestHasilTakHinggaMenjadiNull(t *testing.T) {
	huge := strings.Repeat("9", 200)
	for _, src := range []string{"{A} * {A}", "{A} + {A} * {A}", "{A} / 0.000000000000001 * {A}"} {
		v := eval(t, src, map[string]string{"A": huge})
		if !v.IsNull() {
			t.Errorf("%s = %q, harus null", src, v.String())
		}
	}
	if got := eval(t, "2 * 3", nil).String(); got != "6" {
		t.Errorf("aritmetika normal rusak: %q", got)
	}
}
