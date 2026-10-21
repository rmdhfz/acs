package main

import (
	"bytes"
	"io"
	"net/http"
	"regexp"
	"strconv"
)

// CPE nyata menulis SOAP dengan awalan namespace (<soap-env:Envelope>, <cwmp:Inform>), sedangkan encoding/xml Go (dipakai
// simulator) menulis namespace bawaan (<Envelope xmlns=...>, <Inform xmlns="urn:dslforum-org:cwmp-1-2">). ACS kita menerima
// keduanya, tetapi GenieACS 1.2.x menentukan versi CWMP dari awalan elemen Inform lalu menimpa hasilnya dengan xmlns bawaan
// milik Envelope — sehingga Inform gaya-Go ditolak "Unrecognized CWMP version" (dibuktikan dari kode GenieACS). Agar kedua ACS
// dibandingkan dengan pesan yang IDENTIK dan mirip perangkat nyata, benchmark menulis ulang pesan simulator ke gaya berawalan.

var (
	reEnvOpen   = regexp.MustCompile(`<Envelope xmlns="http://schemas\.xmlsoap\.org/soap/envelope/"`)
	reRPCOpen   = regexp.MustCompile(`<([A-Za-z]+) xmlns="urn:dslforum-org:cwmp-\d-\d">`)
	reIDOpen    = regexp.MustCompile(`<ID ([^>]*)>`)
	reSoapFault = regexp.MustCompile(`<Fault>`)
)

// toPrefixedSOAP menulis ulang pesan gaya-Go simulator ke gaya berawalan seperti CPE nyata. Pesan tanpa Envelope (POST kosong)
// dikembalikan apa adanya.
func toPrefixedSOAP(b []byte) []byte {
	if !bytes.Contains(b, []byte("<Envelope")) {
		return b
	}
	s := string(b)
	s = reEnvOpen.ReplaceAllString(s, `<soap-env:Envelope xmlns:soap-env="http://schemas.xmlsoap.org/soap/envelope/"`)
	s = replaceAll(s, "</Envelope>", "</soap-env:Envelope>")
	s = replaceAll(s, "<Header>", "<soap-env:Header>")
	s = replaceAll(s, "</Header>", "</soap-env:Header>")
	s = replaceAll(s, "<Body>", "<soap-env:Body>")
	s = replaceAll(s, "</Body>", "</soap-env:Body>")
	s = reSoapFault.ReplaceAllString(s, "<soap-env:Fault>")
	s = replaceAll(s, "</Fault>", "</soap-env:Fault>") // hanya Fault tingkat SOAP; Fault CWMP bernamespace diproses di bawah
	s = reIDOpen.ReplaceAllString(s, `<cwmp:ID $1>`)
	s = replaceAll(s, "</ID>", "</cwmp:ID>")
	// elemen RPC bernamespace cwmp -> cwmp:Nama (penutup disesuaikan per nama)
	for _, m := range reRPCOpen.FindAllStringSubmatch(s, -1) {
		name := m[1]
		s = replaceAll(s, m[0], "<cwmp:"+name+">")
		s = replaceAll(s, "</"+name+">", "</cwmp:"+name+">")
	}
	return []byte(s)
}

func replaceAll(s, old, new string) string {
	return string(bytes.ReplaceAll([]byte(s), []byte(old), []byte(new)))
}

// prefixedTransport menerapkan toPrefixedSOAP pada body request.
type prefixedTransport struct{ rt http.RoundTripper }

func (t prefixedTransport) RoundTrip(req *http.Request) (*http.Response, error) {
	if req.Body != nil && req.Method == http.MethodPost {
		raw, err := io.ReadAll(req.Body)
		_ = req.Body.Close()
		if err != nil {
			return nil, err
		}
		out := toPrefixedSOAP(raw)
		req = req.Clone(req.Context())
		req.Body = io.NopCloser(bytes.NewReader(out))
		req.ContentLength = int64(len(out))
		req.Header.Set("Content-Length", strconv.Itoa(len(out)))
		req.GetBody = func() (io.ReadCloser, error) { return io.NopCloser(bytes.NewReader(out)), nil }
	}
	return t.rt.RoundTrip(req)
}
