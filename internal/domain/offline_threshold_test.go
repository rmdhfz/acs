package domain

import (
	"testing"
	"time"
)

func TestOfflineThreshold(t *testing.T) {
	u := func(v uint32) *uint32 { return &v }
	min := 15 * time.Minute
	cases := []struct {
		name     string
		interval *uint32
		want     time.Duration
	}{
		{"interval belum diketahui = batas minimum", nil, min},
		{"interval nol = batas minimum", u(0), min},
		{"interval pendek tidak menurunkan di bawah minimum (60 dtk -> 150 dtk < 15 mnt)", u(60), min},
		{"interval 5 menit -> 12,5 menit, tetap minimum", u(300), min},
		{"interval 10 menit -> 25 menit", u(600), 25 * time.Minute},
		{"interval 12 jam (bawaan Telkom) -> 30 jam", u(43200), 30 * time.Hour},
		{"pembulatan ke atas (1 dtk x 2,5 -> 3 dtk) tetap di bawah minimum", u(1), min},
		{"interval 1001 dtk -> ceil(2502,5) = 2503 dtk", u(1001), 2503 * time.Second},
		{"dibatasi 7 hari", u(4294967295), MaxOfflineThreshold},
		{"3 hari x 2,5 = 7,5 hari -> dibatasi 7 hari", u(3 * 24 * 3600), MaxOfflineThreshold},
	}
	for _, c := range cases {
		t.Run(c.name, func(t *testing.T) {
			if got := OfflineThreshold(c.interval, min); got != c.want {
				t.Errorf("OfflineThreshold = %v, mau %v", got, c.want)
			}
		})
	}
}

// Batas minimum lebih besar dari batas maksimum: SQL (GREATEST(min, LEAST(max, x))) memilih minimum — fungsi ini harus sama.
func TestOfflineThreshold_MinimumMenangAtasMaksimum(t *testing.T) {
	min := 10 * 24 * time.Hour
	if got := OfflineThreshold(nil, min); got != min {
		t.Errorf("got %v, mau %v", got, min)
	}
	iv := uint32(4294967295)
	if got := OfflineThreshold(&iv, min); got != min {
		t.Errorf("got %v, mau %v", got, min)
	}
}
