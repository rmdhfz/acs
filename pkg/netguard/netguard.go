// Package netguard menolak target permintaan keluar (outbound HTTP/UDP) yang
// menunjuk alamat internal — pertahanan SSRF untuk jalur di mana URL/host
// berasal dari input yang tidak sepenuhnya tepercaya (Connection Request URL
// yang dilaporkan CPE, target_url webhook yang diisi admin tenant, dsb).
//
// Kebijakan: tolak loopback, link-local (termasuk 169.254.169.254 metadata
// cloud), multicast, dan unspecified. IP privat (RFC1918) DIIZINKAN karena
// deployment enterprise sah menaruh ACS + CPE / ACS + receiver webhook di
// jaringan privat yang sama. Pemanggil yang butuh aturan lebih ketat
// (mis. "host harus == RemoteIP device") melakukannya sendiri sebelum ini.
package netguard

import (
	"fmt"
	"net"
	"net/url"
	"strings"
)

// CheckURL memvalidasi sebuah URL absolut: skema harus http/https, dan setiap
// IP yang di-resolve dari host-nya tidak boleh internal/link-local.
func CheckURL(raw string) error {
	u, err := url.Parse(strings.TrimSpace(raw))
	if err != nil || u.Host == "" {
		return fmt.Errorf("netguard: URL tidak valid")
	}
	if u.Scheme != "http" && u.Scheme != "https" {
		return fmt.Errorf("netguard: skema %q tidak diizinkan (hanya http/https)", u.Scheme)
	}
	return CheckHostPort(u.Host)
}

// CheckHostPort menerima "host" atau "host:port" (IPv6 literal boleh dalam
// tanda kurung). Melakukan DNS lookup bila host bukan literal IP.
func CheckHostPort(hostPort string) error {
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
		if isBlocked(ip) {
			return fmt.Errorf("netguard: alamat %s tidak diizinkan (internal/link-local)", ip)
		}
	}
	return nil
}

func isBlocked(ip net.IP) bool {
	return ip.IsLoopback() ||
		ip.IsLinkLocalUnicast() ||
		ip.IsLinkLocalMulticast() ||
		ip.IsInterfaceLocalMulticast() ||
		ip.IsMulticast() ||
		ip.IsUnspecified()
}
