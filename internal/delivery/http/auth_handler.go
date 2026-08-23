package http

import (
	"net/http"
	"time"

	"github.com/labstack/echo/v5"
)

type loginRequest struct {
	Username string `json:"username"`
	Password string `json:"password"`
}

func (r *Router) login(c *echo.Context) error {
	var req loginRequest
	if err := c.Bind(&req); err != nil {
		return echo.NewHTTPError(http.StatusBadRequest, "payload tidak valid")
	}
	if req.Username == "" || req.Password == "" {
		return echo.NewHTTPError(http.StatusBadRequest, "username dan password wajib diisi")
	}
	u, token, err := r.Auth.Login(c.Request().Context(), req.Username, req.Password)
	if err != nil {
		return echo.NewHTTPError(http.StatusUnauthorized, err.Error())
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
	actor := ActorFrom(c)
	var req changeOwnPasswordRequest
	if err := c.Bind(&req); err != nil {
		return echo.NewHTTPError(http.StatusBadRequest, "payload tidak valid")
	}
	if req.CurrentPassword == "" || req.NewPassword == "" {
		return echo.NewHTTPError(http.StatusBadRequest, "current_password dan new_password wajib diisi")
	}
	if err := r.Auth.ChangeOwnPassword(c.Request().Context(), actor, req.CurrentPassword, req.NewPassword); err != nil {
		return handleErr(c, err)
	}
	return c.NoContent(http.StatusNoContent)
}

type issueTokenRequest struct {
	Name      string     `json:"name"`
	TenantID  *uint64    `json:"tenant_id"`
	ExpiresAt *time.Time `json:"expires_at"`
}

func (r *Router) issueAPIToken(c *echo.Context) error {
	actor := ActorFrom(c)
	var req issueTokenRequest
	if err := c.Bind(&req); err != nil {
		return echo.NewHTTPError(http.StatusBadRequest, "payload tidak valid")
	}
	if req.Name == "" {
		return echo.NewHTTPError(http.StatusBadRequest, "name wajib diisi")
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
