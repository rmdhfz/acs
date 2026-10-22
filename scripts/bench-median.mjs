// Meringkas BEBERAPA run benchmark (bin/bench-*.json dari cmd/acsbench) menjadi median/min/maks per fase, dan membandingkan dua sistem.
//   node scripts/bench-median.mjs ours1.json ours2.json ours3.json --vs genie1.json genie2.json genie3.json
//   node scripts/bench-median.mjs run*.json                       # satu sistem saja
// Keluaran: tabel Markdown di stdout. Satu run tidak cukup untuk klaim: antar-run di mesin bersama bisa bergeser ±30%; laporkan median dan
// rentangnya, dan anggap berarti hanya selisih yang searah pada SEMUA run (kolom "searah").
import fs from 'node:fs'

const args = process.argv.slice(2)
const vsAt = args.indexOf('--vs')
const A = (vsAt < 0 ? args : args.slice(0, vsAt)).filter((a) => !a.startsWith('--'))
const B = vsAt < 0 ? [] : args.slice(vsAt + 1).filter((a) => !a.startsWith('--'))
if (A.length === 0) {
  console.error('pakai: node scripts/bench-median.mjs a1.json a2.json ... [--vs b1.json b2.json ...]')
  process.exit(2)
}

const load = (files) => files.map((f) => JSON.parse(fs.readFileSync(f, 'utf8')))
const median = (xs) => {
  const s = [...xs].sort((a, b) => a - b)
  const m = s.length >> 1
  return s.length % 2 ? s[m] : (s[m - 1] + s[m]) / 2
}
const fmt = (n, d = 0) => n.toFixed(d)

// fase -> { perSec: [...], p95: [...], p50: [...], failed }
function collect(runs) {
  const out = new Map()
  for (const r of runs) {
    for (const p of r.phases) {
      const k = p.name
      if (!out.has(k)) out.set(k, { perSec: [], p50: [], p95: [], failed: 0 })
      const e = out.get(k)
      e.perSec.push(p.sessions_per_sec)
      e.p50.push(p.p50_ms)
      e.p95.push(p.p95_ms)
      e.failed += p.failed || 0
    }
  }
  return out
}

const range = (xs, d = 0) => `${fmt(median(xs), d)} (${fmt(Math.min(...xs), d)}–${fmt(Math.max(...xs), d)})`

const ra = load(A)
const ca = collect(ra)
const rb = B.length ? load(B) : []
const cb = collect(rb)
const labelA = ra[0].label || 'A'
const labelB = rb[0]?.label || 'B'

console.log(`Run: ${labelA} n=${ra.length}${rb.length ? `, ${labelB} n=${rb.length}` : ''}. Sel = median (min–maks). ${ra[0].devices} perangkat, concurrency ${ra[0].concurrency}.\n`)
if (!rb.length) {
  console.log('| Fase | sesi/dtk | p50 ms | p95 ms | galat |')
  console.log('|---|---|---|---|---|')
  for (const [k, e] of ca) console.log(`| ${k} | ${range(e.perSec)} | ${range(e.p50)} | ${range(e.p95)} | ${e.failed} |`)
} else {
  console.log(`| Fase | ${labelA} sesi/dtk | ${labelB} sesi/dtk | ${labelA} p95 ms | ${labelB} p95 ms | selisih throughput | searah |`)
  console.log('|---|---|---|---|---|---|---|')
  for (const [k, e] of ca) {
    const g = cb.get(k)
    if (!g) continue
    const diff = (median(e.perSec) / median(g.perSec) - 1) * 100
    // "searah": SEMUA pasangan run (min A vs maks B, dan sebaliknya) memberi tanda yang sama => tidak ada tumpang-tindih rentang.
    const aWinsAll = Math.min(...e.perSec) > Math.max(...g.perSec)
    const bWinsAll = Math.max(...e.perSec) < Math.min(...g.perSec)
    const verdict = aWinsAll ? `${labelA} (tanpa tumpang-tindih)` : bWinsAll ? `${labelB} (tanpa tumpang-tindih)` : 'tumpang-tindih → setara'
    console.log(`| ${k} | ${range(e.perSec)} | ${range(g.perSec)} | ${range(e.p95)} | ${range(g.p95)} | ${diff >= 0 ? '+' : ''}${fmt(diff)}% | ${verdict} |`)
  }
}

// sumber daya: median rata-rata CPU dan memori maks per kontainer
function resources(runs) {
  const m = new Map()
  for (const r of runs) for (const x of r.resources || []) {
    if (!m.has(x.container)) m.set(x.container, { cpu: [], mem: [] })
    m.get(x.container).cpu.push(x.cpu_avg_pct)
    m.get(x.container).mem.push(x.mem_max_mib)
  }
  return m
}
for (const [label, runs] of [[labelA, ra], [labelB, rb]]) {
  if (!runs.length) continue
  console.log(`\nSumber daya ${label} (median antar-run):\n\n| Kontainer | CPU rata-rata % | memori maks MiB |\n|---|---|---|`)
  for (const [c, v] of resources(runs)) console.log(`| ${c} | ${fmt(median(v.cpu))} | ${fmt(median(v.mem), 1)} |`)
}
