package http

import (
	"net/http"
	"strconv"

	"acs/internal/domain"
	"acs/internal/usecase/tag"
	"acs/internal/usecase/preset"

	"github.com/labstack/echo/v5"
)

// ---- TAGS ----

func (r *Router) createTag(c *echo.Context) error {
	actor := ActorFrom(c)
	var in domain.Tag
	if err := c.Bind(&in); err != nil {
		return echo.NewHTTPError(http.StatusBadRequest, err.Error())
	}
	res, err := r.Tags.Create(c.Request().Context(), actor, in)
	if err != nil {
		return handleErr(c, err)
	}
	return c.JSON(http.StatusCreated, res)
}

func (r *Router) listTags(c *echo.Context) error {
	actor := ActorFrom(c)
	p := parsePagination(c)
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
	id, err := strconv.ParseUint(c.PathParam("id"), 10, 64)
	if err != nil {
		return echo.NewHTTPError(http.StatusBadRequest, "invalid id")
	}
	if err := r.Tags.Delete(c.Request().Context(), actor, id); err != nil {
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
	res, err := r.Presets.Create(c.Request().Context(), actor, in)
	if err != nil {
		return handleErr(c, err)
	}
	return c.JSON(http.StatusCreated, res)
}

func (r *Router) listPresets(c *echo.Context) error {
	actor := ActorFrom(c)
	p := parsePagination(c)
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
	id, err := strconv.ParseUint(c.PathParam("id"), 10, 64)
	if err != nil {
		return echo.NewHTTPError(http.StatusBadRequest, "invalid id")
	}
	var in domain.Preset
	if err := c.Bind(&in); err != nil {
		return echo.NewHTTPError(http.StatusBadRequest, err.Error())
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
	id, err := strconv.ParseUint(c.PathParam("id"), 10, 64)
	if err != nil {
		return echo.NewHTTPError(http.StatusBadRequest, "invalid id")
	}
	if err := r.Presets.Delete(c.Request().Context(), actor, id); err != nil {
		return handleErr(c, err)
	}
	return c.NoContent(http.StatusNoContent)
}
