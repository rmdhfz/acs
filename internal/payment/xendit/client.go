// Package xendit adalah klien HTTP kecil untuk Xendit Invoice API v2 (membuat dan membaca invoice).
//
// Keputusan keamanan:
//   - autentikasi HTTP Basic (username = kunci rahasia, password kosong); kunci TIDAK PERNAH masuk ke pesan galat,
//     URL, atau log -- galat dibangun dari status HTTP + potongan body yang sudah disanitasi;
//   - timeout 15 detik, batas ukuran respons 1 MiB, redirect TIDAK diikuti (kunci tidak boleh ikut terbawa ke host lain);
//   - id invoice divalidasi (huruf/angka/-/_) sebelum masuk ke path URL (anti path injection);
//   - nominal selalu bilangan bulat rupiah.
//
// ASUMSI tentang Xendit (berdasarkan dokumentasi Invoice API v2; BELUM diuji terhadap Xendit nyata/sandbox):
//   - POST {base}/v2/invoices -> JSON berisi id, external_id, status, invoice_url, expiry_date, amount;
//   - GET  {base}/v2/invoices/{id} -> status PENDING|PAID|SETTLED|EXPIRED, amount, paid_amount, paid_at, external_id;
//   - angka bisa datang sebagai bilangan bulat atau desimal ".0"; tanggal berformat ISO-8601/RFC 3339.
package xendit

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"math"
	"net/http"
	"net/url"
	"regexp"
	"strconv"
	"strings"
	"time"
	"unicode"

	"acs/internal/domain"
)

const (
	// DefaultBaseURL = API produksi Xendit (mode test memakai host yang sama dengan kunci development).
	DefaultBaseURL = "https://api.xendit.co"

	requestTimeout = 15 * time.Second
	maxResponse    = 1 << 20 // 1 MiB
	maxErrSnippet  = 200
)

var (
	idRe = regexp.MustCompile(`^[A-Za-z0-9_-]{1,64}$`)
	// keyLike = apa pun yang mirip kunci Xendit (xnd_development_..., xnd_production_...) atau header Basic.
	keyLike   = regexp.MustCompile(`(?i)xnd_[A-Za-z0-9_]+|basic\s+[A-Za-z0-9+/=_-]{8,}`)
	longToken = regexp.MustCompile(`[A-Za-z0-9+/=_-]{32,}`)
)

// Client memenuhi domain.PaymentGateway.
type Client struct {
	baseURL string
	secret  string
	http    *http.Client
}

var _ domain.PaymentGateway = (*Client)(nil)

// New membuat klien. baseURL kosong = DefaultBaseURL. Kunci rahasia wajib.
func New(secretKey, baseURL string) (*Client, error) {
	if strings.TrimSpace(secretKey) == "" {
		return nil, errors.New("xendit: kunci rahasia kosong")
	}
	if baseURL == "" {
		baseURL = DefaultBaseURL
	}
	u, err := url.Parse(baseURL)
	if err != nil || u.Host == "" || (u.Scheme != "https" && u.Scheme != "http") || u.User != nil {
		return nil, errors.New("xendit: base URL tidak valid")
	}
	return &Client{
		baseURL: strings.TrimRight(baseURL, "/"),
		secret:  secretKey,
		http: &http.Client{
			Timeout: requestTimeout,
			// Jangan ikuti redirect: respons 3xx diperlakukan sebagai galat (kredensial tidak ikut ke host lain).
			CheckRedirect: func(*http.Request, []*http.Request) error { return http.ErrUseLastResponse },
		},
	}, nil
}

// ---- bentuk JSON ----

type createRequest struct {
	ExternalID         string        `json:"external_id"`
	Amount             uint64        `json:"amount"`
	PayerEmail         string        `json:"payer_email,omitempty"`
	Description        string        `json:"description,omitempty"`
	Currency           string        `json:"currency"`
	InvoiceDuration    int           `json:"invoice_duration,omitempty"`
	SuccessRedirectURL string        `json:"success_redirect_url,omitempty"`
	FailureRedirectURL string        `json:"failure_redirect_url,omitempty"`
	Items              []requestItem `json:"items,omitempty"`
}

type requestItem struct {
	Name     string `json:"name"`
	Quantity int    `json:"quantity"`
	Price    uint64 `json:"price"`
}

type invoiceResponse struct {
	ID         string      `json:"id"`
	ExternalID string      `json:"external_id"`
	Status     string      `json:"status"`
	InvoiceURL string      `json:"invoice_url"`
	ExpiryDate string      `json:"expiry_date"`
	Amount     json.Number `json:"amount"`
	PaidAmount json.Number `json:"paid_amount"`
	PaidAt     string      `json:"paid_at"`
}

// CreateInvoice -> POST /v2/invoices.
func (c *Client) CreateInvoice(ctx context.Context, in domain.GatewayInvoiceInput) (*domain.GatewayInvoice, error) {
	req := createRequest{
		ExternalID: in.ExternalID, Amount: in.AmountIDR, PayerEmail: in.PayerEmail, Description: in.Description,
		Currency: "IDR", InvoiceDuration: in.DurationSeconds,
		SuccessRedirectURL: in.SuccessRedirectURL, FailureRedirectURL: in.FailureRedirectURL,
	}
	for _, it := range in.Items {
		req.Items = append(req.Items, requestItem{Name: it.Name, Quantity: it.Quantity, Price: it.PriceIDR})
	}
	body, err := json.Marshal(req)
	if err != nil {
		return nil, errors.New("xendit: gagal menyusun permintaan")
	}
	return c.do(ctx, http.MethodPost, "/v2/invoices", body)
}

// GetInvoice -> GET /v2/invoices/{id}.
func (c *Client) GetInvoice(ctx context.Context, id string) (*domain.GatewayInvoice, error) {
	if !idRe.MatchString(id) {
		return nil, errors.New("xendit: id invoice tidak valid")
	}
	return c.do(ctx, http.MethodGet, "/v2/invoices/"+url.PathEscape(id), nil)
}

func (c *Client) do(ctx context.Context, method, path string, body []byte) (*domain.GatewayInvoice, error) {
	var rd io.Reader
	if body != nil {
		rd = bytes.NewReader(body)
	}
	req, err := http.NewRequestWithContext(ctx, method, c.baseURL+path, rd)
	if err != nil {
		return nil, errors.New("xendit: gagal membuat permintaan")
	}
	req.SetBasicAuth(c.secret, "")
	req.Header.Set("Accept", "application/json")
	if body != nil {
		req.Header.Set("Content-Type", "application/json")
	}
	resp, err := c.http.Do(req)
	if err != nil {
		// Galat transport (timeout, DNS, TLS). Disanitasi: tidak ada kunci di sini, tapi tetap disaring.
		return nil, fmt.Errorf("xendit: permintaan gagal: %s", c.sanitize(err.Error()))
	}
	defer resp.Body.Close()

	data, err := io.ReadAll(io.LimitReader(resp.Body, maxResponse+1))
	if err != nil {
		return nil, errors.New("xendit: gagal membaca respons")
	}
	if len(data) > maxResponse {
		return nil, errors.New("xendit: respons melebihi batas ukuran")
	}
	if resp.StatusCode < 200 || resp.StatusCode > 299 {
		return nil, fmt.Errorf("xendit: HTTP %d: %s", resp.StatusCode, c.sanitize(string(data)))
	}
	var r invoiceResponse
	dec := json.NewDecoder(bytes.NewReader(data))
	dec.UseNumber()
	if err := dec.Decode(&r); err != nil {
		return nil, errors.New("xendit: respons bukan JSON invoice yang valid")
	}
	return c.convert(&r)
}

func (c *Client) convert(r *invoiceResponse) (*domain.GatewayInvoice, error) {
	if !idRe.MatchString(r.ID) {
		return nil, errors.New("xendit: respons tanpa id invoice yang valid")
	}
	amount, err := parseRupiah(r.Amount)
	if err != nil {
		return nil, errors.New("xendit: nominal amount pada respons tidak valid")
	}
	paid, err := parseRupiah(r.PaidAmount)
	if err != nil {
		return nil, errors.New("xendit: nominal paid_amount pada respons tidak valid")
	}
	out := &domain.GatewayInvoice{
		ID: r.ID, ExternalID: r.ExternalID, Status: strings.ToUpper(strings.TrimSpace(r.Status)),
		InvoiceURL: r.InvoiceURL, AmountIDR: amount, PaidAmountIDR: paid,
	}
	if t, ok := parseTime(r.ExpiryDate); ok {
		out.ExpiryDate = &t
	}
	if t, ok := parseTime(r.PaidAt); ok {
		out.PaidAt = &t
	}
	return out, nil
}

// parseRupiah menerima bilangan bulat (atau desimal bernilai bulat, mis. "199000.0"); kosong = 0.
func parseRupiah(n json.Number) (uint64, error) {
	s := string(n)
	if s == "" {
		return 0, nil
	}
	if v, err := strconv.ParseUint(s, 10, 64); err == nil {
		return v, nil
	}
	f, err := strconv.ParseFloat(s, 64)
	if err != nil || f < 0 || f != math.Trunc(f) || f > 1e15 {
		return 0, errors.New("bukan rupiah bulat")
	}
	return uint64(f), nil
}

func parseTime(s string) (time.Time, bool) {
	if s == "" {
		return time.Time{}, false
	}
	for _, layout := range []string{time.RFC3339Nano, time.RFC3339, "2006-01-02T15:04:05.000Z0700"} {
		if t, err := time.Parse(layout, s); err == nil {
			return t, true
		}
	}
	return time.Time{}, false
}

// sanitize memotong ke <= 200 karakter, membuang karakter kontrol, dan menutupi apa pun yang mirip kunci/token.
func (c *Client) sanitize(s string) string {
	if c.secret != "" {
		s = strings.ReplaceAll(s, c.secret, "[disamarkan]")
	}
	s = keyLike.ReplaceAllString(s, "[disamarkan]")
	s = longToken.ReplaceAllString(s, "[disamarkan]")
	var b strings.Builder
	for _, r := range s {
		if unicode.IsControl(r) {
			b.WriteRune(' ')
			continue
		}
		b.WriteRune(r)
	}
	out := strings.Join(strings.Fields(b.String()), " ")
	if rs := []rune(out); len(rs) > maxErrSnippet {
		out = string(rs[:maxErrSnippet]) + "…"
	}
	return out
}
