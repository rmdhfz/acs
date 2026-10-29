package selfservice

import (
	"context"
	"encoding/json"
	"errors"
	"strings"
	"testing"
	"time"

	"acs/internal/domain"
)

type fakeDiagnoser struct {
	d     *domain.DeviceDiagnosis
	err   error
	calls int
	got   *domain.Device
}

func (f *fakeDiagnoser) DiagnoseDevice(_ context.Context, dev *domain.Device) (*domain.DeviceDiagnosis, error) {
	f.calls++
	f.got = dev
	return f.d, f.err
}

// staffDiagnosis = diagnosis staf yang SETIAP field teksnya memuat sentinel rahasia; tak satu pun boleh sampai ke pelanggan.
func staffDiagnosis(verdict string, codes ...string) *domain.DeviceDiagnosis {
	const secret = "RAHASIA"
	rx, idx, vs := -31.25, "ONU-IDX-RAHASIA", "vendor-state-RAHASIA"
	d := &domain.DeviceDiagnosis{
		DeviceID: 1, Status: "OFFLINE", Verdict: verdict, Summary: "Dugaan utama RAHASIA " + secret,
		InformIntervalParameter: "InternetGatewayDevice.ManagementServer.PeriodicInformInterval-RAHASIA",
		OfflineThresholdSeconds: 987654,
		OLT:                     &domain.DiagnosisOLT{OLTName: "OLT-RAHASIA", PonPort: "PON-RAHASIA", ONUIndex: &idx, State: "LOS", VendorState: &vs, RxPowerDBM: &rx},
		Limits:                  "limits-RAHASIA", Actions: []string{"CONNECTION_REQUEST"},
	}
	for _, c := range codes {
		d.Findings = append(d.Findings, domain.DiagnosisFinding{Code: c, Severity: domain.SeverityCritical,
			Title: "judul-RAHASIA", Evidence: "bukti RX -31.25 dBm di OLT-RAHASIA", Hint: "saran-RAHASIA"})
	}
	return d
}

func svcFor(d *fakeDiagnoser) *Service {
	return &Service{
		userDevices: fakeMap{mapped: map[[2]uint64]bool{{10, 1}: true}},
		devices:     fakeDevs{byID: map[uint64]*domain.Device{1: {ID: 1, TenantID: tid(7), SerialNumber: "SN-RAHASIA"}, 2: {ID: 2, TenantID: tid(7)}}},
		diagnoser:   d,
	}
}

var cust = domain.Actor{UserID: 10, TenantID: tid(7), Roles: []string{domain.RoleEndUser}}

func TestDiagnoseMyDevice_TidakMembocorkanDataInternal(t *testing.T) {
	cases := []struct {
		name    string
		verdict string
		codes   []string
		cause   string
		online  bool
	}{
		{"terhubung", domain.VerdictReachable, []string{"WITHIN_TOLERANCE", "OPTICAL_BAD"}, CustomerCauseConnected, true},
		{"belum pernah", domain.VerdictNeverConnected, []string{"NEVER_INFORMED"}, CustomerCauseNew, false},
		{"dying gasp", domain.VerdictOffline, []string{"OLT_DYING_GASP"}, CustomerCausePower, false},
		{"reboot sebelum diam", domain.VerdictOffline, []string{"REBOOT_BEFORE_SILENCE"}, CustomerCausePower, false},
		{"LOS", domain.VerdictOffline, []string{"OLT_LOS"}, CustomerCauseFiber, false},
		{"optik buruk", domain.VerdictOffline, []string{"OPTICAL_BAD"}, CustomerCauseFiber, false},
		{"tanpa temuan", domain.VerdictOffline, nil, CustomerCauseSilent, false},
		{"temuan tak dikenal", domain.VerdictOffline, []string{"KODE_BARU_DI_MASA_DEPAN"}, CustomerCauseSilent, false},
	}
	for _, c := range cases {
		fd := &fakeDiagnoser{d: staffDiagnosis(c.verdict, c.codes...)}
		out, err := svcFor(fd).DiagnoseMyDevice(context.Background(), cust, 1)
		if err != nil {
			t.Fatalf("%s: %v", c.name, err)
		}
		if out.Cause != c.cause || out.Online != c.online {
			t.Errorf("%s: cause=%q online=%v, mau %q/%v", c.name, out.Cause, out.Online, c.cause, c.online)
		}
		raw, _ := json.Marshal(out)
		for _, leak := range []string{"RAHASIA", "31.25", "OLT", "PON", "987654", "SN-", "vendor-state", "PeriodicInform", "ManagementServer", "dBm", "ONU"} {
			if strings.Contains(string(raw), leak) {
				t.Errorf("%s: JSON pelanggan membocorkan %q: %s", c.name, leak, raw)
			}
		}
		if out.Headline == "" || len(out.Steps) == 0 || out.Disclaimer == "" || out.GeneratedAt.IsZero() || time.Since(out.GeneratedAt) > time.Minute {
			t.Errorf("%s: isi tak lengkap: %+v", c.name, out)
		}
		// bentuk respons tertutup: hanya field yang disepakati
		var m map[string]any
		_ = json.Unmarshal(raw, &m)
		if len(m) != 7 {
			t.Errorf("%s: field respons = %d, mau tepat 7 (online,cause,headline,steps,can_reboot,disclaimer,generated_at): %v", c.name, len(m), m)
		}
	}
}

func TestDiagnoseMyDevice_PrioritasDayaDiatasSerat(t *testing.T) {
	// dying gasp + LOS bersamaan: dying gasp (ONU kehilangan daya) menjelaskan LOS, jadi saran listrik didahulukan
	fd := &fakeDiagnoser{d: staffDiagnosis(domain.VerdictOffline, "OLT_LOS", "OLT_DYING_GASP")}
	out, _ := svcFor(fd).DiagnoseMyDevice(context.Background(), cust, 1)
	if out.Cause != CustomerCausePower {
		t.Errorf("cause = %q", out.Cause)
	}
}

func TestDiagnoseMyDevice_KepemilikanDitegakkanSebelumDiagnosis(t *testing.T) {
	fd := &fakeDiagnoser{d: staffDiagnosis(domain.VerdictReachable)}
	svc := svcFor(fd)
	ctx := context.Background()
	if _, err := svc.DiagnoseMyDevice(ctx, cust, 2); !errors.Is(err, domain.ErrNotFound) { // ada, tetapi bukan milik akun
		t.Errorf("perangkat orang lain: %v", err)
	}
	if _, err := svc.DiagnoseMyDevice(ctx, cust, 99); !errors.Is(err, domain.ErrNotFound) {
		t.Errorf("perangkat tak ada: %v", err)
	}
	if _, err := svc.DiagnoseMyDevice(ctx, domain.Actor{TenantID: tid(7)}, 1); !errors.Is(err, domain.ErrForbidden) {
		t.Errorf("tanpa user: %v", err)
	}
	if _, err := svc.DiagnoseMyDevice(ctx, domain.Actor{UserID: 10, TenantID: tid(8)}, 1); !errors.Is(err, domain.ErrNotFound) {
		t.Errorf("tenant berbeda: %v", err)
	}
	if fd.calls != 0 {
		t.Errorf("diagnosis tak boleh dijalankan untuk perangkat yang bukan milik akun (%d panggilan)", fd.calls)
	}
	if _, err := svc.DiagnoseMyDevice(ctx, cust, 1); err != nil || fd.calls != 1 || fd.got.ID != 1 {
		t.Errorf("milik sendiri: err=%v calls=%d", err, fd.calls)
	}
}

func TestDiagnoseMyDevice_GalatDanTanpaDiagnoser(t *testing.T) {
	boom := errors.New("db mati")
	if _, err := svcFor(&fakeDiagnoser{err: boom}).DiagnoseMyDevice(context.Background(), cust, 1); !errors.Is(err, boom) {
		t.Errorf("galat harus diteruskan: %v", err)
	}
	s := svcFor(nil)
	s.diagnoser = nil
	if _, err := s.DiagnoseMyDevice(context.Background(), cust, 1); !errors.Is(err, domain.ErrNotFound) {
		t.Errorf("tanpa diagnoser: %v", err)
	}
}
