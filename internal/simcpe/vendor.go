// Package simcpe adalah simulator CPE TR-069/CWMP multi-vendor yang dipakai
// bersama oleh cmd/cpesim (CLI manual) dan cmd/e2e (skenario otomatis).
//
// Simulator ini adalah klien CWMP sungguhan: menjalankan siklus sesi penuh
// (Inform -> InformResponse -> loop RPC sampai ACS menutup sesi), membalas
// setiap RPC yang dikirim ACS, dan menyimpan data model in-memory yang
// BENAR-BENAR berubah saat di-SetParameterValues.
//
// BUKAN pengganti PENGUJIAN_LAPANGAN.md — tidak menjalankan firmware vendor
// nyata, tidak punya kuirk per-firmware, dan path parameter yang dipakai
// adalah asumsi instance pertama (.1) yang sama dengan migrasi 0004. Gunanya:
// membuktikan alur ACS end-to-end berulang & sebagai jaring regresi CWMP.
package simcpe

import "fmt"

// DataModel membedakan root TR-069 antar generasi/vendor. Simulator TIDAK
// mengasumsikan salah satu — tiap profil vendor menyatakannya eksplisit,
// persis seperti device fisik (CLAUDE.md: "Root data model berbeda antar
// generasi/vendor: InternetGatewayDevice.* (TR-098) vs Device.* (TR-181)").
type DataModel string

const (
	TR098 DataModel = "TR-098" // root InternetGatewayDevice.
	TR181 DataModel = "TR-181" // root Device.
)

// Profile adalah kepribadian satu model CPE: OUI, string identitas, versi
// namespace CWMP yang DIDEKLARASIKAN pada envelope (sengaja bervariasi antar
// profil untuk menguji echo namespace di pkg/cwmpxml + delivery/cwmp), root
// data model, dan kuirk.
type Profile struct {
	Key          string
	Manufacturer string
	OUI          string // 6 karakter; cocokkan ke vendor_ouis agar vendor_id ter-resolve
	ProductClass string
	ModelName    string
	Namespace    string // xmlns:cwmp (urn:dslforum-org:cwmp-1-0 / 1-1 / 1-2)
	DataModel    DataModel
	HardwareVer  string
	SoftwareVer  string
	// OmitConnectionRequestURL: sebagian firmware tidak melaporkan
	// ConnectionRequestURL pada Inform — menguji jalur Connection Request
	// tanpa URL yang diketahui.
	OmitConnectionRequestURL bool
}

// RootPrefix mengembalikan awalan path parameter sesuai data model.
func (p Profile) RootPrefix() string {
	if p.DataModel == TR181 {
		return "Device."
	}
	return "InternetGatewayDevice."
}

// Profiles — 5 vendor sesuai PENGUJIAN_LAPANGAN.md / migrasi 0004. Namespace &
// data model sengaja dicampur supaya satu run fleet melatih semua kombinasi.
//
// OUI di sini placeholder yang JELAS bukan format IEEE (mengandung huruf) —
// konsisten dengan kebijakan migrasi 0004 yang sengaja mengosongkan
// vendor_ouis. cmd/e2e menyuntik OUI ini ke Catalog supaya vendor_id
// ter-resolve; di luar itu device muncul "vendor belum diketahui" (ekspektasi).
var Profiles = map[string]Profile{
	"zte": {
		Key: "zte", Manufacturer: "ZTE", OUI: "ZTE0A1", ProductClass: "F670L",
		ModelName: "ZXHN F670L", Namespace: "urn:dslforum-org:cwmp-1-0",
		DataModel: TR098, HardwareVer: "V1.1", SoftwareVer: "V1.1.20P3N2",
	},
	"huawei": {
		Key: "huawei", Manufacturer: "Huawei Technologies", OUI: "HW00E0", ProductClass: "EG8145V5",
		ModelName: "EchoLife EG8145V5", Namespace: "urn:dslforum-org:cwmp-1-2",
		DataModel: TR098, HardwareVer: "702.A", SoftwareVer: "V5R020C10S115",
	},
	"fiberhome": {
		Key: "fiberhome", Manufacturer: "FiberHome", OUI: "FH0011", ProductClass: "HG6145F",
		ModelName: "AN5506-04-FA", Namespace: "urn:dslforum-org:cwmp-1-1",
		DataModel: TR098, HardwareVer: "WKE2.094.331", SoftwareVer: "RP2521",
	},
	"nokia": {
		Key: "nokia", Manufacturer: "Nokia", OUI: "NOK1AF", ProductClass: "BeaconG6",
		ModelName: "G-1425G-A", Namespace: "urn:dslforum-org:cwmp-1-2",
		DataModel: TR181, HardwareVer: "3FE48927AAAA", SoftwareVer: "3FE49362IJHK92",
		OmitConnectionRequestURL: true,
	},
	"cdata": {
		Key: "cdata", Manufacturer: "C-Data Technology", OUI: "CDT007", ProductClass: "FD704GW",
		ModelName: "FD704GW-DF", Namespace: "urn:dslforum-org:cwmp-1-0",
		DataModel: TR098, HardwareVer: "1.0", SoftwareVer: "2.5.7_2.1.0",
	},
}

// ProfileKeys — urutan stabil untuk iterasi (map Go tidak terurut).
var ProfileKeys = []string{"zte", "huawei", "fiberhome", "nokia", "cdata"}

// ProfileByKey mengambil profil berdasarkan -vendor <key>.
func ProfileByKey(key string) (Profile, error) {
	p, ok := Profiles[key]
	if !ok {
		return Profile{}, fmt.Errorf("profil vendor %q tidak dikenal (pilihan: %v)", key, ProfileKeys)
	}
	return p, nil
}
