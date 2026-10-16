// Package netguard menolak target permintaan keluar (outbound HTTP/UDP) yang
// menunjuk alamat internal — pertahanan SSRF untuk jalur di mana URL/host
// berasal dari input yang tidak sepenuhnya tepercaya (Connection Request URL
// yang dilaporkan CPE, target_url webhook yang diisi admin tenant, dsb).
//
// Dua lapis pertahanan:
//  1. Validasi (CheckURL/CheckHostPort): menolak input yang jelas menunjuk alamat terlarang, agar
//     pengguna mendapat pesan galat yang jelas saat menyimpan.
//  2. Penjaga saat SAMBUNGAN (SafeDialer/SafeTransport): memeriksa alamat IP yang BENAR-BENAR
//     dihubungi, setelah DNS di-resolve. Tanpa lapis ini validasi mudah dilewati lewat DNS
//     rebinding (host me-resolve ke IP publik saat validasi, lalu ke 127.0.0.1/metadata saat
//     menyambung) dan lewat redirect.
//
// Kebijakan dasar: tolak loopback, link-local (termasuk 169.254.169.254 metadata cloud), multicast,
// unspecified, 0.0.0.0/8, 240.0.0.0/4 dan metadata IPv6 AWS. IP privat (RFC1918) DIIZINKAN secara
// bawaan karena deployment enterprise sah menaruh ACS + CPE / ACS + receiver webhook di jaringan
// privat yang sama. Untuk SaaS publik (target webhook pasti publik) aktifkan blockPrivate: menolak
// juga RFC1918, ULA (fc00::/7), CGNAT (100.64/10) dan rentang khusus lain.
package netguard

import (
	"context"
	"fmt"
	"net"
	"net/http"
	"net/url"
	"strings"
	"syscall"
	"time"
)

var (
	// rentang yang SELALU ditolak (selain yang sudah ditangani method net.IP).
	alwaysBlocked = mustCIDRs("0.0.0.0/8", "240.0.0.0/4", "fd00:ec2::/32")
	// rentang tambahan bila blockPrivate: CGNAT, benchmarking, protokol IETF.
	privateExtra = mustCIDRs("100.64.0.0/10", "198.18.0.0/15", "192.0.0.0/24")
)

func mustCIDRs(cidrs ...string) []*net.IPNet {
	out := make([]*net.IPNet, 0, len(cidrs))
	for _, c := range cidrs {
		_, n, err := net.ParseCIDR(c)
		if err != nil {
			panic(err)
		}
		out = append(out, n)
	}
	return out
}

// CheckURL memvalidasi sebuah URL absolut: skema harus http/https, dan setiap
// IP yang di-resolve dari host-nya tidak boleh internal/link-local.
func CheckURL(raw string) error { return CheckURLPolicy(raw, false) }

// CheckURLPolicy = CheckURL dengan pilihan memblokir juga jaringan privat.
func CheckURLPolicy(raw string, blockPrivate bool) error {
	u, err := url.Parse(strings.TrimSpace(raw))
	if err != nil || u.Host == "" {
		return fmt.Errorf("netguard: URL tidak valid")
	}
	if u.Scheme != "http" && u.Scheme != "https" {
		return fmt.Errorf("netguard: skema %q tidak diizinkan (hanya http/https)", u.Scheme)
	}
	return CheckHostPortPolicy(u.Host, blockPrivate)
}

// CheckHostPort menerima "host" atau "host:port" (IPv6 literal boleh dalam
// tanda kurung). Melakukan DNS lookup bila host bukan literal IP.
func CheckHostPort(hostPort string) error { return CheckHostPortPolicy(hostPort, false) }

// CheckHostPortPolicy = CheckHostPort dengan pilihan memblokir juga jaringan privat.
func CheckHostPortPolicy(hostPort string, blockPrivate bool) error {
	host := hostPort
	if h, _, err := net.SplitHostPort(hostPort); err == nil {
		host = h
	}
	host = strings.Trim(host, "[]")
	if host == "" {
		return fmt.Errorf("netguard: host kosong")
	}

	var ips []net.IP
	if ip := net.ParseIP(host); ip != nil {
		ips = []net.IP{ip}
	} else {
		resolved, err := net.LookupIP(host)
		if err != nil || len(resolved) == 0 {
			return fmt.Errorf("netguard: host %q tidak dapat di-resolve", host)
		}
		ips = resolved
	}
	for _, ip := range ips {
		if isBlocked(ip, blockPrivate) {
			return fmt.Errorf("netguard: alamat %s tidak diizinkan (internal/link-local)", ip)
		}
	}
	return nil
}

func isBlocked(ip net.IP, blockPrivate bool) bool {
	if ip.IsLoopback() ||
		ip.IsLinkLocalUnicast() ||
		ip.IsLinkLocalMulticast() ||
		ip.IsInterfaceLocalMulticast() ||
		ip.IsMulticast() ||
		ip.IsUnspecified() {
		return true
	}
	for _, n := range alwaysBlocked {
		if n.Contains(ip) {
			return true
		}
	}
	if blockPrivate {
		if ip.IsPrivate() {
			return true
		}
		for _, n := range privateExtra {
			if n.Contains(ip) {
				return true
			}
		}
	}
	return false
}

// SafeDialer mengembalikan net.Dialer yang memeriksa IP tujuan SETELAH DNS di-resolve, pada setiap
// percobaan sambungan (TCP maupun UDP) — menutup DNS rebinding dan redirect ke alamat internal.
func SafeDialer(blockPrivate bool) *net.Dialer {
	return &net.Dialer{
		Timeout: 10 * time.Second,
		Control: func(network, address string, _ syscall.RawConn) error {
			host, _, err := net.SplitHostPort(address)
			if err != nil {
				return fmt.Errorf("netguard: alamat tujuan tidak valid: %w", err)
			}
			ip := net.ParseIP(host)
			if ip == nil {
				return fmt.Errorf("netguard: tujuan %q bukan alamat IP", host)
			}
			if isBlocked(ip, blockPrivate) {
				return fmt.Errorf("netguard: sambungan ke %s ditolak (internal/link-local)", ip)
			}
			return nil
		},
	}
}

// SafeTransport — http.Transport yang memakai SafeDialer. Proxy lingkungan SENGAJA dimatikan: lewat
// proxy, tujuan sebenarnya tidak lagi terlihat oleh penjaga sambungan.
func SafeTransport(blockPrivate bool) *http.Transport {
	d := SafeDialer(blockPrivate)
	return &http.Transport{
		Proxy:                 nil,
		DialContext:           func(ctx context.Context, network, addr string) (net.Conn, error) { return d.DialContext(ctx, network, addr) },
		TLSHandshakeTimeout:   10 * time.Second,
		ResponseHeaderTimeout: 15 * time.Second,
		MaxIdleConns:          20,
		IdleConnTimeout:       30 * time.Second,
	}
}
