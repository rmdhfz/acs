package hotcache

import (
	"context"
	"testing"
	"time"

	"acs/internal/domain"
)

type fakeRules struct {
	domain.ZeroTouchRuleRepository
	calls int
	rules []domain.ZeroTouchRule
}

func (f *fakeRules) ListActiveOrdered(context.Context, *uint64) ([]domain.ZeroTouchRule, error) {
	f.calls++
	return f.rules, nil
}
func (f *fakeRules) Create(context.Context, *domain.ZeroTouchRule) error { return nil }

func TestZeroTouchRules_CachesWithinTTLAndInvalidatesOnMutation(t *testing.T) {
	inner := &fakeRules{rules: []domain.ZeroTouchRule{{Priority: 1}}}
	repo := WrapZeroTouchRules(inner, time.Minute)
	ctx := context.Background()
	tenant := uint64(7)

	for i := 0; i < 3; i++ {
		got, err := repo.ListActiveOrdered(ctx, &tenant)
		if err != nil || len(got) != 1 {
			t.Fatalf("list #%d: %v %v", i, got, err)
		}
	}
	if inner.calls != 1 {
		t.Fatalf("SELECT harus 1x selama TTL, got %d", inner.calls)
	}

	// tenant lain = kunci lain
	if _, _ = repo.ListActiveOrdered(ctx, nil); inner.calls != 2 {
		t.Fatalf("tenant berbeda harus query sendiri, calls=%d", inner.calls)
	}

	// mutasi membuang cache
	if err := repo.Create(ctx, &domain.ZeroTouchRule{}); err != nil {
		t.Fatal(err)
	}
	_, _ = repo.ListActiveOrdered(ctx, &tenant)
	if inner.calls != 3 {
		t.Fatalf("setelah Create harus query ulang, calls=%d", inner.calls)
	}
}

func TestZeroTouchRules_ExpiresAfterTTL(t *testing.T) {
	inner := &fakeRules{}
	repo := WrapZeroTouchRules(inner, 20*time.Millisecond)
	ctx := context.Background()
	_, _ = repo.ListActiveOrdered(ctx, nil)
	time.Sleep(40 * time.Millisecond)
	_, _ = repo.ListActiveOrdered(ctx, nil)
	if inner.calls != 2 {
		t.Fatalf("setelah TTL harus query ulang, calls=%d", inner.calls)
	}
}

func TestZeroTouchRules_CallerCannotCorruptCache(t *testing.T) {
	inner := &fakeRules{rules: []domain.ZeroTouchRule{{Priority: 1}, {Priority: 2}}}
	repo := WrapZeroTouchRules(inner, time.Minute)
	ctx := context.Background()
	first, _ := repo.ListActiveOrdered(ctx, nil)
	first[0].Priority = 99
	second, _ := repo.ListActiveOrdered(ctx, nil)
	if second[0].Priority != 1 {
		t.Fatalf("mutasi slice pemanggil bocor ke cache: %d", second[0].Priority)
	}
}

func TestWrap_TTLZeroDisablesCache(t *testing.T) {
	inner := &fakeRules{}
	if WrapZeroTouchRules(inner, 0) != domain.ZeroTouchRuleRepository(inner) {
		t.Fatal("ttl 0 harus mengembalikan repo asli")
	}
}

type fakePresets struct {
	domain.PresetRepository
	calls int
}

func (f *fakePresets) ListActiveEnforce(context.Context, *uint64) ([]domain.Preset, error) {
	f.calls++
	return []domain.Preset{{Name: "p"}}, nil
}
func (f *fakePresets) Update(context.Context, *domain.Preset) error { return nil }

func TestPresets_CachesAndInvalidatesOnUpdate(t *testing.T) {
	inner := &fakePresets{}
	repo := WrapPresets(inner, time.Minute)
	ctx := context.Background()
	_, _ = repo.ListActiveEnforce(ctx, nil)
	_, _ = repo.ListActiveEnforce(ctx, nil)
	if inner.calls != 1 {
		t.Fatalf("calls=%d", inner.calls)
	}
	_ = repo.Update(ctx, &domain.Preset{})
	_, _ = repo.ListActiveEnforce(ctx, nil)
	if inner.calls != 2 {
		t.Fatalf("setelah Update harus query ulang, calls=%d", inner.calls)
	}
}
