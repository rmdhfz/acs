#!/usr/bin/env node
// api-parity.mjs — bandingkan route REST backend dengan pemanggilan API di frontend.
//
// Tujuan: memastikan pekerjaan API tidak sia-sia — setiap endpoint yang dibuat di
// internal/delivery/http/router.go benar-benar dipakai frontend, dan tidak ada
// pemanggilan frontend yang menunjuk route yang tidak ada (calon 404 di produksi).
//
// Bagian ini MEKANIS (cakupan endpoint). Kesetiaan kontrak per-field
// (required/optional, minLength/maxLength, tipe) butuh penilaian dan dikerjakan
// agent `api-frontend-parity` — lihat .claude/agents/api-frontend-parity.md.
//
// Pemakaian:
//   node scripts/api-parity.mjs           # ringkasan + daftar temuan
//   node scripts/api-parity.mjs --json    # keluaran JSON untuk diproses lanjut
//
// Exit code: 1 bila ada panggilan frontend yang tidak cocok route mana pun
// (itu bug nyata), 0 bila hanya ada endpoint yang belum dipakai (informasi).

import { readFileSync, readdirSync, statSync } from 'node:fs'
import { join, extname } from 'node:path'

const HTTP_DIR = 'internal/delivery/http'
const FRONTEND_SRC = 'frontend/src'
const API_BASE = '/api/v1' // frontend memakai VITE_API_URL yang sudah memuat basis ini
const asJson = process.argv.includes('--json')

// `${...}` yang boleh memuat satu tingkat kurung kurawal bersarang,
// mis. ${buildQuery({ tenant_id: tenantId })}.
const INTERP = /\$\{(?:[^{}]|\{[^{}]*\})*\}/g

/** Samakan bentuk path: semua parameter jadi `:p` supaya bisa dibandingkan. */
function normalize(path) {
  let p = path.split('?')[0]
  p = p.replace(new RegExp('/' + INTERP.source, 'g'), '/:p') // /${id} -> /:p (param path)
  p = p.replace(INTERP, '') //                                  ${buildQuery()} -> '' (query)
  p = p.replace(/\/:[A-Za-z_][A-Za-z0-9_]*/g, '/:p') //          /:id -> /:p (gaya Echo)
  if (p.length > 1 && p.endsWith('/')) p = p.slice(0, -1)
  return p || '/'
}

// ---------- 1. Route backend dari SEMUA file yang mendaftarkan route ----------
// Route tidak hanya di router.go: selfservice_handler.go mendaftarkan grupnya
// sendiri (`ss := api.Group("/self-service", ...)`). Prefix grup harus ikut
// diperhitungkan, kalau tidak seluruh /self-service terbaca hilang.
function parseBackend() {
  const files = readdirSync(HTTP_DIR)
    .filter((f) => f.endsWith('.go'))
    .map((f) => join(HTTP_DIR, f))

  // Kumpulkan definisi grup dulu dari seluruh file: nama -> {parent, prefix}
  const groupDefs = new Map()
  const sources = new Map()
  for (const file of files) {
    const src = readFileSync(file, 'utf8')
    sources.set(file, src)
    const re = /\b([A-Za-z_][A-Za-z0-9_]*)\s*:?=\s*([A-Za-z_][A-Za-z0-9_.]*)\.Group\(\s*"([^"]*)"([^\n]*)/g
    let m
    while ((m = re.exec(src)) !== null) {
      const [, name, parent, prefix, rest] = m
      groupDefs.set(name, { parent, prefix, rest })
    }
  }

  const resolve = (name, depth = 0) => {
    if (depth > 8) return { prefix: '', roles: null, authed: false }
    const def = groupDefs.get(name)
    if (!def) return { prefix: '', roles: null, authed: false }
    const up = /\.Group|^e$/.test(def.parent) ? { prefix: '', roles: null, authed: false } : resolve(def.parent, depth + 1)
    const roles = /RequireRoles\(([A-Za-z0-9_.]+)/.exec(def.rest)
    return {
      prefix: up.prefix + def.prefix,
      roles: roles ? roles[1] : up.roles,
      authed: up.authed || /AuthMiddleware/.test(def.rest),
    }
  }

  const routes = []
  for (const [file, src] of sources) {
    const re = /\b([A-Za-z_][A-Za-z0-9_]*)\.(GET|POST|PUT|PATCH|DELETE)\(\s*"([^"]*)"([^\n]*)/g
    let m
    while ((m = re.exec(src)) !== null) {
      const [, group, method, path, rest] = m
      if (!groupDefs.has(group)) continue // bukan grup route (mis. pemanggilan lain)
      const g = resolve(group)
      let full = g.prefix + path
      if (full.startsWith(API_BASE)) full = full.slice(API_BASE.length)
      const roles = /RequireRoles\(([A-Za-z0-9_.]+)/.exec(rest)
      routes.push({
        method,
        path: normalize(full),
        raw: full,
        authed: g.authed || /AuthMiddleware/.test(rest),
        roles: roles ? roles[1] : g.roles,
        file,
        line: src.slice(0, m.index).split('\n').length,
      })
    }
  }
  return routes
}

// ---------- 2. Pemanggilan API dari frontend ----------
function walk(dir, out = []) {
  for (const name of readdirSync(dir)) {
    const full = join(dir, name)
    if (statSync(full).isDirectory()) walk(full, out)
    else if (['.ts', '.tsx'].includes(extname(name))) out.push(full)
  }
  return out
}

const VERB = { get: 'GET', post: 'POST', postForm: 'POST', put: 'PUT', patch: 'PATCH', del: 'DELETE' }

function parseFrontend() {
  const calls = []
  for (const file of walk(FRONTEND_SRC)) {
    const src = readFileSync(file, 'utf8')
    // api.get<T>('/path')  |  api.post<T>(`/path/${id}`)
    // (?:<[^()]*>)? menampung generic bersarang seperti <ListResponse<Device>>;
    // pakai [^>]* di sini akan berhenti di '>' pertama dan melewatkan panggilannya.
    const re = /\bapi\.(get|post|postForm|put|patch|del)\s*(?:<[^()]*>)?\s*\(\s*(['"`])([^'"`]*)\2/g
    let m
    while ((m = re.exec(src)) !== null) {
      const [, verb, , path] = m
      if (!path.startsWith('/')) continue
      // Bentuk konkatenasi: api.del('/files/' + id) — literalnya berakhir '/'
      // dan parameter menyusul di luar string. Perlakukan sebagai /files/:p.
      const after = src.slice(m.index + m[0].length, m.index + m[0].length + 40)
      const concat = path.endsWith('/') && /^\s*\+/.test(after)
      calls.push({
        method: VERB[verb],
        path: normalize(concat ? path + ':p' : path),
        raw: concat ? path + "' + <param>" : path,
        file,
        line: src.slice(0, m.index).split('\n').length,
      })
    }
  }
  return calls
}

// ---------- 3. Bandingkan ----------
const backend = parseBackend()
const frontend = parseFrontend()

const key = (r) => `${r.method} ${r.path}`
const feKeys = new Set(frontend.map(key))
const beKeys = new Set(backend.map(key))

const unusedEndpoints = backend
  .filter((r) => !feKeys.has(key(r)))
  .sort((a, b) => key(a).localeCompare(key(b)))

const seen = new Set()
const orphanCalls = frontend
  .filter((c) => !beKeys.has(key(c)))
  .filter((c) => (seen.has(key(c) + c.file) ? false : seen.add(key(c) + c.file)))
  .sort((a, b) => key(a).localeCompare(key(b)))

// Endpoint yang memang BUKAN untuk dipanggil lewat `api.*` — bukan gap.
// Tambahkan di sini hanya dengan alasan yang jelas, jangan untuk membungkam temuan.
const EXCEPTIONS = new Map([
  ['GET /ws', 'upgrade WebSocket, dipakai lib/ws.tsx bukan fetch'],
  ['GET /metrics', 'scrape Prometheus, bukan konsumsi UI'],
  ['GET /auth/oidc/login', 'redirect browser, bukan fetch'],
  ['GET /auth/oidc/callback', 'redirect browser, bukan fetch'],
])

// Endpoint yang BISA dipanggil frontend tapi memang sengaja belum/tidak dipakai,
// dan sudah ditinjau. Beda dari EXCEPTIONS: ini soal keputusan desain UI, bukan
// soal endpoint yang mustahil dipanggil lewat `api.*`. Isi alasannya dengan
// hasil pemeriksaan nyata — jangan dipakai untuk menyembunyikan gap fitur.
const REVIEWED_UNUSED = new Map([
  ['GET /tasks/:p', 'TasksPage merender dari data list; tidak ada tampilan detail per task'],
  ['GET /webhooks/:p', 'WebhooksPage merender dari data list; modal hanya create/delete'],
  ['GET /firmware/rollout-batches/:p', 'FirmwarePage memakai useFirmwareRolloutBatches (list) untuk tab rollout'],
  ['GET /self-service/devices/:p', 'SelfServicePage merender field device langsung dari list'],
])

const excepted = unusedEndpoints.filter((r) => EXCEPTIONS.has(key(r)))
const reviewed = unusedEndpoints.filter((r) => REVIEWED_UNUSED.has(key(r)))
for (let i = unusedEndpoints.length - 1; i >= 0; i--) {
  const k = key(unusedEndpoints[i])
  if (EXCEPTIONS.has(k) || REVIEWED_UNUSED.has(k)) unusedEndpoints.splice(i, 1)
}

const covered = backend.length - unusedEndpoints.length - excepted.length - reviewed.length
const relevant = backend.length - excepted.length - reviewed.length
const pct = relevant ? ((covered / relevant) * 100).toFixed(1) : '0.0'

if (asJson) {
  const exceptions = excepted.map((r) => ({ endpoint: key(r), reason: EXCEPTIONS.get(key(r)) }))
  const reviewedUnused = reviewed.map((r) => ({ endpoint: key(r), reason: REVIEWED_UNUSED.get(key(r)) }))
  console.log(JSON.stringify({ summary: { backend: backend.length, relevant, covered, pct }, unusedEndpoints, orphanCalls, exceptions, reviewedUnused }, null, 2))
} else {
  console.log(`\n=== API Parity: backend ↔ frontend ===`)
  console.log(`Route backend      : ${backend.length}`)
  console.log(`  - dikecualikan   : ${excepted.length} (bukan lewat api.*: WebSocket/Prometheus/redirect)`)
  console.log(`  - sengaja unused : ${reviewed.length} (sudah ditinjau, data sudah ada di list)`)
  console.log(`  - relevan        : ${relevant}`)
  console.log(`Dipakai frontend   : ${covered} (${pct}%)`)
  console.log(`Belum dipakai      : ${unusedEndpoints.length}`)
  console.log(`Panggilan yatim    : ${orphanCalls.length}  <- calon 404, ini bug\n`)

  if (orphanCalls.length) {
    console.log(`--- Panggilan frontend TANPA route backend yang cocok (perbaiki dulu) ---`)
    for (const c of orphanCalls) console.log(`  ${key(c).padEnd(48)} ${c.file}:${c.line}  (raw: ${c.raw})`)
    console.log('')
  }
  if (unusedEndpoints.length) {
    console.log(`--- Endpoint backend yang BELUM dipakai frontend ---`)
    for (const r of unusedEndpoints) {
      const gate = r.authed ? (r.roles ? `authed:${r.roles}` : 'authed') : 'PUBLIC'
      console.log(`  ${key(r).padEnd(48)} ${r.file}:${r.line}  [${gate}]`)
    }
    console.log('')
  }
}

process.exit(orphanCalls.length > 0 ? 1 : 0)
