package http

import (
	"encoding/json"
	"fmt"
	"net/http"
	"strings"
	"time"

	"acs/internal/domain"

	"github.com/labstack/echo/v5"
)

// Query perangkat bebas pada GET /devices (padanan query/projection/sort GenieACS, tetapi tertutup & ter-bind):
//
//	param=<nama>~<op>[~<nilai>]   berulang (maks 5, AND); op: eq ne like prefix gt ge lt le exists missing
//	params=<nama,nama,...>        sertakan nilai parameter ini pada tiap baris (maks 20) -> field "parameters"
//	fields=<kunci,kunci,...>      proyeksi: hanya kunci JSON ini yang dikirim (id selalu ikut)
//	sort=<kolom> | -<kolom>       urutan (domain.DeviceSortFields); default id menurun
//	last_inform_before|after=<RFC3339>

// deviceFieldKeys = kunci JSON yang boleh diminta lewat fields= (diturunkan dari bentuk respons, bukan ditulis ulang).
var deviceFieldKeys = func() map[string]bool {
	zero := domain.DeviceListItem{Parameters: map[string]string{"x": "y"}, Virtual: map[string]string{"x": "y"}}
	b, _ := json.Marshal(zero)
	var m map[string]json.RawMessage
	_ = json.Unmarshal(b, &m)
	keys := make(map[string]bool, len(m))
	for k := range m {
		keys[k] = true
	}
	return keys
}()

func badRequest(format string, a ...any) error {
	return echo.NewHTTPError(http.StatusBadRequest, fmt.Sprintf(format, a...))
}

// deviceFilterFromQuery membentuk filter dasar perangkat dari query string (filter kolom + serial); dipakai bersama oleh daftar
// (GET /devices) dan peta (GET /devices/map) supaya keduanya menyaring identik. Query bebas param= dst. ada di parseDeviceQuery.
func deviceFilterFromQuery(c *echo.Context) (domain.DeviceFilter, error) {
	f := domain.DeviceFilter{
		Search:         c.QueryParam("search"),
		SerialNumber:   c.QueryParam("serial_number"),
		VendorID:       queryUint64(c, "vendor_id"),
		DeviceModelID:  queryUint64(c, "device_model_id"),
		DeviceStatusID: queryUint64(c, "device_status_id"),
		TenantID:       queryUint64(c, "tenant_id"),
		TagID:          queryUint64(c, "tag_id"),
		OpticalLevel:   c.QueryParam("optical_level"),
		NoVendor:       c.QueryParam("no_vendor") == "true",
	}
	if f.OpticalLevel != "" && !domain.ValidOpticalLevel(f.OpticalLevel) {
		return f, echo.NewHTTPError(http.StatusBadRequest, "optical_level harus ok, warn, bad, atau unknown")
	}
	switch v := c.QueryParam("has_location"); v {
	case "":
	case "true", "false":
		b := v == "true"
		f.HasLocation = &b
	default:
		return f, echo.NewHTTPError(http.StatusBadRequest, "has_location harus true atau false")
	}
	if err := checkMaxLen(lenRule{"serial_number", f.SerialNumber, 128}); err != nil {
		return f, err
	}
	return f, nil
}

// parseDeviceQuery mengisi bagian query bebas dari filter; mengembalikan daftar fields= dan vparams= (kosong = tidak diminta).
func parseDeviceQuery(c *echo.Context, f *domain.DeviceFilter) (fields, vparams []string, err error) {
	raws := c.QueryParams()["param"]
	if len(raws) > domain.MaxParamConds {
		return nil, nil, badRequest("maksimal %d kondisi param per permintaan", domain.MaxParamConds)
	}
	for _, raw := range raws {
		cond, perr := domain.ParseParamCond(raw)
		if perr != nil {
			return nil, nil, badRequest("%s", strings.TrimPrefix(perr.Error(), domain.ErrInvalidInput.Error()+": "))
		}
		f.Params = append(f.Params, cond)
	}

	if v := c.QueryParam("params"); v != "" {
		names := splitList(v)
		if len(names) > domain.MaxProjectParams {
			return nil, nil, badRequest("maksimal %d nama pada params", domain.MaxProjectParams)
		}
		for _, n := range names {
			if !domain.ValidParamName(n) {
				return nil, nil, badRequest("nama parameter tidak valid pada params: %q", n)
			}
		}
		f.ProjectParams = names
	}

	if v := c.QueryParam("vparams"); v != "" {
		vparams = splitList(v)
		if len(vparams) > maxVParamProjection {
			return nil, nil, badRequest("maksimal %d nama pada vparams", maxVParamProjection)
		}
		for _, n := range vparams {
			if !domain.ValidVParamName(n) {
				return nil, nil, badRequest("nama virtual parameter tidak valid pada vparams: %q", n)
			}
		}
	}

	if f.Sort, f.SortDesc, err = domain.ParseSort(c.QueryParam("sort")); err != nil {
		return nil, nil, badRequest("%s", strings.TrimPrefix(err.Error(), domain.ErrInvalidInput.Error()+": "))
	}
	if f.InformBefore, err = queryTime(c, "last_inform_before"); err != nil {
		return nil, nil, err
	}
	if f.InformAfter, err = queryTime(c, "last_inform_after"); err != nil {
		return nil, nil, err
	}

	if v := c.QueryParam("fields"); v != "" {
		fields = splitList(v)
		for _, k := range fields {
			if !deviceFieldKeys[k] {
				return nil, nil, badRequest("fields berisi kunci tidak dikenal: %q", k)
			}
		}
	}
	return fields, vparams, nil
}

// maxVParamProjection membatasi jumlah virtual parameter per permintaan daftar (biaya evaluasi dikali ukuran halaman).
const maxVParamProjection = 10

// maxVParamEvaluations = batas len(vparams) x page_size per permintaan GET /devices.
const maxVParamEvaluations = 1000

func splitList(s string) []string {
	var out []string
	for _, p := range strings.Split(s, ",") {
		if p = strings.TrimSpace(p); p != "" {
			out = append(out, p)
		}
	}
	return out
}

func queryTime(c *echo.Context, name string) (*time.Time, error) {
	v := c.QueryParam(name)
	if v == "" {
		return nil, nil
	}
	t, err := time.Parse(time.RFC3339, v)
	if err != nil {
		return nil, badRequest("%s harus berformat RFC3339 (mis. 2026-10-22T03:00:00Z)", name)
	}
	return &t, nil
}

// projectDeviceFields menyisakan hanya kunci yang diminta (+ id). Urutan kunci mengikuti encoding JSON Go (alfabetis).
func projectDeviceFields(items []domain.DeviceListItem, fields []string) ([]map[string]json.RawMessage, error) {
	keep := map[string]bool{"id": true}
	for _, k := range fields {
		keep[k] = true
	}
	out := make([]map[string]json.RawMessage, 0, len(items))
	for i := range items {
		b, err := json.Marshal(items[i])
		if err != nil {
			return nil, err
		}
		var full map[string]json.RawMessage
		if err := json.Unmarshal(b, &full); err != nil {
			return nil, err
		}
		row := make(map[string]json.RawMessage, len(keep))
		for k := range keep {
			if v, ok := full[k]; ok {
				row[k] = v
			}
		}
		out = append(out, row)
	}
	return out, nil
}
