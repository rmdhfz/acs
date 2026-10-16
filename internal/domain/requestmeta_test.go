package domain

import (
	"context"
	"testing"
)

func TestRequestMetaRoundTrip(t *testing.T) {
	if _, ok := RequestMetaFrom(context.Background()); ok {
		t.Fatal("konteks kosong tidak boleh punya meta")
	}
	ctx := WithRequestMeta(context.Background(), RequestMeta{IP: "203.0.113.9", UserAgent: "ua"})
	m, ok := RequestMetaFrom(ctx)
	if !ok || m.IP != "203.0.113.9" || m.UserAgent != "ua" {
		t.Fatalf("%+v %v", m, ok)
	}
}
