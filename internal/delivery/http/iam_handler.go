package http

import (
	"net/http"

	"github.com/labstack/echo/v5"

	"acs/internal/usecase/iam"
)

type createTenantRequest struct {
	Code string `json:"code"`
	Name string `json:"name"`
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
	t, err := r.IAM.CreateTenant(c.Request().Context(), actor, req.Code, req.Name)
	if err != nil {
		return handleErr(c, err)
	}
	return c.JSON(http.StatusCreated, t)
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
