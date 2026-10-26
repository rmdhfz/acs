// Package oltdriver = registry driver OLT (domain.OLTDriver). Driver vendor nyata didaftarkan dari cmd/acsd; driver simulator
// (oltdriver/sim) hanya bila diaktifkan lewat konfigurasi. Menambah vendor = paket driver baru + Register, tanpa mengubah usecase.
package oltdriver

import (
	"fmt"
	"regexp"
	"sort"
	"sync"

	"acs/internal/domain"
)

var codeRe = regexp.MustCompile(`^[a-z0-9][a-z0-9-]{0,31}$`)

type Registry struct {
	mu      sync.RWMutex
	drivers map[string]domain.OLTDriver
}

func NewRegistry() *Registry { return &Registry{drivers: map[string]domain.OLTDriver{}} }

// Register menambah driver. Kode harus unik dan berbentuk [a-z0-9-] (sama dengan batas kolom olts.driver_code).
func (r *Registry) Register(d domain.OLTDriver) error {
	code := d.Code()
	if !codeRe.MatchString(code) {
		return fmt.Errorf("oltdriver: kode driver %q tidak valid (huruf kecil/angka/strip, maks. %d karakter)", code, domain.OLTDriverCodeMaxLen)
	}
	r.mu.Lock()
	defer r.mu.Unlock()
	if _, dup := r.drivers[code]; dup {
		return fmt.Errorf("oltdriver: driver %q sudah terdaftar", code)
	}
	r.drivers[code] = d
	return nil
}

func (r *Registry) Get(code string) (domain.OLTDriver, bool) {
	r.mu.RLock()
	defer r.mu.RUnlock()
	d, ok := r.drivers[code]
	return d, ok
}

// Codes mengembalikan kode driver terdaftar, terurut.
func (r *Registry) Codes() []string {
	r.mu.RLock()
	defer r.mu.RUnlock()
	out := make([]string, 0, len(r.drivers))
	for c := range r.drivers {
		out = append(out, c)
	}
	sort.Strings(out)
	return out
}
