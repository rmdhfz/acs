package automation

import (
	"context"
	"fmt"
	"strings"

	"acs/internal/domain"
)

// ---- Wildcard instance pada set_parameter (lihat domain/param_pattern.go) ----
//
// Nama target boleh memuat segmen '*' (satu instance tabel TR-069): aksi dijalankan untuk SETIAP instance yang pernah dilaporkan perangkat,
// dengan nilai yang dihitung per instance. Rujukan {…} pada ekspresi nilai yang memuat '*' terikat ke instance YANG SAMA dengan nama target
// (urutan '*' ke-k pada rujukan = '*' ke-k pada nama), sehingga "SSID instance 2 = nilai milik instance 2" tidak tercampur dengan instance lain.
// Tidak ada perulangan bebas: ekspansi tertutup (maks AutomationMaxWildcardInstances instance), kondisi tidak mendukung wildcard, dan tidak
// ada panggilan ke perangkat untuk menemukan instance (hanya parameter yang sudah dilaporkan perangkat).

// wildcardCandidateLimit membatasi baris yang dibaca untuk mencari instance (LIKE yang kasar bisa melahap subpohon besar).
const wildcardCandidateLimit = 2000

// rewriteRefs menerapkan f pada isi tiap rujukan {…} di LUAR string berkutip; bagian lain ekspresi tidak diubah. Aturan kutip/escape sama
// dengan pemindai pkg/vexpr (backslash melompati karakter berikutnya).
func rewriteRefs(src string, f func(ref string) string) string {
	rs := []rune(src)
	var sb strings.Builder
	for i := 0; i < len(rs); {
		switch r := rs[i]; {
		case r == '\'' || r == '"':
			j := i + 1
			for j < len(rs) && rs[j] != r {
				if rs[j] == '\\' && j+1 < len(rs) {
					j++
				}
				j++
			}
			if j < len(rs) {
				j++ // kutip penutup
			}
			sb.WriteString(string(rs[i:j]))
			i = j
		case r == '{':
			j := i + 1
			for j < len(rs) && rs[j] != '}' {
				j++
			}
			if j >= len(rs) { // tak ditutup: biarkan pengurai yang melaporkan
				sb.WriteString(string(rs[i:]))
				i = len(rs)
				continue
			}
			sb.WriteString("{" + f(strings.TrimSpace(string(rs[i+1:j]))) + "}")
			i = j + 1
		default:
			sb.WriteRune(r)
			i++
		}
	}
	return sb.String()
}

// wildcardRefs mengumpulkan rujukan parameter yang memuat '*' pada src.
func wildcardRefs(src string) []string {
	var out []string
	rewriteRefs(src, func(ref string) string {
		if !strings.HasPrefix(ref, "@") && domain.HasParamWildcard(ref) {
			out = append(out, ref)
		}
		return ref
	})
	return out
}

// bindWildcards mengganti '*' pada rujukan dengan nomor instance caps (urutan sama dengan '*' pada nama target).
func bindWildcards(src string, caps []string) string {
	return rewriteRefs(src, func(ref string) string {
		if strings.HasPrefix(ref, "@") {
			return ref
		}
		return domain.SubstituteParamWildcards(ref, caps)
	})
}

// neutralizeWildcards = bindWildcards dengan instance contoh "1": dipakai saat validasi supaya ekspresi berwildcard dapat diurai dan
// rujukannya diperiksa (rahasia, dsb.) seperti ekspresi biasa.
func neutralizeWildcards(src string) string {
	return bindWildcards(src, []string{"1", "1"})
}

// checkWildcardRefs: rujukan berwildcard pada nilai aksi harus sah dan punya jumlah '*' yang sama dengan nama target (k).
func checkWildcardRefs(at, value string, k int) error {
	for _, ref := range wildcardRefs(value) {
		switch {
		case k == 0:
			return fmt.Errorf("%w: %s: rujukan {%s} memakai '*' tetapi nama target tidak berwildcard", domain.ErrInvalidInput, at, ref)
		case !domain.ValidParamPattern(ref) || domain.ParamWildcardCount(ref) != k:
			return fmt.Errorf("%w: %s: rujukan {%s} harus raw path dengan tepat %d segmen '*' (sama dengan nama target)", domain.ErrInvalidInput, at, ref, k)
		}
	}
	return nil
}

func hasWildcardAction(actions []domain.AutomationAction) bool {
	for _, a := range actions {
		if a.Type == domain.AutomationActionSetParameter && domain.HasParamWildcard(a.Name) {
			return true
		}
	}
	return false
}

// expandWildcards mengubah aksi set_parameter berwildcard menjadi aksi konkret per instance (nama konkret + nilai terikat ke instance itu);
// aksi lain apa adanya. notes = langkah yang DILEWATI beserta alasannya (tak ada instance, terlalu banyak, tak didukung) agar terlihat di
// uji/riwayat, bukan hilang diam-diam. Tanpa aksi berwildcard tidak ada query tambahan.
func (s *Service) expandWildcards(ctx context.Context, dev *domain.Device, actions []domain.AutomationAction) ([]domain.AutomationAction, []step, error) {
	if !hasWildcardAction(actions) {
		return actions, nil, nil
	}
	lister, _ := s.params.(domain.DeviceParameterPathLister)
	out := make([]domain.AutomationAction, 0, len(actions))
	var notes []step
	for _, a := range actions {
		if a.Type != domain.AutomationActionSetParameter || !domain.HasParamWildcard(a.Name) {
			out = append(out, a)
			continue
		}
		skip := func(reason string) { notes = append(notes, step{Action: a, Skip: reason}) }
		if lister == nil {
			skip("penyimpanan parameter tidak mendukung pencarian instance")
			continue
		}
		cands, err := lister.CandidatePaths(ctx, dev.ID, a.Name, wildcardCandidateLimit)
		if err != nil {
			return nil, nil, err
		}
		if len(cands) >= wildcardCandidateLimit {
			skip(fmt.Sprintf("terlalu banyak parameter kandidat (>= %d) untuk pola ini: aksi dilewati", wildcardCandidateLimit))
			continue
		}
		ms := domain.FilterParamPattern(a.Name, cands)
		switch {
		case len(ms) == 0:
			skip("belum ada instance yang cocok pada perangkat ini (hanya parameter yang pernah dilaporkan perangkat yang dikenali)")
		case len(ms) > domain.AutomationMaxWildcardInstances:
			skip(fmt.Sprintf("%d instance cocok, melebihi batas %d: aksi dilewati seluruhnya (tidak diterapkan sebagian)", len(ms), domain.AutomationMaxWildcardInstances))
		default:
			for _, m := range ms {
				c := a
				c.Name = m.Path
				c.Value = bindWildcards(a.Value, m.Caps)
				out = append(out, c)
			}
		}
	}
	return out, notes, nil
}
