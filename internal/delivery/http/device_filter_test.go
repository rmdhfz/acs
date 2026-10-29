package http

import (
	"net/http"
	"net/http/httptest"
	"testing"

	"github.com/labstack/echo/v5"
)

func ctxFor(rawQuery string) *echo.Context {
	req := httptest.NewRequest(http.MethodGet, "/devices?"+rawQuery, nil)
	return echo.New().NewContext(req, httptest.NewRecorder())
}

func TestDeviceFilterFromQuery_HasLocation(t *testing.T) {
	f, err := deviceFilterFromQuery(ctxFor(""))
	if err != nil || f.HasLocation != nil {
		t.Fatalf("tanpa parameter harus nil: %v %v", f.HasLocation, err)
	}
	f, err = deviceFilterFromQuery(ctxFor("has_location=true"))
	if err != nil || f.HasLocation == nil || !*f.HasLocation {
		t.Fatalf("true: %v %v", f.HasLocation, err)
	}
	f, err = deviceFilterFromQuery(ctxFor("has_location=false"))
	if err != nil || f.HasLocation == nil || *f.HasLocation {
		t.Fatalf("false: %v %v", f.HasLocation, err)
	}
	// Nilai lain ditolak (bukan diam-diam dianggap tanpa filter, supaya klien tahu filternya tak berlaku).
	for _, bad := range []string{"has_location=1", "has_location=TRUE", "has_location=ya", "has_location=null"} {
		if _, err := deviceFilterFromQuery(ctxFor(bad)); err == nil {
			t.Errorf("%s harus ditolak", bad)
		} else if he, ok := err.(*echo.HTTPError); !ok || he.Code != http.StatusBadRequest {
			t.Errorf("%s: err = %v, mau HTTP 400", bad, err)
		}
	}
}
