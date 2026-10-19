package http

import (
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	"github.com/labstack/echo/v5"
)

type strictJSONTarget struct {
	Name string `json:"name"`
}

func strictDecode(t *testing.T, body string) (strictJSONTarget, error) {
	t.Helper()
	e := echo.New()
	req := httptest.NewRequest(http.MethodPost, "/x", strings.NewReader(body))
	req.Header.Set(echo.HeaderContentType, echo.MIMEApplicationJSON)
	rec := httptest.NewRecorder()
	c := e.NewContext(req, rec)
	var target strictJSONTarget
	s := StrictJSONSerializer{}
	err := s.Deserialize(c, &target)
	return target, err
}

func TestStrictJSONSerializer_RejectsUnknownField(t *testing.T) {
	_, err := strictDecode(t, `{"name":"a","tenant_id":7}`)
	if err == nil {
		t.Fatal("field asing (tenant_id) harus ditolak")
	}
}

func TestStrictJSONSerializer_AcceptsKnownFields(t *testing.T) {
	target, err := strictDecode(t, `{"name":"a"}`)
	if err != nil {
		t.Fatalf("payload sah harus lolos: %v", err)
	}
	if target.Name != "a" {
		t.Fatalf("name tidak terbaca: %+v", target)
	}
}

func TestStrictJSONSerializer_EmptyBodyOK(t *testing.T) {
	// ContentLength 0 (body kosong) harus tetap lolos tanpa error — dipakai
	// endpoint yang semua fieldnya opsional / request tanpa body.
	e := echo.New()
	req := httptest.NewRequest(http.MethodPost, "/x", nil)
	req.ContentLength = 0
	rec := httptest.NewRecorder()
	c := e.NewContext(req, rec)
	var target strictJSONTarget
	if err := (StrictJSONSerializer{}).Deserialize(c, &target); err != nil {
		t.Fatalf("body kosong harus lolos: %v", err)
	}
}

func TestStrictJSONSerializer_RejectsTrailingData(t *testing.T) {
	_, err := strictDecode(t, `{"name":"a"}{"name":"b"}`)
	if err == nil {
		t.Fatal("dua nilai JSON berurutan harus ditolak (sisa setelah nilai pertama)")
	}
}

func TestStrictJSONSerializer_RejectsTrailingGarbage(t *testing.T) {
	_, err := strictDecode(t, `{"name":"a"} garbage`)
	if err == nil {
		t.Fatal("sampah di belakang nilai JSON harus ditolak")
	}
}

func TestStrictJSONSerializer_RejectsMalformedJSON(t *testing.T) {
	_, err := strictDecode(t, `{"name":`)
	if err == nil {
		t.Fatal("JSON tidak lengkap harus ditolak")
	}
}
