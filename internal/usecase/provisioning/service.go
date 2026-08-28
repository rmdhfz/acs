// Package provisioning menangani provisioning profile dan zero-touch
// provisioning (ZTP) — lihat TECH.md §6, FR-13..FR-18.
package provisioning

import (
	"context"
	"errors"
	"fmt"
	"hash/fnv"
	"regexp"
	"strconv"
	"strings"
	"time"

	"github.com/google/uuid"

	"acs/internal/domain"
	"acs/internal/usecase/auth"
)

type Service struct {
	profiles      domain.ProvisioningProfileRepository
	profileParams domain.ProvisioningProfileParameterRepository
	rules         domain.ZeroTouchRuleRepository
	devices       domain.DeviceRepository
	deviceParams  domain.DeviceParameterRepository
	refs          domain.RefRepository
	enqueuer      domain.TaskEnqueuer
	firmwareSvc   domain.FirmwareScheduler
	activity      domain.ActivityLogRepository
}

func NewService(
	profiles domain.ProvisioningProfileRepository,
	profileParams domain.ProvisioningProfileParameterRepository,
	rules domain.ZeroTouchRuleRepository,
	devices domain.DeviceRepository,
	deviceParams domain.DeviceParameterRepository,
	refs domain.RefRepository,
	enqueuer domain.TaskEnqueuer,
	firmwareSvc domain.FirmwareScheduler,
	activity domain.ActivityLogRepository,
) *Service {
	return &Service{
		profiles: profiles, profileParams: profileParams, rules: rules,
		devices: devices, deviceParams: deviceParams, refs: refs,
		enqueuer: enqueuer, firmwareSvc: firmwareSvc, activity: activity,
	}
}

// requireProfileReadScope mengizinkan baca/terapkan profile milik tenant
// sendiri ATAU profile global (tenant_id NULL, dibuat superadmin sbg default
// lintas tenant — FR-17, konsisten dgn provisioningProfileRepository.List()
// yang juga menampilkan tenant_id NULL ke semua tenant). Beda dari
// auth.RequireTenantScope di bawah (dipakai utk MUTASI profile — Create/
// Update/Delete) yang sengaja menolak tenant_id nil utk non-superadmin:
// profile global cuma boleh DIEDIT/DIHAPUS superadmin (supaya tidak ada satu
// tenant diam-diam mengubah default bersama), tapi boleh DIBACA/DITERAPKAN
// semua tenant.
func requireProfileReadScope(actor domain.Actor, resourceTenantID *uint64) error {
	if actor.IsSuperadmin() || resourceTenantID == nil {
		return nil
	}
	if actor.TenantID == nil || *actor.TenantID != *resourceTenantID {
		return domain.ErrForbidden
	}
	return nil
}

// ---- Provisioning Profile CRUD (FR-16/FR-17) ----

type CreateProfileInput struct {
	TenantID      *uint64
	VendorID      *uint64
	DeviceModelID *uint64
	Name          string
	Description   *string
	IsDefault     bool
	Parameters    []domain.ProvisioningProfileParameter
}

func (s *Service) CreateProfile(ctx context.Context, actor domain.Actor, in CreateProfileInput) (*domain.ProvisioningProfile, error) {
	if err := auth.RequireTenantScope(actor, in.TenantID); err != nil {
		return nil, err
	}
	p := &domain.ProvisioningProfile{
		ProfileUUID:   uuid.NewString(),
		TenantID:      in.TenantID,
		VendorID:      in.VendorID,
		DeviceModelID: in.DeviceModelID,
		Name:          in.Name,
		Description:   in.Description,
		IsDefault:     in.IsDefault,
		IsActive:      true,
		Audit:         domain.Audit{CreatedBy: actor.UserIDPtr()},
	}
	if err := s.profiles.Create(ctx, p); err != nil {
		return nil, err
	}
	if len(in.Parameters) > 0 {
		if err := s.profileParams.Replace(ctx, p.ID, in.Parameters); err != nil {
			return nil, err
		}
	}
	_ = s.activity.Record(ctx, &domain.ActivityLog{
		UserID: actor.UserIDPtr(), TenantID: actor.TenantID,
		Action: "CREATE_PROVISIONING_PROFILE", EntityType: "provisioning_profile", EntityID: &p.ID,
	})
	return p, nil
}

func (s *Service) Get(ctx context.Context, actor domain.Actor, id uint64) (*domain.ProvisioningProfile, []domain.ProvisioningProfileParameter, error) {
	p, err := s.profiles.GetByID(ctx, id)
	if err != nil {
		return nil, nil, err
	}
	if err := requireProfileReadScope(actor, p.TenantID); err != nil {
		return nil, nil, err
	}
	params, err := s.profileParams.ListByProfile(ctx, id)
	if err != nil {
		return nil, nil, err
	}
	return p, params, nil
}

// List — queryTenantID datang dari query param `tenant_id`, cuma dipakai
// kalau actor superadmin (filter opsional lintas tenant). Actor non-superadmin
// SELALU dipaksa ke tenant-nya sendiri, mengabaikan queryTenantID — mencegah
// spoofing lewat query param, dan menolak (bukan diam-diam pass-through nil)
// kalau actor.TenantID kosong (lihat auth.ScopedTenantFilter).
func (s *Service) List(ctx context.Context, actor domain.Actor, queryTenantID *uint64, p domain.Pagination) ([]domain.ProvisioningProfile, int, error) {
	tenantID := queryTenantID
	if !actor.IsSuperadmin() {
		tid, err := auth.ScopedTenantFilter(actor)
		if err != nil {
			return nil, 0, err
		}
		tenantID = tid
	}
	return s.profiles.List(ctx, tenantID, p)
}

func (s *Service) UpdateProfile(ctx context.Context, actor domain.Actor, p *domain.ProvisioningProfile, params []domain.ProvisioningProfileParameter) error {
	existing, err := s.profiles.GetByID(ctx, p.ID)
	if err != nil {
		return err
	}
	if err := auth.RequireTenantScope(actor, existing.TenantID); err != nil {
		return err
	}
	p.UpdatedBy = actor.UserIDPtr()
	if err := s.profiles.Update(ctx, p); err != nil {
		return err
	}
	if params != nil {
		if err := s.profileParams.Replace(ctx, p.ID, params); err != nil {
			return err
		}
	}
	_ = s.activity.Record(ctx, &domain.ActivityLog{
		UserID: actor.UserIDPtr(), TenantID: actor.TenantID,
		Action: "UPDATE_PROVISIONING_PROFILE", EntityType: "provisioning_profile", EntityID: &p.ID,
	})
	return nil
}

func (s *Service) DeleteProfile(ctx context.Context, actor domain.Actor, id uint64) error {
	p, err := s.profiles.GetByID(ctx, id)
	if err != nil {
		return err
	}
	if err := auth.RequireTenantScope(actor, p.TenantID); err != nil {
		return err
	}
	if err := s.profiles.SoftDelete(ctx, id, actor.UserID); err != nil {
		return err
	}
	_ = s.activity.Record(ctx, &domain.ActivityLog{
		UserID: actor.UserIDPtr(), TenantID: actor.TenantID,
		Action: "DELETE_PROVISIONING_PROFILE", EntityType: "provisioning_profile", EntityID: &id,
	})
	return nil
}

// ApplyProfile mengantre task SetParameterValues berisi seluruh parameter
// profile ke device tertentu. Pemanggilan SELALU eksplisit (manual atau dari
// EvaluateZeroTouch) — perubahan pada profile TIDAK otomatis mendorong ulang
// ke device yang sudah terprovisioning (FR-18; keputusan produk yang
// disengaja per CLAUDE.md, jangan diotomatisasi tanpa konfirmasi eksplisit).
func (s *Service) ApplyProfile(ctx context.Context, actor domain.Actor, deviceID, profileID uint64) (*domain.Task, error) {
	dev, err := s.devices.GetByID(ctx, deviceID)
	if err != nil {
		return nil, err
	}
	if err := auth.RequireTenantScope(actor, dev.TenantID); err != nil {
		return nil, err
	}
	profile, err := s.profiles.GetByID(ctx, profileID)
	if err != nil {
		return nil, err
	}
	if err := requireProfileReadScope(actor, profile.TenantID); err != nil {
		return nil, err
	}
	params, err := s.profileParams.ListByProfile(ctx, profileID)
	if err != nil {
		return nil, err
	}
	if len(params) == 0 {
		return nil, fmt.Errorf("provisioning: profile %d tidak punya parameter untuk diterapkan", profileID)
	}
	values := make(map[string]string, len(params))
	for _, pp := range params {
		val := ""
		if pp.ParameterValue != nil {
			val = *pp.ParameterValue
		}
		if isPeriodicInformIntervalKey(pp.ParameterName) {
			val = jitterPeriodicInformInterval(deviceID, val)
		}
		values[pp.ParameterName] = val
	}
	t, err := s.enqueuer.EnqueueSetParameterValues(ctx, actor, deviceID, values, 3)
	if err != nil {
		return nil, err
	}

	dev.ProvisioningProfileID = &profileID
	dev.UpdatedBy = actor.UserIDPtr()
	if err := s.devices.Update(ctx, dev); err != nil {
		return nil, err
	}

	_ = s.activity.Record(ctx, &domain.ActivityLog{
		UserID: actor.UserIDPtr(), TenantID: actor.TenantID,
		Action: "APPLY_PROVISIONING_PROFILE", EntityType: "device", EntityID: &deviceID,
	})
	return t, nil
}

// ---- Zero-Touch Provisioning (TECH.md §6, FR-13/14/15) ----

// EvaluateZeroTouch dipanggil usecase/session pada Inform, dengan
// triggerCodes = himpunan domain.ZtpTriggerEvent* yang berlaku untuk Inform
// INI (lihat usecase/session HandleInform: BOOTSTRAP_ONLY hanya masuk kalau
// event 0 BOOTSTRAP ada; BOOTSTRAP_OR_BOOT masuk kalau 0 BOOTSTRAP atau
// 1 BOOT ada; EVERY_INFORM SELALU masuk apa pun event-nya). Rule pertama yang
// cocok (priority ASC) DAN trigger_event_id-nya termasuk triggerCodes
// langsung dieksekusi aksinya. Bila tidak ada satu pun yang cocok,
// mengembalikan domain.ErrNoMatchingRule — device TETAP di inventory
// menunggu tindakan manual, tidak silently diabaikan (FR-15).
//
// Guard "device sudah pernah diprovisioning" (dev.ProvisioningProfileID != nil)
// HANYA berlaku untuk rule ber-trigger BOOTSTRAP_ONLY — mempertahankan PERSIS
// perilaku asli (ZTP di-bootstrap cuma pernah jalan sekali seumur hidup
// device, FR-18: perubahan konfigurasi tidak boleh otomatis terdorong ulang
// tanpa konfirmasi eksplisit). Rule BOOTSTRAP_OR_BOOT/EVERY_INFORM SENGAJA
// men-skip guard ini — nilai fitur trigger tsb justru evaluasi BERULANG
// (mis. "tiap device reboot, cek apakah firmware masih versi lama, push
// upgrade kalau iya"), bukan sekali di awal.
//
// PERINGATAN OPERASIONAL (didokumentasikan, BUKAN dicegah otomatis di sini):
// kombinasi trigger EVERY_INFORM + PostApplyReboot=true BISA menyebabkan
// reboot loop tak berkesudahan (reboot -> event 1 BOOT -> Inform baru ->
// rule cocok lagi -> reboot lagi -> ...). Satu-satunya pagar yang diterapkan
// di sini untuk rule NON-BOOTSTRAP_ONLY adalah TIDAK menembak aksi baru bila
// device masih punya task pending/queued/sent (HasPendingForDevice) — ini
// memperlambat, TAPI TIDAK MENJAMIN mencegah loop sepenuhnya begitu task
// sebelumnya selesai. Operator penulis rule bertanggung jawab tidak
// memasangkan EVERY_INFORM dengan PostApplyReboot kecuali benar-benar
// dimaksudkan sbg pengawasan berkelanjutan yang idempotent (mis. hanya
// benar-benar mengeksekusi saat MatchParameterName menunjukkan kondisi
// yang PERLU diperbaiki, bukan kondisi yang selalu true).
func (s *Service) EvaluateZeroTouch(ctx context.Context, actor domain.Actor, dev *domain.Device, triggerCodes []string) (*domain.ZeroTouchRule, error) {
	triggerIDs, bootstrapOnlyID, err := s.resolveTriggerEventIDs(ctx, triggerCodes)
	if err != nil {
		return nil, err
	}
	rules, err := s.rules.ListActiveOrdered(ctx, dev.TenantID)
	if err != nil {
		return nil, err
	}
	for i := range rules {
		r := &rules[i]
		if !triggerIDs[r.TriggerEventID] {
			continue
		}
		if r.TriggerEventID == bootstrapOnlyID && dev.ProvisioningProfileID != nil {
			continue
		}
		matched, err := s.matchZeroTouchRule(ctx, r, dev)
		if err != nil {
			return nil, err
		}
		if !matched {
			continue
		}
		// Pagar anti-reboot-loop -- hanya diterapkan utk rule NON-bootstrap-only,
		// karena rule bootstrap-only sudah dijamin sekali-jalan lewat guard di
		// atas. DUA lapis (temuan review kode+keamanan independen: lapis
		// pertama saja TIDAK CUKUP):
		//  1. HasPendingForDevice -- cegah tembakan kedua yang tumpang tindih
		//     SELAGI task dari tembakan pertama masih berjalan (sesi yang sama
		//     atau berdekatan).
		//  2. withinActionCooldown -- cegah tembakan ULANG lintas SESI: begitu
		//     task tembakan pertama selesai (apa pun hasilnya) DAN device
		//     kembali online lewat Inform baru, lapis 1 saja akan "bersih"
		//     lagi -- kalau precondition rule masih cocok (mis. upgrade
		//     firmware gagal, software_version tak berubah), rule akan
		//     menembak lagi TANPA BATAS tanpa lapis 2 ini (reboot loop tak
		//     berkesudahan thd device pelanggan nyata).
		if r.TriggerEventID != bootstrapOnlyID {
			pending, err := s.enqueuer.HasPendingForDevice(ctx, dev.ID)
			if err != nil {
				return nil, err
			}
			if pending {
				continue
			}
			cooling, err := s.withinActionCooldown(ctx, dev, r)
			if err != nil {
				return nil, err
			}
			if cooling {
				continue
			}
		}
		if err := s.executeZeroTouchActions(ctx, actor, dev, r); err != nil {
			// Rule COCOK tapi eksekusi aksi GAGAL (mis. kuota task queue tenant
			// penuh, ROADMAP.md Fase 2) HARUS tetap kelihatan operator/NOC —
			// FR-15 eksplisit bilang device tidak boleh silently diabaikan.
			desc := err.Error()
			_ = s.activity.Record(ctx, &domain.ActivityLog{
				TenantID: dev.TenantID, Action: "ZERO_TOUCH_APPLY_FAILED", EntityType: "device", EntityID: &dev.ID,
				Description: &desc,
			})
			return nil, err
		}
		// rule_id disematkan di Description (bukan kolom terpisah) supaya
		// withinActionCooldown bisa mencari kembali "kapan terakhir RULE INI
		// menembak aksi utk DEVICE INI" tanpa migrasi skema baru.
		matchDesc := fmt.Sprintf("rule_id=%d", r.ID)
		_ = s.activity.Record(ctx, &domain.ActivityLog{
			TenantID: dev.TenantID, Action: "ZERO_TOUCH_MATCH", EntityType: "device", EntityID: &dev.ID,
			Description: &matchDesc,
		})
		return r, nil
	}
	return nil, domain.ErrNoMatchingRule
}

// ztpActionCooldown -- lihat komentar pagar anti-reboot-loop di
// EvaluateZeroTouch. Nilai tetap (belum ada field per-rule utk override) --
// 1 jam dipilih sbg default yang longgar utk operasi wajar (mis. mau cek
// ulang kondisi tiap beberapa periodic inform) tapi cukup ketat mencegah
// reboot/firmware-push storm dalam hitungan menit.
const ztpActionCooldown = 1 * time.Hour

// ztpActionCooldownLookback -- jumlah entry activity_logs TERBARU milik
// device ini yang diperiksa withinActionCooldown utk mencari kapan terakhir
// rule INI menembak aksi. KETERBATASAN YANG DITERIMA (bukan diam-diam
// diabaikan): device yang sangat aktif (banyak activity_logs lain selain
// ZTP dalam window ini) scr teori bisa mendorong entry ZTP lama keluar dari
// jendela sebelum cooldown-nya sendiri habis, membuat cooldown "lupa" lebih
// cepat dari niatnya. Diterima sbg pendekatan pragmatis (reuse activity_logs
// yang sudah ada, tanpa kolom/tabel baru) -- kandidat perbaikan lanjutan:
// tabel khusus last-fired per (rule, device) kalau pola pemakaian nyata
// menunjukkan ini jadi masalah.
const ztpActionCooldownLookback = 20

// withinActionCooldown mengecek apakah rule r pernah menembak aksi thd
// device dev dalam ztpActionCooldown terakhir (lihat EvaluateZeroTouch).
func (s *Service) withinActionCooldown(ctx context.Context, dev *domain.Device, r *domain.ZeroTouchRule) (bool, error) {
	logs, _, err := s.activity.ListByEntity(ctx, "device", dev.ID, domain.Pagination{Page: 1, PageSize: ztpActionCooldownLookback})
	if err != nil {
		return false, err
	}
	marker := fmt.Sprintf("rule_id=%d", r.ID)
	cutoff := time.Now().Add(-ztpActionCooldown)
	for _, l := range logs {
		if l.Action != "ZERO_TOUCH_MATCH" || l.Description == nil || !strings.Contains(*l.Description, marker) {
			continue
		}
		// ListByEntity terurut id DESC (terbaru dulu) -- entry PERTAMA yang
		// cocok marker rule ini otomatis yang PALING BARU, tidak perlu terus
		// mencari sisanya.
		return l.CreatedAt.After(cutoff), nil
	}
	return false, nil
}

// executeZeroTouchActions menjalankan SEMUA aksi rule yang cocok — profile
// (opsional sejak migrations/0009), reboot (opsional), firmware push
// (opsional) — bisa gabungan lebih dari satu sekaligus pada rule yang sama.
func (s *Service) executeZeroTouchActions(ctx context.Context, actor domain.Actor, dev *domain.Device, r *domain.ZeroTouchRule) error {
	if r.ProvisioningProfileID != nil {
		if _, err := s.ApplyProfile(ctx, actor, dev.ID, *r.ProvisioningProfileID); err != nil {
			return err
		}
	}
	if r.FirmwareFileID != nil {
		if _, err := s.firmwareSvc.ScheduleUpgrade(ctx, actor, dev.ID, *r.FirmwareFileID, nil); err != nil {
			return fmt.Errorf("provisioning: aksi firmware push rule ZTP gagal: %w", err)
		}
	}
	if r.PostApplyReboot {
		if _, err := s.enqueuer.EnqueueReboot(ctx, actor, dev.ID, 5); err != nil {
			return fmt.Errorf("provisioning: aksi reboot rule ZTP gagal: %w", err)
		}
	}
	return nil
}

// resolveTriggerEventIDs meng-cache-kan resolusi kode ref_ztp_trigger_event
// (dipanggil setiap Inform, jadi sengaja query kecil & flat, bukan JOIN
// berat) menjadi (himpunan ID yang berlaku untuk Inform ini, ID BOOTSTRAP_ONLY
// utk pengecekan guard di EvaluateZeroTouch).
func (s *Service) resolveTriggerEventIDs(ctx context.Context, triggerCodes []string) (map[uint64]bool, uint64, error) {
	bootstrapOnly, err := s.refs.GetByCode(ctx, domain.RefTableZtpTriggerEvent, domain.ZtpTriggerEventBootstrapOnly)
	if err != nil {
		return nil, 0, err
	}
	ids := make(map[uint64]bool, len(triggerCodes))
	for _, code := range triggerCodes {
		ref, err := s.refs.GetByCode(ctx, domain.RefTableZtpTriggerEvent, code)
		if err != nil {
			return nil, 0, err
		}
		ids[ref.ID] = true
	}
	return ids, bootstrapOnly.ID, nil
}

// matchZeroTouchRule mengevaluasi seluruh precondition rule: yang sudah ada
// sejak awal (VendorID/DeviceModelID/OUI/SerialPattern) DAN yang ditambahkan
// migrations/0009 (SoftwareVersionPattern, MatchParameterName +
// MatchParameterValuePattern — precondition tambahan opsional yang
// mensyaratkan device punya baris device_parameters dengan nama tsb yang
// nilainya cocok pola SQL LIKE MatchParameterValuePattern).
func (s *Service) matchZeroTouchRule(ctx context.Context, r *domain.ZeroTouchRule, dev *domain.Device) (bool, error) {
	if r.VendorID != nil {
		if dev.VendorID == nil || *r.VendorID != *dev.VendorID {
			return false, nil
		}
	}
	if r.DeviceModelID != nil {
		if dev.DeviceModelID == nil || *r.DeviceModelID != *dev.DeviceModelID {
			return false, nil
		}
	}
	if r.OUI != nil {
		if dev.OUI == nil || !strings.EqualFold(*r.OUI, *dev.OUI) {
			return false, nil
		}
	}
	if r.SerialPattern != nil {
		if !matchSQLLike(*r.SerialPattern, dev.SerialNumber) {
			return false, nil
		}
	}
	if r.SoftwareVersionPattern != nil {
		if dev.SoftwareVersion == nil || !matchSQLLike(*r.SoftwareVersionPattern, *dev.SoftwareVersion) {
			return false, nil
		}
	}
	if r.MatchParameterName != nil {
		// MatchParameterValuePattern DIJAMIN turut terisi (CHECK constraint DB
		// chk_ztr_match_parameter_pair, migrations/0009) -- tidak perlu nil-check.
		param, err := s.deviceParams.Get(ctx, dev.ID, *r.MatchParameterName)
		if err != nil {
			if errors.Is(err, domain.ErrNotFound) {
				return false, nil
			}
			return false, err
		}
		if param.ParameterValue == nil || !matchSQLLike(*r.MatchParameterValuePattern, *param.ParameterValue) {
			return false, nil
		}
	}
	return true, nil
}

// isPeriodicInformIntervalKey mengenali logical key ATAU raw path TR-069
// (TR-098/TR-181, keduanya berujung "...PeriodicInformInterval") untuk
// parameter PeriodicInformInterval -- dipakai jitterPeriodicInformInterval
// di ApplyProfile agar tidak semua device dapat nilai identik persis
// (mitigasi "boot storm" tersinkron setelah pemadaman listrik massal,
// panduan arsitektur ACS §10.2). Baik raw path TR-098
// (InternetGatewayDevice.ManagementServer.PeriodicInformInterval) maupun
// TR-181 (Device.ManagementServer.PeriodicInformInterval) berujung sama.
func isPeriodicInformIntervalKey(name string) bool {
	return name == "device.periodic_inform_interval" || strings.HasSuffix(name, ".PeriodicInformInterval")
}

// jitterPeriodicInformInterval menambahkan offset deterministik (berbasis
// hash deviceID, BUKAN random murni -- supaya reapply profile yang sama ke
// device yang sama menghasilkan nilai identik, tidak "melompat" tiap kali
// profile diterapkan ulang) sebesar hingga ±10% dari nilai dasar. Nilai
// non-numerik (mis. placeholder/typo) dikembalikan apa adanya -- jitter
// SENGAJA tidak memaksakan parsing, ini murni mitigasi tambahan, bukan
// validasi (validasi tipe parameter di luar cakupan fungsi ini).
func jitterPeriodicInformInterval(deviceID uint64, baseValue string) string {
	base, err := strconv.Atoi(strings.TrimSpace(baseValue))
	if err != nil || base <= 0 {
		return baseValue
	}
	const jitterPercent = 10
	spread := base * jitterPercent / 100
	if spread <= 0 {
		return baseValue
	}
	h := fnv.New32a()
	_, _ = h.Write([]byte(strconv.FormatUint(deviceID, 10)))
	offset := int(h.Sum32()%uint32(2*spread+1)) - spread
	jittered := base + offset
	if jittered < 1 {
		jittered = 1
	}
	return strconv.Itoa(jittered)
}

// matchSQLLike meniru semantik SQL LIKE (% dan _) di memori — jumlah rule ZTP
// kecil dan dievaluasi sekali per Inform, jadi tidak perlu query per rule.
//
// [BUG DITEMUKAN & DIPERBAIKI] Implementasi awal memanggil
// regexp.QuoteMeta(pattern) DULU baru mencari-ganti "\%"/"\_" jadi ".*"/".".
// regexp.QuoteMeta TIDAK meng-escape "%"/"_" (keduanya bukan karakter
// spesial regex) -- jadi tidak pernah ada "\%"/"\_" utk dicari, ReplaceAll
// jadi no-op, dan wildcard SQL LIKE (% maupun _) TIDAK PERNAH benar-benar
// berfungsi sejak awal (SerialPattern dgn wildcard silently selalu gagal
// cocok). Ketahuan lewat unit test baru (TestEvaluateZeroTouch_
// SoftwareVersionPattern) -- diperbaiki dgn escape per-karakter: setiap
// karakter selain % dan _ di-QuoteMeta satu-satu, sehingga % dan _ dari
// pattern SELALU sampai ke tahap penerjemahan wildcard, tidak pernah
// ke-escape duluan oleh QuoteMeta massal.
func matchSQLLike(pattern, s string) bool {
	var re strings.Builder
	re.WriteString("^")
	for _, r := range pattern {
		switch r {
		case '%':
			re.WriteString(".*")
		case '_':
			re.WriteString(".")
		default:
			re.WriteString(regexp.QuoteMeta(string(r)))
		}
	}
	re.WriteString("$")
	compiled, err := regexp.Compile("(?is)" + re.String())
	if err != nil {
		return false
	}
	return compiled.MatchString(s)
}

// ListZeroTouchRules — pola sama seperti List (profile) di atas: queryTenantID
// cuma berlaku utk superadmin, non-superadmin selalu dipaksa ke tenant sendiri
// dan ditolak (bukan fail-open) kalau actor.TenantID kosong.
func (s *Service) ListZeroTouchRules(ctx context.Context, actor domain.Actor, queryTenantID *uint64) ([]domain.ZeroTouchRule, error) {
	tenantID := queryTenantID
	if !actor.IsSuperadmin() {
		tid, err := auth.ScopedTenantFilter(actor)
		if err != nil {
			return nil, err
		}
		tenantID = tid
	}
	return s.rules.ListActiveOrdered(ctx, tenantID)
}

// validateMatchParameterPair menegakkan di layer usecase (respons 400 yang
// bersih) constraint yang juga ditegakkan DB (chk_ztr_match_parameter_pair,
// migrations/0009) sbg pengaman terakhir -- tanpa ini, kirim salah satu dari
// MatchParameterName/MatchParameterValuePattern saja akan lolos sampai ke
// MariaDB dan balik sbg error constraint mentah yang tidak ramah klien
// (temuan acs-code-reviewer).
func validateMatchParameterPair(r *domain.ZeroTouchRule) error {
	if (r.MatchParameterName == nil) != (r.MatchParameterValuePattern == nil) {
		return fmt.Errorf("%w: match_parameter_name dan match_parameter_value_pattern harus diisi berpasangan (keduanya atau tidak sama sekali)", domain.ErrInvalidInput)
	}
	return nil
}

func (s *Service) CreateZeroTouchRule(ctx context.Context, actor domain.Actor, r *domain.ZeroTouchRule) error {
	if err := validateMatchParameterPair(r); err != nil {
		return err
	}
	if err := auth.RequireTenantScope(actor, r.TenantID); err != nil {
		return err
	}
	r.CreatedBy = actor.UserIDPtr()
	if err := s.rules.Create(ctx, r); err != nil {
		return err
	}
	_ = s.activity.Record(ctx, &domain.ActivityLog{
		UserID: actor.UserIDPtr(), TenantID: actor.TenantID,
		Action: "CREATE_ZERO_TOUCH_RULE", EntityType: "zero_touch_rule", EntityID: &r.ID,
	})
	return nil
}

func (s *Service) UpdateZeroTouchRule(ctx context.Context, actor domain.Actor, r *domain.ZeroTouchRule) error {
	if err := validateMatchParameterPair(r); err != nil {
		return err
	}
	existing, err := s.rules.GetByID(ctx, r.ID)
	if err != nil {
		return err
	}
	if err := auth.RequireTenantScope(actor, existing.TenantID); err != nil {
		return err
	}
	r.UpdatedBy = actor.UserIDPtr()
	if err := s.rules.Update(ctx, r); err != nil {
		return err
	}
	_ = s.activity.Record(ctx, &domain.ActivityLog{
		UserID: actor.UserIDPtr(), TenantID: actor.TenantID,
		Action: "UPDATE_ZERO_TOUCH_RULE", EntityType: "zero_touch_rule", EntityID: &r.ID,
	})
	return nil
}

func (s *Service) DeleteZeroTouchRule(ctx context.Context, actor domain.Actor, id uint64) error {
	r, err := s.rules.GetByID(ctx, id)
	if err != nil {
		return err
	}
	if err := auth.RequireTenantScope(actor, r.TenantID); err != nil {
		return err
	}
	if err := s.rules.SoftDelete(ctx, id, actor.UserID); err != nil {
		return err
	}
	_ = s.activity.Record(ctx, &domain.ActivityLog{
		UserID: actor.UserIDPtr(), TenantID: actor.TenantID,
		Action: "DELETE_ZERO_TOUCH_RULE", EntityType: "zero_touch_rule", EntityID: &id,
	})
	return nil
}
