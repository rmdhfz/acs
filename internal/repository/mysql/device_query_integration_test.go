package mysql

import (
	"context"
	"errors"
	"os"
	"testing"
	"time"

	"github.com/google/uuid"

	"acs/internal/domain"
)

// Test integrasi MariaDB NYATA untuk query perangkat bebas (kondisi parameter, sort, filter waktu Inform, proyeksi nilai).
// Dilewati bila ACS_TEST_DSN tidak diisi.
func TestDeviceRepository_FreeQuery_Integration(t *testing.T) {
	dsn := os.Getenv("ACS_TEST_DSN")
	if dsn == "" {
		t.Skip("ACS_TEST_DSN tidak diisi — test integrasi MariaDB dilewati")
	}
	db, err := Connect(dsn)
	if err != nil {
		t.Fatalf("connect: %v", err)
	}
	t.Cleanup(func() { db.Close() })
	ctx := context.Background()
	repo := NewDeviceRepository(db)
	params := NewDeviceParameterRepository(db)

	var statusID uint64
	if err := db.Get(&statusID, `SELECT id FROM ref_device_status WHERE code = 'UNREGISTERED'`); err != nil {
		t.Fatalf("status: %v", err)
	}

	const (
		ssid = "InternetGatewayDevice.LANDevice.1.WLANConfiguration.1.SSID"
		ver  = "InternetGatewayDevice.DeviceInfo.SoftwareVersion"
		rx   = "InternetGatewayDevice.X_ZTE.OpticalRx"
		uns  = "InternetGatewayDevice.DeviceInfo.UpTimeNote" // sengaja bernilai bukan angka
	)
	prefix := "FQ" + uuid.NewString()[:6]
	oui := "ABCDEF"
	now := time.Now()
	type spec struct {
		name   string
		inform time.Time
		vals   map[string]string
	}
	specs := []spec{
		{"a", now.Add(-48 * time.Hour), map[string]string{ssid: "Rumah-Budi", ver: "V5.2.1", rx: "-21.5", uns: "abc"}},
		{"b", now.Add(-1 * time.Hour), map[string]string{ssid: "rumah-siti", ver: "V5.2.9", rx: "-27.4", uns: "abc"}},
		{"c", now.Add(-30 * time.Minute), map[string]string{ssid: "Kantor 100%", ver: "V6.0.0", rx: "-8"}},
		{"d", now.Add(-2 * time.Hour), map[string]string{}}, // belum melaporkan parameter apa pun
	}
	ids := map[string]uint64{}
	for _, s := range specs {
		inf := s.inform
		d := &domain.Device{DeviceUUID: uuid.NewString(), DeviceStatusID: statusID, OUI: &oui, SerialNumber: prefix + "-" + s.name, LastInformAt: &inf}
		if err := repo.Create(ctx, d); err != nil {
			t.Fatalf("create %s: %v", s.name, err)
		}
		ids[s.name] = d.ID
		var ps []domain.DeviceParameter
		for k, v := range s.vals {
			v := v
			ps = append(ps, domain.DeviceParameter{DeviceID: d.ID, ParameterName: k, ParameterValue: &v})
		}
		if err := params.UpsertBatch(ctx, ps); err != nil {
			t.Fatalf("params %s: %v", s.name, err)
		}
	}
	t.Cleanup(func() {
		for _, id := range ids {
			_, _ = db.Exec(`DELETE FROM device_parameters WHERE device_id = ?`, id)
			_, _ = db.Exec(`DELETE FROM devices WHERE id = ?`, id)
		}
	})

	// serials: nama pendek perangkat yang cocok (dibatasi prefix unik lewat Search agar data lain tak ikut).
	list := func(f domain.DeviceFilter) []string {
		t.Helper()
		f.Search = prefix
		rows, _, err := repo.List(ctx, f, domain.Pagination{Page: 1, PageSize: 50})
		if err != nil {
			t.Fatalf("list %+v: %v", f, err)
		}
		var out []string
		for _, r := range rows {
			out = append(out, r.SerialNumber[len(prefix)+1:])
		}
		return out
	}
	cond := func(name, op, val string) domain.ParamCond { return domain.ParamCond{Name: name, Op: op, Value: val} }
	same := func(got []string, want ...string) {
		t.Helper()
		if len(got) != len(want) {
			t.Fatalf("got %v, want %v", got, want)
		}
		seen := map[string]bool{}
		for _, g := range got {
			seen[g] = true
		}
		for _, w := range want {
			if !seen[w] {
				t.Fatalf("got %v, want %v", got, want)
			}
		}
	}

	// eq tidak peka huruf besar/kecil (collation tabel)
	same(list(domain.DeviceFilter{Params: []domain.ParamCond{cond(ssid, domain.ParamOpEq, "RUMAH-SITI")}}), "b")
	// ne: parameter ada DAN berbeda — d (tanpa parameter) tidak ikut
	same(list(domain.DeviceFilter{Params: []domain.ParamCond{cond(ver, domain.ParamOpNe, "V5.2.1")}}), "b", "c")
	// prefix & like; '%' pada nilai dicocokkan literal (bukan wildcard)
	same(list(domain.DeviceFilter{Params: []domain.ParamCond{cond(ver, domain.ParamOpPrefix, "V5.")}}), "a", "b")
	same(list(domain.DeviceFilter{Params: []domain.ParamCond{cond(ssid, domain.ParamOpLike, "rumah")}}), "a", "b")
	same(list(domain.DeviceFilter{Params: []domain.ParamCond{cond(ssid, domain.ParamOpLike, "100%")}}), "c")
	same(list(domain.DeviceFilter{Params: []domain.ParamCond{cond(ssid, domain.ParamOpLike, "%")}}), "c")
	// numerik: nilai non-angka tidak pernah cocok ('abc' bukan 0)
	same(list(domain.DeviceFilter{Params: []domain.ParamCond{cond(rx, domain.ParamOpLt, "-25")}}), "b")
	same(list(domain.DeviceFilter{Params: []domain.ParamCond{cond(rx, domain.ParamOpGe, "-21.5")}}), "a", "c")
	same(list(domain.DeviceFilter{Params: []domain.ParamCond{cond(uns, domain.ParamOpLt, "100")}}))
	same(list(domain.DeviceFilter{Params: []domain.ParamCond{cond(uns, domain.ParamOpGt, "-100")}}))
	// exists / missing
	same(list(domain.DeviceFilter{Params: []domain.ParamCond{cond(uns, domain.ParamOpExists, "")}}), "a", "b")
	same(list(domain.DeviceFilter{Params: []domain.ParamCond{cond(uns, domain.ParamOpMissing, "")}}), "c", "d")
	// AND beberapa kondisi
	same(list(domain.DeviceFilter{Params: []domain.ParamCond{cond(ver, domain.ParamOpPrefix, "V5."), cond(rx, domain.ParamOpLt, "-25")}}), "b")
	// injeksi pada nilai/nama hanya menjadi data (nilai di-bind; nama tak valid ditolak parser, bukan repo)
	same(list(domain.DeviceFilter{Params: []domain.ParamCond{cond(ssid, domain.ParamOpEq, "x' OR '1'='1")}}))

	// filter waktu Inform
	cut := now.Add(-90 * time.Minute)
	same(list(domain.DeviceFilter{InformBefore: &cut}), "a", "d")
	same(list(domain.DeviceFilter{InformAfter: &cut}), "b", "c")

	// sort: last_inform_at naik / turun; kolom tak dikenal ditolak
	order := func(sort string, desc bool) []string { return list(domain.DeviceFilter{Sort: sort, SortDesc: desc}) }
	asc, desc := order("last_inform_at", false), order("last_inform_at", true)
	wantAsc := []string{"a", "d", "b", "c"}
	for i := range wantAsc {
		if asc[i] != wantAsc[i] || desc[len(desc)-1-i] != wantAsc[i] {
			t.Fatalf("sort last_inform_at asc=%v desc=%v", asc, desc)
		}
	}
	if sn := order("serial_number", true); sn[0] != "d" {
		t.Fatalf("sort serial_number desc = %v", sn)
	}
	if _, _, err := repo.List(ctx, domain.DeviceFilter{Sort: "password; DROP TABLE devices"}, domain.Pagination{Page: 1, PageSize: 5}); !errors.Is(err, domain.ErrInvalidInput) {
		t.Fatalf("sort tak dikenal harus ErrInvalidInput, got %v", err)
	}

	// ValuesFor: hanya nama yang diminta, hanya yang terisi, per device
	vals, err := params.ValuesFor(ctx, []uint64{ids["a"], ids["c"], ids["d"]}, []string{ver, ssid, "Tidak.Ada"})
	if err != nil {
		t.Fatalf("ValuesFor: %v", err)
	}
	if vals[ids["a"]][ver] != "V5.2.1" || vals[ids["c"]][ssid] != "Kantor 100%" || len(vals[ids["a"]]) != 2 {
		t.Fatalf("ValuesFor salah: %v", vals)
	}
	if _, ok := vals[ids["d"]]; ok {
		t.Fatalf("device tanpa parameter tidak boleh punya entri: %v", vals)
	}
}
