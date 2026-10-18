package http

import (
	"net/http"
	"strconv"
	"strings"
	"unicode/utf8"

	"github.com/labstack/echo/v5"
)

// Batas query-string. page_size 200 = batas kontrak openapi.yaml; page dibatasi agar OFFSET tidak overflow/men-scan
// terlalu dalam; panjang nilai query dibatasi agar LIKE/pencarian tidak menerima pola raksasa.
const (
	maxQueryPage       = 100000
	maxQueryPageSize   = 200
	maxQueryValueRunes = 255
	maxQueryValues     = 20
)

// QueryGuard menolak (400) query-string yang rusak SEBELUM handler berjalan, sehingga filter yang salah ketik tidak
// diam-diam berubah menjadi "tanpa filter" (fail-open) dan nilai di luar rentang tidak sampai ke SQL:
//   - page: bilangan bulat 1..100000; page_size: bilangan bulat 1..200;
//   - parameter berakhiran _id: bilangan bulat positif (uint64);
//   - setiap nilai maks. 255 karakter dan UTF-8 valid; satu kunci maks. 20 nilai.
//
// Dipasang pada grup /api/v1; jalur yang membaca query non-numerik (mis. OIDC callback `code`/`state`) tidak memakai
// akhiran _id sehingga tidak terpengaruh.
func QueryGuard(next echo.HandlerFunc) echo.HandlerFunc {
	return func(c *echo.Context) error {
		for key, values := range c.QueryParams() {
			if len(values) > maxQueryValues {
				return echo.NewHTTPError(http.StatusBadRequest, "parameter query "+key+" terlalu banyak nilai")
			}
			for _, v := range values {
				if !utf8.ValidString(v) || utf8.RuneCountInString(v) > maxQueryValueRunes {
					return echo.NewHTTPError(http.StatusBadRequest, "parameter query "+key+" tidak valid atau terlalu panjang")
				}
				if v == "" {
					continue // filter kosong = tidak dipakai (queryUint64 memperlakukannya sebagai nil)
				}
				switch {
				case key == "page":
					if n, err := strconv.Atoi(v); err != nil || n < 1 || n > maxQueryPage {
						return echo.NewHTTPError(http.StatusBadRequest, "page harus bilangan bulat 1-100000")
					}
				case key == "page_size":
					if n, err := strconv.Atoi(v); err != nil || n < 1 || n > maxQueryPageSize {
						return echo.NewHTTPError(http.StatusBadRequest, "page_size harus bilangan bulat 1-200")
					}
				case strings.HasSuffix(key, "_id"):
					if n, err := strconv.ParseUint(v, 10, 64); err != nil || n == 0 {
						return echo.NewHTTPError(http.StatusBadRequest, key+" harus bilangan bulat positif")
					}
				}
			}
		}
		return next(c)
	}
}
