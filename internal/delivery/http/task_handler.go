package http

import (
	"net/http"
	"time"

	"github.com/labstack/echo/v5"

	"acs/internal/domain"
)

type createTaskRequest struct {
	DeviceID    uint64                 `json:"device_id"`
	TaskType    string                 `json:"task_type"`
	Priority    uint8                  `json:"priority"`
	Parameters  map[string]interface{} `json:"parameters"`
	MaxRetries  uint32                 `json:"max_retries"`
	ScheduledAt *time.Time             `json:"scheduled_at"`
	ExpiresAt   *time.Time             `json:"expires_at"`
}

func (r *Router) createTask(c *echo.Context) error {
	actor := ActorFrom(c)
	var req createTaskRequest
	if err := c.Bind(&req); err != nil {
		return echo.NewHTTPError(http.StatusBadRequest, "payload tidak valid")
	}
	if req.DeviceID == 0 || req.TaskType == "" {
		return echo.NewHTTPError(http.StatusBadRequest, "device_id dan task_type wajib diisi")
	}
	if _, err := r.Refs.GetByCode(c.Request().Context(), domain.RefTableTaskTypes, req.TaskType); err != nil {
		return echo.NewHTTPError(http.StatusBadRequest, "task_type tidak valid")
	}
	t, err := r.Tasks.CreateTask(c.Request().Context(), actor, domain.CreateTaskInput{
		DeviceID: req.DeviceID, TaskType: req.TaskType, Priority: req.Priority,
		Parameters: req.Parameters, MaxRetries: req.MaxRetries, ScheduledAt: req.ScheduledAt, ExpiresAt: req.ExpiresAt,
	})
	if err != nil {
		return handleErr(c, err)
	}
	return c.JSON(http.StatusCreated, t)
}

func (r *Router) listTasks(c *echo.Context) error {
	actor := ActorFrom(c)
	f := domain.TaskFilter{
		DeviceID:     queryUint64(c, "device_id"),
		TaskStatusID: queryUint64(c, "task_status_id"),
		TaskTypeCode: c.QueryParam("task_type"),
		// TenantID dari query hanya efektif utk superadmin — Tasks.List
		// menimpanya dgn actor.TenantID utk non-superadmin (RBAC scope tenant).
		TenantID: queryUint64(c, "tenant_id"),
	}
	tasks, total, err := r.Tasks.List(c.Request().Context(), actor, f, paginationFromQuery(c))
	if err != nil {
		return handleErr(c, err)
	}
	return c.JSON(http.StatusOK, listResponse{Data: tasks, Total: total})
}

func (r *Router) taskStats(c *echo.Context) error {
	actor := ActorFrom(c)
	stats, err := r.Tasks.Stats(c.Request().Context(), actor)
	if err != nil {
		return handleErr(c, err)
	}
	return c.JSON(http.StatusOK, stats)
}

func (r *Router) getTask(c *echo.Context) error {
	actor := ActorFrom(c)
	id, err := parseUint64Param(c, "id")
	if err != nil {
		return echo.NewHTTPError(http.StatusBadRequest, "id tidak valid")
	}
	t, err := r.Tasks.Get(c.Request().Context(), actor, id)
	if err != nil {
		return handleErr(c, err)
	}
	return c.JSON(http.StatusOK, t)
}

func (r *Router) cancelTask(c *echo.Context) error {
	actor := ActorFrom(c)
	id, err := parseUint64Param(c, "id")
	if err != nil {
		return echo.NewHTTPError(http.StatusBadRequest, "id tidak valid")
	}
	if err := r.Tasks.Cancel(c.Request().Context(), actor, id); err != nil {
		return handleErr(c, err)
	}
	return c.NoContent(http.StatusNoContent)
}
