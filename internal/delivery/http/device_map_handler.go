package http

import (
	"net/http"

	"github.com/labstack/echo/v5"

	"acs/internal/domain"
)

// deviceMap — GET /devices/map: titik perangkat bergeolokasi untuk peta. Filter dasar sama dengan GET /devices (deviceFilterFromQuery) dan
// query bebas param= (parseDeviceQuery); opsional bbox=barat,selatan,timur,utara. Scope tenant dan proteksi parameter rahasia di usecase.
func (r *Router) deviceMap(c *echo.Context) error {
	f, err := deviceFilterFromQuery(c)
	if err != nil {
		return err
	}
	if _, _, err := parseDeviceQuery(c, &f); err != nil {
		return err
	}
	var box *domain.BBox
	if raw := c.QueryParam("bbox"); raw != "" {
		if box, err = domain.ParseBBox(raw); err != nil {
			return badRequest("%s", err.Error())
		}
	}
	res, err := r.Devices.ListForMap(c.Request().Context(), ActorFrom(c), f, box)
	if err != nil {
		return handleErr(c, err)
	}
	return c.JSON(http.StatusOK, res)
}
