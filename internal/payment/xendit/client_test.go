package xendit

import (
	"context"
	"encoding/base64"
	"encoding/json"
	"io"
	"net/http"
	"net/http/httptest"
	"strings"
	"sync/atomic"
	"testing"
	"time"

	"acs/internal/domain"
)

const testKey = "xnd_development_RAHASIA1234567890abcdefABCDEF"

func newTestClient(t *testing.T, h http.Handler) (*Client, *httptest.Server) {
	t.Helper()
	srv := httptest.NewServer(h)
	t.Cleanup(srv.Close)
	c, err := New(testKey, srv.URL)
	if err != nil {
		t.Fatal(err)
	}
	return c, srv
}

func TestCreateInvoice_RequestShape(t *testing.T) {
	var gotAuth, gotCT, gotMethod, gotPath string
	var gotBody map[string]any
	c, _ := newTestClient(t, http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		gotAuth, gotCT, gotMethod, gotPath = r.Header.Get("Authorization"), r.Header.Get("Content-Type"), r.Method, r.URL.Path
		b, _ := io.ReadAll(r.Body)
		_ = json.Unmarshal(b, &gotBody)
		w.Header().Set("Content-Type", "application/json")
		w.WriteHeader(http.StatusOK)
		_, _ = w.Write([]byte(`{"id":"inv_123","external_id":"ACS-1-9-abcd","status":"PENDING","invoice_url":"https://checkout.xendit.co/web/inv_123","expiry_date":"2026-10-18T10:00:00.000Z","amount":199000}`))
	}))
	got, err := c.CreateInvoice(context.Background(), domain.GatewayInvoiceInput{
		ExternalID: "ACS-1-9-abcd", AmountIDR: 199000, PayerEmail: "a@b.co", Description: "Langganan STARTER",
		DurationSeconds: 86400, SuccessRedirectURL: "https://acs.example/billing?status=success", FailureRedirectURL: "https://acs.example/billing?status=failed",
	})
	if err != nil {
		t.Fatal(err)
	}
	wantAuth := "Basic " + base64.StdEncoding.EncodeToString([]byte(testKey+":"))
	if gotAuth != wantAuth {
		t.Errorf("header Authorization salah: %q", gotAuth)
	}
	if gotMethod != http.MethodPost || gotPath != "/v2/invoices" {
		t.Errorf("method/path salah: %s %s", gotMethod, gotPath)
	}
	if !strings.HasPrefix(gotCT, "application/json") {
		t.Errorf("content-type salah: %q", gotCT)
	}
	if gotBody["external_id"] != "ACS-1-9-abcd" || gotBody["amount"].(float64) != 199000 || gotBody["currency"] != "IDR" ||
		gotBody["payer_email"] != "a@b.co" || gotBody["invoice_duration"].(float64) != 86400 ||
		gotBody["success_redirect_url"] == "" || gotBody["failure_redirect_url"] == "" {
		t.Errorf("body tidak sesuai: %v", gotBody)
	}
	if got.ID != "inv_123" || got.Status != "PENDING" || got.AmountIDR != 199000 || got.ExternalID != "ACS-1-9-abcd" ||
		got.InvoiceURL != "https://checkout.xendit.co/web/inv_123" || got.ExpiryDate == nil || got.ExpiryDate.Year() != 2026 {
		t.Errorf("hasil parse salah: %+v", got)
	}
}

func TestGetInvoice_ParsesPaid(t *testing.T) {
	var gotPath, gotMethod string
	c, _ := newTestClient(t, http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		gotPath, gotMethod = r.URL.Path, r.Method
		_, _ = w.Write([]byte(`{"id":"inv_9","external_id":"X","status":"paid","amount":199000.0,"paid_amount":199000,"paid_at":"2026-10-17T03:04:05.123Z","ignored_field":{"a":1}}`))
	}))
	got, err := c.GetInvoice(context.Background(), "inv_9")
	if err != nil {
		t.Fatal(err)
	}
	if gotMethod != http.MethodGet || gotPath != "/v2/invoices/inv_9" {
		t.Errorf("method/path salah: %s %s", gotMethod, gotPath)
	}
	if got.Status != "PAID" || got.AmountIDR != 199000 || got.PaidAmountIDR != 199000 || got.PaidAt == nil {
		t.Errorf("hasil parse salah: %+v", got)
	}
}

func TestGetInvoice_RejectsBadID(t *testing.T) {
	var hits int32
	c, _ := newTestClient(t, http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) { atomic.AddInt32(&hits, 1) }))
	for _, id := range []string{"", "../admin", "a/b", "a b", "inv?x=1", strings.Repeat("a", 65), "id\n"} {
		if _, err := c.GetInvoice(context.Background(), id); err == nil {
			t.Errorf("id %q harus ditolak", id)
		}
	}
	if atomic.LoadInt32(&hits) != 0 {
		t.Error("id tidak valid tidak boleh sampai ke server")
	}
}

func TestResponseTooLarge(t *testing.T) {
	c, _ := newTestClient(t, http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		_, _ = w.Write([]byte(`{"id":"inv_1","status":"PENDING","pad":"`))
		_, _ = w.Write([]byte(strings.Repeat("x", 2<<20)))
		_, _ = w.Write([]byte(`"}`))
	}))
	_, err := c.GetInvoice(context.Background(), "inv_1")
	if err == nil || !strings.Contains(err.Error(), "batas ukuran") {
		t.Fatalf("respons >1 MiB harus ditolak, err=%v", err)
	}
}

func TestDoesNotFollowRedirect(t *testing.T) {
	var otherHits int32
	var otherAuth atomic.Value
	other := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		atomic.AddInt32(&otherHits, 1)
		otherAuth.Store(r.Header.Get("Authorization"))
		_, _ = w.Write([]byte(`{"id":"inv_1","status":"PAID","amount":1}`))
	}))
	defer other.Close()
	c, _ := newTestClient(t, http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		http.Redirect(w, r, other.URL+"/v2/invoices/inv_1", http.StatusFound)
	}))
	_, err := c.GetInvoice(context.Background(), "inv_1")
	if err == nil {
		t.Fatal("redirect harus menghasilkan galat")
	}
	if atomic.LoadInt32(&otherHits) != 0 {
		t.Fatalf("redirect diikuti (Authorization bocor ke host lain: %v)", otherAuth.Load())
	}
}

func TestErrorsNeverContainSecret(t *testing.T) {
	longBody := testKey + " " + strings.Repeat("Z", 5000) + " Authorization: Basic " + base64.StdEncoding.EncodeToString([]byte(testKey+":"))
	c, _ := newTestClient(t, http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		// Server nakal yang memantulkan kredensial di body galat.
		w.WriteHeader(http.StatusUnauthorized)
		_, _ = w.Write([]byte(longBody + "\x00\x1b[31m kontrol \n baris"))
	}))
	_, err := c.GetInvoice(context.Background(), "inv_1")
	if err == nil {
		t.Fatal("harus galat")
	}
	msg := err.Error()
	for _, bad := range []string{testKey, "xnd_", base64.StdEncoding.EncodeToString([]byte(testKey + ":")), "Basic eG5k"} {
		if strings.Contains(msg, bad) {
			t.Fatalf("galat memuat rahasia %q: %s", bad, msg)
		}
	}
	if !strings.Contains(msg, "HTTP 401") {
		t.Errorf("galat harus memuat status HTTP: %s", msg)
	}
	if n := len([]rune(msg)); n > 260 {
		t.Errorf("galat terlalu panjang (%d)", n)
	}
	if strings.ContainsAny(msg, "\x00\x1b\n") {
		t.Errorf("galat memuat karakter kontrol: %q", msg)
	}
}

func TestTransportErrorNeverContainsSecret(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {}))
	c, _ := New(testKey, srv.URL)
	srv.Close() // koneksi ditolak
	_, err := c.GetInvoice(context.Background(), "inv_1")
	if err == nil {
		t.Fatal("harus galat")
	}
	if strings.Contains(err.Error(), testKey) {
		t.Fatalf("galat transport memuat kunci: %v", err)
	}
}

func TestContextCancel(t *testing.T) {
	c, _ := newTestClient(t, http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		select {
		case <-r.Context().Done():
		case <-time.After(5 * time.Second):
		}
	}))
	ctx, cancel := context.WithTimeout(context.Background(), 100*time.Millisecond)
	defer cancel()
	start := time.Now()
	if _, err := c.GetInvoice(ctx, "inv_1"); err == nil {
		t.Fatal("harus galat")
	}
	if time.Since(start) > 3*time.Second {
		t.Error("context tidak dihormati")
	}
}

func TestInvalidResponses(t *testing.T) {
	cases := map[string]string{
		"bukan json":          `<html>`,
		"tanpa id":            `{"status":"PENDING","amount":1000}`,
		"nominal pecahan":     `{"id":"inv_1","status":"PENDING","amount":1000.5}`,
		"nominal negatif":     `{"id":"inv_1","status":"PENDING","amount":-5}`,
		"nominal teks":        `{"id":"inv_1","status":"PENDING","amount":"abc"}`,
		"id berkarakter aneh": `{"id":"inv/1","status":"PENDING","amount":1}`,
	}
	for name, body := range cases {
		t.Run(name, func(t *testing.T) {
			c, _ := newTestClient(t, http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) { _, _ = w.Write([]byte(body)) }))
			if _, err := c.GetInvoice(context.Background(), "inv_1"); err == nil {
				t.Errorf("respons %q harus ditolak", body)
			}
		})
	}
}

func TestNewValidation(t *testing.T) {
	if _, err := New("", ""); err == nil {
		t.Error("kunci kosong harus ditolak")
	}
	if _, err := New(testKey, "ftp://x"); err == nil {
		t.Error("skema selain http/https harus ditolak")
	}
	if _, err := New(testKey, "https://user:pw@api.xendit.co"); err == nil {
		t.Error("userinfo di URL harus ditolak")
	}
	c, err := New(testKey, "")
	if err != nil || c.baseURL != DefaultBaseURL {
		t.Errorf("base URL kosong harus memakai bawaan: %v %v", c, err)
	}
}
