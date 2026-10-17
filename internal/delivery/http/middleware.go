package http

import (
	"errors"
	"log"
	"net/http"
	"strings"

	"github.com/labstack/echo/v5"

	"acs/internal/domain"
	"acs/internal/usecase/auth"
)

const actorContextKey = "actor"

// AuthMiddleware mewajibkan Bearer token (JWT sesi user atau API token
// "acs_...") pada setiap request — tidak ada endpoint mutasi tanpa
// autentikasi (CLAUDE.md).
func AuthMiddleware(authSvc *auth.Service) echo.MiddlewareFunc {
	return func(next echo.HandlerFunc) echo.HandlerFunc {
		return func(c *echo.Context) error {
			tokenStr := bearerFromRequest(c.Request())
			if tokenStr == "" {
				return echo.NewHTTPError(http.StatusUnauthorized, "missing bearer token")
			}

			// ResolveActor menyatukan parsing JWT/API token DAN cek tenant aktif
			// (auth.ErrTenantInactive) -- lihat komentar lengkap di
			// usecase/auth.Service.ResolveActor soal kenapa ini dicek di SETIAP
			// request, bukan cuma saat login.
			actor, err := authSvc.ResolveActor(c.Request().Context(), tokenStr)
			if err != nil {
				switch {
				case errors.Is(err, auth.ErrTenantInactive):
					return echo.NewHTTPError(http.StatusUnauthorized, err.Error())
				case errors.Is(err, auth.ErrInvalidToken):
					return echo.NewHTTPError(http.StatusUnauthorized, "invalid token")
				default:
					// Error SISTEM (mis. DB timeout saat checkTenantActive),
					// BUKAN token yang salah -- jangan disamarkan jadi 401
					// "invalid token" (temuan review: itu menyulitkan on-call
					// membedakan insiden nyata dari gangguan infrastruktur
					// sesaat). Di-log server-side, klien dapat 500 generik
					// tanpa detail error internal.
					log.Printf("auth middleware: gagal resolve actor: %v", err)
					return echo.NewHTTPError(http.StatusInternalServerError, "internal error")
				}
			}
			if actor == nil {
				return echo.NewHTTPError(http.StatusUnauthorized, "invalid token")
			}
			c.Set(actorContextKey, *actor)
			if actor.ImpersonatorID != 0 {
				// audit: setiap catatan selama impersonate memuat impersonated_by
				req := c.Request()
				meta, _ := domain.RequestMetaFrom(req.Context())
				meta.ImpersonatorID = actor.ImpersonatorID
				c.SetRequest(req.WithContext(domain.WithRequestMeta(req.Context(), meta)))
			}
			return next(c)
		}
	}
}

// wsBearerProtocol adalah penanda subprotocol WebSocket: peramban tidak bisa
// mengirim header Authorization pada new WebSocket(), jadi klien menawarkan
// ["acs.bearer", "<token>"] lewat Sec-WebSocket-Protocol. Dibanding query
// string, token di header ini tidak masuk access log/riwayat/Referer.
const wsBearerProtocol = "acs.bearer"

// bearerFromRequest mengambil token dari header Authorization; khusus
// permintaan upgrade WebSocket juga dari Sec-WebSocket-Protocol. Kosong = tidak ada.
func bearerFromRequest(r *http.Request) string {
	if h := r.Header.Get("Authorization"); h != "" {
		if strings.HasPrefix(h, "Bearer ") {
			return strings.TrimPrefix(h, "Bearer ")
		}
		return ""
	}
	if !strings.EqualFold(r.Header.Get("Upgrade"), "websocket") {
		return ""
	}
	var offered []string
	for _, v := range r.Header.Values("Sec-WebSocket-Protocol") {
		for _, p := range strings.Split(v, ",") {
			if p = strings.TrimSpace(p); p != "" {
				offered = append(offered, p)
			}
		}
	}
	for i, p := range offered {
		if p == wsBearerProtocol && i+1 < len(offered) {
			return offered[i+1]
		}
	}
	return ""
}

func ActorFrom(c *echo.Context) domain.Actor {
	if a, ok := c.Get(actorContextKey).(domain.Actor); ok {
		return a
	}
	return domain.Actor{}
}

// RequireRoles menolak request bila actor tidak superadmin dan tidak punya
// salah satu role yang diizinkan (RBAC scope, CLAUDE.md).
func RequireRoles(roles ...string) echo.MiddlewareFunc {
	return func(next echo.HandlerFunc) echo.HandlerFunc {
		return func(c *echo.Context) error {
			if err := auth.RequireRole(ActorFrom(c), roles...); err != nil {
				return echo.NewHTTPError(http.StatusForbidden, "insufficient role")
			}
			return next(c)
		}
	}
}

// staffOnly menolak principal yang tidak memiliki satu pun role staf internal
// (SUPERADMIN/ADMIN/NOC/VIEWER). Tujuannya mengurung token ENDUSER (portal
// pelanggan) agar tidak bisa memanggil API staf — GET /devices, /tasks, dll.
// yang hanya ter-scope tenant, bukan role, dan akan membocorkan data seluruh
// tenant ke satu pelanggan. Portal ENDUSER punya grup sendiri (/self-service).
func staffOnly(next echo.HandlerFunc) echo.HandlerFunc {
	return func(c *echo.Context) error {
		a := ActorFrom(c)
		if a.IsSuperadmin() || a.HasRole(domain.RoleAdmin) ||
			a.HasRole(domain.RoleNOC) || a.HasRole(domain.RoleViewer) {
			return next(c)
		}
		return echo.NewHTTPError(http.StatusForbidden, "akses terbatas: portal pelanggan hanya /self-service")
	}
}

func handleErr(c *echo.Context, err error) error {
	switch {
	// auth.ErrInvalidCredentials -- dipakai auth.Service.ChangeOwnPassword saat
	// current_password tidak cocok (bukan cuma di Login). Bukan domain.Err*
	// krn ini spesifik ke package auth (dan supaya pesan bisa disamakan dgn
	// pesan login gagal biasa) -- dicek terpisah di sini, sebelum default 500.
	case errors.Is(err, auth.ErrInvalidCredentials):
		return echo.NewHTTPError(http.StatusUnauthorized, err.Error())
	case errors.Is(err, auth.ErrInvalidMFACode):
		// 400 (bukan 401): pada endpoint ber-sesi, kode salah bukan berarti sesi habis.
		return echo.NewHTTPError(http.StatusBadRequest, err.Error())
	case errors.Is(err, domain.ErrNotFound):
		return echo.NewHTTPError(http.StatusNotFound, err.Error())
	case errors.Is(err, domain.ErrForbidden):
		return echo.NewHTTPError(http.StatusForbidden, err.Error())
	case errors.Is(err, domain.ErrUnauthorized):
		return echo.NewHTTPError(http.StatusUnauthorized, err.Error())
	case errors.Is(err, domain.ErrConflict):
		return echo.NewHTTPError(http.StatusConflict, err.Error())
	case errors.Is(err, domain.ErrInvalidInput):
		return echo.NewHTTPError(http.StatusBadRequest, err.Error())
	case errors.Is(err, domain.ErrQuotaExceeded):
		return echo.NewHTTPError(http.StatusTooManyRequests, err.Error())
	case errors.Is(err, domain.ErrUpstreamUnavailable):
		// CPE offline / tak dapat dihubungi — kondisi operasional wajar,
		// bukan bug ACS. 502, bukan 500.
		return echo.NewHTTPError(http.StatusBadGateway, err.Error())
	default:
		// Error sistem (DB timeout, query error, dsb.) — JANGAN di-expose
		// ke client (bisa bocor detail internal: query SQL, stack trace,
		// nama tabel, dsb.). Di-log server-side dengan detail penuh, client
		// hanya dapat pesan generik. Berbeda dari domain error di atas yang
		// memang dirancang untuk dikembalikan ke client (domain.ErrNotFound
		// dsb. tidak mengandung detail implementasi).
		log.Printf("handleErr: unhandled error: %v", err)
		return echo.NewHTTPError(http.StatusInternalServerError, "internal server error")
	}
}
