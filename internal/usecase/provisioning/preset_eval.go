package provisioning

import (
	"context"
	"crypto/sha256"
	"encoding/hex"
	"errors"
	"fmt"
	"sort"
	"strings"
	"time"

	"acs/internal/domain"
)

// Engine preset v1 — PRESET_ENGINE_DESIGN.md, keputusan 2026-09-02 (Opsi C +
// 5a/5b). Preset ber-`enforce=1` dijaga sesuai konfigurasi tiap sesi CWMP:
// bila device menyimpang (drift), satu SetParameterValues gabungan diantre.
// Setelah device konvergen, evaluasi jadi no-op sampai ada yang mengubah
// nilai di luar ACS ("self-healing" gaya GenieACS).
//
// BEDA dari provisioning profile: FR-18 ("profil tidak auto re-push") SENGAJA
// tidak berlaku untuk preset — itu keputusan produk eksplisit (5a). Pengaman:
// `enforce` default OFF, jadi preset lama tidak tiba-tiba aktif (5b).
const (
	// presetApplyCooldown — jarak minimum antar-apply preset yang SAMA ke
	// device yang SAMA. Menjaga bila CPE menolak nilai & terus melapor nilai
	// lama (drift "abadi") supaya tidak spam SetParameterValues tiap sesi.
	presetApplyCooldown = 15 * time.Minute
	// presetMaxConsecutiveFailures — setelah sekian kegagalan berturut, preset
	// ditandai FAILED untuk device itu & berhenti dicoba (NOC sudah dapat
	// activity_logs PRESET_APPLY_FAILED). FR-15: tidak silently diabaikan.
	presetMaxConsecutiveFailures = 3
)

// EvaluatePresets dipanggil usecase/session tepat setelah EvaluateZeroTouch,
// setelah device_parameters ter-update dari Inform. Nil-safe: bila engine tidak
// di-wire (presets/presetApps nil) langsung return nil. Error TIDAK boleh
// menggagalkan seluruh Inform — pemanggil me-log & lanjut (pola sama ZTP).
func (s *Service) EvaluatePresets(ctx context.Context, actor domain.Actor, dev *domain.Device) error {
	if s.presets == nil || s.presetApps == nil {
		return nil
	}
	presets, err := s.presets.ListActiveEnforce(ctx, dev.TenantID)
	if err != nil {
		return err
	}
	if len(presets) == 0 {
		return nil
	}

	// Pagar: jangan tembak apa pun kalau device masih punya task in-flight
	// (pola sama pagar anti-loop EvaluateZeroTouch).
	pending, err := s.enqueuer.HasPendingForDevice(ctx, dev.ID)
	if err != nil {
		return err
	}
	if pending {
		return nil
	}

	// Kumpulkan drift lintas SEMUA preset yang cocok, digabung jadi satu
	// SetParameterValues. ListActiveEnforce urut weight ASC -> preset weight
	// lebih besar dievaluasi belakangan -> menimpa saat key bentrok (§4).
	drift := map[string]string{} // raw TR-069 path -> nilai target
	var matchedPresetIDs []uint64
	matchedPresetHashes := map[uint64]string{} // preset id -> hash drift preset itu

	for i := range presets {
		p := &presets[i]

		pc, err := p.ParsedPrecondition()
		if err != nil {
			s.recordPresetConfigIssue(ctx, dev, p, err.Error())
			continue
		}
		matched, err := s.matchPresetPrecondition(ctx, &pc, dev)
		if err != nil {
			return err
		}
		if !matched {
			continue
		}

		app, err := s.presetApps.Get(ctx, p.ID, dev.ID)
		if err != nil && !errors.Is(err, domain.ErrNotFound) {
			return err
		}
		if app != nil {
			if app.Status == domain.PresetApplicationStatusFailed &&
				app.ConsecutiveFailures >= presetMaxConsecutiveFailures {
				continue // sudah menyerah — NOC sudah diberi tahu
			}
			if app.LastAppliedAt != nil && time.Since(*app.LastAppliedAt) < presetApplyCooldown {
				continue
			}
		}

		ops, err := p.ParsedConfigurations()
		if err != nil {
			s.recordPresetConfigIssue(ctx, dev, p, err.Error())
			continue
		}
		presetDrift, err := s.presetSetParameterDrift(ctx, dev, ops)
		if err != nil {
			return err
		}
		if len(presetDrift) == 0 {
			// device sudah sesuai preset ini
			if app == nil || app.Status != domain.PresetApplicationStatusConverged {
				_ = s.presetApps.Upsert(ctx, &domain.PresetApplication{
					PresetID: p.ID, DeviceID: dev.ID,
					LastAppliedAt:       presetLastApplied(app),
					ConsecutiveFailures: 0,
					Status:              domain.PresetApplicationStatusConverged,
				})
			}
			continue
		}

		// Anti-spam: kalau drift preset ini PERSIS SAMA dengan yang terakhir
		// kita push (last_drift_hash) dan statusnya masih PENDING, berarti
		// push sebelumnya tidak membuat device konvergen (CPE menolak nilai,
		// atau parameter read-only). Hitung sebagai kegagalan; setelah
		// presetMaxConsecutiveFailures, tandai FAILED & berhenti mencoba
		// (NOC diberi tahu lewat activity_logs) -- FR-15.
		presetHash := hashDriftSet(presetDrift)
		if app != nil && app.LastDriftHash != nil && *app.LastDriftHash == presetHash &&
			app.Status == domain.PresetApplicationStatusPending {
			fails := app.ConsecutiveFailures + 1
			status := domain.PresetApplicationStatusPending
			if fails >= presetMaxConsecutiveFailures {
				status = domain.PresetApplicationStatusFailed
				desc := fmt.Sprintf("preset_id=%d menyerah setelah %d kali push tanpa konvergensi (CPE menolak nilai / parameter read-only?)", p.ID, fails)
				_ = s.activity.Record(ctx, &domain.ActivityLog{
					TenantID: dev.TenantID, Action: "PRESET_APPLY_FAILED",
					EntityType: "device", EntityID: &dev.ID, Description: &desc,
				})
			}
			_ = s.presetApps.Upsert(ctx, &domain.PresetApplication{
				PresetID: p.ID, DeviceID: dev.ID,
				LastAppliedAt:       app.LastAppliedAt,
				LastDriftHash:       &presetHash,
				ConsecutiveFailures: fails,
				Status:              status,
			})
			if status == domain.PresetApplicationStatusFailed {
				continue
			}
		}

		for k, v := range presetDrift {
			drift[k] = v
		}
		matchedPresetIDs = append(matchedPresetIDs, p.ID)
		matchedPresetHashes[p.ID] = presetHash
	}

	if len(drift) == 0 {
		return nil
	}

	_, enqErr := s.enqueuer.EnqueueSetParameterValues(ctx, actor, dev.ID, drift, 3)
	now := time.Now()

	if enqErr != nil {
		for _, pid := range matchedPresetIDs {
			s.bumpPresetFailure(ctx, pid, dev.ID)
		}
		desc := fmt.Sprintf("preset apply gagal (preset_ids=%s): %v", joinUint64s(matchedPresetIDs), enqErr)
		_ = s.activity.Record(ctx, &domain.ActivityLog{
			TenantID: dev.TenantID, Action: "PRESET_APPLY_FAILED",
			EntityType: "device", EntityID: &dev.ID, Description: &desc,
		})
		return enqErr
	}

	for _, pid := range matchedPresetIDs {
		h := matchedPresetHashes[pid]
		prevFails := 0
		if a, err := s.presetApps.Get(ctx, pid, dev.ID); err == nil && a != nil &&
			a.LastDriftHash != nil && *a.LastDriftHash == h {
			// hash sama = kita meng-enqueue drift yang sama lagi; pertahankan
			// hitungan kegagalan (di-bump di loop atas) supaya give-up tetap jalan.
			prevFails = a.ConsecutiveFailures
		}
		_ = s.presetApps.Upsert(ctx, &domain.PresetApplication{
			PresetID: pid, DeviceID: dev.ID,
			LastAppliedAt:       &now,
			LastDriftHash:       &h,
			ConsecutiveFailures: prevFails,
			Status:              domain.PresetApplicationStatusPending,
		})
	}
	desc := fmt.Sprintf("preset_ids=%s drift=%d", joinUint64s(matchedPresetIDs), len(drift))
	_ = s.activity.Record(ctx, &domain.ActivityLog{
		TenantID: dev.TenantID, Action: "PRESET_APPLY",
		EntityType: "device", EntityID: &dev.ID, Description: &desc,
	})
	return nil
}

// matchPresetPrecondition — kosakata & semantik IDENTIK matchZeroTouchRule
// (reuse matchSQLLike). Precondition kosong = cocok semua.
func (s *Service) matchPresetPrecondition(ctx context.Context, pc *domain.PresetPrecondition, dev *domain.Device) (bool, error) {
	if pc.VendorID != nil {
		if dev.VendorID == nil || *pc.VendorID != *dev.VendorID {
			return false, nil
		}
	}
	if pc.DeviceModelID != nil {
		if dev.DeviceModelID == nil || *pc.DeviceModelID != *dev.DeviceModelID {
			return false, nil
		}
	}
	if pc.OUI != nil {
		if dev.OUI == nil || !strings.EqualFold(*pc.OUI, *dev.OUI) {
			return false, nil
		}
	}
	if pc.SerialPattern != nil {
		if !matchSQLLike(*pc.SerialPattern, dev.SerialNumber) {
			return false, nil
		}
	}
	if pc.SoftwareVersionPattern != nil {
		if dev.SoftwareVersion == nil || !matchSQLLike(*pc.SoftwareVersionPattern, *dev.SoftwareVersion) {
			return false, nil
		}
	}
	if pc.MatchParameterName != nil && pc.MatchParameterValuePattern != nil {
		param, err := s.deviceParams.Get(ctx, dev.ID, *pc.MatchParameterName)
		if err != nil {
			if errors.Is(err, domain.ErrNotFound) {
				return false, nil
			}
			return false, err
		}
		if param.ParameterValue == nil || !matchSQLLike(*pc.MatchParameterValuePattern, *param.ParameterValue) {
			return false, nil
		}
	}
	return true, nil
}

// presetSetParameterDrift menghitung parameter mana yang nilainya menyimpang
// dari target preset. Hanya op `set_parameter` yang diproses di v1 (apply_profile
// & refresh direncanakan v1.1). Key di-resolve ke raw TR-069 path supaya bisa
// dibandingkan dengan device_parameters (yang di-key oleh raw path). Op yang
// key-nya tidak bisa di-resolve (belum ada mapping, vendor device belum
// diketahui) DILEWATI dengan catatan — tidak menggagalkan evaluasi.
func (s *Service) presetSetParameterDrift(ctx context.Context, dev *domain.Device, ops []domain.PresetConfigOp) (map[string]string, error) {
	out := map[string]string{}
	for _, op := range ops {
		if op.Op != domain.PresetOpSetParameter || op.Key == "" {
			continue
		}
		path, err := s.enqueuer.ResolveParameterPath(ctx, dev.ID, op.Key)
		if err != nil {
			// Tidak fatal — mis. logical key belum dipetakan untuk vendor ini.
			continue
		}
		want := op.Value
		if isPeriodicInformIntervalKey(op.Key) || isPeriodicInformIntervalKey(path) {
			want = jitterPeriodicInformInterval(dev.ID, want)
		}
		cur, err := s.deviceParams.Get(ctx, dev.ID, path)
		if err != nil {
			if errors.Is(err, domain.ErrNotFound) {
				out[path] = want // parameter belum pernah terlihat -> push
				continue
			}
			return nil, err
		}
		got := ""
		if cur.ParameterValue != nil {
			got = *cur.ParameterValue
		}
		if got != want {
			out[path] = want
		}
	}
	return out, nil
}

func (s *Service) bumpPresetFailure(ctx context.Context, presetID, deviceID uint64) {
	app, err := s.presetApps.Get(ctx, presetID, deviceID)
	fails := 1
	var lastApplied *time.Time
	if err == nil && app != nil {
		fails = app.ConsecutiveFailures + 1
		lastApplied = app.LastAppliedAt
	}
	status := domain.PresetApplicationStatusPending
	if fails >= presetMaxConsecutiveFailures {
		status = domain.PresetApplicationStatusFailed
	}
	_ = s.presetApps.Upsert(ctx, &domain.PresetApplication{
		PresetID: presetID, DeviceID: deviceID,
		LastAppliedAt:       lastApplied,
		ConsecutiveFailures: fails,
		Status:              status,
	})
}

func (s *Service) recordPresetConfigIssue(ctx context.Context, dev *domain.Device, p *domain.Preset, msg string) {
	desc := fmt.Sprintf("preset_id=%d: %s", p.ID, msg)
	_ = s.activity.Record(ctx, &domain.ActivityLog{
		TenantID: dev.TenantID, Action: "PRESET_CONFIG_INVALID",
		EntityType: "device", EntityID: &dev.ID, Description: &desc,
	})
}

func presetLastApplied(app *domain.PresetApplication) *time.Time {
	if app == nil {
		return nil
	}
	return app.LastAppliedAt
}

// hashDriftSet — sha256 hex deterministik dari drift-set (key diurutkan).
func hashDriftSet(drift map[string]string) string {
	keys := make([]string, 0, len(drift))
	for k := range drift {
		keys = append(keys, k)
	}
	sort.Strings(keys)
	h := sha256.New()
	for _, k := range keys {
		h.Write([]byte(k))
		h.Write([]byte{'='})
		h.Write([]byte(drift[k]))
		h.Write([]byte{'\n'})
	}
	return hex.EncodeToString(h.Sum(nil))
}

func joinUint64s(ids []uint64) string {
	parts := make([]string, len(ids))
	for i, id := range ids {
		parts[i] = fmt.Sprintf("%d", id)
	}
	return strings.Join(parts, ",")
}
