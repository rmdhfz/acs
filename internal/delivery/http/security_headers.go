package http

import "github.com/labstack/echo/v5"

// SecurityHeaders memasang header pengerasan standar (OWASP A05) pada setiap
// respons REST. API ini hanya menyajikan JSON/teks — tidak pernah HTML — sehingga
// CSP "default-src 'none'" aman dan menutup semua eksekusi konten bila sebuah
// respons pernah ter-render di browser.
//
//   - nosniff: browser tidak menebak tipe konten.
//   - DENY + frame-ancestors 'none': tidak bisa di-frame (clickjacking).
//   - no-referrer: URL (yang bisa memuat id/kueri) tidak bocor lewat Referer.
//   - no-store: respons berisi token/data tenant tidak di-cache proxy/browser.
//   - HSTS hanya bila request https (langsung atau lewat proxy yang menandai
//     X-Forwarded-Proto): mengirimnya lewat http polos tidak berarti apa-apa.
//
// Hanya dipasang di REST, TIDAK di endpoint CWMP (lawan bicaranya CPE, bukan browser).
func SecurityHeaders() echo.MiddlewareFunc {
	return func(next echo.HandlerFunc) echo.HandlerFunc {
		return func(c *echo.Context) error {
			h := c.Response().Header()
			h.Set("X-Content-Type-Options", "nosniff")
			h.Set("X-Frame-Options", "DENY")
			h.Set("Referrer-Policy", "no-referrer")
			h.Set("Content-Security-Policy", "default-src 'none'; frame-ancestors 'none'")
			h.Set("Cache-Control", "no-store")
			if requestIsSecure(c.Request()) {
				h.Set("Strict-Transport-Security", "max-age=31536000; includeSubDomains")
			}
			return next(c)
		}
	}
}
