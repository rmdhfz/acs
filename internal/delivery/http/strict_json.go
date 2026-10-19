package http

import (
	"bytes"
	"encoding/json"
	"errors"

	"github.com/labstack/echo/v5"
)

// StrictJSONSerializer — seperti echo.DefaultJSONSerializer, tapi Deserialize
// menolak field asing di body (DisallowUnknownFields) alih-alih diam-diam
// mengabaikannya. Ditemukan di audit validasi masukan (R9, CHECKLIST_SAAS.md
// §D) sebagai gap yang belum ditutup: json.Unmarshal (dipakai
// DefaultJSONSerializer) tidak pernah menolak field yang tidak dikenal
// request DTO, jadi klien bisa mengirim field tambahan tanpa ada yang
// memberi tahu (typo nama field pun lolos senyap, bukan 400 yang jelas).
//
// Hanya mempengaruhi Content-Type application/json (satu-satunya jalur yang
// didelegasikan DefaultBinder ke JSONSerializer.Deserialize); query/path
// param binding dan form/multipart tidak tersentuh. Dipasang di restEcho
// saja (lihat cmd/acsd/main.go) — endpoint CWMP/USP memakai XML/SOAP, bukan
// JSONSerializer, jadi tidak relevan di sana.
type StrictJSONSerializer struct {
	echo.DefaultJSONSerializer
}

// Deserialize membaca body lalu decode dengan DisallowUnknownFields. Pesan
// error TIDAK dipantulkan ke klien oleh handler manapun (lihat
// docs/laporan/PROGRESS.md LANJUTAN 13: "pesan Bind tidak lagi
// dipantulkan") — handler selalu mengembalikan "payload tidak valid" generik,
// jadi detail nama field asing di error ini aman tidak pernah sampai ke
// response, cukup untuk log server bila diperlukan.
func (s StrictJSONSerializer) Deserialize(c *echo.Context, target any) error {
	req := c.Request()
	if req.ContentLength == 0 {
		return nil
	}
	dec := json.NewDecoder(req.Body)
	dec.DisallowUnknownFields()
	if err := dec.Decode(target); err != nil {
		var hErr *echo.HTTPError
		if errors.As(err, &hErr) {
			return err
		}
		return echo.ErrBadRequest.Wrap(err)
	}
	// json.Decoder tidak menolak data tersisa setelah objek pertama (mis.
	// `{}{}` atau sampah di belakang) — json.Unmarshal (dipakai serializer
	// bawaan) menolaknya. Samakan perilakunya: body harus persis satu nilai
	// JSON. Buffered() sendiri tidak cukup (dokumentasinya eksplisit: harus
	// disambung dengan sisa io.Reader untuk mendapat seluruh byte setelah
	// nilai JSON terakhir) — baca keduanya sebelum menilai ada sisa atau
	// tidak.
	var rest bytes.Buffer
	rest.ReadFrom(dec.Buffered())
	rest.ReadFrom(req.Body)
	if len(bytes.TrimSpace(rest.Bytes())) > 0 {
		return echo.ErrBadRequest.Wrap(errors.New("unexpected trailing data after JSON value"))
	}
	return nil
}
