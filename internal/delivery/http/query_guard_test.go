package http

import (
	"net/http"
	"net/http/httptest"
	"net/url"
	"strings"
	"testing"

	"github.com/labstack/echo/v5"
)

func guardStatus(t *testing.T, rawQuery string) int {
	t.Helper()
	e := echo.New()
	req := httptest.NewRequest(http.MethodGet, "/x?"+rawQuery, nil)
	rec := httptest.NewRecorder()
	c := e.NewContext(req, rec)
	h := QueryGuard(func(c *echo.Context) error { return c.NoContent(http.StatusOK) })
	if err := h(c); err != nil {
		if he, ok := err.(*echo.HTTPError); ok {
			return he.Code
		}
		t.Fatalf("error tak terduga: %v", err)
	}
	return rec.Code
}

func TestQueryGuard(t *testing.T) {
	ok := []string{
		"", "page=1&page_size=200", "page=100000", "tenant_id=7", "tenant_id=&device_id=", "search=abc&prefix=xyz",
		"code=abc&state=xyz", // parameter OIDC tidak berakhiran _id
		"tag_id=18446744073709551615",
	}
	for _, q := range ok {
		if got := guardStatus(t, q); got != http.StatusOK {
			t.Errorf("%q harus lolos, dapat %d", q, got)
		}
	}

	bad := []string{
		"page=0", "page=-1", "page=abc", "page=100001", "page=99999999999999999999",
		"page_size=0", "page_size=201", "page_size=x",
		"tenant_id=abc", "device_id=-5", "vendor_id=0", "device_status_id=1.5", "tenant_id=18446744073709551616",
		"search=" + url.QueryEscape(strings.Repeat("a", maxQueryValueRunes+1)),
		"search=%ff%fe", // bukan UTF-8
		"tenant_id=1&tenant_id=abc",
		strings.Repeat("k=1&", maxQueryValues+1),
	}
	for _, q := range bad {
		if got := guardStatus(t, q); got != http.StatusBadRequest {
			t.Errorf("%q harus 400, dapat %d", q, got)
		}
	}
}
