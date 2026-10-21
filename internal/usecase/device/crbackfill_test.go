package device

import (
	"context"
	"net"
	"testing"

	"acs/internal/domain"
)

type bfDevices struct {
	domain.DeviceRepository
	dev     *domain.Device
	updated int
}

func (r *bfDevices) GetByID(context.Context, uint64) (*domain.Device, error) {
	cp := *r.dev
	return &cp, nil
}
func (r *bfDevices) Update(_ context.Context, d *domain.Device) error {
	r.updated++
	cp := *d
	r.dev = &cp
	return nil
}

type bfParams struct {
	domain.DeviceParameterRepository
	value string
	has   bool
}

func (r *bfParams) GetBySuffix(_ context.Context, _ uint64, suffix string) (*domain.DeviceParameter, error) {
	if !r.has || suffix != ".ManagementServer.ConnectionRequestURL" {
		return nil, domain.ErrNotFound
	}
	v := r.value
	return &domain.DeviceParameter{ParameterValue: &v}, nil
}

func s(v string) *string { return &v }

func trusted10(t *testing.T) []*net.IPNet {
	t.Helper()
	_, n, _ := net.ParseCIDR("10.0.0.0/8")
	return []*net.IPNet{n}
}

func bfService(dev *domain.Device, params *bfParams, trusted []*net.IPNet) (*Service, *bfDevices) {
	repo := &bfDevices{dev: dev}
	return &Service{devices: repo, deviceParams: params, crTrusted: trusted}, repo
}

var superActor = domain.Actor{UserID: 1, Roles: []string{domain.RoleSuperadmin}}

// CPE yang Inform sebelum rentang tepercaya aktif: URL dari parameter yang sudah pernah dilaporkan diisi saat device dibuka.
func TestGet_BackfillCRURLDariParameter(t *testing.T) {
	dev := &domain.Device{ID: 5, IPAddress: s("172.21.0.1")}
	svc, repo := bfService(dev, &bfParams{has: true, value: "http://10.0.123.11:58000"}, trusted10(t))
	d, err := svc.Get(context.Background(), superActor, 5)
	if err != nil {
		t.Fatal(err)
	}
	if d.ConnectionRequestURL == nil || *d.ConnectionRequestURL != "http://10.0.123.11:58000" || repo.updated != 1 {
		t.Fatalf("URL harus terisi & tersimpan: %v (update=%d)", d.ConnectionRequestURL, repo.updated)
	}
	// panggilan berikutnya tidak menulis lagi (URL sudah ada)
	if _, err := svc.Get(context.Background(), superActor, 5); err != nil || repo.updated != 1 {
		t.Fatalf("tidak boleh menulis ulang: update=%d err=%v", repo.updated, err)
	}
}

func TestGet_BackfillTidakBerlakuBilaTidakAman(t *testing.T) {
	cases := map[string]struct {
		dev     *domain.Device
		params  *bfParams
		trusted func(*testing.T) []*net.IPNet
	}{
		"tanpa rentang tepercaya (default ketat)": {&domain.Device{ID: 5, IPAddress: s("172.21.0.1")}, &bfParams{has: true, value: "http://10.0.123.11:58000"}, func(*testing.T) []*net.IPNet { return nil }},
		"host di luar rentang tepercaya":          {&domain.Device{ID: 5, IPAddress: s("172.21.0.1")}, &bfParams{has: true, value: "http://203.0.113.9:58000"}, trusted10},
		"metadata cloud tetap ditolak":            {&domain.Device{ID: 5, IPAddress: s("172.21.0.1")}, &bfParams{has: true, value: "http://169.254.169.254/x"}, trusted10},
		"parameter belum pernah dilaporkan":       {&domain.Device{ID: 5, IPAddress: s("172.21.0.1")}, &bfParams{has: false}, trusted10},
		"IP sumber belum diketahui":               {&domain.Device{ID: 5}, &bfParams{has: true, value: "http://10.0.123.11:58000"}, trusted10},
		"URL sudah terisi tidak ditimpa":          {&domain.Device{ID: 5, IPAddress: s("172.21.0.1"), ConnectionRequestURL: s("http://10.9.9.9:1/x")}, &bfParams{has: true, value: "http://10.0.123.11:58000"}, trusted10},
	}
	for name, c := range cases {
		svc, repo := bfService(c.dev, c.params, c.trusted(t))
		d, err := svc.Get(context.Background(), superActor, 5)
		if err != nil {
			t.Fatalf("%s: %v", name, err)
		}
		if repo.updated != 0 {
			t.Errorf("%s: tidak boleh menulis device (update=%d)", name, repo.updated)
		}
		if name == "URL sudah terisi tidak ditimpa" {
			if d.ConnectionRequestURL == nil || *d.ConnectionRequestURL != "http://10.9.9.9:1/x" {
				t.Errorf("%s: URL lama ditimpa: %v", name, d.ConnectionRequestURL)
			}
		} else if d.ConnectionRequestURL != nil {
			t.Errorf("%s: URL tidak boleh terisi: %v", name, *d.ConnectionRequestURL)
		}
	}
}

// Flag kredensial Connection Request: hanya "sudah diisi atau belum", tanpa membocorkan nilainya.
func TestGet_FlagKredensialConnectionRequest(t *testing.T) {
	cases := map[string]struct {
		user *string
		pass []byte
		want bool
	}{
		"keduanya terisi":   {s("acs"), []byte{1}, true},
		"username kosong":   {s(""), []byte{1}, false},
		"tanpa username":    {nil, []byte{1}, false},
		"tanpa password":    {s("acs"), nil, false},
		"keduanya kosong":   {nil, nil, false},
		"password kosong[]": {s("acs"), []byte{}, false},
	}
	for name, c := range cases {
		svc, _ := bfService(&domain.Device{ID: 5, ConnectionRequestUsername: c.user, ConnectionRequestPasswordEnc: c.pass}, &bfParams{}, nil)
		d, err := svc.Get(context.Background(), superActor, 5)
		if err != nil || d.CRCredentialsSet != c.want {
			t.Errorf("%s: CRCredentialsSet=%v err=%v, ingin %v", name, d != nil && d.CRCredentialsSet, err, c.want)
		}
	}
}
