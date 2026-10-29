package session

import (
	"context"
	"testing"
	"time"

	"acs/internal/domain"
	"acs/internal/usecase/device"
	"acs/internal/usecase/task"
)

// Penangkapan & penemuan interval Periodic Inform (migrations/0046): dasar ambang offline per perangkat.

type ivlDevices struct {
	domain.DeviceRepository
	calls []*uint32
}

func (r *ivlDevices) SetInformInterval(_ context.Context, _ uint64, sec *uint32) error {
	r.calls = append(r.calls, sec)
	return nil
}

func u32p(v uint32) *uint32 { return &v }

func newIvlService(devs *ivlDevices) *Service {
	return &Service{deviceSvc: device.NewService(devs, nil, nil, nil, nil, nil, nil, nil, nil, nil, nil, nil, nil, nil)}
}

func TestNoteInformInterval(t *testing.T) {
	const name098 = "InternetGatewayDevice.ManagementServer.PeriodicInformInterval"
	const name181 = "Device.ManagementServer.PeriodicInformInterval"
	other := InformParameter{Name: "InternetGatewayDevice.DeviceInfo.SoftwareVersion", Value: "V1"}
	cases := []struct {
		name      string
		known     *uint32
		params    []InformParameter
		wantOK    bool
		wantValue *uint32
		wantCalls int
	}{
		{"TR-098 baru disimpan", nil, []InformParameter{other, {Name: name098, Value: "300"}}, true, u32p(300), 1},
		{"TR-181 baru disimpan", nil, []InformParameter{{Name: name181, Value: " 43200 "}}, true, u32p(43200), 1},
		{"nilai sama tidak menulis ulang", u32p(300), []InformParameter{{Name: name098, Value: "300"}}, true, u32p(300), 0},
		{"nilai berubah ditulis", u32p(300), []InformParameter{{Name: name098, Value: "600"}}, true, u32p(600), 1},
		{"tidak dilaporkan: tidak ada yang terjadi", nil, []InformParameter{other}, false, nil, 0},
		{"bukan angka diabaikan", nil, []InformParameter{{Name: name098, Value: "abc"}}, false, nil, 0},
		{"negatif diabaikan", nil, []InformParameter{{Name: name098, Value: "-5"}}, false, nil, 0},
		{"melebihi uint32 diabaikan", nil, []InformParameter{{Name: name098, Value: "4294967296"}}, false, nil, 0},
		{"nol = dimatikan, tersimpan NULL: tak ada yang perlu ditulis, tetap dianggap dilaporkan", nil, []InformParameter{{Name: name098, Value: "0"}}, true, nil, 0},
		{"nol setelah ada nilai: ditulis sebagai tidak diketahui", u32p(300), []InformParameter{{Name: name098, Value: "0"}}, true, nil, 1},
		{"akhiran harus segmen penuh", nil, []InformParameter{{Name: "X_Vendor.ManagementServerPeriodicInformInterval", Value: "300"}}, false, nil, 0},
	}
	for _, c := range cases {
		t.Run(c.name, func(t *testing.T) {
			devs := &ivlDevices{}
			got, ok := newIvlService(devs).noteInformInterval(context.Background(), 5, c.known, c.params)
			if ok != c.wantOK || (got == nil) != (c.wantValue == nil) || (got != nil && *got != *c.wantValue) {
				t.Errorf("got=%v ok=%v, mau %v ok=%v", got, ok, c.wantValue, c.wantOK)
			}
			if len(devs.calls) != c.wantCalls {
				t.Errorf("penulisan = %d, mau %d", len(devs.calls), c.wantCalls)
			}
		})
	}
}

// ---- penemuan lewat tugas baca ----

type ivlTasks struct {
	*engTasks
	pending bool
	asked   int
}

func (r *ivlTasks) HasPendingForDevice(context.Context, uint64) (bool, error) {
	r.asked++
	return r.pending, nil
}

type ivlParams struct {
	*engParams
	prefixCalls int
}

// HasPrefix dipanggil EnqueueInformIntervalPoll sebagai langkah pertama; false = model data belum diketahui (tanpa membuat task),
// sehingga tes ini dapat memastikan sebuah percobaan antre TERJADI tanpa memerlukan seluruh rantai CreateTask.
func (r *ivlParams) HasPrefix(context.Context, uint64, string) (bool, error) {
	r.prefixCalls++
	return false, nil
}

// attempts = jumlah percobaan antre: EnqueueInformIntervalPoll memeriksa dua root (TR-098 lalu TR-181), jadi dua panggilan HasPrefix.
func (r *ivlParams) attempts() int { return r.prefixCalls / 2 }

func newDiscoveryRig(pending bool) (*Service, *ivlTasks, *ivlParams) {
	tasks := &ivlTasks{engTasks: &engTasks{byID: map[uint64]*domain.Task{}, retries: map[uint64]int{}}, pending: pending}
	params := &ivlParams{engParams: &engParams{}}
	svc := &Service{taskSvc: task.NewService(tasks, nil, params, nil, nil, engRefs{}, nil, nil, nil, nil)}
	return svc, tasks, params
}

func TestPlanInformIntervalDiscovery(t *testing.T) {
	ctx := context.Background()

	t.Run("interval sudah diketahui: tidak merencanakan dan tidak menyentuh antrean", func(t *testing.T) {
		svc, tasks, params := newDiscoveryRig(false)
		if run := svc.planInformIntervalDiscovery(ctx, &domain.Device{ID: 1, InformIntervalSeconds: u32p(300)}); run != nil {
			t.Fatal("tidak boleh ada rencana")
		}
		if tasks.asked != 0 || params.prefixCalls != 0 {
			t.Errorf("seharusnya tak ada akses: pending=%d prefix=%d", tasks.asked, params.prefixCalls)
		}
	})

	t.Run("sedang sibuk: tidak merencanakan DAN tidak mengunci percobaan berikutnya", func(t *testing.T) {
		svc, tasks, params := newDiscoveryRig(true)
		dev := &domain.Device{ID: 2}
		if run := svc.planInformIntervalDiscovery(ctx, dev); run != nil {
			t.Fatal("tak boleh merencanakan saat ada task terbuka")
		}
		if params.prefixCalls != 0 {
			t.Fatalf("tak boleh mengantre saat sibuk: prefix=%d", params.prefixCalls)
		}
		if _, locked := svc.intervalNext.Load(dev.ID); locked {
			t.Fatal("sibuk tidak boleh menulis intervalNext: perangkat ber-interval 12 jam tak akan pernah mendapat giliran")
		}
		// Inform berikutnya, kini tidak sibuk: direncanakan dan dijalankan.
		tasks.pending = false
		run := svc.planInformIntervalDiscovery(ctx, dev)
		if run == nil {
			t.Fatal("setelah tidak sibuk harus direncanakan")
		}
		run()
		if params.attempts() != 1 {
			t.Fatalf("percobaan harus terjadi: percobaan=%d", params.attempts())
		}
	})

	// Inti perbaikan: rencana dibuat SEBELUM poll lain, jadi task yang dibuat poll pada Inform yang sama tidak membatalkannya.
	t.Run("task yang muncul sesudah perencanaan (poll lain pada Inform yang sama) tidak membatalkan penemuan", func(t *testing.T) {
		svc, tasks, params := newDiscoveryRig(false)
		dev := &domain.Device{ID: 4}
		run := svc.planInformIntervalDiscovery(ctx, dev)
		if run == nil {
			t.Fatal("harus direncanakan saat perangkat idle")
		}
		tasks.pending = true // poll optik/PPPoE baru saja mengantre
		run()
		if params.attempts() != 1 {
			t.Fatalf("penemuan tetap harus diantre: percobaan=%d", params.attempts())
		}
	})

	t.Run("percobaan dikunci 24 jam agar perangkat yang tak mengungkapkannya tidak dibaca tiap Inform", func(t *testing.T) {
		svc, _, params := newDiscoveryRig(false)
		dev := &domain.Device{ID: 3}
		for i := 0; i < 3; i++ {
			if run := svc.planInformIntervalDiscovery(ctx, dev); run != nil {
				run()
			}
		}
		if params.attempts() != 1 {
			t.Fatalf("hanya satu percobaan yang diizinkan: percobaan=%d", params.attempts())
		}
		v, ok := svc.intervalNext.Load(dev.ID)
		if !ok {
			t.Fatal("intervalNext harus tercatat")
		}
		if d := time.Until(v.(time.Time)); d < 23*time.Hour || d > 24*time.Hour+time.Minute {
			t.Errorf("kunci = %v, mau ±24 jam", d)
		}
	})

	t.Run("perangkat berbeda tidak saling mengunci", func(t *testing.T) {
		svc, _, params := newDiscoveryRig(false)
		for _, id := range []uint64{10, 11} {
			if run := svc.planInformIntervalDiscovery(ctx, &domain.Device{ID: id}); run != nil {
				run()
			}
		}
		if params.attempts() != 2 {
			t.Errorf("percobaan=%d, mau 2", params.attempts())
		}
	})
}
