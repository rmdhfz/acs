package vexpr

import "runtime"

// runtimeMemStats mengukur total byte yang dialokasikan (kumulatif) di antara dua titik.
type runtimeMemStats struct{ total uint64 }

func (m *runtimeMemStats) read() {
	var ms runtime.MemStats
	runtime.ReadMemStats(&ms)
	m.total = ms.TotalAlloc
}
