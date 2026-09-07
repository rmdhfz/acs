package main

import (
	"bytes"
	"crypto/tls"
	"encoding/json"
	"fmt"
	"io"
	"mime/multipart"
	"net/http"
	"net/url"
	"strings"
	"time"
)

// Client adalah REST client tipis untuk mendorong ACS dari sisi operator
// (BSS/NOC) selama skenario e2e.
type Client struct {
	base  string
	http  *http.Client
	token string
}

func NewClient(base string, insecure bool) *Client {
	return &Client{
		base: strings.TrimRight(base, "/"),
		http: &http.Client{
			Timeout: 30 * time.Second,
			Transport: &http.Transport{
				TLSClientConfig: &tls.Config{InsecureSkipVerify: insecure}, //nolint:gosec // dev/e2e
			},
		},
	}
}

// WithToken mengembalikan salinan client yang memakai token berbeda (untuk
// menguji sebagai user tenant lain tanpa mengganggu sesi superadmin).
func (c *Client) WithToken(tok string) *Client {
	cp := *c
	cp.token = tok
	return &cp
}

type apiError struct {
	Status int
	Body   string
}

func (e *apiError) Error() string {
	return fmt.Sprintf("HTTP %d: %s", e.Status, strings.TrimSpace(e.Body))
}

// do menjalankan request JSON. out boleh nil. Mengembalikan apiError untuk
// status >= 400. Retry otomatis pada 429 (rate limiter REST 30/s, /auth/login
// 2/s) — di produksi tiap konsumen API punya kuota sendiri.
func (c *Client) do(method, path string, body, out any) error {
	var payload []byte
	if body != nil {
		b, err := json.Marshal(body)
		if err != nil {
			return err
		}
		payload = b
	}
	var lastErr error
	for attempt := 0; attempt < 5; attempt++ {
		var rdr io.Reader
		if payload != nil {
			rdr = bytes.NewReader(payload)
		}
		req, err := http.NewRequest(method, c.base+path, rdr)
		if err != nil {
			return err
		}
		if payload != nil {
			req.Header.Set("Content-Type", "application/json")
		}
		if c.token != "" {
			req.Header.Set("Authorization", "Bearer "+c.token)
		}
		resp, err := c.http.Do(req)
		if err != nil {
			return err
		}
		raw, _ := io.ReadAll(resp.Body)
		resp.Body.Close()
		if resp.StatusCode == http.StatusTooManyRequests {
			lastErr = &apiError{Status: 429, Body: string(raw)}
			time.Sleep(time.Duration(attempt+1) * 900 * time.Millisecond)
			continue
		}
		if resp.StatusCode >= 400 {
			return &apiError{Status: resp.StatusCode, Body: string(raw)}
		}
		if out != nil && len(bytes.TrimSpace(raw)) > 0 {
			if err := json.Unmarshal(raw, out); err != nil {
				return fmt.Errorf("decode %s %s: %w (body: %s)", method, path, err, snippet(raw))
			}
		}
		return nil
	}
	return lastErr
}

// status hanya mengembalikan kode status (untuk assertion 403/404/dst).
func (c *Client) status(method, path string, body any) (int, string) {
	err := c.do(method, path, body, nil)
	if err == nil {
		return 200, ""
	}
	if ae, ok := err.(*apiError); ok {
		return ae.Status, ae.Body
	}
	return 0, err.Error()
}

// uploadMultipart mengirim form multipart (untuk POST /firmware).
func (c *Client) uploadMultipart(path string, fields map[string]string, fileField, fileName string, data []byte, out any) error {
	var buf bytes.Buffer
	w := multipart.NewWriter(&buf)
	for k, v := range fields {
		_ = w.WriteField(k, v)
	}
	fw, err := w.CreateFormFile(fileField, fileName)
	if err != nil {
		return err
	}
	if _, err := fw.Write(data); err != nil {
		return err
	}
	_ = w.Close()

	req, err := http.NewRequest(http.MethodPost, c.base+path, &buf)
	if err != nil {
		return err
	}
	req.Header.Set("Content-Type", w.FormDataContentType())
	if c.token != "" {
		req.Header.Set("Authorization", "Bearer "+c.token)
	}
	resp, err := c.http.Do(req)
	if err != nil {
		return err
	}
	defer resp.Body.Close()
	raw, _ := io.ReadAll(resp.Body)
	if resp.StatusCode >= 400 {
		return &apiError{Status: resp.StatusCode, Body: string(raw)}
	}
	if out != nil && len(bytes.TrimSpace(raw)) > 0 {
		return json.Unmarshal(raw, out)
	}
	return nil
}

// --- helper endpoint ---

func (c *Client) Login(username, password string) error {
	var resp struct {
		AccessToken string `json:"access_token"`
	}
	if err := c.do(http.MethodPost, "/api/v1/auth/login", map[string]string{
		"username": username, "password": password,
	}, &resp); err != nil {
		return err
	}
	if resp.AccessToken == "" {
		return fmt.Errorf("login %s: access_token kosong", username)
	}
	c.token = resp.AccessToken
	return nil
}

type refLookup struct {
	ID   uint64 `json:"id"`
	Code string `json:"code"`
	Name string `json:"name"`
}

// refMap mengembalikan map code -> id untuk satu tabel ref_*.
func (c *Client) refMap(table string) (map[string]uint64, error) {
	var rows []refLookup
	if err := c.do(http.MethodGet, "/api/v1/refs/"+table, nil, &rows); err != nil {
		return nil, err
	}
	m := make(map[string]uint64, len(rows))
	for _, r := range rows {
		m[r.Code] = r.ID
	}
	return m, nil
}

type deviceJSON struct {
	ID              uint64  `json:"id"`
	TenantID        *uint64 `json:"tenant_id"`
	VendorID        *uint64 `json:"vendor_id"`
	OUI             *string `json:"oui"`
	SerialNumber    string  `json:"serial_number"`
	SoftwareVersion *string `json:"software_version"`
	DeviceStatusID  uint64  `json:"device_status_id"`
}

type listEnvelope[T any] struct {
	Data  []T `json:"data"`
	Total int `json:"total"`
}

func (c *Client) listDevices(query url.Values) ([]deviceJSON, error) {
	var env listEnvelope[deviceJSON]
	p := "/api/v1/devices"
	if len(query) > 0 {
		p += "?" + query.Encode()
	}
	if err := c.do(http.MethodGet, p, nil, &env); err != nil {
		return nil, err
	}
	return env.Data, nil
}

func (c *Client) findDeviceBySerial(serial string) (*deviceJSON, error) {
	q := url.Values{}
	q.Set("search", serial)
	devs, err := c.listDevices(q)
	if err != nil {
		return nil, err
	}
	for i := range devs {
		if devs[i].SerialNumber == serial {
			return &devs[i], nil
		}
	}
	return nil, fmt.Errorf("device %q tidak ditemukan", serial)
}

type taskJSON struct {
	ID           uint64  `json:"id"`
	DeviceID     uint64  `json:"device_id"`
	TaskStatusID uint64  `json:"task_status_id"`
	TaskTypeID   uint64  `json:"task_type_id"`
	TaskTypeCode string  `json:"task_type_code"` // hanya diisi sebagian query; fallback ke TaskTypeID
	RetryCount   uint32  `json:"retry_count"`
	ErrorMessage *string `json:"error_message"`
}

func (c *Client) listDeviceTasks(deviceID uint64) ([]taskJSON, error) {
	var env listEnvelope[taskJSON]
	q := url.Values{}
	q.Set("device_id", fmt.Sprint(deviceID))
	if err := c.do(http.MethodGet, "/api/v1/tasks?"+q.Encode(), nil, &env); err != nil {
		return nil, err
	}
	return env.Data, nil
}

type eventJSON struct {
	EventCodeID uint64 `json:"event_code_id"`
	Code        string `json:"code"`
	EventCode   string `json:"event_code"`
}

func (c *Client) listDeviceEvents(deviceID uint64) ([]eventJSON, error) {
	// GET /devices/:id/events -> {data,total} ATAU array — coba dua bentuk.
	raw := json.RawMessage{}
	if err := c.do(http.MethodGet, fmt.Sprintf("/api/v1/devices/%d/events", deviceID), nil, &raw); err != nil {
		return nil, err
	}
	var env listEnvelope[eventJSON]
	if json.Unmarshal(raw, &env) == nil && env.Data != nil {
		return env.Data, nil
	}
	var arr []eventJSON
	_ = json.Unmarshal(raw, &arr)
	return arr, nil
}

type paramJSON struct {
	ParameterName  string  `json:"parameter_name"`
	ParameterValue *string `json:"parameter_value"`
}

func (c *Client) listDeviceParameters(deviceID uint64) ([]paramJSON, error) {
	raw := json.RawMessage{}
	if err := c.do(http.MethodGet, fmt.Sprintf("/api/v1/devices/%d/parameters", deviceID), nil, &raw); err != nil {
		return nil, err
	}
	var env listEnvelope[paramJSON]
	if json.Unmarshal(raw, &env) == nil && env.Data != nil {
		return env.Data, nil
	}
	var arr []paramJSON
	_ = json.Unmarshal(raw, &arr)
	return arr, nil
}

func snippet(b []byte) string {
	s := strings.TrimSpace(string(b))
	if len(s) > 300 {
		return s[:300] + "..."
	}
	return s
}
