package http

import (
	"net/http"
	"strconv"

	"acs/internal/domain"

	"github.com/labstack/echo/v5"
)

// ---- TAGS ----

func (r *Router) createTag(c *echo.Context) error {
	actor := ActorFrom(c)
	var in domain.Tag
	if err := c.Bind(&in); err != nil {
		return echo.NewHTTPError(http.StatusBadRequest, err.Error())
	}
	if err := checkMaxLen(
		lenRule{"name", in.Name, maxTagName},
		lenRule{"color", optStr(in.Color), maxTagColor},
	); err != nil {
		return err
	}
	res, err := r.Tags.Create(c.Request().Context(), actor, in)
	if err != nil {
		return handleErr(c, err)
	}
	return c.JSON(http.StatusCreated, res)
}

func (r *Router) listTags(c *echo.Context) error {
	actor := ActorFrom(c)
	p := paginationFromQuery(c)
	tags, total, err := r.Tags.List(c.Request().Context(), actor, p)
	if err != nil {
		return handleErr(c, err)
	}
	return c.JSON(http.StatusOK, map[string]interface{}{
		"data": tags,
		"meta": map[string]interface{}{
			"total": total,
			"page":  p.Page,
			"limit": p.PageSize,
		},
	})
}

func (r *Router) deleteTag(c *echo.Context) error {
	actor := ActorFrom(c)
	id, err := strconv.ParseUint(c.Param("id"), 10, 64)
	if err != nil {
		return echo.NewHTTPError(http.StatusBadRequest, "invalid id")
	}
	if err := r.Tags.Delete(c.Request().Context(), actor, id); err != nil {
		return handleErr(c, err)
	}
	return c.NoContent(http.StatusNoContent)
}

// ---- DEVICE <-> TAG ----

type assignDeviceTagRequest struct {
	TagID uint64 `json:"tag_id"`
}

// listDeviceTags — tag yang menempel pada satu device. Kepemilikan device
// divalidasi lewat Devices.Get (enforce tenant scope) sebelum menyentuh tag.
func (r *Router) listDeviceTags(c *echo.Context) error {
	actor := ActorFrom(c)
	deviceID, err := parseUint64Param(c, "id")
	if err != nil {
		return echo.NewHTTPError(http.StatusBadRequest, "id tidak valid")
	}
	if _, err := r.Devices.Get(c.Request().Context(), actor, deviceID); err != nil {
		return handleErr(c, err)
	}
	tags, err := r.Tags.ListByDevice(c.Request().Context(), actor, deviceID)
	if err != nil {
		return handleErr(c, err)
	}
	return c.JSON(http.StatusOK, map[string]interface{}{"data": tags})
}

func (r *Router) assignDeviceTag(c *echo.Context) error {
	actor := ActorFrom(c)
	deviceID, err := parseUint64Param(c, "id")
	if err != nil {
		return echo.NewHTTPError(http.StatusBadRequest, "id tidak valid")
	}
	var req assignDeviceTagRequest
	if err := c.Bind(&req); err != nil {
		return echo.NewHTTPError(http.StatusBadRequest, err.Error())
	}
	if req.TagID == 0 {
		return echo.NewHTTPError(http.StatusBadRequest, "tag_id wajib diisi")
	}
	if _, err := r.Devices.Get(c.Request().Context(), actor, deviceID); err != nil {
		return handleErr(c, err)
	}
	if err := r.Tags.AssignToDevice(c.Request().Context(), actor, deviceID, req.TagID); err != nil {
		return handleErr(c, err)
	}
	return c.NoContent(http.StatusNoContent)
}

func (r *Router) removeDeviceTag(c *echo.Context) error {
	actor := ActorFrom(c)
	deviceID, err := parseUint64Param(c, "id")
	if err != nil {
		return echo.NewHTTPError(http.StatusBadRequest, "id tidak valid")
	}
	tagID, err := parseUint64Param(c, "tagId")
	if err != nil {
		return echo.NewHTTPError(http.StatusBadRequest, "tagId tidak valid")
	}
	if _, err := r.Devices.Get(c.Request().Context(), actor, deviceID); err != nil {
		return handleErr(c, err)
	}
	if err := r.Tags.RemoveFromDevice(c.Request().Context(), actor, deviceID, tagID); err != nil {
		return handleErr(c, err)
	}
	return c.NoContent(http.StatusNoContent)
}

// ---- PRESETS ----

func (r *Router) createPreset(c *echo.Context) error {
	actor := ActorFrom(c)
	var in domain.Preset
	if err := c.Bind(&in); err != nil {
		return echo.NewHTTPError(http.StatusBadRequest, err.Error())
	}
	if err := checkMaxLen(
		lenRule{"name", in.Name, maxPresetName},
		lenRule{"channel", optStr(in.Channel), maxPresetChan},
	); err != nil {
		return err
	}
	res, err := r.Presets.Create(c.Request().Context(), actor, in)
	if err != nil {
		return handleErr(c, err)
	}
	return c.JSON(http.StatusCreated, res)
}

func (r *Router) listPresets(c *echo.Context) error {
	actor := ActorFrom(c)
	p := paginationFromQuery(c)
	presets, total, err := r.Presets.List(c.Request().Context(), actor, p)
	if err != nil {
		return handleErr(c, err)
	}
	return c.JSON(http.StatusOK, map[string]interface{}{
		"data": presets,
		"meta": map[string]interface{}{
			"total": total,
			"page":  p.Page,
			"limit": p.PageSize,
		},
	})
}

func (r *Router) updatePreset(c *echo.Context) error {
	actor := ActorFrom(c)
	id, err := strconv.ParseUint(c.Param("id"), 10, 64)
	if err != nil {
		return echo.NewHTTPError(http.StatusBadRequest, "invalid id")
	}
	var in domain.Preset
	if err := c.Bind(&in); err != nil {
		return echo.NewHTTPError(http.StatusBadRequest, err.Error())
	}
	if err := checkMaxLen(
		lenRule{"name", in.Name, maxPresetName},
		lenRule{"channel", optStr(in.Channel), maxPresetChan},
	); err != nil {
		return err
	}
	if err := r.Presets.Update(c.Request().Context(), actor, id, in); err != nil {
		return handleErr(c, err)
	}
	res, err := r.Presets.Get(c.Request().Context(), actor, id)
	if err != nil {
		return handleErr(c, err)
	}
	return c.JSON(http.StatusOK, res)
}

func (r *Router) deletePreset(c *echo.Context) error {
	actor := ActorFrom(c)
	id, err := strconv.ParseUint(c.Param("id"), 10, 64)
	if err != nil {
		return echo.NewHTTPError(http.StatusBadRequest, "invalid id")
	}
	if err := r.Presets.Delete(c.Request().Context(), actor, id); err != nil {
		return handleErr(c, err)
	}
	return c.NoContent(http.StatusNoContent)
}
