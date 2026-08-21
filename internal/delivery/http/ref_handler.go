package http

import (
	"net/http"

	"github.com/labstack/echo/v5"
)

// listRefs mengekspos tabel ref_* (device status, task status, event code,
// dst.) agar frontend bisa menerjemahkan *_id ke label yang bisa dibaca
// tanpa hardcode mapping di sisi klien. Nama tabel divalidasi lewat whitelist
// yang sama dengan repository/mysql/ref_repository.go — request ke tabel di
// luar whitelist itu mengembalikan error, bukan diteruskan mentah ke SQL.
func (r *Router) listRefs(c *echo.Context) error {
	table := c.Param("table")
	rows, err := r.Refs.List(c.Request().Context(), table)
	if err != nil {
		return handleErr(c, err)
	}
	return c.JSON(http.StatusOK, rows)
}
