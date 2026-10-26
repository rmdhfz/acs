package netguard

import (
	"fmt"
	"net"
	"net/url"
	"strings"
)

// HostAllowlist membatasi host yang boleh dituju URL transfer berkas (task Download/Upload TR-069). Berbeda dari
// penjaga SSRF di atas (yang melindungi ACS dari alamat internal), ini melindungi PERANGKAT: tanpa batas, siapa pun yang
// memegang token ADMIN tenant dapat mendorong firmware/konfigurasi dari host mana pun, atau menyuruh CPE mengunggah
// konfigurasi/log ke host mana pun.
//
// Anggota daftar (dipisah koma): nama host persis ("files.example.com"), subdomain ("*.example.com" — TIDAK mencakup
// example.com sendiri), IP literal ("10.0.0.5"), atau CIDR untuk host berupa IP literal ("10.0.0.0/24"). Port diabaikan.
// Pointer nil = daftar tidak aktif (semua host diizinkan), perilaku sebelum fitur ini ada.
type HostAllowlist struct {
	hosts    map[string]struct{}
	suffixes []string // ".example.com"
	ips      map[string]struct{}
	nets     []*net.IPNet
}

// ParseHostAllowlist mem-parse daftar dipisah koma. Daftar kosong menghasilkan (nil, nil) = tidak aktif. Entri yang
// tidak valid menggagalkan parsing (gagal keras saat start, bukan diam-diam membuka atau menutup akses).
func ParseHostAllowlist(csv string) (*HostAllowlist, error) {
	a := &HostAllowlist{hosts: map[string]struct{}{}, ips: map[string]struct{}{}}
	n := 0
	for _, raw := range strings.Split(csv, ",") {
		e := strings.TrimSpace(raw)
		if e == "" {
			continue
		}
		n++
		switch {
		case strings.Contains(e, "/") && !strings.Contains(e, "://"):
			_, cidr, err := net.ParseCIDR(e)
			if err != nil {
				return nil, fmt.Errorf("%q bukan CIDR yang valid", e)
			}
			a.nets = append(a.nets, cidr)
		case strings.HasPrefix(e, "*."):
			base := strings.ToLower(strings.TrimSuffix(e[2:], "."))
			if err := checkHostname(base); err != nil {
				return nil, fmt.Errorf("%q: %v", e, err)
			}
			a.suffixes = append(a.suffixes, "."+base)
		default:
			if ip := net.ParseIP(e); ip != nil {
				a.ips[ip.String()] = struct{}{}
				continue
			}
			h := strings.ToLower(strings.TrimSuffix(e, "."))
			if err := checkHostname(h); err != nil {
				return nil, fmt.Errorf("%q: %v", e, err)
			}
			a.hosts[h] = struct{}{}
		}
	}
	if n == 0 {
		return nil, nil
	}
	return a, nil
}

func checkHostname(h string) error {
	if h == "" || strings.ContainsAny(h, "*:/@ \t?#") || strings.HasPrefix(h, ".") {
		return fmt.Errorf("nama host tidak valid (tanpa skema, port, jalur, atau wildcard selain awalan *.)")
	}
	return nil
}

// Allows melaporkan apakah host pada rawURL diizinkan. nil = tidak aktif = true. URL yang tidak dapat di-parse atau tanpa
// host ditolak.
func (a *HostAllowlist) Allows(rawURL string) bool {
	if a == nil {
		return true
	}
	u, err := url.Parse(rawURL)
	if err != nil || u.Hostname() == "" {
		return false
	}
	host := strings.ToLower(strings.TrimSuffix(u.Hostname(), "."))
	if ip := net.ParseIP(host); ip != nil {
		if _, ok := a.ips[ip.String()]; ok {
			return true
		}
		for _, n := range a.nets {
			if n.Contains(ip) {
				return true
			}
		}
		return false
	}
	// Bukan IP literal (termasuk bentuk desimal/oktal ala "2130706433": bukan nama host terdaftar → ditolak di bawah).
	if _, ok := a.hosts[host]; ok {
		return true
	}
	for _, s := range a.suffixes {
		if len(host) > len(s) && strings.HasSuffix(host, s) {
			return true
		}
	}
	return false
}
