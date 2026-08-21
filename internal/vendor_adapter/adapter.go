// Package vendor_adapter menyediakan titik ekstensi untuk vendor dengan
// kuirk protokol CWMP nonstandar (urutan RPC aneh, encoding response
// nonstandar, dll). Menambah vendor/model yang mengikuti spec CWMP standar
// TIDAK butuh kode di sini — itu operasi data (ref_vendors, vendor_ouis,
// device_models, vendor_parameter_mappings), lihat TECH.md §5.1.
//
// Belum ada vendor yang butuh adapter khusus saat ini; package ini hanya
// mendefinisikan kontrak + registry agar penambahan adapter di kemudian hari
// tidak memerlukan percabangan if vendor == "..." di kode inti (CLAUDE.md).
package vendoradapter

import "acs/pkg/cwmpxml"

// Adapter menormalisasi kuirk non-standar sebelum/sesudah pemrosesan umum.
// Implementasikan hanya bagian yang benar-benar menyimpang dari spec;
// biarkan sisanya melalui alur standar di internal/delivery/cwmp.
type Adapter interface {
	VendorCode() string
	// NormalizeInform boleh memperbaiki payload Inform nonstandar sebelum
	// diteruskan ke usecase/session (mis. urutan field yang tertukar).
	NormalizeInform(inf *cwmpxml.Inform) *cwmpxml.Inform
}

type Registry struct {
	adapters map[string]Adapter
}

func NewRegistry() *Registry {
	return &Registry{adapters: make(map[string]Adapter)}
}

func (r *Registry) Register(a Adapter) {
	r.adapters[a.VendorCode()] = a
}

func (r *Registry) Get(vendorCode string) (Adapter, bool) {
	a, ok := r.adapters[vendorCode]
	return a, ok
}
