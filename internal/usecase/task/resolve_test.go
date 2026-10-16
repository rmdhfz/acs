package task

import (
	"context"
	"errors"
	"testing"

	"acs/internal/domain"
)

type rvDevices struct {
	domain.DeviceRepository
	dev *domain.Device
}

func (r rvDevices) GetByID(context.Context, uint64) (*domain.Device, error) { return r.dev, nil }

type rvParams struct {
	domain.DeviceParameterRepository
	prefixes map[string]bool
}

func (r rvParams) HasPrefix(_ context.Context, _ uint64, p string) (bool, error) { return r.prefixes[p], nil }

type rvMappings struct {
	domain.VendorParameterMappingRepository
	calls []uint64 // data_model_version_id yang diminta
}

func (r *rvMappings) Resolve(_ context.Context, _ uint64, dmv uint64, _ *uint64, key string, _ *string) (*domain.VendorParameterMapping, error) {
	r.calls = append(r.calls, dmv)
	if key == "wifi.ssid" {
		path := "InternetGatewayDevice.LANDevice.1.WLANConfiguration.1.SSID"
		if dmv == 2 {
			path = "Device.WiFi.SSID.1.SSID"
		}
		return &domain.VendorParameterMapping{TR069Path: path}, nil
	}
	return nil, domain.ErrNotFound
}

type rvRefs struct{ domain.RefRepository }

func (rvRefs) GetByCode(_ context.Context, table, code string) (domain.RefLookup, error) {
	if table == domain.RefTableDataModelVersions {
		return map[string]domain.RefLookup{"TR098": {ID: 1, Code: "TR098"}, "TR181": {ID: 2, Code: "TR181"}}[code], nil
	}
	return domain.RefLookup{}, domain.ErrNotFound
}

func newResolveSvc(prefixes map[string]bool, mm *rvMappings) *Service {
	vendor := uint64(1)
	return &Service{
		devices:       rvDevices{dev: &domain.Device{ID: 7, VendorID: &vendor}}, // model BELUM diketahui
		deviceParams:  rvParams{prefixes: prefixes},
		paramMappings: mm,
		refs:          rvRefs{},
	}
}

// ONT baru tanpa model: data model dideteksi dari parameter yang dilaporkan CPE.
func TestResolveParameterPath_DeteksiDataModelDariParameter(t *testing.T) {
	for _, c := range []struct {
		name     string
		prefixes map[string]bool
		want     string
	}{
		{"TR-098", map[string]bool{"InternetGatewayDevice.": true}, "InternetGatewayDevice.LANDevice.1.WLANConfiguration.1.SSID"},
		{"TR-181", map[string]bool{"Device.": true}, "Device.WiFi.SSID.1.SSID"},
	} {
		s := newResolveSvc(c.prefixes, &rvMappings{})
		got, err := s.ResolveParameterPath(context.Background(), 7, "wifi.ssid")
		if err != nil || got != c.want {
			t.Errorf("%s: got %q err=%v, want %q", c.name, got, err, c.want)
		}
	}
	// belum ada parameter sama sekali -> 400, bukan menebak
	s := newResolveSvc(map[string]bool{}, &rvMappings{})
	if _, err := s.ResolveParameterPath(context.Background(), 7, "wifi.ssid"); !errors.Is(err, domain.ErrInvalidInput) {
		t.Errorf("tanpa parameter harus ErrInvalidInput, got %v", err)
	}
}

func TestResolveLogicalKeys_SetGetDanAttributes(t *testing.T) {
	s := newResolveSvc(map[string]bool{"Device.": true}, &rvMappings{})
	ctx := context.Background()

	out, err := s.resolveLogicalKeys(ctx, 7, domain.TaskTypeSetParameterValues, map[string]interface{}{
		"values": map[string]interface{}{"wifi.ssid": "Baru", "Device.X.Raw": "1"},
	})
	if err != nil {
		t.Fatal(err)
	}
	vals := out["values"].(map[string]string)
	if vals["Device.WiFi.SSID.1.SSID"] != "Baru" || vals["Device.X.Raw"] != "1" || len(vals) != 2 {
		t.Fatalf("values salah: %v", vals)
	}

	out, err = s.resolveLogicalKeys(ctx, 7, domain.TaskTypeGetParameterValues, map[string]interface{}{"names": []interface{}{"wifi.ssid"}})
	if err != nil || out["names"].([]string)[0] != "Device.WiFi.SSID.1.SSID" {
		t.Fatalf("names: %v %v", out, err)
	}

	out, err = s.resolveLogicalKeys(ctx, 7, domain.TaskTypeSetParameterAttributes, map[string]interface{}{
		"attributes": []interface{}{map[string]interface{}{"name": "wifi.ssid", "notification_change": true, "notification": 2}},
	})
	if err != nil || out["attributes"].([]map[string]interface{})[0]["name"] != "Device.WiFi.SSID.1.SSID" {
		t.Fatalf("attributes: %v %v", out, err)
	}

	// logical key tak terpetakan -> ErrInvalidInput (400)
	if _, err := s.resolveLogicalKeys(ctx, 7, domain.TaskTypeSetParameterValues, map[string]interface{}{"values": map[string]interface{}{"tidak.ada": "x"}}); !errors.Is(err, domain.ErrInvalidInput) {
		t.Fatalf("key tak terpetakan harus ErrInvalidInput, got %v", err)
	}
}
