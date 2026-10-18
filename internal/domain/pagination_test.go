package domain

import "testing"

func TestPaginationClampsAndNeverOverflows(t *testing.T) {
	cases := []struct {
		p           Pagination
		limit, offs int
	}{
		{Pagination{}, DefaultPageSize, 0},
		{Pagination{Page: 1, PageSize: 10}, 10, 0},
		{Pagination{Page: 3, PageSize: 10}, 10, 20},
		{Pagination{Page: 2, PageSize: 5000}, MaxPageSize, MaxPageSize}, // dijepit ke 200, bukan di-reset ke 50
		{Pagination{Page: -4, PageSize: -1}, DefaultPageSize, 0},
		{Pagination{Page: int(^uint(0) >> 1), PageSize: MaxPageSize}, MaxPageSize, (MaxPage - 1) * MaxPageSize}, // MaxInt tidak overflow
	}
	for _, c := range cases {
		if got := c.p.Limit(); got != c.limit {
			t.Errorf("%+v: Limit = %d, mau %d", c.p, got, c.limit)
		}
		got := c.p.Offset()
		if got != c.offs || got < 0 {
			t.Errorf("%+v: Offset = %d, mau %d (tidak boleh negatif)", c.p, got, c.offs)
		}
	}
}
