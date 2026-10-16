package device

import (
	"crypto/hmac"
	"crypto/rand"
	"crypto/sha1"
	"encoding/hex"
	"fmt"
	"net"
	"net/url"
	"strconv"
	"strings"
	"time"
)

// UDP Connection Request (TR-069 Amendment, Annex G; dipakai CPE di belakang NAT/CGNAT
// bersama STUN TR-111). CPE sendiri yang menjalankan klien STUN (STUNEnable /
// STUNServerAddress) untuk menjaga pemetaan NAT dan melaporkan alamat publiknya lewat
// ManagementServer.UDPConnectionRequestAddress. ACS TIDAK berbicara STUN: ia mengirim
// satu datagram UDP berformat pesan HTTP GET ke alamat itu:
//
//	GET http://<alamat>?ts=<ts>&id=<id>&un=<user>&cn=<cnonce>&sig=<sig> HTTP/1.1
//	Host: <alamat>
//
// ts = epoch detik, id = ID unik per pesan, un = username Connection Request,
// cn = cnonce acak, sig = HMAC-SHA1(password, ts+id+un+cn) heksadesimal huruf besar.
// CPE tidak membalas; ia memverifikasi sig, menolak ts basi/id berulang, lalu membuka
// sesi Inform (event "6 CONNECTION REQUEST").
//
// CATATAN VERIFIKASI: format di atas ditulis dari spesifikasi Annex G dan teruji hanya
// terhadap penerima uji (udp_connreq_test.go); BELUM teruji terhadap CPE nyata.

// BuildUDPConnectionRequest membangun datagram Annex G. now/ids diinjeksi agar bisa diuji.
func BuildUDPConnectionRequest(address, username, password string, now time.Time, id, cnonce string) []byte {
	ts := strconv.FormatInt(now.Unix(), 10)
	mac := hmac.New(sha1.New, []byte(password))
	mac.Write([]byte(ts + id + username + cnonce))
	sig := strings.ToUpper(hex.EncodeToString(mac.Sum(nil)))
	q := url.Values{}
	q.Set("ts", ts)
	q.Set("id", id)
	q.Set("un", username)
	q.Set("cn", cnonce)
	q.Set("sig", sig)
	return []byte(fmt.Sprintf("GET http://%s?%s HTTP/1.1\r\nHost: %s\r\n\r\n", address, q.Encode(), address))
}

func randomHex(n int) string {
	b := make([]byte, n)
	_, _ = rand.Read(b)
	return hex.EncodeToString(b)
}

// WakeUpViaUDP mengirim UDP Connection Request ke address ("host:port") TANPA penjaga SSRF
// (dipakai alat/uji). Jalur produksi memakai wakeUpViaUDP dengan netguard.SafeDialer.
func WakeUpViaUDP(address, username, password string) error {
	return wakeUpViaUDP(&net.Dialer{Timeout: 5 * time.Second}, address, username, password)
}

func wakeUpViaUDP(dialer *net.Dialer, address, username, password string) error {
	conn, err := dialer.Dial("udp", address)
	if err != nil {
		return fmt.Errorf("gagal membuka koneksi udp: %w", err)
	}
	defer conn.Close()
	pkt := BuildUDPConnectionRequest(address, username, password, time.Now(), randomHex(8), randomHex(8))
	_ = conn.SetWriteDeadline(time.Now().Add(3 * time.Second))
	if _, err := conn.Write(pkt); err != nil {
		return fmt.Errorf("gagal mengirim paket udp: %w", err)
	}
	return nil
}

// UDPAddressParamSuffix — nama parameter (di bawah root data model apa pun, TR-098
// InternetGatewayDevice.* maupun TR-181 Device.*) yang memuat alamat UDP CPE.
const UDPAddressParamSuffix = "ManagementServer.UDPConnectionRequestAddress"

// ExtractUDPAddress mencari alamat UDP Connection Request pada parameter Inform.
func ExtractUDPAddress(params map[string]string) string {
	for k, v := range params {
		if strings.HasSuffix(k, UDPAddressParamSuffix) {
			return v
		}
	}
	return ""
}
