// Command loadtest mensimulasikan banyak sesi CWMP concurrent (Inform ->
// InformResponse -> POST kosong penutup) untuk mengukur p95 latency Inform,
// sesuai NFR di ROADMAP.md Fase 3 ("Load test: ribuan sesi CWMP concurrent,
// ukur p95 latency Inform->InformResponse, target <300ms").
//
// Bukan pengganti uji fisik CPE nyata (PENGUJIAN_LAPANGAN.md) — device
// simulasi di sini hanya mengirim Inform minimal (event PERIODIC) dengan
// serial number unik per sesi, tanpa RPC lanjutan (GetParameterValues dkk),
// jadi angka yang keluar adalah batas atas kapasitas lapisan HTTP/auth/DB
// upsert device, bukan throughput RPC penuh.
//
// PENTING — rate limiter /cwmp: endpoint CWMP dibatasi 5 req/detik PER IP
// (cmd/acsd/main.go, `middleware.NewRateLimiterMemoryStore(5)`, sengaja
// dipasang sbg mitigasi brute-force kredensial, lihat ROADMAP.md Fase 0).
// Menjalankan tool ini dari SATU mesin/IP thd endpoint produksi/dev normal
// akan langsung kena 429 begitu concurrency > 5 — itu BUKAN bug, itu rate
// limiter bekerja sesuai desain. Untuk mengukur kapasitas backend yang
// sesungguhnya (p95 latency di atas throughput realistis), jalankan ini
// terhadap instance ACS terpisah dengan rate limiter dinaikkan/dimatikan
// SEMENTARA (ubah angka `5` di main.go, JANGAN pernah deploy begitu ke
// endpoint yang menjaga kredensial sungguhan) — atau tetap pakai -concurrency
// <=5 untuk memvalidasi latency di batas rate limit yang sengaja dipasang.
//
// Usage:
//
//	loadtest -url http://localhost:17547/cwmp -user <cwmp_inform_username> -pass <cwmp_inform_password> -sessions 1000 -concurrency 50
package main

import (
	"bytes"
	"flag"
	"fmt"
	"log"
	"math/rand"
	"net/http"
	"sort"
	"sync"
	"sync/atomic"
	"time"

	"acs/pkg/cwmpxml"
)

func buildInform(serial string) ([]byte, error) {
	// ns: simulasi CPE nyata yang mendeklarasikan cwmp-1-2 (mayoritas
	// implementasi TR-069) — lihat pkg/cwmpxml/rpc.go soal kenapa XMLName di
	// sini wajib di-set eksplisit sekarang (namespace tidak lagi hardcoded
	// di tag struct, lihat komentar lengkap di rpc.go).
	const ns = cwmpxml.NSCWMP
	env := cwmpxml.NewEnvelope("loadtest-1", ns, cwmpxml.Body{
		Inform: &cwmpxml.Inform{
			XMLName: cwmpxml.RPCName(ns, "Inform"),
			DeviceId: cwmpxml.DeviceIDStruct{
				Manufacturer: "LoadTest", OUI: "AABBCC", ProductClass: "LoadTestModel", SerialNumber: serial,
			},
			Event:        cwmpxml.EventList{Items: []cwmpxml.EventStruct{{EventCode: "2 PERIODIC"}}},
			MaxEnvelopes: 1,
			CurrentTime:  time.Now().UTC().Format(time.RFC3339),
			RetryCount:   0,
		},
	})
	return cwmpxml.Marshal(env)
}

type result struct {
	latency time.Duration
	err     error
	status  int
}

func runSession(client *http.Client, url, user, pass string, idx int) result {
	serial := fmt.Sprintf("LOADTEST-%08d-%d", idx, rand.Intn(1_000_000))
	body, err := buildInform(serial)
	if err != nil {
		return result{err: fmt.Errorf("build envelope: %w", err)}
	}

	start := time.Now()
	req, err := http.NewRequest(http.MethodPost, url, bytes.NewReader(body))
	if err != nil {
		return result{err: err}
	}
	req.SetBasicAuth(user, pass)
	req.Header.Set("Content-Type", "text/xml; charset=utf-8")
	resp, err := client.Do(req)
	if err != nil {
		return result{err: fmt.Errorf("Inform request: %w", err)}
	}
	resp.Body.Close()
	latency := time.Since(start)
	if resp.StatusCode != http.StatusOK {
		return result{latency: latency, status: resp.StatusCode, err: fmt.Errorf("Inform status %d (bukan 200)", resp.StatusCode)}
	}
	// Cookie sesi dari InformResponse — HARUS dibawa di POST kosong penutup,
	// kalau tidak ACS tidak bisa meresolve sesi & baris device_sessions
	// tertinggal status OPEN selamanya (persis kondisi 348 sesi basi yang
	// ditemukan 2026-08-31). client di sini sengaja tanpa cookie jar bersama
	// supaya sesi antar-goroutine tidak saling tercampur — jadi cookie
	// diteruskan manual.
	sessionCookies := resp.Cookies()

	// Tutup sesi dgn POST kosong (TECH.md §3 — sesi CWMP tetap terbuka sampai
	// tidak ada task lanjutan; device simulasi ini tidak punya task pending).
	closeReq, err := http.NewRequest(http.MethodPost, url, bytes.NewReader(nil))
	if err == nil {
		closeReq.SetBasicAuth(user, pass)
		for _, ck := range sessionCookies {
			closeReq.AddCookie(ck)
		}
		if closeResp, err := client.Do(closeReq); err == nil {
			closeResp.Body.Close()
		}
	}

	return result{latency: latency, status: resp.StatusCode}
}

func percentile(sorted []time.Duration, p float64) time.Duration {
	if len(sorted) == 0 {
		return 0
	}
	idx := int(float64(len(sorted)-1) * p)
	return sorted[idx]
}

func main() {
	url := flag.String("url", "http://localhost:17547/cwmp", "endpoint CWMP ACS")
	user := flag.String("user", "", "cwmp_inform_username tenant pilot (wajib)")
	pass := flag.String("pass", "", "cwmp_inform_password tenant pilot (wajib)")
	sessions := flag.Int("sessions", 1000, "jumlah sesi Inform simulasi")
	concurrency := flag.Int("concurrency", 50, "jumlah worker concurrent")
	rate := flag.Float64("rate", 0, "batasi laju MULAI sesi baru per detik (0 = tanpa batas -- akan langsung kena rate limiter /cwmp kalau > 5). Set <=5 utk mengukur latency sustained TANPA kena 429, mensimulasikan banyak device yg Inform terdistribusi dari waktu ke waktu alih-alih burst serentak dari satu IP.")
	timeout := flag.Duration("timeout", 10*time.Second, "timeout per request HTTP")
	flag.Parse()

	if *user == "" || *pass == "" {
		log.Fatal("wajib isi -user dan -pass (shared secret Inform CWMP tenant pilot, lihat PENGUJIAN_LAPANGAN.md)")
	}

	client := &http.Client{Timeout: *timeout}

	var (
		mu          sync.Mutex
		latencies   []time.Duration
		okCount     int64
		failCount   int64
		rateLimited int64
	)

	sem := make(chan struct{}, *concurrency)
	var wg sync.WaitGroup
	var ticker *time.Ticker
	if *rate > 0 {
		ticker = time.NewTicker(time.Duration(float64(time.Second) / *rate))
		defer ticker.Stop()
	}
	log.Printf("mulai load test: %d sesi, concurrency %d, rate %.1f/s, target %s", *sessions, *concurrency, *rate, *url)
	start := time.Now()

	for i := 0; i < *sessions; i++ {
		if ticker != nil {
			<-ticker.C
		}
		wg.Add(1)
		sem <- struct{}{}
		go func(idx int) {
			defer wg.Done()
			defer func() { <-sem }()
			r := runSession(client, *url, *user, *pass, idx)
			if r.err != nil {
				n := atomic.AddInt64(&failCount, 1)
				if r.status == http.StatusTooManyRequests {
					atomic.AddInt64(&rateLimited, 1)
				}
				if idx < 5 || n <= 10 {
					log.Printf("sesi %d gagal: %v", idx, r.err)
				}
				return
			}
			atomic.AddInt64(&okCount, 1)
			mu.Lock()
			latencies = append(latencies, r.latency)
			mu.Unlock()
		}(i)
	}
	wg.Wait()
	elapsed := time.Since(start)

	sort.Slice(latencies, func(i, j int) bool { return latencies[i] < latencies[j] })

	fmt.Println()
	fmt.Println("=== Hasil Load Test CWMP (Inform -> InformResponse) ===")
	fmt.Printf("Total sesi     : %d (berhasil %d, gagal %d)\n", *sessions, okCount, failCount)
	fmt.Printf("Durasi total   : %s\n", elapsed)
	if okCount > 0 {
		fmt.Printf("Throughput     : %.1f sesi/detik\n", float64(okCount)/elapsed.Seconds())
		fmt.Printf("Latency p50    : %s\n", percentile(latencies, 0.50))
		fmt.Printf("Latency p95    : %s (target NFR: <300ms)\n", percentile(latencies, 0.95))
		fmt.Printf("Latency p99    : %s\n", percentile(latencies, 0.99))
		fmt.Printf("Latency max    : %s\n", latencies[len(latencies)-1])
	}
	if failCount > 0 {
		fmt.Printf("\nPERINGATAN: %d/%d sesi gagal — cek log di atas sebelum menyimpulkan kapasitas.\n", failCount, *sessions)
	}
	if rateLimited > 0 {
		fmt.Printf("%d dari kegagalan itu adalah 429 (rate limiter /cwmp, 5 req/s per IP) — ini EKSPEKTASI kalau -concurrency > 5, bukan bug backend. Lihat komentar di atas main() soal cara mengukur kapasitas asli.\n", rateLimited)
	}
}
