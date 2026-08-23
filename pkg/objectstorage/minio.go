// Package objectstorage adalah wrapper tipis di atas SDK MinIO (S3-compatible)
// — infra concern, dipakai usecase lewat interface domain.ObjectStorage
// (bukan dipakai langsung), analog pkg/cryptoutil untuk enkripsi kredensial.
package objectstorage

import (
	"context"
	"fmt"
	"io"
	"net/url"
	"time"

	"github.com/minio/minio-go/v7"
	"github.com/minio/minio-go/v7/pkg/credentials"
)

// Client mengimplementasikan domain.ObjectStorage dengan MinIO Go SDK.
type Client struct {
	mc     *minio.Client
	bucket string
}

// Config adalah parameter koneksi MinIO/S3-compatible. Endpoint & kredensial
// WAJIB berasal dari konfigurasi eksplisit (env var) — lihat internal/config,
// jangan pernah hardcode kredensial object storage.
type Config struct {
	Endpoint  string
	AccessKey string
	SecretKey string
	Bucket    string
	UseSSL    bool
}

// New membuat klien MinIO dan memastikan bucket target sudah ada (idempotent
// — aman dipanggil setiap kali acsd start; kalau bucket belum ada saat
// pertama kali start, dibuat otomatis alih-alih gagal).
func New(ctx context.Context, cfg Config) (*Client, error) {
	mc, err := minio.New(cfg.Endpoint, &minio.Options{
		Creds:  credentials.NewStaticV4(cfg.AccessKey, cfg.SecretKey, ""),
		Secure: cfg.UseSSL,
	})
	if err != nil {
		return nil, fmt.Errorf("objectstorage: gagal membuat klien minio: %w", err)
	}

	exists, err := mc.BucketExists(ctx, cfg.Bucket)
	if err != nil {
		return nil, fmt.Errorf("objectstorage: gagal cek bucket %q: %w", cfg.Bucket, err)
	}
	if !exists {
		// BucketExists lalu MakeBucket BUKAN operasi atomik -- acsd didesain
		// stateless/multi-instance (TECH.md §9), jadi beberapa instance bisa
		// start bersamaan menghadapi bucket yang sama-sama belum ada. Kalau
		// dua instance sama-sama lolos cek `!exists` lalu sama-sama panggil
		// MakeBucket, yang kalah race dapat error dari server MinIO
		// (BucketAlreadyOwnedByYou/BucketAlreadyExists) -- itu BUKAN
		// kegagalan sungguhan (bucket toh akhirnya ada), jadi diperlakukan
		// sbg sukses, bukan bikin acsd log.Fatalf saat startup (temuan
		// acs-code-reviewer, review fitur MinIO object storage).
		if err := mc.MakeBucket(ctx, cfg.Bucket, minio.MakeBucketOptions{}); err != nil {
			errResp := minio.ToErrorResponse(err)
			if errResp.Code != "BucketAlreadyOwnedByYou" && errResp.Code != "BucketAlreadyExists" {
				return nil, fmt.Errorf("objectstorage: gagal membuat bucket %q: %w", cfg.Bucket, err)
			}
		}
	}

	return &Client{mc: mc, bucket: cfg.Bucket}, nil
}

// Upload menyimpan isi reader sebagai objectKey di bucket yang dikonfigurasi.
func (c *Client) Upload(ctx context.Context, objectKey string, reader io.Reader, size int64, contentType string) error {
	_, err := c.mc.PutObject(ctx, c.bucket, objectKey, reader, size, minio.PutObjectOptions{
		ContentType: contentType,
	})
	if err != nil {
		return fmt.Errorf("objectstorage: upload %q gagal: %w", objectKey, err)
	}
	return nil
}

// PresignedGetURL menghasilkan URL GET sementara — dikirim langsung sebagai
// URL Download RPC CWMP ke CPE (TECH.md §7); lihat catatan di
// domain.ObjectStorage soal implikasi jangkauan jaringan.
func (c *Client) PresignedGetURL(ctx context.Context, objectKey string, expiry time.Duration) (string, error) {
	u, err := c.mc.PresignedGetObject(ctx, c.bucket, objectKey, expiry, url.Values{})
	if err != nil {
		return "", fmt.Errorf("objectstorage: presigned URL %q gagal: %w", objectKey, err)
	}
	return u.String(), nil
}

// Delete menghapus objectKey — dipakai untuk cleanup bila insert metadata ke
// DB gagal setelah file berhasil ter-upload.
func (c *Client) Delete(ctx context.Context, objectKey string) error {
	if err := c.mc.RemoveObject(ctx, c.bucket, objectKey, minio.RemoveObjectOptions{}); err != nil {
		return fmt.Errorf("objectstorage: delete %q gagal: %w", objectKey, err)
	}
	return nil
}
