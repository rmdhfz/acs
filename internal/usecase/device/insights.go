package device

import (
	"context"
	"strconv"
	"strings"
	"time"

	"acs/internal/domain"
	"acs/internal/usecase/auth"
)

// WithFleetInsights memasang pembaca baris armada untuk dashboard (opsional; tanpa ini Insights mengembalikan ErrNotFound).
func (s *Service) WithFleetInsights(r domain.FleetInsightsRepository) *Service {
	s.fleet = r
	return s
}

// Insights — distribusi armada untuk dashboard, dibatasi tenant actor (SUPERADMIN: seluruh platform).
func (s *Service) Insights(ctx context.Context, actor domain.Actor) (*domain.FleetInsights, error) {
	if s.fleet == nil {
		return nil, domain.ErrNotFound
	}
	tenantID, err := auth.ScopedTenantFilter(actor)
	if err != nil {
		return nil, err
	}
	rows, err := s.fleet.FleetRows(ctx, tenantID)
	if err != nil {
		return nil, err
	}
	return BuildFleetInsights(rows, time.Now()), nil
}

// BuildFleetInsights mengelompokkan baris mentah ke keranjang distribusi. Fungsi murni (mudah diuji).
//   - Redaman RX, suhu, uptime, dan mode PON: perangkat ONLINE saja (perangkat offline membawa nilai basi).
//   - Lama offline: perangkat OFFLINE, dari Inform terakhir; tanpa Inform tercatat = unknown.
//   - RX dibaca lewat domain.NormalizeRXDBM (menebak satuan mentah); yang tak masuk akal = unknown.
func BuildFleetInsights(rows []domain.FleetRow, now time.Time) *domain.FleetInsights {
	out := &domain.FleetInsights{
		PonMode:     buckets(domain.PonGPON, domain.PonEPON, domain.InsightUnknown),
		Uptime:      buckets(domain.UptimeLT1D, domain.Uptime1to7D, domain.Uptime7to30D, domain.UptimeGT30D, domain.InsightUnknown),
		OpticalRX:   buckets(domain.OpticalOK, domain.OpticalWarn, domain.OpticalBad, domain.OpticalOverload, domain.InsightUnknown),
		Temperature: buckets(domain.TempNormal, domain.TempWarning, domain.TempHigh, domain.TempCritical, domain.InsightUnknown),
		OfflineAge:  buckets(domain.OfflineLT1D, domain.Offline1to3D, domain.Offline3to7D, domain.Offline7to30D, domain.OfflineGT30D, domain.InsightUnknown),
	}
	add := func(b []domain.BucketCount, key string) {
		for i := range b {
			if b[i].Key == key {
				b[i].Count++
				return
			}
		}
	}
	for _, r := range rows {
		switch r.Status {
		case domain.DeviceStatusOnline:
			out.Online++
			switch {
			case r.Gpon:
				add(out.PonMode, domain.PonGPON)
			case r.Epon:
				add(out.PonMode, domain.PonEPON)
			default:
				add(out.PonMode, domain.InsightUnknown)
			}
			add(out.Uptime, uptimeBucket(r.UptimeRaw))
			add(out.OpticalRX, rxBucket(r.RxDBM))
			add(out.Temperature, tempBucket(r.TempC))
		case domain.DeviceStatusOffline:
			out.Offline++
			add(out.OfflineAge, offlineBucket(r.LastInformAt, now))
		default:
			out.Other++
		}
	}
	return out
}

func buckets(keys ...string) []domain.BucketCount {
	b := make([]domain.BucketCount, len(keys))
	for i, k := range keys {
		b[i].Key = k
	}
	return b
}

func uptimeBucket(raw *string) string {
	if raw == nil {
		return domain.InsightUnknown
	}
	secs, err := strconv.ParseInt(strings.TrimSpace(*raw), 10, 64)
	if err != nil || secs < 0 {
		return domain.InsightUnknown
	}
	switch d := time.Duration(secs) * time.Second; {
	case d < 24*time.Hour:
		return domain.UptimeLT1D
	case d < 7*24*time.Hour:
		return domain.Uptime1to7D
	case d < 30*24*time.Hour:
		return domain.Uptime7to30D
	default:
		return domain.UptimeGT30D
	}
}

func rxBucket(rx *float64) string {
	if rx == nil {
		return domain.InsightUnknown
	}
	dbm, ok := domain.NormalizeRXDBM(*rx)
	if !ok {
		return domain.InsightUnknown
	}
	if dbm > domain.OpticalRXOverloadAboveDBM {
		return domain.OpticalOverload
	}
	return domain.OpticalLevelOf(&dbm) // ok / warn / bad (lemah)
}

func tempBucket(t *float64) string {
	switch {
	case t == nil:
		return domain.InsightUnknown
	case *t < -40 || *t > 150: // di luar rentang fisik: satuan/parsing salah, jangan dinilai
		return domain.InsightUnknown
	case *t >= domain.TempCriticalAtC:
		return domain.TempCritical
	case *t >= domain.TempHighAtC:
		return domain.TempHigh
	case *t >= domain.TempWarnAtC:
		return domain.TempWarning
	default:
		return domain.TempNormal
	}
}

func offlineBucket(last *time.Time, now time.Time) string {
	if last == nil {
		return domain.InsightUnknown
	}
	switch d := now.Sub(*last); {
	case d < 24*time.Hour:
		return domain.OfflineLT1D
	case d < 3*24*time.Hour:
		return domain.Offline1to3D
	case d < 7*24*time.Hour:
		return domain.Offline3to7D
	case d < 30*24*time.Hour:
		return domain.Offline7to30D
	default:
		return domain.OfflineGT30D
	}
}
