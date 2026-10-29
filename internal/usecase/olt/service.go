// Package olt = OLT Controller irisan 1 (docs/arsitektur/OLT_CONTROLLER_DESIGN.md): inventaris OLT per tenant, tes koneksi, dan
// antrean ONU yang dilaporkan OLT sebagai belum terdaftar. READ-ONLY: tidak ada jalur untuk menulis ke OLT. Vendor-spesifik hidup di
// driver (domain.OLTDriver); usecase ini tidak pernah bercabang per vendor.
package olt

import (
	"context"
	"errors"
	"fmt"
	"log"
	"net"
	"regexp"
	"strings"
	"sync"
	"time"
	"unicode"
	"unicode/utf8"

	"github.com/google/uuid"

	"acs/internal/domain"
	"acs/internal/usecase/auth"
	"acs/pkg/cryptoutil"
	"acs/pkg/netguard"
)

// driverTimeout membatasi satu panggilan driver (koneksi + perintah). OLT nyata sering lambat dan membatasi jumlah sesi.
const driverTimeout = 30 * time.Second

var hostRe = regexp.MustCompile(`^[A-Za-z0-9]([A-Za-z0-9._-]*[A-Za-z0-9])?$`)

type Service struct {
	repo     domain.OLTRepository
	drivers  domain.OLTDriverRegistry
	refs     domain.RefRepository
	enc      *cryptoutil.Encryptor
	activity domain.ActivityLogRepository

	blockPrivate bool
	// Opsional (WithInventory/WithPublisher/WithDevices): inventaris ONU terdaftar, siaran realtime, dan pencarian per perangkat.
	inv       domain.OLTInventoryRepository
	publisher domain.EventPublisher
	devices   DeviceReader
	// Opsional (WithNotifier/WithWebhooks): notifikasi ONU_STATE_ALERT (LOS/dying gasp) dan webhook ONU_STATE_CHANGED.
	notifier domain.Notifier
	webhooks domain.WebhookEnqueuer
	// polling — penjaga satu polling per OLT dalam satu instance: OLT nyata membatasi sesi bersamaan, dan dua polling
	// serentak hanya menggandakan beban. (Antar-instance tidak dijaga; rekonsiliasi idempoten.)
	polling sync.Map
}

func NewService(repo domain.OLTRepository, drivers domain.OLTDriverRegistry, refs domain.RefRepository, enc *cryptoutil.Encryptor, activity domain.ActivityLogRepository) *Service {
	return &Service{repo: repo, drivers: drivers, refs: refs, enc: enc, activity: activity}
}

// WithBlockPrivate menolak host OLT di jaringan privat (RFC1918/ULA/CGNAT), mis. SaaS publik. Bawaan: izinkan (OLT hampir selalu
// di jaringan manajemen privat).
func (s *Service) WithBlockPrivate(b bool) *Service {
	s.blockPrivate = b
	return s
}

// Drivers mengembalikan kode driver yang terdaftar (terurut).
func (s *Service) Drivers() []string { return s.drivers.Codes() }

// ---- validasi ----

func (s *Service) validateHost(raw string) (string, error) {
	h, err := domain.CleanText("host", raw, 1, domain.OLTHostMaxLen, false)
	if err != nil {
		return "", err
	}
	if net.ParseIP(h) == nil && !hostRe.MatchString(h) {
		return "", fmt.Errorf("%w: host harus alamat IP atau nama host (huruf, angka, titik, strip), tanpa skema, port, atau jalur", domain.ErrInvalidInput)
	}
	if err := netguard.CheckHostPortPolicy(h, s.blockPrivate); err != nil {
		return "", fmt.Errorf("%w: host OLT tidak diizinkan (alamat loopback/link-local/internal, atau tidak dapat di-resolve)", domain.ErrInvalidInput)
	}
	return h, nil
}

func validatePort(field string, v int) (uint16, error) {
	if v < 1 || v > 65535 {
		return 0, fmt.Errorf("%w: %s harus 1-65535", domain.ErrInvalidInput, field)
	}
	return uint16(v), nil
}

func (s *Service) validateDriver(code string) (string, error) {
	code = strings.TrimSpace(code)
	if _, ok := s.drivers.Get(code); !ok {
		return "", fmt.Errorf("%w: driver %q tidak terdaftar (lihat GET /olt-drivers)", domain.ErrInvalidInput, code)
	}
	return code, nil
}

// secret memvalidasi kredensial TANPA memangkas spasi (password sah boleh berspasi di tepi), lalu mengenkripsinya.
func (s *Service) sealSecret(field, v string) ([]byte, error) {
	if !utf8.ValidString(v) || utf8.RuneCountInString(v) > domain.OLTCredentialMaxLen || strings.ContainsFunc(v, unicode.IsControl) {
		return nil, fmt.Errorf("%w: %s maksimal %d karakter, UTF-8 valid, tanpa karakter kontrol", domain.ErrInvalidInput, field, domain.OLTCredentialMaxLen)
	}
	if s.enc == nil {
		return nil, errors.New("olt: enkripsi kredensial tidak dikonfigurasi")
	}
	return s.enc.Encrypt(v)
}

func (s *Service) openSecret(b []byte) (string, error) {
	if len(b) == 0 {
		return "", nil
	}
	if s.enc == nil {
		return "", errors.New("olt: enkripsi kredensial tidak dikonfigurasi")
	}
	return s.enc.Decrypt(b)
}

// decorate mengisi penanda kredensial (nilainya tidak pernah ikut) sebelum OLT dikembalikan ke pemanggil.
func decorate(o *domain.OLT) *domain.OLT {
	o.HasSNMPCommunity = len(o.SNMPCommunityEnc) > 0
	o.HasSSHPassword = len(o.SSHPasswordEnc) > 0
	return o
}

func ptrOrNil(s string) *string {
	if s == "" {
		return nil
	}
	return &s
}

// ---- inventaris ----

type CreateInput struct {
	TenantID      *uint64 // wajib bagi superadmin; non-superadmin selalu tenant-nya sendiri
	Name          string
	DriverCode    string
	Host          string
	SNMPPort      *int
	SSHPort       *int
	SNMPCommunity string
	SSHUsername   string
	SSHPassword   string
	Notes         string
	IsActive      *bool
}

func (s *Service) Create(ctx context.Context, actor domain.Actor, in CreateInput) (*domain.OLT, error) {
	if err := auth.RequireRole(actor, domain.RoleAdmin); err != nil {
		return nil, err
	}
	tenantID, err := resolveWriteTenant(actor, in.TenantID)
	if err != nil {
		return nil, err
	}
	by := actor.UserIDPtr()
	if by == nil {
		return nil, domain.ErrForbidden
	}
	name, err := domain.CleanText("name", in.Name, 1, domain.OLTNameMaxLen, false)
	if err != nil {
		return nil, err
	}
	driver, err := s.validateDriver(in.DriverCode)
	if err != nil {
		return nil, err
	}
	host, err := s.validateHost(in.Host)
	if err != nil {
		return nil, err
	}
	snmpPort, sshPort := uint16(161), uint16(22)
	if in.SNMPPort != nil {
		if snmpPort, err = validatePort("snmp_port", *in.SNMPPort); err != nil {
			return nil, err
		}
	}
	if in.SSHPort != nil {
		if sshPort, err = validatePort("ssh_port", *in.SSHPort); err != nil {
			return nil, err
		}
	}
	notes, err := domain.CleanText("notes", in.Notes, 0, domain.OLTNotesMaxLen, true)
	if err != nil {
		return nil, err
	}
	sshUser, err := domain.CleanText("ssh_username", in.SSHUsername, 0, domain.OLTCredentialMaxLen, false)
	if err != nil {
		return nil, err
	}
	unknown, err := s.refs.GetByCode(ctx, domain.RefTableOLTPollStatus, domain.OLTPollUnknown)
	if err != nil {
		return nil, err
	}

	o := &domain.OLT{
		UUID: uuid.NewString(), TenantID: tenantID, Name: name, DriverCode: driver, Host: host, SNMPPort: snmpPort, SSHPort: sshPort,
		SSHUsername: ptrOrNil(sshUser), Notes: ptrOrNil(notes), IsActive: in.IsActive == nil || *in.IsActive,
		PollStatusID: unknown.ID, PollStatus: domain.OLTPollUnknown, Audit: domain.Audit{CreatedBy: by},
	}
	if in.SNMPCommunity != "" {
		if o.SNMPCommunityEnc, err = s.sealSecret("snmp_community", in.SNMPCommunity); err != nil {
			return nil, err
		}
	}
	if in.SSHPassword != "" {
		if o.SSHPasswordEnc, err = s.sealSecret("ssh_password", in.SSHPassword); err != nil {
			return nil, err
		}
	}
	if err := s.repo.Create(ctx, o); err != nil {
		return nil, err
	}
	s.record(ctx, actor, &o.TenantID, "CREATE_OLT", o.ID, map[string]any{"name": o.Name, "driver_code": o.DriverCode, "host": o.Host})
	return decorate(o), nil
}

// UpdateInput — nil = tidak berubah. Untuk kredensial dan catatan, string kosong MENGOSONGKAN nilainya.
type UpdateInput struct {
	Name          *string
	DriverCode    *string
	Host          *string
	SNMPPort      *int
	SSHPort       *int
	SNMPCommunity *string
	SSHUsername   *string
	SSHPassword   *string
	Notes         *string
	IsActive      *bool
}

func (s *Service) Update(ctx context.Context, actor domain.Actor, uuid string, in UpdateInput) (*domain.OLT, error) {
	if err := auth.RequireRole(actor, domain.RoleAdmin); err != nil {
		return nil, err
	}
	o, err := s.load(ctx, actor, uuid)
	if err != nil {
		return nil, err
	}
	by := actor.UserIDPtr()
	if by == nil {
		return nil, domain.ErrForbidden
	}
	if in.Name != nil {
		if o.Name, err = domain.CleanText("name", *in.Name, 1, domain.OLTNameMaxLen, false); err != nil {
			return nil, err
		}
	}
	if in.DriverCode != nil {
		if o.DriverCode, err = s.validateDriver(*in.DriverCode); err != nil {
			return nil, err
		}
	}
	if in.Host != nil {
		if o.Host, err = s.validateHost(*in.Host); err != nil {
			return nil, err
		}
	}
	if in.SNMPPort != nil {
		if o.SNMPPort, err = validatePort("snmp_port", *in.SNMPPort); err != nil {
			return nil, err
		}
	}
	if in.SSHPort != nil {
		if o.SSHPort, err = validatePort("ssh_port", *in.SSHPort); err != nil {
			return nil, err
		}
	}
	if in.Notes != nil {
		n, err := domain.CleanText("notes", *in.Notes, 0, domain.OLTNotesMaxLen, true)
		if err != nil {
			return nil, err
		}
		o.Notes = ptrOrNil(n)
	}
	if in.SSHUsername != nil {
		u, err := domain.CleanText("ssh_username", *in.SSHUsername, 0, domain.OLTCredentialMaxLen, false)
		if err != nil {
			return nil, err
		}
		o.SSHUsername = ptrOrNil(u)
	}
	if in.SNMPCommunity != nil {
		o.SNMPCommunityEnc = nil
		if *in.SNMPCommunity != "" {
			if o.SNMPCommunityEnc, err = s.sealSecret("snmp_community", *in.SNMPCommunity); err != nil {
				return nil, err
			}
		}
	}
	if in.SSHPassword != nil {
		o.SSHPasswordEnc = nil
		if *in.SSHPassword != "" {
			if o.SSHPasswordEnc, err = s.sealSecret("ssh_password", *in.SSHPassword); err != nil {
				return nil, err
			}
		}
	}
	if in.IsActive != nil {
		o.IsActive = *in.IsActive
	}
	o.UpdatedBy = by
	if err := s.repo.Update(ctx, o); err != nil {
		return nil, err
	}
	// Daftar bidang yang diubah (tanpa nilai kredensial) untuk audit trail.
	changed := []string{}
	for name, set := range map[string]bool{
		"name": in.Name != nil, "driver_code": in.DriverCode != nil, "host": in.Host != nil, "snmp_port": in.SNMPPort != nil,
		"ssh_port": in.SSHPort != nil, "snmp_community": in.SNMPCommunity != nil, "ssh_username": in.SSHUsername != nil,
		"ssh_password": in.SSHPassword != nil, "notes": in.Notes != nil, "is_active": in.IsActive != nil,
	} {
		if set {
			changed = append(changed, name)
		}
	}
	s.record(ctx, actor, &o.TenantID, "UPDATE_OLT", o.ID, map[string]any{"fields": changed})
	return decorate(o), nil
}

func (s *Service) Delete(ctx context.Context, actor domain.Actor, uuid string) error {
	if err := auth.RequireRole(actor, domain.RoleAdmin); err != nil {
		return err
	}
	o, err := s.load(ctx, actor, uuid)
	if err != nil {
		return err
	}
	by := actor.UserIDPtr()
	if by == nil {
		return domain.ErrForbidden
	}
	if err := s.repo.SoftDelete(ctx, o.ID, *by); err != nil {
		return err
	}
	s.record(ctx, actor, &o.TenantID, "DELETE_OLT", o.ID, map[string]any{"name": o.Name})
	return nil
}

// List — non-superadmin otomatis dibatasi ke tenant-nya (tenant_id dari klien hanya efektif untuk superadmin).
func (s *Service) List(ctx context.Context, actor domain.Actor, f domain.OLTFilter, p domain.Pagination) ([]domain.OLT, int, error) {
	if !actor.IsSuperadmin() {
		tid, err := auth.ScopedTenantFilter(actor)
		if err != nil {
			return nil, 0, err
		}
		f.TenantID = tid
	}
	rows, total, err := s.repo.List(ctx, f, p)
	if err != nil {
		return nil, 0, err
	}
	for i := range rows {
		hideSecretsFrom(actor, decorate(&rows[i]))
	}
	return rows, total, nil
}

// Get: OLT milik tenant lain dijawab ErrForbidden (pola yang sama dengan fault/task).
func (s *Service) Get(ctx context.Context, actor domain.Actor, uuid string) (*domain.OLT, error) {
	o, err := s.load(ctx, actor, uuid)
	if err != nil {
		return nil, err
	}
	return hideSecretsFrom(actor, decorate(o)), nil
}

// hideSecretsFrom menyembunyikan username SSH dari peran yang tidak boleh melihat rahasia (hanya ADMIN/SUPERADMIN; pola yang sama
// dengan Actor.CanSeeSecrets di modul lain). Password/community tidak pernah dikembalikan siapa pun.
func hideSecretsFrom(actor domain.Actor, o *domain.OLT) *domain.OLT {
	if !actor.CanSeeSecrets() {
		o.SSHUsername = nil
	}
	return o
}

func (s *Service) load(ctx context.Context, actor domain.Actor, uuid string) (*domain.OLT, error) {
	o, err := s.repo.GetByUUID(ctx, uuid)
	if err != nil {
		return nil, err
	}
	if err := auth.RequireTenantScope(actor, &o.TenantID); err != nil {
		return nil, err
	}
	return o, nil
}

// ---- koneksi ----

// connect membuka kredensial terenkripsi menjadi OLTConnection (plaintext hanya di memori) dan mencari driver-nya.
func (s *Service) connect(o *domain.OLT) (domain.OLTDriver, domain.OLTConnection, error) {
	drv, ok := s.drivers.Get(o.DriverCode)
	if !ok {
		return nil, domain.OLTConnection{}, fmt.Errorf("%w: driver %q untuk OLT ini tidak tersedia di server", domain.ErrConflict, o.DriverCode)
	}
	community, err := s.openSecret(o.SNMPCommunityEnc)
	if err != nil {
		return nil, domain.OLTConnection{}, err
	}
	password, err := s.openSecret(o.SSHPasswordEnc)
	if err != nil {
		return nil, domain.OLTConnection{}, err
	}
	conn := domain.OLTConnection{Host: o.Host, SNMPPort: o.SNMPPort, SSHPort: o.SSHPort, SNMPCommunity: community, SSHPassword: password}
	if o.SSHUsername != nil {
		conn.SSHUsername = *o.SSHUsername
	}
	return drv, conn, nil
}

// classify memetakan galat driver ke kode status poll, pesan bersih untuk disimpan, dan galat untuk klien. Teks mentah dari
// driver/perangkat TIDAK ikut ke klien maupun ke database.
func classify(err error) (code, msg string, out error) {
	switch {
	case errors.Is(err, domain.ErrOLTUnreachable):
		return domain.OLTPollUnreachable, "OLT tidak dapat dijangkau", domain.ErrOLTUnreachable
	case errors.Is(err, domain.ErrOLTAuthFailed):
		return domain.OLTPollAuthFailed, "OLT menolak autentikasi", domain.ErrOLTAuthFailed
	case errors.Is(err, context.DeadlineExceeded), errors.Is(err, context.Canceled):
		return domain.OLTPollUnreachable, "waktu tunggu koneksi ke OLT habis", domain.ErrOLTUnreachable
	}
	log.Printf("olt: galat driver tak terklasifikasi: %v", err)
	return domain.OLTPollError, "galat tak terduga dari driver OLT", fmt.Errorf("%w: galat dari driver OLT", domain.ErrUpstreamUnavailable)
}

func (s *Service) record(ctx context.Context, actor domain.Actor, tenantID *uint64, action string, oltID uint64, details map[string]any) {
	if s.activity == nil {
		return
	}
	_ = s.activity.Record(ctx, &domain.ActivityLog{
		UserID: actor.UserIDPtr(), TenantID: tenantID, Action: action, EntityType: "olt", EntityID: &oltID, Details: details,
	})
}

func (s *Service) recordPoll(ctx context.Context, o *domain.OLT, statusCode string, errMsg *string, info *domain.OLTInfo) error {
	st, err := s.refs.GetByCode(ctx, domain.RefTableOLTPollStatus, statusCode)
	if err != nil {
		return err
	}
	return s.repo.RecordPoll(ctx, o.ID, domain.OLTPollResult{StatusID: st.ID, Error: errMsg, Info: info})
}

func truncRunes(s string, max int) string {
	s = strings.TrimSpace(s)
	if utf8.RuneCountInString(s) <= max {
		return s
	}
	return string([]rune(s)[:max])
}

// cleanInfo memangkas identitas dari perangkat (tak tepercaya) dan membuang karakter kontrol.
func cleanInfo(in domain.OLTInfo) *domain.OLTInfo {
	clean := func(s string) string {
		s = strings.Map(func(r rune) rune {
			if unicode.IsControl(r) || !utf8.ValidRune(r) {
				return -1
			}
			return r
		}, s)
		return truncRunes(s, domain.OLTInfoMaxLen)
	}
	return &domain.OLTInfo{Name: clean(in.Name), Version: clean(in.Version)}
}

// Test menguji koneksi (Probe) dan mencatat hasilnya pada OLT. Galat koneksi tetap dicatat sebagai status sebelum dikembalikan.
func (s *Service) Test(ctx context.Context, actor domain.Actor, uuid string) (*domain.OLT, error) {
	if err := auth.RequireRole(actor, domain.RoleAdmin); err != nil {
		return nil, err
	}
	o, err := s.load(ctx, actor, uuid)
	if err != nil {
		return nil, err
	}
	drv, conn, err := s.connect(o)
	if err != nil {
		return nil, err
	}
	cctx, cancel := context.WithTimeout(ctx, driverTimeout)
	defer cancel()
	info, derr := drv.Probe(cctx, conn)
	if derr != nil {
		code, msg, out := classify(derr)
		if err := s.recordPoll(ctx, o, code, &msg, nil); err != nil {
			return nil, err
		}
		s.record(ctx, actor, &o.TenantID, "TEST_OLT", o.ID, map[string]any{"result": code})
		return nil, out
	}
	if err := s.recordPoll(ctx, o, domain.OLTPollOK, nil, cleanInfo(info)); err != nil {
		return nil, err
	}
	s.record(ctx, actor, &o.TenantID, "TEST_OLT", o.ID, map[string]any{"result": domain.OLTPollOK})
	return s.Get(ctx, actor, uuid)
}

// PollSummary = ringkasan rekonsiliasi satu polling.
type PollSummary struct {
	Seen        int `json:"seen"`         // ONU yang dilaporkan OLT (setelah dibersihkan)
	New         int `json:"new"`          // belum pernah ada di antrean
	Reappeared  int `json:"reappeared"`   // sebelumnya RESOLVED, muncul lagi → PENDING
	Resolved    int `json:"resolved"`     // sebelumnya PENDING/IGNORED, kini tidak muncul → RESOLVED
	Skipped     int `json:"skipped"`      // entri dari perangkat yang tidak valid/duplikat dan dilewati
	QueueLength int `json:"queue_length"` // PENDING sesudah rekonsiliasi
	// Registered = ringkasan ONU terdaftar; nil bila driver OLT ini tidak mendukung pembacaan ONU terdaftar (atau inventaris tidak aktif).
	Registered *RegisteredSummary `json:"registered,omitempty"`
}

// Poll membaca ONU belum terdaftar dari OLT lalu merekonsiliasi antrean. Satu polling per OLT dalam satu waktu.
func (s *Service) Poll(ctx context.Context, actor domain.Actor, uuid string) (*PollSummary, error) {
	if err := auth.RequireRole(actor, domain.RoleAdmin, domain.RoleNOC); err != nil {
		return nil, err
	}
	o, err := s.load(ctx, actor, uuid)
	if err != nil {
		return nil, err
	}
	if !o.IsActive {
		return nil, fmt.Errorf("%w: OLT dinonaktifkan; aktifkan dulu untuk polling", domain.ErrConflict)
	}
	drv, conn, err := s.connect(o)
	if err != nil {
		return nil, err
	}
	if _, busy := s.polling.LoadOrStore(o.ID, struct{}{}); busy {
		return nil, fmt.Errorf("%w: polling OLT ini sedang berjalan", domain.ErrConflict)
	}
	defer s.polling.Delete(o.ID)

	cctx, cancel := context.WithTimeout(ctx, driverTimeout)
	defer cancel()
	reported, derr := drv.ListUnregisteredONUs(cctx, conn)
	if derr != nil {
		code, msg, out := classify(derr)
		if err := s.recordPoll(ctx, o, code, &msg, nil); err != nil {
			return nil, err
		}
		s.record(ctx, actor, &o.TenantID, "POLL_OLT", o.ID, map[string]any{"result": code})
		return nil, out
	}

	clean, skipped := cleanONUs(reported)
	if len(clean) > domain.OLTMaxUnregisteredONUs {
		// Jangan merekonsiliasi daftar yang terpotong: ONU di luar batas akan salah ditandai RESOLVED.
		msg := fmt.Sprintf("OLT melaporkan lebih dari %d ONU belum terdaftar; hasil polling tidak diterapkan", domain.OLTMaxUnregisteredONUs)
		if err := s.recordPoll(ctx, o, domain.OLTPollError, &msg, nil); err != nil {
			return nil, err
		}
		return nil, fmt.Errorf("%w: %s", domain.ErrUpstreamUnavailable, msg)
	}
	sum, err := s.reconcile(ctx, o, clean)
	if err != nil {
		return nil, err
	}
	sum.Skipped = skipped
	// ONU terdaftar (kemampuan opsional driver). Antrean belum-terdaftar di atas sudah tersimpan; bila bagian ini gagal polling dicatat
	// gagal dan galatnya dikembalikan, supaya status OK tidak menyembunyikan inventaris yang basi.
	if invDrv, ok := drv.(domain.OLTInventoryDriver); ok && s.inv != nil {
		rs, ierr := s.pollInventory(ctx, o, invDrv, conn)
		if ierr != nil {
			code, msg, out := classify(ierr)
			if errors.Is(ierr, errInventoryTooLarge) {
				code, msg, out = domain.OLTPollError, ierr.Error(), fmt.Errorf("%w: %s", domain.ErrUpstreamUnavailable, ierr)
			}
			if err := s.recordPoll(ctx, o, code, &msg, nil); err != nil {
				return nil, err
			}
			s.record(ctx, actor, &o.TenantID, "POLL_OLT", o.ID, map[string]any{"result": code, "stage": "registered"})
			return nil, out
		}
		sum.Registered = rs
	}
	if err := s.recordPoll(ctx, o, domain.OLTPollOK, nil, nil); err != nil {
		return nil, err
	}
	s.record(ctx, actor, &o.TenantID, "POLL_OLT", o.ID, map[string]any{
		"result": domain.OLTPollOK, "seen": sum.Seen, "new": sum.New, "resolved": sum.Resolved,
	})
	return sum, nil
}

// cleanONUs membersihkan entri dari perangkat (tak tepercaya): serial wajib 1..32 tanpa karakter kontrol, port dan tipe dipangkas,
// duplikat serial dibuang. Entri tak valid dilewati (dihitung), bukan menggagalkan seluruh polling.
func cleanONUs(in []domain.UnregisteredONU) ([]domain.UnregisteredONU, int) {
	out := make([]domain.UnregisteredONU, 0, len(in))
	seen := make(map[string]struct{}, len(in))
	skipped := 0
	for _, o := range in {
		serial, err := domain.CleanText("serial", o.SerialNumber, 1, domain.ONUSerialMaxLen, false)
		if err != nil {
			skipped++
			continue
		}
		if _, dup := seen[serial]; dup {
			skipped++
			continue
		}
		port, perr := domain.CleanText("pon_port", o.PonPort, 0, domain.ONUPortMaxLen, false)
		typ, terr := domain.CleanText("onu_type", o.ONUType, 0, domain.ONUTypeMaxLen, false)
		if perr != nil || terr != nil {
			skipped++
			continue
		}
		seen[serial] = struct{}{}
		out = append(out, domain.UnregisteredONU{PonPort: port, SerialNumber: serial, ONUType: typ})
	}
	return out, skipped
}

// reconcile: SN baru → PENDING; SN yang sudah ada → last_seen_at diperbarui (IGNORED tetap IGNORED); SN RESOLVED yang muncul
// lagi → PENDING; SN PENDING/IGNORED yang tidak lagi dilaporkan → RESOLVED. Riwayat RESOLVED dipangkas.
func (s *Service) reconcile(ctx context.Context, o *domain.OLT, clean []domain.UnregisteredONU) (*PollSummary, error) {
	pending, err := s.refs.GetByCode(ctx, domain.RefTableONUDiscoveryStatus, domain.ONUStatusPending)
	if err != nil {
		return nil, err
	}
	ignored, err := s.refs.GetByCode(ctx, domain.RefTableONUDiscoveryStatus, domain.ONUStatusIgnored)
	if err != nil {
		return nil, err
	}
	resolved, err := s.refs.GetByCode(ctx, domain.RefTableONUDiscoveryStatus, domain.ONUStatusResolved)
	if err != nil {
		return nil, err
	}
	existing, err := s.repo.ListONUsByOLT(ctx, o.ID)
	if err != nil {
		return nil, err
	}
	bySerial := make(map[string]domain.DiscoveredONU, len(existing))
	for _, e := range existing {
		bySerial[e.SerialNumber] = e
	}

	now := time.Now()
	sum := &PollSummary{Seen: len(clean)}
	rows := make([]domain.DiscoveredONU, 0, len(clean))
	seen := make(map[string]struct{}, len(clean))
	for _, c := range clean {
		seen[c.SerialNumber] = struct{}{}
		row := domain.DiscoveredONU{
			UUID: uuid.NewString(), PonPort: c.PonPort, SerialNumber: c.SerialNumber, ONUType: ptrOrNil(c.ONUType),
			StatusID: pending.ID, FirstSeenAt: now, LastSeenAt: now,
		}
		if ex, ok := bySerial[c.SerialNumber]; ok {
			row.UUID, row.FirstSeenAt = ex.UUID, ex.FirstSeenAt
			switch ex.Status {
			case domain.ONUStatusIgnored:
				row.StatusID = ignored.ID
			case domain.ONUStatusResolved:
				sum.Reappeared++
			}
		} else {
			sum.New++
		}
		rows = append(rows, row)
	}
	if err := s.repo.UpsertONUs(ctx, o.ID, rows); err != nil {
		return nil, err
	}
	gone := []uint64{}
	for _, e := range existing {
		if _, still := seen[e.SerialNumber]; !still && (e.Status == domain.ONUStatusPending || e.Status == domain.ONUStatusIgnored) {
			gone = append(gone, e.ID)
		}
	}
	if len(gone) > 0 {
		if err := s.repo.SetONUsStatus(ctx, gone, resolved.ID, &now, nil); err != nil {
			return nil, err
		}
	}
	sum.Resolved = len(gone)
	if _, err := s.repo.PruneResolvedONUs(ctx, o.ID, resolved.ID, domain.OLTKeepResolvedONUs); err != nil {
		return nil, err
	}
	for _, r := range rows {
		if r.StatusID == pending.ID {
			sum.QueueLength++
		}
	}
	return sum, nil
}

// ---- antrean ONU ----

type ONUListInput struct {
	OLTUUID  string
	Status   string // kode ref_onu_discovery_status; kosong = semua
	Search   string
	TenantID *uint64 // hanya efektif untuk superadmin
}

func (s *Service) ListONUs(ctx context.Context, actor domain.Actor, in ONUListInput, p domain.Pagination) ([]domain.DiscoveredONU, int, error) {
	f := domain.ONUFilter{Search: in.Search}
	if actor.IsSuperadmin() {
		f.TenantID = in.TenantID
	} else {
		tid, err := auth.ScopedTenantFilter(actor)
		if err != nil {
			return nil, 0, err
		}
		f.TenantID = tid
	}
	if in.OLTUUID != "" {
		o, err := s.load(ctx, actor, in.OLTUUID)
		if err != nil {
			return nil, 0, err
		}
		f.OLTID = &o.ID
	}
	if in.Status != "" {
		st, err := s.refs.GetByCode(ctx, domain.RefTableONUDiscoveryStatus, in.Status)
		if err != nil {
			return nil, 0, fmt.Errorf("%w: status tidak dikenal", domain.ErrInvalidInput)
		}
		f.StatusID = &st.ID
	}
	return s.repo.ListONUs(ctx, f, p)
}

// SetONUIgnored mengabaikan (ignored=true: PENDING → IGNORED) atau memulihkan (ignored=false: IGNORED → PENDING) sebuah entri.
// Entri RESOLVED tidak dapat diubah. Mengulang aksi yang sudah berlaku tidak error (idempoten).
func (s *Service) SetONUIgnored(ctx context.Context, actor domain.Actor, uuid string, ignored bool) (*domain.DiscoveredONU, error) {
	if err := auth.RequireRole(actor, domain.RoleAdmin, domain.RoleNOC); err != nil {
		return nil, err
	}
	n, err := s.repo.GetONUByUUID(ctx, uuid)
	if err != nil {
		return nil, err
	}
	if err := auth.RequireTenantScope(actor, &n.TenantID); err != nil {
		return nil, err
	}
	by := actor.UserIDPtr()
	if by == nil {
		return nil, domain.ErrForbidden
	}
	from, to, action := domain.ONUStatusPending, domain.ONUStatusIgnored, "IGNORE_OLT_ONU"
	if !ignored {
		from, to, action = domain.ONUStatusIgnored, domain.ONUStatusPending, "RESTORE_OLT_ONU"
	}
	switch n.Status {
	case to:
		return n, nil
	case from:
	default:
		return nil, fmt.Errorf("%w: entri berstatus %s tidak dapat diubah", domain.ErrConflict, n.Status)
	}
	st, err := s.refs.GetByCode(ctx, domain.RefTableONUDiscoveryStatus, to)
	if err != nil {
		return nil, err
	}
	if err := s.repo.SetONUsStatus(ctx, []uint64{n.ID}, st.ID, nil, by); err != nil {
		return nil, err
	}
	s.record(ctx, actor, &n.TenantID, action, n.OLTID, map[string]any{"onu_id": n.UUID, "serial_number": n.SerialNumber})
	return s.repo.GetONUByUUID(ctx, uuid)
}

func resolveWriteTenant(actor domain.Actor, requested *uint64) (uint64, error) {
	if actor.IsSuperadmin() {
		if requested == nil || *requested == 0 {
			return 0, fmt.Errorf("%w: tenant_id wajib diisi (superadmin tidak terikat tenant)", domain.ErrInvalidInput)
		}
		return *requested, nil
	}
	if actor.TenantID == nil {
		return 0, domain.ErrForbidden
	}
	if requested != nil && *requested != *actor.TenantID {
		return 0, fmt.Errorf("%w: hanya boleh mengelola OLT tenant sendiri", domain.ErrForbidden)
	}
	return *actor.TenantID, nil
}
