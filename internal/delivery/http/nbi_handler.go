package http

import (
	"net/http"
	"strconv"

	"github.com/labstack/echo/v5"

	"acs/internal/domain"
)

// nbiListDevices — GET /nbi/devices: daftar perangkat bergaya NBI GenieACS (lihat nbi.go untuk subset yang didukung).
// Hanya menerjemahkan masukan lalu memakai Devices.ListWithSummary; scope tenant/RBAC tetap ditegakkan di usecase.
func (r *Router) nbiListDevices(c *echo.Context) error {
	var f domain.DeviceFilter
	if err := parseNBIQuery(c.QueryParam("query"), &f); err != nil {
		return err
	}
	if err := parseNBISort(c.QueryParam("sort"), &f); err != nil {
		return err
	}
	params, err := parseNBIProjection(c.QueryParam("projection"))
	if err != nil {
		return err
	}
	f.ProjectParams = params
	pg, err := nbiPagination(c.QueryParam("skip"), c.QueryParam("limit"))
	if err != nil {
		return err
	}

	devices, total, err := r.Devices.ListWithSummary(c.Request().Context(), ActorFrom(c), f, pg)
	if err != nil {
		return handleErr(c, err)
	}
	docs := make([]map[string]any, 0, len(devices))
	for i := range devices {
		docs = append(docs, nbiDeviceDoc(devices[i], params))
	}
	c.Response().Header().Set("X-Total-Count", strconv.Itoa(total))
	return c.JSON(http.StatusOK, docs)
}
