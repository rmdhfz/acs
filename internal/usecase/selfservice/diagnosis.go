package selfservice

import (
	"context"
	"time"

	"acs/internal/domain"
)

// ---- "Internet saya bermasalah?" — diagnosis versi pelanggan ----
//
// Diagnosis staf (usecase/troubleshoot) memuat nilai internal: redaman optik, nama OLT, port PON, serial/ONU, ambang, riwayat task. Pelanggan TIDAK
// boleh menerimanya. Karena itu hasil untuk pelanggan TIDAK diturunkan dengan menyaring field diagnosis staf, melainkan dibangun dari NOL dari
// daftar putih: kategori tertutup (CustomerCause*) -> teks tetap yang ditulis di sini. Tak ada string dari diagnosis staf yang diteruskan.
// Pelanggan tidak punya cara menyentuh perangkat lain (cek kepemilikan sama dengan endpoint self-service lainnya).

// Kategori penyebab (daftar tertutup; kode ini bagian dari kontrak API).
const (
	CustomerCauseConnected = "CONNECTED"         // perangkat terhubung ke jaringan penyedia
	CustomerCauseNew       = "NOT_YET_CONNECTED" // perangkat belum pernah terhubung
	CustomerCausePower     = "POWER"             // dugaan listrik/adaptor
	CustomerCauseFiber     = "FIBER"             // dugaan jalur serat optik
	CustomerCauseSilent    = "NO_RECENT_CONTACT" // tak melapor, penyebab belum dapat dipastikan
)

// CustomerDiagnosis = hasil yang aman untuk pelanggan.
type CustomerDiagnosis struct {
	Online      bool      `json:"online"`
	Cause       string    `json:"cause"`    // salah satu CustomerCause*
	Headline    string    `json:"headline"` // satu kalimat ringkas
	Steps       []string  `json:"steps"`    // langkah yang dapat dicoba pelanggan sendiri
	CanReboot   bool      `json:"can_reboot"`
	Disclaimer  string    `json:"disclaimer"`
	GeneratedAt time.Time `json:"generated_at"`
}

const customerDisclaimer = "Ini perkiraan otomatis dari data yang diterima sistem, bukan kepastian. Jika masalah berlanjut, hubungi penyedia internet Anda."

// Diagnoser dipenuhi usecase/troubleshoot.Service.DiagnoseDevice (perangkat sudah diautorisasi oleh pemanggil).
type Diagnoser interface {
	DiagnoseDevice(ctx context.Context, dev *domain.Device) (*domain.DeviceDiagnosis, error)
}

// WithDiagnoser mengaktifkan DiagnoseMyDevice.
func (s *Service) WithDiagnoser(d Diagnoser) *Service {
	s.diagnoser = d
	return s
}

// DiagnoseMyDevice menjawab "kenapa internet saya bermasalah?" untuk perangkat milik pelanggan. Hanya baca.
func (s *Service) DiagnoseMyDevice(ctx context.Context, actor domain.Actor, deviceID uint64) (*CustomerDiagnosis, error) {
	dev, err := s.requireOwnership(ctx, actor, deviceID)
	if err != nil {
		return nil, err
	}
	if s.diagnoser == nil {
		return nil, domain.ErrNotFound
	}
	d, err := s.diagnoser.DiagnoseDevice(ctx, dev)
	if err != nil {
		return nil, err
	}
	out := customerView(d)
	out.GeneratedAt = time.Now().UTC()
	return &out, nil
}

// customerView memetakan diagnosis staf ke tampilan pelanggan. Hanya VERDICT dan KODE temuan yang dibaca (bukan teksnya).
func customerView(d *domain.DeviceDiagnosis) CustomerDiagnosis {
	has := map[string]bool{}
	for _, f := range d.Findings {
		has[f.Code] = true
	}
	out := CustomerDiagnosis{Disclaimer: customerDisclaimer}
	switch {
	case d.Verdict == domain.VerdictReachable:
		out.Online, out.Cause = true, CustomerCauseConnected
		out.Headline = "Perangkat Anda terhubung ke jaringan penyedia."
		out.Steps = []string{
			"Jika internet terasa lambat atau putus-putus, coba matikan lalu nyalakan kembali perangkat WiFi/router di rumah Anda.",
			"Coba dari perangkat lain (ponsel/laptop) untuk memastikan masalahnya bukan pada satu perangkat saja.",
			"Bila masih bermasalah, hubungi penyedia internet Anda.",
		}
		out.CanReboot = true
	case d.Verdict == domain.VerdictNeverConnected:
		out.Cause = CustomerCauseNew
		out.Headline = "Perangkat Anda belum pernah terhubung ke jaringan penyedia."
		out.Steps = []string{
			"Pastikan perangkat menyala dan kabel serat optik terpasang.",
			"Bila baru dipasang, hubungi penyedia internet Anda untuk aktivasi.",
		}
	case has["OLT_DYING_GASP"] || has["REBOOT_BEFORE_SILENCE"]:
		out.Cause = CustomerCausePower
		out.Headline = "Perangkat Anda tampaknya kehilangan daya listrik."
		out.Steps = []string{
			"Pastikan adaptor perangkat tertancap kuat dan stopkontak berlistrik.",
			"Coba cabut adaptor, tunggu 10 detik, lalu pasang kembali dan tunggu beberapa menit.",
			"Bila listrik di rumah menyala normal tetapi perangkat tetap mati, hubungi penyedia internet Anda.",
		}
	case has["OLT_LOS"] || has["OLT_ONU_OFFLINE"] || has["OPTICAL_BAD"] || has["OPTICAL_WEAK"]:
		out.Cause = CustomerCauseFiber
		out.Headline = "Sinyal serat optik ke perangkat Anda tampaknya terganggu."
		out.Steps = []string{
			"Periksa kabel serat optik (kabel tipis dari dinding ke perangkat): pastikan tidak tertekuk tajam, terjepit, atau terlepas.",
			"Jangan menarik atau membengkokkan kabel serat dengan paksa.",
			"Bila lampu LOS pada perangkat menyala/berkedip merah, atau kabel tampak rusak, hubungi penyedia internet Anda.",
		}
	default:
		out.Cause = CustomerCauseSilent
		out.Headline = "Perangkat Anda sedang tidak melapor ke jaringan penyedia."
		out.Steps = []string{
			"Pastikan perangkat menyala dan adaptornya terpasang.",
			"Tunggu beberapa menit, lalu coba lagi.",
			"Bila tetap tidak terhubung, hubungi penyedia internet Anda.",
		}
	}
	if out.Steps == nil {
		out.Steps = []string{}
	}
	return out
}
