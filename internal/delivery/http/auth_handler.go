package http

import (
	"context"
	"errors"
	"log"
	"net/http"
	"net/url"
	"strings"
	"time"

	"github.com/labstack/echo/v5"

	"acs/internal/usecase/auth"
	"acs/pkg/passwordhash"
)

// forgotSlots membatasi jumlah pemrosesan forgot-password yang berjalan
// bersamaan (goroutine terlepas dari request). Penuh -> permintaan tetap dijawab
// 202 seragam tetapi tidak diproses (sedang dibanjiri; limiter per-IP sudah
// membatasi sumbernya).
var forgotSlots = make(chan struct{}, 32)

// loginRequest — login memakai EMAIL + password. Field username tetap
// diterima (kompatibilitas skrip/klien lama); bila keduanya ada, email menang.
type loginRequest struct {
	Email    string `json:"email"`
	Username string `json:"username"`
	Password string `json:"password"`
}

func (r *Router) login(c *echo.Context) error {
	var req loginRequest
	if err := c.Bind(&req); err != nil {
		return echo.NewHTTPError(http.StatusBadRequest, "payload tidak valid")
	}
	identifier := strings.TrimSpace(req.Email)
	if identifier == "" {
		identifier = strings.TrimSpace(req.Username)
	}
	if identifier == "" || req.Password == "" {
		return echo.NewHTTPError(http.StatusBadRequest, "email dan password wajib diisi")
	}
	u, token, err := r.Auth.Login(c.Request().Context(), identifier, req.Password)
	if errors.Is(err, auth.ErrMFARequired) {
		// Password benar, tahap kedua dibutuhkan: token ini BUKAN sesi (hanya untuk /auth/mfa/verify).
		c.Response().Header().Set("Cache-Control", "no-store")
		return c.JSON(http.StatusOK, map[string]interface{}{"mfa_required": true, "mfa_token": token})
	}
	if err != nil {
		switch {
		case errors.Is(err, auth.ErrInvalidCredentials), errors.Is(err, auth.ErrTenantInactive):
			return echo.NewHTTPError(http.StatusUnauthorized, err.Error())
		case errors.Is(err, passwordhash.ErrBusy):
			// Antrean hashing penuh — BUKAN kredensial salah.
			return echo.NewHTTPError(http.StatusServiceUnavailable, "server sedang sibuk, coba lagi sebentar")
		default:
			// Error sistem (DB, dll.): jangan bocorkan teksnya ke klien anonim
			// dan jangan menyamarkannya sebagai "kredensial salah" (handleErr: 500 generik + log).
			return handleErr(c, err)
		}
	}
	return c.JSON(http.StatusOK, map[string]interface{}{
		"access_token": token,
		"token_type":   "Bearer",
		"user": map[string]interface{}{
			"id": u.ID, "uuid": u.UserUUID, "username": u.Username, "roles": u.Roles, "tenant_id": u.TenantID,
		},
	})
}

type changeOwnPasswordRequest struct {
	CurrentPassword string `json:"current_password"`
	NewPassword     string `json:"new_password"`
}

// changeOwnPassword -- user ganti password SENDIRI dgn verifikasi password
// lama (BEDA dari resetUserPassword di iam_handler.go yang itu admin mereset
// password ORANG LAIN). Actor diambil dari JWT (bukan :id di path) -- semua
// role yang sudah login boleh memanggil ini, tidak ada RequireRoles di router.go.
func (r *Router) changeOwnPassword(c *echo.Context) error {
	if err := denyIfImpersonating(c); err != nil {
		return err
	}
	actor := ActorFrom(c)
	var req changeOwnPasswordRequest
	if err := c.Bind(&req); err != nil {
		return echo.NewHTTPError(http.StatusBadRequest, "payload tidak valid")
	}
	if req.CurrentPassword == "" || req.NewPassword == "" {
		return echo.NewHTTPError(http.StatusBadRequest, "current_password dan new_password wajib diisi")
	}
	// Pergantian password mencabut semua sesi lama; respons membawa JWT baru
	// supaya pemanggil tetap login.
	token, err := r.Auth.ChangeOwnPassword(c.Request().Context(), actor, req.CurrentPassword, req.NewPassword)
	if err != nil {
		if errors.Is(err, passwordhash.ErrBusy) {
			return echo.NewHTTPError(http.StatusServiceUnavailable, "server sedang sibuk, coba lagi sebentar")
		}
		return handleErr(c, err)
	}
	return c.JSON(http.StatusOK, map[string]string{"access_token": token, "token_type": "Bearer"})
}

type issueTokenRequest struct {
	Name      string     `json:"name"`
	TenantID  *uint64    `json:"tenant_id"`
	ExpiresAt *time.Time `json:"expires_at"`
}

func (r *Router) issueAPIToken(c *echo.Context) error {
	if err := denyIfImpersonating(c); err != nil {
		return err
	}
	actor := ActorFrom(c)
	// API token tidak boleh menerbitkan token baru (token curian bisa beranak-pinak
	// dan bertahan walau token asalnya dicabut); hanya sesi login (JWT).
	if strings.HasPrefix(bearerFromRequest(c.Request()), "acs_") {
		return echo.NewHTTPError(http.StatusForbidden, "API token tidak dapat menerbitkan token baru; gunakan sesi login")
	}
	var req issueTokenRequest
	if err := c.Bind(&req); err != nil {
		return echo.NewHTTPError(http.StatusBadRequest, "payload tidak valid")
	}
	if req.Name == "" {
		return echo.NewHTTPError(http.StatusBadRequest, "name wajib diisi")
	}
	if err := checkMaxLen(lenRule{"name", req.Name, maxAPITokenName}); err != nil {
		return err
	}
	tenantID := req.TenantID
	if tenantID == nil {
		tenantID = actor.TenantID
	}
	plain, rec, err := r.Auth.IssueAPIToken(c.Request().Context(), actor, req.Name, tenantID, req.ExpiresAt)
	if err != nil {
		return handleErr(c, err)
	}
	// Token plaintext hanya ditampilkan sekali di respons ini — tidak pernah
	// disimpan/di-log lagi setelahnya (hanya hash-nya yang tersimpan).
	return c.JSON(http.StatusCreated, map[string]interface{}{
		"token": plain,
		"id":    rec.ID,
		"uuid":  rec.TokenUUID,
	})
}

// listAPITokens -- RBAC/tenant-scope penuh ada di usecase (Auth.ListAPITokens),
// handler hanya parsing query & memanggil usecase (CLAUDE.md).
func (r *Router) listAPITokens(c *echo.Context) error {
	actor := ActorFrom(c)
	tokens, total, err := r.Auth.ListAPITokens(c.Request().Context(), actor, queryUint64(c, "tenant_id"), paginationFromQuery(c))
	if err != nil {
		return handleErr(c, err)
	}
	return c.JSON(http.StatusOK, listResponse{Data: tokens, Total: total})
}

// revokeAPIToken -- RBAC/tenant-scope penuh ada di usecase
// (Auth.RevokeAPIToken: resolve tenant pemilik token dulu baru cek scope).
func (r *Router) revokeAPIToken(c *echo.Context) error {
	actor := ActorFrom(c)
	id, err := parseUint64Param(c, "id")
	if err != nil {
		return echo.NewHTTPError(http.StatusBadRequest, "id tidak valid")
	}
	if err := r.Auth.RevokeAPIToken(c.Request().Context(), actor, id); err != nil {
		return handleErr(c, err)
	}
	return c.NoContent(http.StatusNoContent)
}

func (r *Router) oidcLogin(c *echo.Context) error {
	// Cek konfigurasi SEBELUM menyentuh cookie: OIDC mati = tidak ada efek samping.
	if !r.Auth.HasOIDC() {
		return echo.NewHTTPError(http.StatusInternalServerError, "auth: OIDC tidak dikonfigurasi")
	}
	state, nonce, err := startOIDCFlow(c)
	if err != nil {
		return echo.NewHTTPError(http.StatusInternalServerError, "gagal memulai login OIDC")
	}
	url, err := r.Auth.GetOIDCAuthURL(state, nonce)
	if err != nil {
		return echo.NewHTTPError(http.StatusInternalServerError, err.Error())
	}
	return c.Redirect(http.StatusFound, url)
}

func (r *Router) oidcCallback(c *echo.Context) error {
	// state dicek lebih dulu dari apa pun: tanpa cookie yang cocok, kode
	// otorisasi ini bukan hasil alur yang dimulai browser ini (login-CSRF).
	nonce, err := finishOIDCFlow(c)
	if err != nil {
		return handleErr(c, err)
	}
	code := c.QueryParam("code")
	if code == "" {
		return echo.NewHTTPError(http.StatusBadRequest, "kode OIDC tidak ditemukan")
	}

	_, token, err := r.Auth.OIDCLogin(c.Request().Context(), code, nonce)
	c.Response().Header().Set("Cache-Control", "no-store")
	if err != nil {
		// Pesan asli bisa memuat respons IdP, atau membedakan "akun belum terdaftar"
		// (enumerasi email) — detail hanya ke log; pengguna dikembalikan ke halaman login.
		log.Printf("oidc callback: login gagal: %v", err)
		return c.Redirect(http.StatusFound, r.PublicURL+"/login?sso=error")
	}
	// Token dikirim lewat FRAGMENT (#token=) ke Console: tidak ikut permintaan HTTP/log/Referer.
	return c.Redirect(http.StatusFound, r.PublicURL+"/sso#token="+url.QueryEscape(token))
}

// authConfig — konfigurasi publik untuk halaman login (tanpa rahasia).
func (r *Router) authConfig(c *echo.Context) error {
	return c.JSON(http.StatusOK, map[string]any{"sso_enabled": r.Auth.OIDCConfigured(), "sso_provider": r.Auth.OIDCProvider(), "mfa_available": r.Auth.MFAEnabledFeature(), "self_signup_enabled": r.signupEnabled()})
}

// me — profil akun yang sedang login.
func (r *Router) me(c *echo.Context) error {
	u, err := r.Auth.Me(c.Request().Context(), ActorFrom(c))
	if err != nil {
		return handleErr(c, err)
	}
	return c.JSON(http.StatusOK, map[string]interface{}{
		"id": u.ID, "uuid": u.UserUUID, "username": u.Username, "roles": u.Roles, "tenant_id": u.TenantID,
		"email": u.Email, "full_name": u.FullName, "mfa_enabled": u.TOTPEnabled,
		"impersonator_id": ActorFrom(c).ImpersonatorID,
	})
}

type forgotPasswordRequest struct {
	Email string `json:"email"`
}

// forgotPassword — PUBLIK (tanpa token) karena pemanggilnya justru user yang tidak
// bisa login; pengecualian setara /auth/login atas aturan "tidak ada endpoint
// mutasi tanpa autentikasi", dengan kontrol: rate limit per-IP ketat (router),
// batas permintaan per akun per jam (usecase), respons SERAGAM terlepas dari
// apakah email terdaftar, dan pemrosesan di goroutine terpisah supaya waktu
// respons tidak membocorkan keberadaan akun.
func (r *Router) forgotPassword(c *echo.Context) error {
	var req forgotPasswordRequest
	if err := c.Bind(&req); err != nil {
		return echo.NewHTTPError(http.StatusBadRequest, "payload tidak valid")
	}
	if strings.TrimSpace(req.Email) == "" {
		return echo.NewHTTPError(http.StatusBadRequest, "email wajib diisi")
	}
	if !r.Auth.PasswordResetEnabled() {
		return echo.NewHTTPError(http.StatusServiceUnavailable, "reset password lewat email belum diaktifkan — hubungi administrator")
	}
	email, ip := req.Email, c.RealIP()
	select {
	case forgotSlots <- struct{}{}:
		go func() {
			defer func() { <-forgotSlots }()
			// Goroutine terlepas dari request: Recover middleware Echo TIDAK
			// melindunginya, jadi panic di sini harus ditangkap sendiri.
			defer func() {
				if rec := recover(); rec != nil {
					log.Printf("forgotPassword: panic: %v", rec)
				}
			}()
			ctx, cancel := context.WithTimeout(context.Background(), 20*time.Second)
			defer cancel()
			r.Auth.RequestPasswordReset(ctx, email, ip)
		}()
	default:
		// Slot habis (banjir): tidak diproses, respons tetap seragam.
	}
	return c.JSON(http.StatusAccepted, map[string]string{
		"message": "Jika email terdaftar, tautan reset password telah dikirim. Periksa kotak masuk (dan folder spam).",
	})
}

type resetPasswordRequest struct {
	Token       string `json:"token"`
	NewPassword string `json:"new_password"`
}

// resetPassword — PUBLIK, tetapi hanya bisa berhasil dengan token sekali-pakai
// dari email (hash-nya di DB, berumur pendek). Dilindungi rate limit per-IP.
func (r *Router) resetPassword(c *echo.Context) error {
	var req resetPasswordRequest
	if err := c.Bind(&req); err != nil {
		return echo.NewHTTPError(http.StatusBadRequest, "payload tidak valid")
	}
	if req.Token == "" || req.NewPassword == "" {
		return echo.NewHTTPError(http.StatusBadRequest, "token dan new_password wajib diisi")
	}
	err := r.Auth.ResetPassword(c.Request().Context(), req.Token, req.NewPassword)
	switch {
	case err == nil:
		return c.NoContent(http.StatusNoContent)
	case errors.Is(err, auth.ErrInvalidResetToken):
		return echo.NewHTTPError(http.StatusBadRequest, "tautan reset tidak valid atau sudah kedaluwarsa — minta tautan baru")
	case errors.Is(err, auth.ErrPasswordResetDisabled):
		return echo.NewHTTPError(http.StatusServiceUnavailable, "reset password lewat email belum diaktifkan")
	case errors.Is(err, passwordhash.ErrBusy):
		return echo.NewHTTPError(http.StatusServiceUnavailable, "server sedang sibuk, coba lagi sebentar")
	}
	return handleErr(c, err)
}
