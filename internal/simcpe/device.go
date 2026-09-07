package simcpe

import (
	"fmt"
	"sort"
	"strings"
	"sync"
)

// Device adalah satu CPE simulasi dengan data model in-memory yang BENAR-BENAR
// berubah: SetParameterValues dari ACS mengubah map, dan GetParameterValues
// berikutnya mengembalikan nilai baru itu.
type Device struct {
	Profile      Profile
	SerialNumber string
	ConnReqURL   string // diisi bila listener Connection Request aktif

	mu     sync.Mutex
	params map[string]string
}

// NewDevice membangun data model awal sesuai profil vendor. Parameter yang
// diisi mencakup 12 logical key yang dipetakan migrasi 0004 (berlaku di
// kelima vendor), di-resolve ke path TR-098/TR-181 sesuai data model profil.
func NewDevice(p Profile, serial string) *Device {
	d := &Device{
		Profile:      p,
		SerialNumber: serial,
		params:       map[string]string{},
	}
	r := p.RootPrefix()
	set := func(rel, val string) { d.params[r+rel] = val }

	set("DeviceInfo.Manufacturer", p.Manufacturer)
	set("DeviceInfo.ManufacturerOUI", strings.ToUpper(p.OUI))
	set("DeviceInfo.ModelName", p.ModelName)
	set("DeviceInfo.ProductClass", p.ProductClass)
	set("DeviceInfo.SerialNumber", serial)
	set("DeviceInfo.HardwareVersion", p.HardwareVer)
	set("DeviceInfo.SoftwareVersion", p.SoftwareVer)
	set("DeviceInfo.UpTime", "7200")
	set("ManagementServer.PeriodicInformEnable", "true")
	set("ManagementServer.PeriodicInformInterval", "300")
	set("ManagementServer.ConnectionRequestUsername", "acs-"+serial)

	if p.DataModel == TR181 {
		set("WiFi.SSID.1.SSID", "Home-"+shortSerial(serial))
		set("WiFi.AccessPoint.1.Security.KeyPassphrase", "changeme-"+shortSerial(serial))
		set("PPP.Interface.1.Username", "pppoe-"+shortSerial(serial))
		set("PPP.Interface.1.Password", "pppoe-secret")
	} else {
		set("LANDevice.1.WLANConfiguration.1.SSID", "Home-"+shortSerial(serial))
		set("LANDevice.1.WLANConfiguration.1.KeyPassphrase", "changeme-"+shortSerial(serial))
		set("WANDevice.1.WANConnectionDevice.1.WANPPPConnection.1.Username", "pppoe-"+shortSerial(serial))
		set("WANDevice.1.WANConnectionDevice.1.WANPPPConnection.1.Password", "pppoe-secret")
	}
	return d
}

func shortSerial(s string) string {
	if len(s) <= 6 {
		return s
	}
	return s[len(s)-6:]
}

func (d *Device) softwareVersionPath() string {
	return d.Profile.RootPrefix() + "DeviceInfo.SoftwareVersion"
}

// SoftwareVersion untuk dilaporkan & untuk precondition ZTP/preset.
func (d *Device) SoftwareVersion() string {
	d.mu.Lock()
	defer d.mu.Unlock()
	return d.params[d.softwareVersionPath()]
}

// SetSoftwareVersion mensimulasikan firmware ter-upgrade (dipanggil setelah
// TransferComplete sukses, supaya Inform berikutnya melaporkan versi baru).
func (d *Device) SetSoftwareVersion(v string) {
	d.mu.Lock()
	defer d.mu.Unlock()
	d.params[d.softwareVersionPath()] = v
}

// Param mengembalikan satu nilai parameter (untuk assertion e2e). ok=false
// bila tidak ada.
func (d *Device) Param(name string) (string, bool) {
	d.mu.Lock()
	defer d.mu.Unlock()
	v, ok := d.params[name]
	return v, ok
}

// InformParams — subset parameter yang dilaporkan pada tiap Inform. ACS
// memakai suffix "SoftwareVersion"/"HardwareVersion" (paramSuffix di
// handler.go), jadi nama parameter di sini harus diakhiri string itu.
func (d *Device) InformParams() []NameValue {
	d.mu.Lock()
	defer d.mu.Unlock()
	r := d.Profile.RootPrefix()
	want := []string{
		"DeviceInfo.Manufacturer", "DeviceInfo.ManufacturerOUI", "DeviceInfo.ModelName",
		"DeviceInfo.ProductClass", "DeviceInfo.SerialNumber",
		"DeviceInfo.HardwareVersion", "DeviceInfo.SoftwareVersion", "DeviceInfo.UpTime",
		"ManagementServer.PeriodicInformInterval",
	}
	out := make([]NameValue, 0, len(want)+1)
	for _, rel := range want {
		if v, ok := d.params[r+rel]; ok {
			out = append(out, NameValue{Name: r + rel, Value: v})
		}
	}
	if !d.Profile.OmitConnectionRequestURL && d.ConnReqURL != "" {
		key := r + "ManagementServer.ConnectionRequestURL"
		d.params[key] = d.ConnReqURL
		out = append(out, NameValue{Name: key, Value: d.ConnReqURL})
	}
	return out
}

// Get mengembalikan nilai untuk daftar nama. Nama yang tidak ada -> error
// (mensimulasikan CPE yang menolak parameter tak dikenal). Nama diakhiri "."
// atau kosong diperlakukan sebagai partial path.
func (d *Device) Get(names []string) ([]NameValue, error) {
	d.mu.Lock()
	defer d.mu.Unlock()
	var out []NameValue
	for _, n := range names {
		if n == "" || strings.HasSuffix(n, ".") {
			for k, v := range d.params {
				if n == "" || strings.HasPrefix(k, n) {
					out = append(out, NameValue{Name: k, Value: v})
				}
			}
			continue
		}
		v, ok := d.params[n]
		if !ok {
			return nil, fmt.Errorf("parameter %q tidak ada di device ini", n)
		}
		out = append(out, NameValue{Name: n, Value: v})
	}
	sort.Slice(out, func(i, j int) bool { return out[i].Name < out[j].Name })
	return out, nil
}

// Set menerapkan SetParameterValues. Parameter baru diterima (device nyata
// umumnya mengizinkan set pada parameter writable walau belum pernah dibaca).
func (d *Device) Set(values map[string]string) {
	d.mu.Lock()
	defer d.mu.Unlock()
	for k, v := range values {
		d.params[k] = v
	}
}

// ParameterNames untuk GetParameterNamesResponse.
func (d *Device) ParameterNames(path string, nextLevel bool) []ParamInfo {
	d.mu.Lock()
	defer d.mu.Unlock()
	base := path
	if base == "" {
		base = d.Profile.RootPrefix()
	}
	seen := map[string]bool{}
	var out []ParamInfo
	for k := range d.params {
		if !strings.HasPrefix(k, base) {
			continue
		}
		if nextLevel {
			rest := strings.TrimPrefix(k, base)
			seg := rest
			if i := strings.IndexByte(rest, '.'); i >= 0 {
				seg = rest[:i+1]
			}
			name := base + seg
			if !seen[name] {
				seen[name] = true
				out = append(out, ParamInfo{Name: name, Writable: !strings.HasSuffix(name, ".")})
			}
			continue
		}
		if !seen[k] {
			seen[k] = true
			out = append(out, ParamInfo{Name: k, Writable: true})
		}
	}
	sort.Slice(out, func(i, j int) bool { return out[i].Name < out[j].Name })
	return out
}

// AddObject menambah instance baru di bawah objName, mengembalikan nomor instance.
func (d *Device) AddObject(objName string) int {
	d.mu.Lock()
	defer d.mu.Unlock()
	objName = strings.TrimSuffix(objName, ".") + "."
	max := 0
	for k := range d.params {
		if !strings.HasPrefix(k, objName) {
			continue
		}
		rest := strings.TrimPrefix(k, objName)
		if i := strings.IndexByte(rest, '.'); i > 0 {
			if n := atoiSafe(rest[:i]); n > max {
				max = n
			}
		}
	}
	inst := max + 1
	d.params[fmt.Sprintf("%s%d.Enable", objName, inst)] = "false"
	return inst
}

// DeleteObject menghapus semua parameter di bawah instance yang diberikan.
func (d *Device) DeleteObject(objName string) {
	d.mu.Lock()
	defer d.mu.Unlock()
	prefix := strings.TrimSuffix(objName, ".") + "."
	for k := range d.params {
		if strings.HasPrefix(k, prefix) {
			delete(d.params, k)
		}
	}
}

// Snapshot mengembalikan salinan seluruh data model (untuk assertion e2e).
func (d *Device) Snapshot() map[string]string {
	d.mu.Lock()
	defer d.mu.Unlock()
	cp := make(map[string]string, len(d.params))
	for k, v := range d.params {
		cp[k] = v
	}
	return cp
}

func atoiSafe(s string) int {
	n := 0
	for _, c := range s {
		if c < '0' || c > '9' {
			return 0
		}
		n = n*10 + int(c-'0')
	}
	return n
}

// NameValue & ParamInfo — tipe polos lokal (paket ini tidak bergantung
// delivery/cwmp).
type NameValue struct{ Name, Value string }

type ParamInfo struct {
	Name     string
	Writable bool
}
