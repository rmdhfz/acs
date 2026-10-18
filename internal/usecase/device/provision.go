package device

import (
	"context"
	"errors"
	"fmt"
	"net/url"
	"regexp"
	"strings"
	"unicode"

	"github.com/google/uuid"

	"acs/internal/domain"
	"acs/internal/usecase/auth"
)

// Pra-registrasi & impor massal perangkat.
//
// Perangkat TR-069 normalnya muncul sendiri saat CPE mengirim Inform pertama.
// Pra-registrasi membuat baris inventory LEBIH DULU (OUI + serial + tenant),
// bertatus UNREGISTERED: saat CPE itu Inform kelak, jalur Inform
// (FindOrCreateFromInform) menemukan baris lewat (oui, serial) dan
// memakainya — tenant sudah terkunci ke pemilik pra-registrasi, sehingga
// tenant lain tidak bisa mengklaimnya.

const (
	// MaxImportRows — batas baris per permintaan impor (klien memecah file besar).
	MaxImportRows = 1000

	ImportModeSkip   = "skip"   // baris yang sudah ada dilewati
	ImportModeUpdate = "update" // baris yang sudah ada diperbarui bila ada perbedaan

	ActionCreate    = "create"
	ActionUpdate    = "update"
	ActionUnchanged = "unchanged"
	ActionSkip      = "skip"
	ActionError     = "error"

	maxSerialLen   = 128
	maxProductLen  = 128
	maxNotesLen    = 255
	maxCRUserLen   = 128
	maxCRPassLen   = 128
	maxCRURLLen    = 255
	macCanonLength = 17
)

var (
	// OUI: 6 karakter alfanumerik. Standar TR-069 menyebut heksadesimal, tetapi jalur
	// Inform menyimpan OUI apa adanya dari CPE (perangkat white-label/simulator bisa
	// mengirim huruf non-hex) — validasi yang lebih ketat membuat perangkat yang SUDAH
	// ada tidak bisa diekspor→diimpor ulang. Kolom devices.oui = CHAR(6).
	ouiRe    = regexp.MustCompile(`^[0-9A-Za-z]{6}$`)
	macHexRe = regexp.MustCompile(`^[0-9A-Fa-f]{12}$`)
)

// CreateDeviceInput — data pra-registrasi satu perangkat (juga satu baris impor).
type CreateDeviceInput struct {
	// TenantID: superadmin WAJIB mengisi; ADMIN dikunci ke tenant-nya (nil atau sama).
	TenantID                  *uint64
	OUI                       string
	SerialNumber              string
	ProductClass              *string
	MACAddress                *string
	Notes                     *string
	VendorID                  *uint64
	DeviceModelID             *uint64
	Latitude                  *float64
	Longitude                 *float64
	ConnectionRequestURL      *string
	ConnectionRequestUsername *string
	// ConnectionRequestPassword plaintext dari pemanggil; dienkripsi sebelum
	// disimpan, tidak pernah dikembalikan/di-log (CLAUDE.md).
	ConnectionRequestPassword *string
}

func invalid(format string, a ...any) error {
	return fmt.Errorf("%w: %s", domain.ErrInvalidInput, fmt.Sprintf(format, a...))
}

func trimPtr(p *string) *string {
	if p == nil {
		return nil
	}
	v := strings.TrimSpace(*p)
	return &v
}

func hasControl(s string) bool {
	for _, r := range s {
		if unicode.IsControl(r) {
			return true
		}
	}
	return false
}

// canonicalMAC memvalidasi & menormalkan MAC ke AA:BB:CC:DD:EE:FF. Menerima
// pemisah ":" "-" "." atau tanpa pemisah.
func canonicalMAC(s string) (string, error) {
	clean := strings.NewReplacer(":", "", "-", "", ".", "", " ", "").Replace(s)
	if !macHexRe.MatchString(clean) {
		return "", invalid("MAC address %q tidak valid (contoh: AA:BB:CC:DD:EE:FF)", s)
	}
	clean = strings.ToUpper(clean)
	var b strings.Builder
	for i := 0; i < 12; i += 2 {
		if i > 0 {
			b.WriteByte(':')
		}
		b.WriteString(clean[i : i+2])
	}
	return b.String(), nil
}

// normalizeCreate memvalidasi & menormalkan input pra-registrasi. String kosong
// pada field opsional berarti "tidak diisi".
func normalizeCreate(in CreateDeviceInput) (CreateDeviceInput, error) {
	in.OUI = strings.ToUpper(strings.TrimSpace(in.OUI))
	if !ouiRe.MatchString(in.OUI) {
		return in, invalid("OUI harus 6 karakter huruf/angka, umumnya heksadesimal (contoh: 001E73)")
	}
	in.SerialNumber = strings.TrimSpace(in.SerialNumber)
	if in.SerialNumber == "" || len(in.SerialNumber) > maxSerialLen || hasControl(in.SerialNumber) {
		return in, invalid("serial_number wajib diisi (maks. %d karakter, tanpa karakter kontrol)", maxSerialLen)
	}
	in.ProductClass = trimPtr(in.ProductClass)
	if in.ProductClass != nil {
		if len(*in.ProductClass) > maxProductLen || hasControl(*in.ProductClass) {
			return in, invalid("product_class maks. %d karakter", maxProductLen)
		}
		if *in.ProductClass == "" {
			in.ProductClass = nil
		}
	}
	in.MACAddress = trimPtr(in.MACAddress)
	if in.MACAddress != nil {
		if *in.MACAddress == "" {
			in.MACAddress = nil
		} else {
			m, err := canonicalMAC(*in.MACAddress)
			if err != nil {
				return in, err
			}
			in.MACAddress = &m
		}
	}
	in.Notes = trimPtr(in.Notes)
	if in.Notes != nil {
		if len(*in.Notes) > maxNotesLen || hasControl(strings.ReplaceAll(*in.Notes, "\n", " ")) {
			return in, invalid("notes maks. %d karakter", maxNotesLen)
		}
		if *in.Notes == "" {
			in.Notes = nil
		}
	}
	if (in.Latitude == nil) != (in.Longitude == nil) {
		return in, invalid("latitude dan longitude harus diisi berpasangan")
	}
	if in.Latitude != nil {
		if *in.Latitude < -90 || *in.Latitude > 90 {
			return in, invalid("latitude harus di antara -90 dan 90")
		}
		if *in.Longitude < -180 || *in.Longitude > 180 {
			return in, invalid("longitude harus di antara -180 dan 180")
		}
	}
	in.ConnectionRequestURL = trimPtr(in.ConnectionRequestURL)
	if in.ConnectionRequestURL != nil {
		if *in.ConnectionRequestURL == "" {
			in.ConnectionRequestURL = nil
		} else if err := validateCRURL(*in.ConnectionRequestURL); err != nil {
			return in, err
		}
	}
	in.ConnectionRequestUsername = trimPtr(in.ConnectionRequestUsername)
	if in.ConnectionRequestUsername != nil {
		if len(*in.ConnectionRequestUsername) > maxCRUserLen || hasControl(*in.ConnectionRequestUsername) {
			return in, invalid("connection_request_username maks. %d karakter", maxCRUserLen)
		}
		if *in.ConnectionRequestUsername == "" {
			in.ConnectionRequestUsername = nil
		}
	}
	if in.ConnectionRequestPassword != nil {
		if len(*in.ConnectionRequestPassword) > maxCRPassLen {
			return in, invalid("connection_request_password maks. %d karakter", maxCRPassLen)
		}
		if *in.ConnectionRequestPassword == "" {
			in.ConnectionRequestPassword = nil
		}
	}
	return in, nil
}

func validateCRURL(raw string) error {
	if len(raw) > maxCRURLLen {
		return invalid("connection_request_url maks. %d karakter", maxCRURLLen)
	}
	u, err := url.Parse(raw)
	if err != nil || (u.Scheme != "http" && u.Scheme != "https") || u.Host == "" {
		return invalid("connection_request_url harus URL http/https yang valid")
	}
	return nil
}

// WithCatalog memasang repositori vendor & tenant untuk validasi referensi pada
// pra-registrasi/impor/edit (vendor_id, device_model_id, tenant_id harus ada).
// Tanpa ini validasi referensi dilewati (hanya FK database yang menjaga).
func (s *Service) WithCatalog(vendors domain.VendorRepository, tenants domain.TenantRepository) *Service {
	s.vendors, s.tenants = vendors, tenants
	return s
}

// resolveWriteTenant menetapkan tenant tujuan tulis berdasarkan peran actor.
func resolveWriteTenant(actor domain.Actor, requested *uint64) (uint64, error) {
	if actor.IsSuperadmin() {
		if requested == nil || *requested == 0 {
			return 0, invalid("tenant_id wajib diisi (superadmin tidak terikat tenant)")
		}
		return *requested, nil
	}
	if actor.TenantID == nil {
		return 0, domain.ErrForbidden
	}
	if requested != nil && *requested != *actor.TenantID {
		return 0, fmt.Errorf("%w: hanya boleh mengelola perangkat tenant sendiri", domain.ErrForbidden)
	}
	return *actor.TenantID, nil
}

// refCache menyimpan hasil validasi referensi selama satu permintaan (impor
// 1.000 baris tidak boleh berarti 1.000 query vendor yang sama).
type refCache struct {
	vendorOK map[uint64]bool
	model    map[uint64]*domain.DeviceModel
	tenantOK map[uint64]bool
}

func newRefCache() *refCache {
	return &refCache{vendorOK: map[uint64]bool{}, model: map[uint64]*domain.DeviceModel{}, tenantOK: map[uint64]bool{}}
}

// validateRefs memastikan tenant/vendor/model ada dan konsisten; mengisi vendor
// dari model bila hanya model yang diberikan.
func (s *Service) validateRefs(ctx context.Context, rc *refCache, actor domain.Actor, tenantID uint64, in *CreateDeviceInput) error {
	// tenantID == 0: dipanggil dari Update (edit) — tenant tidak berubah, tak perlu dicek.
	if s.tenants != nil && actor.IsSuperadmin() && tenantID != 0 {
		ok, seen := rc.tenantOK[tenantID]
		if !seen {
			_, err := s.tenants.GetByID(ctx, tenantID)
			switch {
			case err == nil:
				ok = true
			case errors.Is(err, domain.ErrNotFound):
				ok = false
			default:
				return err
			}
			rc.tenantOK[tenantID] = ok
		}
		if !ok {
			return invalid("tenant_id %d tidak ditemukan", tenantID)
		}
	}
	if s.vendors == nil {
		return nil
	}
	if in.DeviceModelID != nil {
		m, ok := rc.model[*in.DeviceModelID]
		if !ok {
			got, err := s.deviceModels.GetByID(ctx, *in.DeviceModelID)
			switch {
			case err == nil:
				m = got
			case errors.Is(err, domain.ErrNotFound):
				m = nil
			default:
				return err
			}
			rc.model[*in.DeviceModelID] = m
		}
		if m == nil {
			return invalid("device_model_id %d tidak ditemukan", *in.DeviceModelID)
		}
		if in.VendorID == nil {
			vid := m.VendorID
			in.VendorID = &vid
		} else if *in.VendorID != m.VendorID {
			return invalid("device_model_id %d bukan milik vendor_id %d", *in.DeviceModelID, *in.VendorID)
		}
	}
	if in.VendorID != nil {
		ok, seen := rc.vendorOK[*in.VendorID]
		if !seen {
			_, err := s.vendors.GetByID(ctx, *in.VendorID)
			switch {
			case err == nil:
				ok = true
			case errors.Is(err, domain.ErrNotFound):
				ok = false
			default:
				return err
			}
			rc.vendorOK[*in.VendorID] = ok
		}
		if !ok {
			return invalid("vendor_id %d tidak ditemukan", *in.VendorID)
		}
	}
	return nil
}

// Create — pra-registrasi satu perangkat (ADMIN+). Perangkat bertatus
// UNREGISTERED sampai CPE-nya Inform pertama kali.
func (s *Service) Create(ctx context.Context, actor domain.Actor, in CreateDeviceInput) (*domain.Device, error) {
	if err := auth.RequireRole(actor, domain.RoleAdmin); err != nil {
		return nil, err
	}
	tenantID, err := resolveWriteTenant(actor, in.TenantID)
	if err != nil {
		return nil, err
	}
	in, err = normalizeCreate(in)
	if err != nil {
		return nil, err
	}
	if err := s.validateRefs(ctx, newRefCache(), actor, tenantID, &in); err != nil {
		return nil, err
	}
	existing, err := s.devices.GetByOUISerial(ctx, in.OUI, in.SerialNumber)
	if err != nil && !errors.Is(err, domain.ErrNotFound) {
		return nil, err
	}
	if existing != nil {
		return nil, fmt.Errorf("%w: perangkat dengan OUI dan serial ini sudah terdaftar", domain.ErrConflict)
	}
	d, err := s.createFromInput(ctx, actor, tenantID, in)
	if err != nil {
		return nil, err
	}
	_ = s.activity.Record(ctx, &domain.ActivityLog{
		UserID: actor.UserIDPtr(), TenantID: d.TenantID, Action: "CREATE_DEVICE", EntityType: "device", EntityID: &d.ID,
		Details: map[string]any{"serial_number": d.SerialNumber, "oui": in.OUI},
	})
	return d, nil
}

// createFromInput menyisipkan baris (atau me-revive baris terhapus tenant yang
// sama) dari input yang SUDAH dinormalkan & divalidasi.
func (s *Service) createFromInput(ctx context.Context, actor domain.Actor, tenantID uint64, in CreateDeviceInput) (*domain.Device, error) {
	unregistered, err := s.refs.GetByCode(ctx, domain.RefTableDeviceStatus, domain.DeviceStatusUnregistered)
	if err != nil {
		return nil, err
	}
	oui := in.OUI
	tid := tenantID
	d := &domain.Device{
		DeviceUUID:                uuid.NewString(),
		TenantID:                  &tid,
		DeviceStatusID:            unregistered.ID,
		OUI:                       &oui,
		SerialNumber:              in.SerialNumber,
		ProductClass:              in.ProductClass,
		MACAddress:                in.MACAddress,
		Notes:                     in.Notes,
		VendorID:                  in.VendorID,
		DeviceModelID:             in.DeviceModelID,
		Latitude:                  in.Latitude,
		Longitude:                 in.Longitude,
		ConnectionRequestURL:      in.ConnectionRequestURL,
		ConnectionRequestUsername: in.ConnectionRequestUsername,
		Audit:                     domain.Audit{CreatedBy: actor.UserIDPtr()},
	}
	if in.ConnectionRequestPassword != nil {
		enc, err := s.enc.Encrypt(*in.ConnectionRequestPassword)
		if err != nil {
			return nil, err
		}
		d.ConnectionRequestPasswordEnc = enc
	}
	// Vendor/model dari katalog OUI bila belum ditentukan operator.
	pc := ""
	if in.ProductClass != nil {
		pc = *in.ProductClass
	}
	s.resolveVendorModel(ctx, d, in.OUI, pc)

	if err := s.requireDeviceCapacity(ctx, d.TenantID); err != nil {
		return nil, err
	}
	if err := s.devices.Create(ctx, d); err != nil {
		if errors.Is(err, domain.ErrConflict) {
			// Baris terhapus milik tenant LAIN (revive lintas tenant ditolak repo).
			return nil, fmt.Errorf("%w: perangkat dengan OUI %s dan serial %s tidak dapat didaftarkan", domain.ErrConflict, in.OUI, in.SerialNumber)
		}
		return nil, err
	}
	// Create me-revive baris terhapus dengan MENGOSONGKAN catatan/kredensial
	// (milik registrasi lama). Tulis ulang nilai yang diminta operator ini agar
	// tidak hilang pada kasus revive.
	d.UpdatedBy = actor.UserIDPtr()
	if err := s.devices.Update(ctx, d); err != nil {
		return nil, err
	}
	return d, nil
}

// ---- Impor massal ----

// ImportInput — satu permintaan impor (<= MaxImportRows baris).
type ImportInput struct {
	Mode   string // ImportModeSkip | ImportModeUpdate
	DryRun bool   // true = hanya klasifikasi, TIDAK menulis apa pun
	Rows   []CreateDeviceInput
}

// ImportRowResult — hasil klasifikasi/eksekusi satu baris.
type ImportRowResult struct {
	Index        int      `json:"index"` // posisi dalam permintaan (0-based)
	Action       string   `json:"action"`
	OUI          string   `json:"oui"`
	SerialNumber string   `json:"serial_number"`
	Changes      []string `json:"changes,omitempty"` // nama field yang berubah (action=update)
	Error        string   `json:"error,omitempty"`
}

// ImportResult — ringkasan + rincian per baris.
type ImportResult struct {
	DryRun    bool              `json:"dry_run"`
	Created   int               `json:"created"`
	Updated   int               `json:"updated"`
	Unchanged int               `json:"unchanged"`
	Skipped   int               `json:"skipped"`
	Failed    int               `json:"failed"`
	Results   []ImportRowResult `json:"results"`
}

// Import mengklasifikasi (dry-run) atau mengeksekusi impor perangkat. Baris
// diproses mandiri (bukan satu transaksi): baris gagal dilaporkan, sisanya
// tetap diproses. Dry-run mengerjakan klasifikasi yang SAMA dengan eksekusi,
// sehingga pratinjau mencerminkan hasil sebenarnya (kecuali perubahan data
// di antara dua panggilan).
func (s *Service) Import(ctx context.Context, actor domain.Actor, in ImportInput) (*ImportResult, error) {
	if err := auth.RequireRole(actor, domain.RoleAdmin); err != nil {
		return nil, err
	}
	if in.Mode != ImportModeSkip && in.Mode != ImportModeUpdate {
		return nil, invalid("mode harus %q atau %q", ImportModeSkip, ImportModeUpdate)
	}
	if len(in.Rows) == 0 {
		return nil, invalid("rows kosong")
	}
	if len(in.Rows) > MaxImportRows {
		return nil, invalid("maksimal %d baris per permintaan (dapat %d) — pecah file", MaxImportRows, len(in.Rows))
	}

	res := &ImportResult{DryRun: in.DryRun, Results: make([]ImportRowResult, 0, len(in.Rows))}
	rc := newRefCache()
	seen := map[string]int{} // oui|serial -> index pertama (deteksi duplikat dalam permintaan)

	for i, raw := range in.Rows {
		r := ImportRowResult{Index: i, OUI: strings.ToUpper(strings.TrimSpace(raw.OUI)), SerialNumber: strings.TrimSpace(raw.SerialNumber)}
		fail := func(err error) {
			r.Action, r.Error = ActionError, errText(err)
			res.Failed++
			res.Results = append(res.Results, r)
		}

		tenantID, err := resolveWriteTenant(actor, raw.TenantID)
		if err != nil {
			fail(err)
			continue
		}
		row, err := normalizeCreate(raw)
		if err != nil {
			fail(err)
			continue
		}
		r.OUI, r.SerialNumber = row.OUI, row.SerialNumber
		key := row.OUI + "|" + row.SerialNumber
		if first, dup := seen[key]; dup {
			fail(invalid("duplikat dalam file (sama dengan baris ke-%d)", first+1))
			continue
		}
		seen[key] = i
		if err := s.validateRefs(ctx, rc, actor, tenantID, &row); err != nil {
			fail(err)
			continue
		}

		existing, err := s.devices.GetByOUISerial(ctx, row.OUI, row.SerialNumber)
		if err != nil && !errors.Is(err, domain.ErrNotFound) {
			fail(err)
			continue
		}

		if existing == nil {
			r.Action = ActionCreate
			if !in.DryRun {
				if _, err := s.createFromInput(ctx, actor, tenantID, row); err != nil {
					fail(err)
					continue
				}
			}
			res.Created++
			res.Results = append(res.Results, r)
			continue
		}

		// Sudah ada. Hanya tenant pemilik (atau superadmin) yang boleh menyentuhnya.
		if existing.TenantID == nil {
			if !actor.IsSuperadmin() {
				fail(fmt.Errorf("%w: perangkat belum memiliki tenant — hubungi superadmin", domain.ErrConflict))
				continue
			}
		} else if *existing.TenantID != tenantID {
			// Pesan generik: jangan membocorkan tenant pemilik.
			fail(fmt.Errorf("%w: perangkat dengan OUI dan serial ini sudah terdaftar dan tidak dapat diubah", domain.ErrConflict))
			continue
		}
		if in.Mode == ImportModeSkip {
			r.Action = ActionSkip
			res.Skipped++
			res.Results = append(res.Results, r)
			continue
		}
		// Bekerja pada SALINAN: dry-run tidak boleh memutasi objek milik repo
		// (aman terhadap implementasi repo yang mengembalikan pointer bersama).
		work := *existing
		changes, err := s.applyImportChanges(&work, tenantID, row)
		if err != nil {
			fail(err)
			continue
		}
		if len(changes) == 0 {
			r.Action = ActionUnchanged
			res.Unchanged++
			res.Results = append(res.Results, r)
			continue
		}
		r.Action, r.Changes = ActionUpdate, changes
		if !in.DryRun {
			work.UpdatedBy = actor.UserIDPtr()
			if err := s.devices.Update(ctx, &work); err != nil {
				fail(err)
				continue
			}
		}
		res.Updated++
		res.Results = append(res.Results, r)
	}

	if !in.DryRun && (res.Created+res.Updated) > 0 {
		_ = s.activity.Record(ctx, &domain.ActivityLog{
			UserID: actor.UserIDPtr(), TenantID: actor.TenantID, Action: "IMPORT_DEVICES", EntityType: "device",
			Details: map[string]any{"mode": in.Mode, "created": res.Created, "updated": res.Updated, "unchanged": res.Unchanged, "skipped": res.Skipped, "failed": res.Failed},
		})
	}
	return res, nil
}

// errText membuang awalan sentinel domain ("domain: ...: ") supaya pesan per
// baris ramah-baca di pratinjau impor.
func errText(err error) string {
	msg := err.Error()
	for _, sentinel := range []error{domain.ErrInvalidInput, domain.ErrConflict, domain.ErrForbidden, domain.ErrNotFound} {
		msg = strings.TrimPrefix(msg, sentinel.Error()+": ")
	}
	return msg
}

// applyImportChanges menerapkan field non-kosong baris impor ke `d` dan
// mengembalikan nama field yang BERBEDA. Sel kosong = biarkan nilai lama.
func (s *Service) applyImportChanges(d *domain.Device, tenantID uint64, in CreateDeviceInput) ([]string, error) {
	var changes []string
	setStr := func(name string, cur **string, v *string) {
		if v == nil {
			return
		}
		if *cur == nil || **cur != *v {
			nv := *v
			*cur = &nv
			changes = append(changes, name)
		}
	}
	setID := func(name string, cur **uint64, v *uint64) {
		if v == nil {
			return
		}
		if *cur == nil || **cur != *v {
			nv := *v
			*cur = &nv
			changes = append(changes, name)
		}
	}
	if d.TenantID == nil {
		tid := tenantID
		d.TenantID = &tid
		changes = append(changes, "tenant_id")
	}
	setStr("product_class", &d.ProductClass, in.ProductClass)
	setStr("mac_address", &d.MACAddress, in.MACAddress)
	setStr("notes", &d.Notes, in.Notes)
	setStr("connection_request_url", &d.ConnectionRequestURL, in.ConnectionRequestURL)
	setStr("connection_request_username", &d.ConnectionRequestUsername, in.ConnectionRequestUsername)
	setID("vendor_id", &d.VendorID, in.VendorID)
	setID("device_model_id", &d.DeviceModelID, in.DeviceModelID)
	if in.Latitude != nil && in.Longitude != nil {
		if d.Latitude == nil || d.Longitude == nil || *d.Latitude != *in.Latitude || *d.Longitude != *in.Longitude {
			la, lo := *in.Latitude, *in.Longitude
			d.Latitude, d.Longitude = &la, &lo
			changes = append(changes, "lokasi")
		}
	}
	if in.ConnectionRequestPassword != nil {
		// Ciphertext tidak bisa dibandingkan (nonce acak) → selalu dianggap berubah.
		enc, err := s.enc.Encrypt(*in.ConnectionRequestPassword)
		if err != nil {
			return nil, err
		}
		d.ConnectionRequestPasswordEnc = enc
		changes = append(changes, "connection_request_password")
	}
	return changes, nil
}
