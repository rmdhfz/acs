package http

import (
	"strconv"

	"github.com/labstack/echo/v5"

	"acs/internal/domain"
)

type listResponse struct {
	Data  interface{} `json:"data"`
	Total int         `json:"total"`
}

func paginationFromQuery(c *echo.Context) domain.Pagination {
	page, _ := strconv.Atoi(c.QueryParam("page"))
	pageSize, _ := strconv.Atoi(c.QueryParam("page_size"))
	return domain.Pagination{Page: page, PageSize: pageSize}
}

func parseUint64Param(c *echo.Context, name string) (uint64, error) {
	return strconv.ParseUint(c.Param(name), 10, 64)
}

func queryUint64(c *echo.Context, name string) *uint64 {
	v := c.QueryParam(name)
	if v == "" {
		return nil
	}
	n, err := strconv.ParseUint(v, 10, 64)
	if err != nil {
		return nil
	}
	return &n
}
