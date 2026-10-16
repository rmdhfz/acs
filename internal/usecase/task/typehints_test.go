package task

import (
	"context"
	"testing"

	"acs/internal/domain"
)

type hintParams struct {
	domain.DeviceParameterRepository
	types map[string]*uint64
}

func (h hintParams) Get(_ context.Context, _ uint64, name string) (*domain.DeviceParameter, error) {
	id, ok := h.types[name]
	if !ok {
		return nil, domain.ErrNotFound
	}
	return &domain.DeviceParameter{ParameterName: name, ParameterTypeID: id}, nil
}

type hintRefs struct{ domain.RefRepository }

func (hintRefs) GetByID(_ context.Context, table string, id uint64) (domain.RefLookup, error) {
	codes := map[uint64]string{1: "string", 2: "unsignedInt", 3: "boolean"}
	if table != domain.RefTableParameterTypes || codes[id] == "" {
		return domain.RefLookup{}, domain.ErrNotFound
	}
	return domain.RefLookup{ID: id, Code: codes[id]}, nil
}

func typeID(v uint64) *uint64 { return &v }

// SetParameterValues harus memakai tipe yang pernah dilaporkan CPE (bukan semua xsd:string).
func TestWithTypeHints(t *testing.T) {
	s := &Service{
		deviceParams: hintParams{types: map[string]*uint64{
			"Dev.Interval": typeID(2), // unsignedInt
			"Dev.Enable":   typeID(3), // boolean
			"Dev.SSID":     typeID(1), // string -> tidak perlu hint
			// "Dev.Baru" tidak pernah terlihat
		}},
		refs: hintRefs{},
	}
	in := map[string]interface{}{"values": map[string]interface{}{
		"Dev.Interval": "300", "Dev.Enable": "true", "Dev.SSID": "x", "Dev.Baru": "1", "Dev.Dipaksa": "5",
	}, "types": map[string]interface{}{"Dev.Dipaksa": "xsd:int"}}

	out := s.withTypeHints(context.Background(), 1, in)
	types, _ := out["types"].(map[string]string)
	want := map[string]string{"Dev.Interval": "xsd:unsignedInt", "Dev.Enable": "xsd:boolean", "Dev.Dipaksa": "xsd:int"}
	if len(types) != len(want) {
		t.Fatalf("types = %v, want %v", types, want)
	}
	for k, v := range want {
		if types[k] != v {
			t.Errorf("types[%s] = %q, want %q", k, types[k], v)
		}
	}
	// input asli tidak boleh dimutasi
	if _, has := in["types"].(map[string]string); has {
		t.Error("withTypeHints memutasi map input")
	}

	// tanpa hint apa pun -> params dikembalikan apa adanya
	plain := map[string]interface{}{"values": map[string]interface{}{"Dev.SSID": "x"}}
	if got := s.withTypeHints(context.Background(), 1, plain); got["types"] != nil {
		t.Errorf("tak ada hint -> tak ada key types, got %v", got["types"])
	}
}

func TestValidateSetParameterTypes(t *testing.T) {
	ok := map[string]interface{}{"values": map[string]interface{}{"A": "1"}, "types": map[string]interface{}{"A": "xsd:unsignedInt"}}
	if err := ValidateTaskInput(domain.TaskTypeSetParameterValues, ok); err != nil {
		t.Fatalf("types valid ditolak: %v", err)
	}
	bad := map[string]interface{}{"values": map[string]interface{}{"A": "1"}, "types": map[string]interface{}{"A": "xsd:float"}}
	if err := ValidateTaskInput(domain.TaskTypeSetParameterValues, bad); err == nil {
		t.Fatal("tipe XSD tak dikenal harus ditolak")
	}
}

func TestNormalizeXSDType(t *testing.T) {
	cases := map[string]string{
		"xsd:string": "string", "xsd:unsignedInt": "unsignedInt", "unsignedint": "unsignedInt", "xsd:boolean": "boolean",
		"xsd:int": "int", "xsd:dateTime": "dateTime", "xsd:base64": "base64", "": "", "xsd:float": "", "bogus": "",
	}
	for in, want := range cases {
		if got := domain.NormalizeXSDType(in); got != want {
			t.Errorf("NormalizeXSDType(%q) = %q, want %q", in, got, want)
		}
	}
}
