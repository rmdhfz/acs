package domain

import (
	"context"
	"io"
	"time"
)

// ObjectStorage adalah kontrak penyimpanan file biner (firmware, dst) di
// object storage S3-compatible (MinIO untuk dev/self-host — ROADMAP.md Fase
// 2, "strategi object storage firmware"). Usecase bergantung pada interface
// ini (bukan langsung ke pkg/objectstorage konkret) supaya testable dengan
// fake, pola sama seperti TenantRepository dkk (CLAUDE.md).
//
// Strategi delivery ke CPE (keputusan produk, TECH.md §7): presigned GET URL
// LANGSUNG dari object storage yang dikirim sebagai URL Download RPC CWMP —
// BUKAN proxy lewat ACS. Implikasi yang disengaja: object storage HARUS
// reachable dari jaringan CPE fisik, bukan cuma dari ACS/dashboard. Ini
// asumsi deployment yang disadari sepenuhnya, jangan "diperbaiki" jadi
// proxy tanpa diskusi eksplisit.
type ObjectStorage interface {
	// Upload menyimpan isi reader sebagai objectKey. size adalah jumlah byte
	// yang akan dibaca dari reader.
	Upload(ctx context.Context, objectKey string, reader io.Reader, size int64, contentType string) error
	// PresignedGetURL menghasilkan URL GET sementara (berlaku selama expiry)
	// yang bisa diakses langsung tanpa kredensial ACS/object storage.
	PresignedGetURL(ctx context.Context, objectKey string, expiry time.Duration) (string, error)
	// Delete menghapus objectKey — dipakai untuk cleanup bila proses upload
	// gagal di tengah jalan (mis. objek sudah tersimpan tapi insert metadata
	// ke DB gagal).
	Delete(ctx context.Context, objectKey string) error
}
