package task

import (
	"context"
	"encoding/json"
	"testing"

	"acs/internal/domain"
)

type discParams struct {
	domain.DeviceParameterRepository
	names []domain.DeviceParameter
}

func (d *discParams) UpsertNames(_ context.Context, ps []domain.DeviceParameter) error {
	d.names = append(d.names, ps...)
	return nil
}
func (d *discParams) HasPrefix(context.Context, uint64, string) (bool, error) { return false, nil }

// Penelusuran rekursif: objek anak menjadi GetParameterNames berantai (depth-1), daun menjadi
// GetParameterValues; nilai yang ada tidak ditimpa (hanya UpsertNames).
func TestAutoDiscovery_RekursifDanTidakMenimpaNilai(t *testing.T) {
	const typeNames, typeVals, pending = 40, 41, 42
	refs := &fakeRefRepo{ids: map[string]uint64{
		domain.TaskTypeGetParameterNames:  typeNames,
		domain.TaskTypeGetParameterValues: typeVals,
		domain.TaskStatusPending:          pending,
	}}
	repo := &fakeTaskRepo{}
	params := &discParams{}
	svc := NewService(repo, &fakeDeviceRepo{dev: &domain.Device{ID: 1, TenantID: u64(100)}}, params, nil, nil, refs, &fakeActivityRepo{}, nil, nil, nil)

	parent := &domain.Task{DeviceID: 1, Parameters: domain.JSONRawMessage(`{"path":"Device.","next_level":true,"discover":{"depth":3,"budget":30}}`)}
	// Bentuk NYATA yang disimpan session engine: seluruh Body CWMP (regresi: dulu hanya bentuk polos yang dibaca).
	resp := domain.JSONRawMessage(`{"Fault":null,"GetParameterNamesResponse":{"ParameterList":{"ArrayType":"","Items":[
		{"Name":"Device.WiFi.","Writable":false},{"Name":"Device.IP.","Writable":false},{"Name":"Device.RootLeaf","Writable":true}]}}}`)
	svc.handleAutoDiscoveryResponse(context.Background(), parent, resp)

	if len(params.names) != 3 {
		t.Fatalf("UpsertNames harus mencatat 3 nama, got %d", len(params.names))
	}
	if !params.names[2].Writable || params.names[2].ParameterValue != nil {
		t.Fatalf("hanya nama+writable, tanpa nilai: %+v", params.names[2])
	}
	var nNames, nVals int
	for _, ct := range repo.created {
		var p struct {
			Path     string `json:"path"`
			Names    []string
			Discover *struct{ Depth, Budget int }
		}
		_ = json.Unmarshal(ct.Parameters, &p)
		switch ct.TaskTypeID {
		case typeNames:
			nNames++
			if p.Discover == nil || p.Discover.Depth != 2 || p.Discover.Budget < 0 || p.Discover.Budget > 14 {
				t.Errorf("anak harus depth 2 & budget terbagi: %+v", p.Discover)
			}
		case typeVals:
			nVals++
		}
	}
	if nNames != 2 || nVals != 1 {
		t.Fatalf("want 2 GetParameterNames anak + 1 GetParameterValues daun, got %d/%d", nNames, nVals)
	}

	// depth habis -> tidak menelusuri lebih dalam
	repo.created = nil
	last := &domain.Task{DeviceID: 1, Parameters: domain.JSONRawMessage(`{"path":"Device.WiFi.","next_level":true,"discover":{"depth":1,"budget":10}}`)}
	svc.handleAutoDiscoveryResponse(context.Background(), last, resp)
	for _, ct := range repo.created {
		if ct.TaskTypeID == typeNames {
			t.Fatal("depth 1 tidak boleh menelusuri anak lagi")
		}
	}
}

// Bentuk polos ({"ParameterList":...}) tetap diterima (kompatibilitas data lama).
func TestAutoDiscovery_BentukPolosTetapDiterima(t *testing.T) {
	refs := &fakeRefRepo{ids: map[string]uint64{
		domain.TaskTypeGetParameterNames:  40,
		domain.TaskTypeGetParameterValues: 41,
		domain.TaskStatusPending:          42,
	}}
	params := &discParams{}
	svc := NewService(&fakeTaskRepo{}, &fakeDeviceRepo{dev: &domain.Device{ID: 1, TenantID: u64(100)}}, params, nil, nil, refs, &fakeActivityRepo{}, nil, nil, nil)
	parent := &domain.Task{DeviceID: 1, Parameters: domain.JSONRawMessage(`{"path":"Device.","next_level":true}`)}
	svc.handleAutoDiscoveryResponse(context.Background(), parent, domain.JSONRawMessage(`{"ParameterList":{"Items":[{"Name":"Device.A","Writable":true}]}}`))
	if len(params.names) != 1 {
		t.Fatalf("bentuk polos harus tetap terbaca: %d", len(params.names))
	}
}
