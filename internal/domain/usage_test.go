package domain

import (
	"errors"
	"fmt"
	"strings"
	"testing"
)

func TestNewUsageConflict(t *testing.T) {
	err := NewUsageConflict(`vendor "ZTE"`, []UsageCount{{Kind: "device", Count: 2}, {Kind: "firmware", Count: 1}})

	// Tetap ErrConflict (handler memetakannya ke HTTP 409), termasuk setelah dibungkus.
	if !errors.Is(err, ErrConflict) {
		t.Fatal("harus errors.Is(ErrConflict)")
	}
	wrapped := fmt.Errorf("%w — nonaktifkan dulu", err)
	if !errors.Is(wrapped, ErrConflict) {
		t.Fatal("pembungkus fmt.Errorf(%w) harus tetap ErrConflict")
	}

	// Pesannya langsung menjelaskan penyebab — BUKAN teks ErrConflict polos
	// "data sudah ada / konflik unique" yang menyesatkan untuk guard hapus.
	msg := wrapped.Error()
	for _, want := range []string{`vendor "ZTE"`, "masih dipakai oleh", "2 device", "1 firmware", "nonaktifkan dulu"} {
		if !strings.Contains(msg, want) {
			t.Errorf("pesan tidak memuat %q: %s", want, msg)
		}
	}
	if strings.Contains(msg, "konflik unique") || strings.Contains(msg, "domain:") {
		t.Errorf("pesan tidak boleh memuat teks ErrConflict polos / awalan teknis: %s", msg)
	}
	// Bukan error domain lain.
	if errors.Is(err, ErrNotFound) || errors.Is(err, ErrForbidden) {
		t.Fatal("tidak boleh cocok dengan error domain lain")
	}
}
