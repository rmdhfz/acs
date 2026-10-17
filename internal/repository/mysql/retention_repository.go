package mysql

import (
	"context"
	"strconv"
	"time"

	"github.com/jmoiron/sqlx"

	"acs/internal/domain"
)

type retentionRepository struct{ db *sqlx.DB }

func NewRetentionRepository(db *sqlx.DB) domain.RetentionRepository {
	return &retentionRepository{db: db}
}

// retentionBatch — baris per DELETE agar tidak mengunci tabel lama/lama (replikasi,
// undo log) saat pruning pertama atas data menahun.
const retentionBatch = 5000

// Pemangkasan FISIK pada tabel LOG volume tinggi ini adalah pengecualian
// eksplisit dari aturan soft-delete (CLAUDE.md §3: "cleanup data log lama sesuai
// kebijakan retensi eksplisit"). Tabel master/transaksional & activity_logs
// (audit trail) TIDAK disentuh.
func (r *retentionRepository) PruneLogs(ctx context.Context, cutoff time.Time) (map[string]int64, error) {
	steps := []struct{ name, query string }{
		// urutan penting: device_events mereferensikan device_sessions (FK)
		{"device_events", `DELETE FROM device_events WHERE occurred_at < ? LIMIT ` + strconv.Itoa(retentionBatch)},
		{"device_sessions", `DELETE FROM device_sessions WHERE status <> 'OPEN' AND started_at < ?
			AND NOT EXISTS (SELECT 1 FROM device_events e WHERE e.session_id = device_sessions.id) LIMIT ` + strconv.Itoa(retentionBatch)},
		{"device_optical_metrics", `DELETE FROM device_optical_metrics WHERE recorded_at < ? LIMIT ` + strconv.Itoa(retentionBatch)},
		{"webhook_deliveries", `DELETE FROM webhook_deliveries WHERE status IN ('DELIVERED','FAILED') AND created_at < ? LIMIT ` + strconv.Itoa(retentionBatch)},
		// notifications (migrations/0030): log notifikasi per penerima, audit minimal -- dihapus FISIK sesuai
		// kebijakan retensi (pengecualian soft-delete yang diizinkan untuk tabel log).
		{"notifications", `DELETE FROM notifications WHERE created_at < ? LIMIT ` + strconv.Itoa(retentionBatch)},
	}
	out := make(map[string]int64, len(steps))
	for _, st := range steps {
		for { // ulangi per batch sampai habis (dibatasi 200 batch per langkah per putaran)
			res, err := r.db.ExecContext(ctx, st.query, cutoff)
			if err != nil {
				return out, translateErr(err)
			}
			n, _ := res.RowsAffected()
			out[st.name] += n
			if n < retentionBatch || out[st.name] >= 200*retentionBatch {
				break
			}
		}
	}
	return out, nil
}
