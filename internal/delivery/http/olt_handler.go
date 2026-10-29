package http

import (
	"net/http"

	"acs/internal/domain"
	"acs/internal/usecase/olt"

	"github.com/labstack/echo/v5"
)

// OLT Controller irisan 1 (migrations/0044, docs/arsitektur/OLT_CONTROLLER_DESIGN.md): inventaris OLT per tenant + antrean ONU
// belum terdaftar. Read-only terhadap OLT. Baca terbuka untuk semua peran staf (scope tenant di usecase); kredensial tidak pernah
// dikembalikan, hanya penanda has_snmp_community / has_ssh_password. Identifier di API adalah UUID, PK tidak pernah diekspos.

// GET /olt-drivers — kode driver yang terdaftar di server ("sim" hanya bila ACS_OLT_SIM_ENABLED=true).
func (r *Router) listOLTDrivers(c *echo.Context) error {
	codes := r.OLTs.Drivers()
	return c.JSON(http.StatusOK, listResponse{Data: codes, Total: len(codes)})
}

// GET /olts?search=&tenant_id=
func (r *Router) listOLTs(c *echo.Context) error {
	f := domain.OLTFilter{Search: c.QueryParam("search"), TenantID: queryUint64(c, "tenant_id")}
	if err := checkMaxLen(lenRule{"search", f.Search, 255}); err != nil {
		return err
	}
	// tenant_id dari query hanya efektif untuk superadmin (OLTs.List menimpanya untuk peran lain).
	rows, total, err := r.OLTs.List(c.Request().Context(), ActorFrom(c), f, paginationFromQuery(c))
	if err != nil {
		return handleErr(c, err)
	}
	return c.JSON(http.StatusOK, listResponse{Data: rows, Total: total})
}

// GET /olts/:uuid
func (r *Router) getOLT(c *echo.Context) error {
	uuid := c.Param("uuid")
	if err := checkMaxLen(lenRule{"uuid", uuid, 36}); err != nil {
		return err
	}
	o, err := r.OLTs.Get(c.Request().Context(), ActorFrom(c), uuid)
	if err != nil {
		return handleErr(c, err)
	}
	return c.JSON(http.StatusOK, o)
}

type createOLTRequest struct {
	TenantID      *uint64 `json:"tenant_id"`
	Name          string  `json:"name"`
	DriverCode    string  `json:"driver_code"`
	Host          string  `json:"host"`
	SNMPPort      *int    `json:"snmp_port"`
	SSHPort       *int    `json:"ssh_port"`
	SNMPCommunity string  `json:"snmp_community"`
	SSHUsername   string  `json:"ssh_username"`
	SSHPassword   string  `json:"ssh_password"`
	Notes         string  `json:"notes"`
	IsActive      *bool   `json:"is_active"`
}

// POST /olts (ADMIN)
func (r *Router) createOLT(c *echo.Context) error {
	var req createOLTRequest
	if err := c.Bind(&req); err != nil {
		return echo.NewHTTPError(http.StatusBadRequest, "payload tidak valid")
	}
	if err := checkMaxLen(
		lenRule{"name", req.Name, domain.OLTNameMaxLen}, lenRule{"driver_code", req.DriverCode, domain.OLTDriverCodeMaxLen},
		lenRule{"host", req.Host, domain.OLTHostMaxLen}, lenRule{"notes", req.Notes, domain.OLTNotesMaxLen},
		lenRule{"ssh_username", req.SSHUsername, domain.OLTCredentialMaxLen},
		lenRule{"snmp_community", req.SNMPCommunity, domain.OLTCredentialMaxLen}, lenRule{"ssh_password", req.SSHPassword, domain.OLTCredentialMaxLen},
	); err != nil {
		return err
	}
	o, err := r.OLTs.Create(c.Request().Context(), ActorFrom(c), olt.CreateInput{
		TenantID: req.TenantID, Name: req.Name, DriverCode: req.DriverCode, Host: req.Host, SNMPPort: req.SNMPPort, SSHPort: req.SSHPort,
		SNMPCommunity: req.SNMPCommunity, SSHUsername: req.SSHUsername, SSHPassword: req.SSHPassword, Notes: req.Notes, IsActive: req.IsActive,
	})
	if err != nil {
		return handleErr(c, err)
	}
	return c.JSON(http.StatusCreated, o)
}

type updateOLTRequest struct {
	Name          *string `json:"name"`
	DriverCode    *string `json:"driver_code"`
	Host          *string `json:"host"`
	SNMPPort      *int    `json:"snmp_port"`
	SSHPort       *int    `json:"ssh_port"`
	SNMPCommunity *string `json:"snmp_community"`
	SSHUsername   *string `json:"ssh_username"`
	SSHPassword   *string `json:"ssh_password"`
	Notes         *string `json:"notes"`
	IsActive      *bool   `json:"is_active"`
}

func deref(s *string) string {
	if s == nil {
		return ""
	}
	return *s
}

// PATCH /olts/:uuid (ADMIN) — field yang tidak dikirim tidak berubah; kredensial/catatan "" mengosongkan.
func (r *Router) updateOLT(c *echo.Context) error {
	uuid := c.Param("uuid")
	if err := checkMaxLen(lenRule{"uuid", uuid, 36}); err != nil {
		return err
	}
	var req updateOLTRequest
	if err := c.Bind(&req); err != nil {
		return echo.NewHTTPError(http.StatusBadRequest, "payload tidak valid")
	}
	if err := checkMaxLen(
		lenRule{"name", deref(req.Name), domain.OLTNameMaxLen}, lenRule{"driver_code", deref(req.DriverCode), domain.OLTDriverCodeMaxLen},
		lenRule{"host", deref(req.Host), domain.OLTHostMaxLen}, lenRule{"notes", deref(req.Notes), domain.OLTNotesMaxLen},
		lenRule{"ssh_username", deref(req.SSHUsername), domain.OLTCredentialMaxLen},
		lenRule{"snmp_community", deref(req.SNMPCommunity), domain.OLTCredentialMaxLen}, lenRule{"ssh_password", deref(req.SSHPassword), domain.OLTCredentialMaxLen},
	); err != nil {
		return err
	}
	o, err := r.OLTs.Update(c.Request().Context(), ActorFrom(c), uuid, olt.UpdateInput{
		Name: req.Name, DriverCode: req.DriverCode, Host: req.Host, SNMPPort: req.SNMPPort, SSHPort: req.SSHPort,
		SNMPCommunity: req.SNMPCommunity, SSHUsername: req.SSHUsername, SSHPassword: req.SSHPassword, Notes: req.Notes, IsActive: req.IsActive,
	})
	if err != nil {
		return handleErr(c, err)
	}
	return c.JSON(http.StatusOK, o)
}

// DELETE /olts/:uuid (ADMIN) — soft delete beserta antrean ONU-nya.
func (r *Router) deleteOLT(c *echo.Context) error {
	uuid := c.Param("uuid")
	if err := checkMaxLen(lenRule{"uuid", uuid, 36}); err != nil {
		return err
	}
	if err := r.OLTs.Delete(c.Request().Context(), ActorFrom(c), uuid); err != nil {
		return handleErr(c, err)
	}
	return c.NoContent(http.StatusNoContent)
}

// POST /olts/:uuid/test (ADMIN) — uji koneksi; hasilnya tercatat pada OLT (poll_status, reported_name/version).
func (r *Router) testOLT(c *echo.Context) error {
	uuid := c.Param("uuid")
	if err := checkMaxLen(lenRule{"uuid", uuid, 36}); err != nil {
		return err
	}
	o, err := r.OLTs.Test(c.Request().Context(), ActorFrom(c), uuid)
	if err != nil {
		return handleErr(c, err)
	}
	return c.JSON(http.StatusOK, o)
}

// POST /olts/:uuid/poll (ADMIN/NOC) — baca ONU belum terdaftar dari OLT dan rekonsiliasi antrean.
func (r *Router) pollOLT(c *echo.Context) error {
	uuid := c.Param("uuid")
	if err := checkMaxLen(lenRule{"uuid", uuid, 36}); err != nil {
		return err
	}
	sum, err := r.OLTs.Poll(c.Request().Context(), ActorFrom(c), uuid)
	if err != nil {
		return handleErr(c, err)
	}
	return c.JSON(http.StatusOK, sum)
}

// GET /olt-onus?olt=<uuid OLT>&status=PENDING|IGNORED|RESOLVED&search=&tenant_id=
// Parameter filter OLT bernama `olt` (bukan `olt_id`): QueryGuard mewajibkan setiap parameter berakhiran _id berupa bilangan bulat,
// sedangkan identifier OLT di API adalah UUID.
func (r *Router) listOLTONUs(c *echo.Context) error {
	in := olt.ONUListInput{
		OLTUUID: c.QueryParam("olt"), Status: c.QueryParam("status"), Search: c.QueryParam("search"),
		TenantID: queryUint64(c, "tenant_id"),
	}
	if err := checkMaxLen(lenRule{"olt", in.OLTUUID, 36}, lenRule{"status", in.Status, 32}, lenRule{"search", in.Search, 255}); err != nil {
		return err
	}
	rows, total, err := r.OLTs.ListONUs(c.Request().Context(), ActorFrom(c), in, paginationFromQuery(c))
	if err != nil {
		return handleErr(c, err)
	}
	return c.JSON(http.StatusOK, listResponse{Data: rows, Total: total})
}

type patchOLTONURequest struct {
	Ignored *bool `json:"ignored"`
}

// PATCH /olt-onus/:uuid (ADMIN/NOC) — {"ignored": true} mengabaikan entri PENDING, {"ignored": false} memulihkan entri IGNORED.
// listOLTRegisteredONUs — GET /olts/:uuid/registered-onus: ONU terdaftar di OLT beserta keadaannya dan tautan ke perangkat ACS.
func (r *Router) listOLTRegisteredONUs(c *echo.Context) error {
	in := olt.RegisteredListInput{
		OLTUUID: c.Param("uuid"), PonPort: c.QueryParam("pon_port"), State: c.QueryParam("state"), Search: c.QueryParam("search"),
	}
	if err := checkMaxLen(lenRule{"uuid", in.OLTUUID, 36}, lenRule{"pon_port", in.PonPort, domain.ONUPortMaxLen},
		lenRule{"state", in.State, 32}, lenRule{"search", in.Search, 255}, lenRule{"sort", c.QueryParam("sort"), 16}); err != nil {
		return err
	}
	switch c.QueryParam("problem") {
	case "", "false":
	case "true":
		in.Problem = true
	default:
		return badRequest("problem harus true atau false")
	}
	in.Sort = c.QueryParam("sort")
	rows, total, err := r.OLTs.ListRegistered(c.Request().Context(), ActorFrom(c), in, paginationFromQuery(c))
	if err != nil {
		return handleErr(c, err)
	}
	return c.JSON(http.StatusOK, listResponse{Data: rows, Total: total})
}

// onuStateHistory — GET /olts/:uuid/registered-onus/:onu/history: riwayat perubahan keadaan satu ONU (terbaru dulu) + ringkasan kestabilan.
func (r *Router) onuStateHistory(c *echo.Context) error {
	oltUUID, onuUUID := c.Param("uuid"), c.Param("onu")
	if err := checkMaxLen(lenRule{"uuid", oltUUID, 36}, lenRule{"onu", onuUUID, 36}); err != nil {
		return err
	}
	h, err := r.OLTs.ONUHistory(c.Request().Context(), ActorFrom(c), oltUUID, onuUUID, paginationFromQuery(c))
	if err != nil {
		return handleErr(c, err)
	}
	return c.JSON(http.StatusOK, map[string]any{"data": h.Events, "total": h.Total, "changes_24h": h.Changes24h, "flapping": h.Flapping})
}

// listOLTPorts — GET /olts/:uuid/ports: ringkasan per port PON dari ONU terdaftar (hanya baca; bukan kontrol port).
func (r *Router) listOLTPorts(c *echo.Context) error {
	uuid := c.Param("uuid")
	if err := checkMaxLen(lenRule{"uuid", uuid, 36}); err != nil {
		return err
	}
	ports, err := r.OLTs.Ports(c.Request().Context(), ActorFrom(c), uuid)
	if err != nil {
		return handleErr(c, err)
	}
	return c.JSON(http.StatusOK, listResponse{Data: ports, Total: len(ports)})
}

// deviceOLT — GET /devices/:id/olt: posisi perangkat di jaringan OLT (OLT, port PON, keadaan menurut OLT).
func (r *Router) deviceOLT(c *echo.Context) error {
	id, err := parseUint64Param(c, "id")
	if err != nil {
		return echo.NewHTTPError(http.StatusBadRequest, "id tidak valid")
	}
	v, err := r.OLTs.ForDevice(c.Request().Context(), ActorFrom(c), id)
	if err != nil {
		return handleErr(c, err)
	}
	return c.JSON(http.StatusOK, v)
}

func (r *Router) updateOLTONU(c *echo.Context) error {
	uuid := c.Param("uuid")
	if err := checkMaxLen(lenRule{"uuid", uuid, 36}); err != nil {
		return err
	}
	var req patchOLTONURequest
	if err := c.Bind(&req); err != nil || req.Ignored == nil {
		return echo.NewHTTPError(http.StatusBadRequest, "payload tidak valid: ignored (true/false) wajib diisi")
	}
	n, err := r.OLTs.SetONUIgnored(c.Request().Context(), ActorFrom(c), uuid, *req.Ignored)
	if err != nil {
		return handleErr(c, err)
	}
	return c.JSON(http.StatusOK, n)
}
