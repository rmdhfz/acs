// Package digestauth mengimplementasikan HTTP Digest Authentication (RFC 7616 /
// RFC 2617) untuk dua sisi: klien (ACS menjawab tantangan CPE pada Connection
// Request TR-069) dan verifier sederhana (dipakai simulator CPE).
//
// Cakupan: algoritma MD5, MD5-sess, SHA-256, SHA-256-sess; qop "auth" atau
// tanpa qop (RFC 2069 lama). qop "auth-int" sengaja tidak didukung — request
// Connection Request tidak punya body, dan CPE nyata tidak menawarkannya.
package digestauth

import (
	"crypto/hmac"
	"crypto/md5" //nolint:gosec // MD5 dipersyaratkan oleh protokol Digest, bukan untuk keamanan kriptografis baru.
	"crypto/rand"
	"crypto/sha256"
	"encoding/hex"
	"errors"
	"fmt"
	"hash"
	"strings"
)

// Challenge adalah isi header WWW-Authenticate: Digest ... dari server.
type Challenge struct {
	Realm     string
	Nonce     string
	Opaque    string
	Algorithm string // "" dianggap MD5
	QOP       string // opsi yang ditawarkan, mis. "auth" atau "auth,auth-int"
	Stale     bool
}

// ErrNoDigestChallenge dikembalikan bila tidak ada tantangan Digest di header.
var ErrNoDigestChallenge = errors.New("digestauth: tidak ada tantangan Digest")

// ParseChallenge mencari tantangan Digest di antara nilai header
// WWW-Authenticate (boleh lebih dari satu header, boleh digabung dalam satu
// header dengan skema lain).
func ParseChallenge(headerValues []string) (*Challenge, error) {
	for _, hv := range headerValues {
		rest, ok := afterScheme(hv, "digest")
		if !ok {
			continue
		}
		p := parseParams(rest)
		c := &Challenge{
			Realm:     p["realm"],
			Nonce:     p["nonce"],
			Opaque:    p["opaque"],
			Algorithm: p["algorithm"],
			QOP:       p["qop"],
			Stale:     strings.EqualFold(p["stale"], "true"),
		}
		if c.Nonce == "" {
			return nil, errors.New("digestauth: tantangan Digest tanpa nonce")
		}
		return c, nil
	}
	return nil, ErrNoDigestChallenge
}

// HasScheme melaporkan apakah salah satu header menawarkan skema tertentu
// (mis. "basic").
func HasScheme(headerValues []string, scheme string) bool {
	for _, hv := range headerValues {
		if _, ok := afterScheme(hv, scheme); ok {
			return true
		}
	}
	return false
}

// afterScheme mengembalikan sisa string setelah token skema, bila skema itu ada
// di awal nilai header atau setelah pemisah ", " (tantangan gabungan).
func afterScheme(hv, scheme string) (string, bool) {
	low := strings.ToLower(hv)
	tok := strings.ToLower(scheme) + " "
	if strings.HasPrefix(low, tok) {
		return hv[len(tok):], true
	}
	if i := strings.Index(low, ", "+tok); i >= 0 {
		return hv[i+2+len(tok):], true
	}
	return "", false
}

// Authorize menghitung nilai header Authorization untuk satu request.
// uri adalah request-target (path + query) persis seperti yang dikirim.
// nc adalah nonce-count (mulai 1). cnonce boleh kosong → dibangkitkan acak.
func (c *Challenge) Authorize(method, uri, username, password, cnonce string, nc int) (string, error) {
	h, algo, sess, err := hashFor(c.Algorithm)
	if err != nil {
		return "", err
	}
	qop, err := pickQOP(c.QOP)
	if err != nil {
		return "", err
	}
	if qop != "" && cnonce == "" {
		cnonce, err = randomHex(8)
		if err != nil {
			return "", err
		}
	}
	ncStr := fmt.Sprintf("%08x", nc)
	resp := computeResponse(h, sess, c.Realm, c.Nonce, method, uri, username, password, cnonce, ncStr, qop)

	var b strings.Builder
	fmt.Fprintf(&b, `Digest username=%s, realm=%s, nonce=%s, uri=%s, response="%s"`,
		quote(username), quote(c.Realm), quote(c.Nonce), quote(uri), resp)
	if c.Algorithm != "" {
		fmt.Fprintf(&b, `, algorithm=%s`, algo)
	}
	if c.Opaque != "" {
		fmt.Fprintf(&b, `, opaque=%s`, quote(c.Opaque))
	}
	if qop != "" {
		fmt.Fprintf(&b, `, qop=%s, nc=%s, cnonce=%s`, qop, ncStr, quote(cnonce))
	}
	return b.String(), nil
}

// NewChallengeHeader membentuk nilai WWW-Authenticate (sisi server).
func NewChallengeHeader(realm, nonce string) string {
	return fmt.Sprintf(`Digest realm=%s, nonce=%s, qop="auth", algorithm=MD5`, quote(realm), quote(nonce))
}

// Verify memeriksa header Authorization dari klien terhadap kredensial yang
// diharapkan (sisi server). realm & nonce harus yang diterbitkan server.
func Verify(method, authorization, username, password, realm, nonce string) bool {
	rest, ok := afterScheme(authorization, "digest")
	if !ok {
		return false
	}
	p := parseParams(rest)
	if p["username"] != username || p["realm"] != realm || p["nonce"] != nonce || p["uri"] == "" {
		return false
	}
	h, _, sess, err := hashFor(p["algorithm"])
	if err != nil {
		return false
	}
	qop := p["qop"]
	if qop != "" && qop != "auth" {
		return false
	}
	want := computeResponse(h, sess, realm, nonce, method, p["uri"], username, password, p["cnonce"], p["nc"], qop)
	return hmac.Equal([]byte(want), []byte(strings.ToLower(p["response"])))
}

func computeResponse(h func() hash.Hash, sess bool, realm, nonce, method, uri, user, pass, cnonce, nc, qop string) string {
	sum := func(s string) string {
		x := h()
		x.Write([]byte(s))
		return hex.EncodeToString(x.Sum(nil))
	}
	ha1 := sum(user + ":" + realm + ":" + pass)
	if sess {
		ha1 = sum(ha1 + ":" + nonce + ":" + cnonce)
	}
	ha2 := sum(method + ":" + uri)
	if qop == "" {
		return sum(ha1 + ":" + nonce + ":" + ha2)
	}
	return sum(ha1 + ":" + nonce + ":" + nc + ":" + cnonce + ":" + qop + ":" + ha2)
}

func hashFor(algorithm string) (func() hash.Hash, string, bool, error) {
	switch strings.ToUpper(algorithm) {
	case "", "MD5":
		return md5.New, "MD5", false, nil
	case "MD5-SESS":
		return md5.New, "MD5-sess", true, nil
	case "SHA-256":
		return sha256.New, "SHA-256", false, nil
	case "SHA-256-SESS":
		return sha256.New, "SHA-256-sess", true, nil
	}
	return nil, "", false, fmt.Errorf("digestauth: algoritma %q tidak didukung", algorithm)
}

// pickQOP memilih "auth" bila ditawarkan; kosong berarti server memakai RFC 2069.
func pickQOP(offered string) (string, error) {
	if strings.TrimSpace(offered) == "" {
		return "", nil
	}
	for _, o := range strings.Split(offered, ",") {
		if strings.EqualFold(strings.TrimSpace(o), "auth") {
			return "auth", nil
		}
	}
	return "", fmt.Errorf("digestauth: qop %q tidak didukung (hanya auth)", offered)
}

func randomHex(n int) (string, error) {
	b := make([]byte, n)
	if _, err := rand.Read(b); err != nil {
		return "", err
	}
	return hex.EncodeToString(b), nil
}

func quote(s string) string {
	s = strings.ReplaceAll(s, `\`, `\\`)
	s = strings.ReplaceAll(s, `"`, `\"`)
	return `"` + s + `"`
}

// parseParams membaca daftar auth-param `k=v, k="v"` (nilai bertanda kutip
// boleh memuat koma dan escape backslash). Kunci diubah ke huruf kecil.
func parseParams(s string) map[string]string {
	out := map[string]string{}
	i := 0
	for i < len(s) {
		for i < len(s) && (s[i] == ' ' || s[i] == ',' || s[i] == '\t') {
			i++
		}
		start := i
		for i < len(s) && s[i] != '=' && s[i] != ',' {
			i++
		}
		if i >= len(s) || s[i] != '=' {
			continue
		}
		key := strings.ToLower(strings.TrimSpace(s[start:i]))
		i++ // '='
		var val strings.Builder
		if i < len(s) && s[i] == '"' {
			i++
			for i < len(s) && s[i] != '"' {
				if s[i] == '\\' && i+1 < len(s) {
					i++
				}
				val.WriteByte(s[i])
				i++
			}
			i++ // kutip penutup
		} else {
			for i < len(s) && s[i] != ',' {
				val.WriteByte(s[i])
				i++
			}
		}
		if key != "" {
			out[key] = strings.TrimSpace(val.String())
		}
	}
	return out
}
