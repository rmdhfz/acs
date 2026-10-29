package domain

import (
	"context"
	"fmt"
	"math"
	"strconv"
	"strings"
	"time"
)

// ---- Peta perangkat: titik ringan bergeolokasi, dibatasi tenant, memakai filter daftar yang sama ----

// MaxMapPoints membatasi titik per permintaan peta (satu halaman peta, bukan ekspor). Lebih dari ini = Truncated, dan klien
// mempersempit lewat filter atau bbox.
const MaxMapPoints = 5000

// BBox = persegi geografis dalam derajat. West > East berarti persegi melintasi garis tanggal internasional (bujur 180).
type BBox struct{ West, South, East, North float64 }

// ParseBBox mengurai "barat,selatan,timur,utara" (urutan GeoJSON/Leaflet). Semua harus angka hingga, lintang -90..90 dengan
// selatan <= utara, bujur -180..180.
func ParseBBox(raw string) (*BBox, error) {
	parts := strings.Split(raw, ",")
	if len(parts) != 4 {
		return nil, fmt.Errorf("%w: bbox harus berbentuk barat,selatan,timur,utara", ErrInvalidInput)
	}
	var v [4]float64
	for i, p := range parts {
		f, err := strconv.ParseFloat(strings.TrimSpace(p), 64)
		if err != nil || math.IsNaN(f) || math.IsInf(f, 0) {
			return nil, fmt.Errorf("%w: bbox memuat angka tidak valid", ErrInvalidInput)
		}
		v[i] = f
	}
	b := &BBox{West: v[0], South: v[1], East: v[2], North: v[3]}
	if b.South < -90 || b.North > 90 || b.South > b.North {
		return nil, fmt.Errorf("%w: lintang bbox harus -90..90 dengan selatan <= utara", ErrInvalidInput)
	}
	if b.West < -180 || b.West > 180 || b.East < -180 || b.East > 180 {
		return nil, fmt.Errorf("%w: bujur bbox harus -180..180", ErrInvalidInput)
	}
	return b, nil
}

// ValidCoordinate memeriksa pasangan lintang/bujur yang masuk akal (bukan 0,0 yang kerap berarti "belum diisi").
func ValidCoordinate(lat, lon float64) bool {
	if math.IsNaN(lat) || math.IsNaN(lon) || math.IsInf(lat, 0) || math.IsInf(lon, 0) {
		return false
	}
	return lat >= -90 && lat <= 90 && lon >= -180 && lon <= 180
}

// DeviceMapPoint = satu perangkat di peta. Status = kode ref_device_status (diisi layanan dari DeviceStatusID).
type DeviceMapPoint struct {
	ID             uint64     `db:"id" json:"id"`
	DeviceUUID     string     `db:"device_uuid" json:"device_uuid"`
	SerialNumber   string     `db:"serial_number" json:"serial_number"`
	ProductClass   *string    `db:"product_class" json:"product_class"`
	Latitude       float64    `db:"latitude" json:"latitude"`
	Longitude      float64    `db:"longitude" json:"longitude"`
	DeviceStatusID uint64     `db:"device_status_id" json:"-"`
	Status         string     `db:"-" json:"status"`
	LastInformAt   *time.Time `db:"last_inform_at" json:"last_inform_at"`
}

// DeviceMapResult = titik peta + hitungan. Total = perangkat yang cocok filter; Located = di antaranya yang punya koordinat
// (Total - Located = belum punya lokasi, tak bisa digambar); Truncated = titik dalam bbox melebihi batas dan dipotong.
type DeviceMapResult struct {
	Points    []DeviceMapPoint `json:"data"`
	Total     int              `json:"total"`
	Located   int              `json:"located"`
	Truncated bool             `json:"truncated"`
}

// DeviceMapReader dibaca repositori perangkat; antarmuka terpisah agar kontrak DeviceRepository tidak melebar.
type DeviceMapReader interface {
	ListForMap(ctx context.Context, f DeviceFilter, box *BBox, limit int) (DeviceMapResult, error)
}
