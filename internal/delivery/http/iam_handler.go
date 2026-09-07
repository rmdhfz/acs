package http

import (
	"net/http"

	"github.com/labstack/echo/v5"

	"acs/internal/usecase/iam"
)

type createTenantRequest struct {
	Code               string  `json:"code"`
	Name               string  `json:"name"`
	CWMPInformUsername *string `json:"cwmp_inform_username"`
	CWMPInformPassword *string `json:"cwmp_inform_password"`
}

func (r *Router) createTenant(c *echo.Context) error {
	actor := ActorFrom(c)
	var req createTenantRequest
	if err := c.Bind(&req); err != nil {
		return echo.NewHTTPError(http.StatusBadRequest, "payload tidak valid")
	}
	if req.Code == "" || req.Name == "" {
		return echo.NewHTTPError(http.StatusBadRequest, "code dan name wajib diisi")
	}
	if err := checkMaxLen(
		lenRule{"code", req.Code, maxTenantCode},
		lenRule{"name", req.Name, maxTenantName},
		lenRule{"cwmp_inform_username", optStr(req.CWMPInformUsername), maxTenantCWMPUser},
	); err != nil {
		return err
	}
	t, err := r.IAM.CreateTenant(c.Request().Context(), actor, iam.CreateTenantInput{
		Code: req.Code, Name: req.Name,
		CWMPInformUsername: req.CWMPInformUsername, CWMPInformPassword: req.CWMPInformPassword,
	})
	if err != nil {
		return handleErr(c, err)
	}
	return c.JSON(http.StatusCreated, t)
}

type setTenantCWMPCredentialsRequest struct {
	Username string `json:"username"`
	Password string `json:"password"`
}

func (r *Router) setTenantCWMPCredentials(c *echo.Context) error {
	actor := ActorFrom(c)
	id, err := parseUint64Param(c, "id")
	if err != nil {
		return echo.NewHTTPError(http.StatusBadRequest, "id tidak valid")
	}
	var req setTenantCWMPCredentialsRequest
	if err := c.Bind(&req); err != nil {
		return echo.NewHTTPError(http.StatusBadRequest, "payload tidak valid")
	}
	if req.Username == "" || req.Password == "" {
		return echo.NewHTTPError(http.StatusBadRequest, "username dan password wajib diisi")
	}
	// Password TIDAK dicek panjangnya di sini: disimpan terenkripsi
	// (VARBINARY), bukan sebagai VARCHAR polos.
	if err := checkMaxLen(lenRule{"username", req.Username, maxTenantCWMPUser}); err != nil {
		return err
	}
	if err := r.IAM.SetCWMPInformCredentials(c.Request().Context(), actor, id, req.Username, req.Password); err != nil {
		return handleErr(c, err)
	}
	return c.NoContent(http.StatusNoContent)
}

// currentTenantResponse — DTO sempit khusus endpoint ini. Beda dari
// listTenants/createTenant (superadmin-only, boleh balikin domain.Tenant
// utuh), endpoint ini dipanggil SEMUA role terautentikasi (ADMIN/NOC/VIEWER)
// hanya untuk keperluan branding — jangan ikut expose field sensitif macam
// cwmp_inform_username (setengah dari shared secret Inform CWMP) atau kolom
// audit internal.
type currentTenantResponse struct {
	ID           uint64  `json:"id"`
	Name         string  `json:"name"`
	BrandName    *string `json:"brand_name"`
	LogoURL      *string `json:"logo_url"`
	PrimaryColor *string `json:"primary_color"`
}

// getCurrentTenant — dipanggil semua role (bukan cuma superadmin) utk
// resolve branding tenant sendiri (ROADMAP.md Fase 2). Beda dari
// listTenants (superadmin-only).
func (r *Router) getCurrentTenant(c *echo.Context) error {
	actor := ActorFrom(c)
	t, err := r.IAM.GetCurrentTenant(c.Request().Context(), actor)
	if err != nil {
		return handleErr(c, err)
	}
	if t == nil {
		return c.NoContent(http.StatusNoContent)
	}
	return c.JSON(http.StatusOK, currentTenantResponse{
		ID: t.ID, Name: t.Name, BrandName: t.BrandName, LogoURL: t.LogoURL, PrimaryColor: t.PrimaryColor,
	})
}

type updateTenantBrandingRequest struct {
	BrandName    *string `json:"brand_name"`
	LogoURL      *string `json:"logo_url"`
	PrimaryColor *string `json:"primary_color"`
}

func (r *Router) updateTenantBranding(c *echo.Context) error {
	actor := ActorFrom(c)
	id, err := parseUint64Param(c, "id")
	if err != nil {
		return echo.NewHTTPError(http.StatusBadRequest, "id tidak valid")
	}
	var req updateTenantBrandingRequest
	if err := c.Bind(&req); err != nil {
		return echo.NewHTTPError(http.StatusBadRequest, "payload tidak valid")
	}
	if err := checkMaxLen(
		lenRule{"brand_name", optStr(req.BrandName), maxTenantBrandName},
		lenRule{"logo_url", optStr(req.LogoURL), maxTenantLogoURL},
		lenRule{"primary_color", optStr(req.PrimaryColor), maxTenantPrimaryHex},
	); err != nil {
		return err
	}
	if err := r.IAM.UpdateBranding(c.Request().Context(), actor, id, req.BrandName, req.LogoURL, req.PrimaryColor); err != nil {
		return handleErr(c, err)
	}
	return c.NoContent(http.StatusNoContent)
}

type setTenantTaskQuotaRequest struct {
	MaxPendingTasks *uint32 `json:"max_pending_tasks"`
}

// setTenantTaskQuota — superadmin only (kebijakan platform-level, bukan
// self-service tenant, lihat usecase/iam.SetTaskQuota). max_pending_tasks
// null berarti tidak dibatasi.
func (r *Router) setTenantTaskQuota(c *echo.Context) error {
	actor := ActorFrom(c)
	id, err := parseUint64Param(c, "id")
	if err != nil {
		return echo.NewHTTPError(http.StatusBadRequest, "id tidak valid")
	}
	var req setTenantTaskQuotaRequest
	if err := c.Bind(&req); err != nil {
		return echo.NewHTTPError(http.StatusBadRequest, "payload tidak valid")
	}
	if err := r.IAM.SetTaskQuota(c.Request().Context(), actor, id, req.MaxPendingTasks); err != nil {
		return handleErr(c, err)
	}
	return c.NoContent(http.StatusNoContent)
}

// updateTenantRequest -- partial update, field nil = tidak diubah (pola sama
// dgn updateUserRequest). IsActive adalah motivasi utama endpoint ini
// (ROADMAP.md: aktivasi/nonaktifkan tenant lewat API, sebelumnya cuma bisa
// lewat DB langsung).
type updateTenantRequest struct {
	Code     *string `json:"code"`
	Name     *string `json:"name"`
	IsActive *bool   `json:"is_active"`
}

// updateTenant -- superadmin only (RBAC gate di router.go + usecase/iam).
// Dampak menonaktifkan tenant (user tenant itu tidak bisa login lagi, termasuk
// yang bearer token-nya masih berlaku) didokumentasikan di usecase/iam.UpdateTenant.
func (r *Router) updateTenant(c *echo.Context) error {
	actor := ActorFrom(c)
	id, err := parseUint64Param(c, "id")
	if err != nil {
		return echo.NewHTTPError(http.StatusBadRequest, "id tidak valid")
	}
	var req updateTenantRequest
	if err := c.Bind(&req); err != nil {
		return echo.NewHTTPError(http.StatusBadRequest, "payload tidak valid")
	}
	if err := checkMaxLen(
		lenRule{"code", optStr(req.Code), maxTenantCode},
		lenRule{"name", optStr(req.Name), maxTenantName},
	); err != nil {
		return err
	}
	t, err := r.IAM.UpdateTenant(c.Request().Context(), actor, id, iam.UpdateTenantInput{
		Code: req.Code, Name: req.Name, IsActive: req.IsActive,
	})
	if err != nil {
		return handleErr(c, err)
	}
	return c.JSON(http.StatusOK, t)
}

func (r *Router) listTenants(c *echo.Context) error {
	actor := ActorFrom(c)
	tenants, total, err := r.IAM.ListTenants(c.Request().Context(), actor, paginationFromQuery(c))
	if err != nil {
		return handleErr(c, err)
	}
	return c.JSON(http.StatusOK, listResponse{Data: tenants, Total: total})
}

type createUserRequest struct {
	TenantID  *uint64  `json:"tenant_id"`
	Username  string   `json:"username"`
	Email     string   `json:"email"`
	Password  string   `json:"password"`
	FullName  string   `json:"full_name"`
	RoleCodes []string `json:"role_codes"`
}

func (r *Router) createUser(c *echo.Context) error {
	actor := ActorFrom(c)
	var req createUserRequest
	if err := c.Bind(&req); err != nil {
		return echo.NewHTTPError(http.StatusBadRequest, "payload tidak valid")
	}
	if req.Username == "" || req.Password == "" {
		return echo.NewHTTPError(http.StatusBadRequest, "username dan password wajib diisi")
	}
	if err := checkMaxLen(
		lenRule{"username", req.Username, maxUsername},
		lenRule{"email", req.Email, maxEmail},
		lenRule{"full_name", req.FullName, maxFullName},
	); err != nil {
		return err
	}
	u, err := r.IAM.CreateUser(c.Request().Context(), actor, iam.CreateUserInput{
		TenantID: req.TenantID, Username: req.Username, Email: req.Email,
		Password: req.Password, FullName: req.FullName, RoleCodes: req.RoleCodes,
	})
	if err != nil {
		return handleErr(c, err)
	}
	return c.JSON(http.StatusCreated, u)
}

func (r *Router) listUsers(c *echo.Context) error {
	actor := ActorFrom(c)
	users, total, err := r.IAM.ListUsers(c.Request().Context(), actor, queryUint64(c, "tenant_id"), paginationFromQuery(c))
	if err != nil {
		return handleErr(c, err)
	}
	return c.JSON(http.StatusOK, listResponse{Data: users, Total: total})
}

// updateUserRequest — partial update, field nil = tidak diubah (pola sama
// dgn updateTenantBrandingRequest tapi semantik nil-nya beda: di sini nil
// memang berarti "biarkan", bukan "kosongkan").
type updateUserRequest struct {
	FullName *string `json:"full_name"`
	Email    *string `json:"email"`
	IsActive *bool   `json:"is_active"`
}

// updateUser — RBAC & self-lockout guard ada di usecase/iam (bukan di sini,
// CLAUDE.md: handler hanya parsing/validasi & memanggil usecase).
func (r *Router) updateUser(c *echo.Context) error {
	actor := ActorFrom(c)
	id, err := parseUint64Param(c, "id")
	if err != nil {
		return echo.NewHTTPError(http.StatusBadRequest, "id tidak valid")
	}
	var req updateUserRequest
	if err := c.Bind(&req); err != nil {
		return echo.NewHTTPError(http.StatusBadRequest, "payload tidak valid")
	}
	if err := checkMaxLen(
		lenRule{"full_name", optStr(req.FullName), maxFullName},
		lenRule{"email", optStr(req.Email), maxEmail},
	); err != nil {
		return err
	}
	u, err := r.IAM.UpdateUser(c.Request().Context(), actor, id, iam.UpdateUserInput{
		FullName: req.FullName, Email: req.Email, IsActive: req.IsActive,
	})
	if err != nil {
		return handleErr(c, err)
	}
	return c.JSON(http.StatusOK, u)
}

type resetUserPasswordRequest struct {
	NewPassword string `json:"new_password"`
}

// resetUserPassword — admin-reset password user lain. Response tidak pernah
// mengandung password (plaintext ataupun hash) — 204 saja (CLAUDE.md §8).
func (r *Router) resetUserPassword(c *echo.Context) error {
	actor := ActorFrom(c)
	id, err := parseUint64Param(c, "id")
	if err != nil {
		return echo.NewHTTPError(http.StatusBadRequest, "id tidak valid")
	}
	var req resetUserPasswordRequest
	if err := c.Bind(&req); err != nil {
		return echo.NewHTTPError(http.StatusBadRequest, "payload tidak valid")
	}
	if req.NewPassword == "" {
		return echo.NewHTTPError(http.StatusBadRequest, "new_password wajib diisi")
	}
	if err := r.IAM.ResetUserPassword(c.Request().Context(), actor, id, req.NewPassword); err != nil {
		return handleErr(c, err)
	}
	return c.NoContent(http.StatusNoContent)
}

type replaceUserRolesRequest struct {
	RoleCodes []string `json:"role_codes"`
}

// replaceUserRoles — full-replace assignment role user (role yang tidak
// disebut di role_codes akan dicabut). Guard privilege-escalation & self-
// lockout ada di usecase/iam.
func (r *Router) replaceUserRoles(c *echo.Context) error {
	actor := ActorFrom(c)
	id, err := parseUint64Param(c, "id")
	if err != nil {
		return echo.NewHTTPError(http.StatusBadRequest, "id tidak valid")
	}
	var req replaceUserRolesRequest
	if err := c.Bind(&req); err != nil {
		return echo.NewHTTPError(http.StatusBadRequest, "payload tidak valid")
	}
	u, err := r.IAM.ReplaceUserRoles(c.Request().Context(), actor, id, req.RoleCodes)
	if err != nil {
		return handleErr(c, err)
	}
	return c.JSON(http.StatusOK, u)
}

// deleteUser — soft delete (is_deleted=1). Self-lockout guard ada di usecase/iam.
func (r *Router) deleteUser(c *echo.Context) error {
	actor := ActorFrom(c)
	id, err := parseUint64Param(c, "id")
	if err != nil {
		return echo.NewHTTPError(http.StatusBadRequest, "id tidak valid")
	}
	if err := r.IAM.DeleteUser(c.Request().Context(), actor, id); err != nil {
		return handleErr(c, err)
	}
	return c.NoContent(http.StatusNoContent)
}
