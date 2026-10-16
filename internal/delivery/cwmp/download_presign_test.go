package cwmp

import (
	"context"
	"errors"
	"io"
	"net/http"
	"strings"
	"testing"
	"time"

	"github.com/labstack/echo/v5"

	"acs/internal/domain"
	"acs/internal/usecase/session"
)

type fakeStorage struct{ err error }

func (fakeStorage) Upload(context.Context, string, io.Reader, int64, string) error { return nil }
func (f fakeStorage) PresignedGetURL(_ context.Context, key string, _ time.Duration) (string, error) {
	if f.err != nil {
		return "", f.err
	}
	return "https://presigned.example/" + key + "?sig=baru", nil
}
func (fakeStorage) Delete(context.Context, string) error { return nil }

func cpeWithStorage(t *testing.T, svc *fakeSessions, st domain.ObjectStorage) *mockCPE {
	t.Helper()
	e := echo.New()
	h := NewHandler(svc, nil)
	if st != nil {
		h = h.WithStorage(st)
	}
	h.Register(e, "/cwmp")
	cpe := &mockCPE{t: t, e: e, https: true}
	if rec := cpe.post(informXML(nsCWMP10), "u", "p"); rec.Code != 200 {
		t.Fatalf("Inform: %d", rec.Code)
	}
	return cpe
}

const downloadByKey = `{"file_type":"1 Firmware Upgrade Image","storage_key":"fw/a.bin","target_file_name":"a.bin"}`

// URL presigned dibuat SAAT RPC dikirim (bukan saat task dibuat, yang bisa berjam-jam sebelumnya).
func TestDownload_StorageKeyDiubahJadiURLPresignedSaatKirim(t *testing.T) {
	svc := &fakeSessions{nextQueue: []nextResult{{rpc: &session.OutboundRPC{
		TaskUUID: "u", TaskType: domain.TaskTypeDownload, Parameters: []byte(downloadByKey),
	}}}}
	rec := cpeWithStorage(t, svc, fakeStorage{}).post(emptyPost, "", "")
	out := rec.Body.String()
	if rec.Code != http.StatusOK || !strings.Contains(out, "Download") || !strings.Contains(out, "https://presigned.example/fw/a.bin?sig=baru") {
		t.Fatalf("HTTP %d, Download harus memuat URL presigned baru: %s", rec.Code, out)
	}
	if strings.Contains(out, "storage_key") || strings.Contains(out, "fw/a.bin&") {
		t.Fatalf("storage_key tidak boleh bocor ke CPE: %s", out)
	}
}

// Tanpa storage / storage error: task ditandai FAILED permanen (ACS-BUILD), CPE tidak menerima 500.
func TestDownload_StorageKeyTanpaStorageAtauGagal(t *testing.T) {
	for name, st := range map[string]domain.ObjectStorage{
		"storage tidak terpasang": nil,
		"storage error":           fakeStorage{err: errors.New("minio mati")},
	} {
		svc := &fakeSessions{nextQueue: []nextResult{{rpc: &session.OutboundRPC{
			TaskUUID: "u", TaskType: domain.TaskTypeDownload, Parameters: []byte(downloadByKey),
		}}}}
		rec := cpeWithStorage(t, svc, st).post(emptyPost, "", "")
		if rec.Code != http.StatusNoContent {
			t.Errorf("%s: HTTP %d, harus 204 (sesi lanjut)", name, rec.Code)
		}
		if len(svc.respIn) != 1 || svc.respIn[0].Fault == nil || svc.respIn[0].Fault.Code != domain.FaultCodeACSBuild {
			t.Errorf("%s: harus fault ACS-BUILD: %+v", name, svc.respIn)
		}
	}
}
