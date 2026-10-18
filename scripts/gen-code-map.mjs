#!/usr/bin/env node
// Menghasilkan docs/arsitektur/PETA_KODE.md: indeks ringkas seluruh codebase (paket Go, endpoint REST, tabel database,
// halaman/komponen frontend, skenario e2e, migrasi). Dipakai agar sesi baru (manusia ATAU asisten AI) bisa langsung
// tahu "apa ada di mana" tanpa menjelajah ulang repo. Tanpa dependensi pihak ketiga; hanya membaca berkas lokal.
//
//   node scripts/gen-code-map.mjs           # tulis docs/arsitektur/PETA_KODE.md
//   node scripts/gen-code-map.mjs --check   # exit 1 bila berkas usang (untuk CI/pra-commit)
import fs from 'node:fs'
import path from 'node:path'

const ROOT = path.resolve(import.meta.dirname, '..')
const OUT = path.join(ROOT, 'docs', 'arsitektur', 'PETA_KODE.md')
const rd = (p) => fs.readFileSync(path.join(ROOT, p), 'utf8').replace(/\r\n/g, '\n')
const exists = (p) => fs.existsSync(path.join(ROOT, p))
const ls = (p) => (exists(p) ? fs.readdirSync(path.join(ROOT, p), { withFileTypes: true }) : [])
const SKIP = new Set(['node_modules', '.git', 'dist', 'dist-ssr', 'bin', 'graphify-out', '.ui-audit', 'vendor'])

function walk(dir, pred, out = []) {
  for (const e of ls(dir)) {
    if (SKIP.has(e.name)) continue
    const rel = path.posix.join(dir, e.name)
    if (e.isDirectory()) walk(rel, pred, out)
    else if (pred(rel)) out.push(rel)
  }
  return out
}

// ---- paket Go ----
function firstSentence(s) {
  const t = s.replace(/\s+/g, ' ').trim()
  const m = /^(.{20,220}?[.!?])(\s|$)/.exec(t)
  return (m ? m[1] : t.slice(0, 200)).replace(/\|/g, '/')
}
function goPackages() {
  const files = walk('.', (p) => p.endsWith('.go') && !p.endsWith('_test.go') && !p.startsWith('frontend/'))
  const byDir = new Map()
  for (const f of files) byDir.set(path.posix.dirname(f), [...(byDir.get(path.posix.dirname(f)) ?? []), f])
  const rows = []
  for (const [dir, list] of [...byDir].sort(([a], [b]) => a.localeCompare(b))) {
    let doc = ''
    for (const f of list) {
      const s = rd(f)
      const m = /((?:^\/\/.*\n)+)\s*package\s+\w+/m.exec(s)
      if (m && /Package\s+\w+|^\/\/ \w+/m.test(m[1])) {
        doc = firstSentence(m[1].replace(/^\/\/ ?/gm, '').replace(/^Package \w+ (adalah |=)?/i, ''))
        if (doc) break
      }
    }
    rows.push(`| \`${dir}\` | ${list.length} | ${doc || '—'} |`)
  }
  return ['| Paket | Berkas | Ringkasan |', '|---|---:|---|', ...rows].join('\n')
}

// ---- endpoint dari openapi.yaml ----
function endpoints() {
  const y = rd('openapi.yaml').split('\n')
  const out = []
  let inPaths = false
  let path_ = ''
  let method = ''
  for (let i = 0; i < y.length; i++) {
    const l = y[i]
    if (/^paths:/.test(l)) { inPaths = true; continue }
    if (inPaths && /^\S/.test(l)) break
    if (!inPaths) continue
    let m = /^  (\/\S*):\s*$/.exec(l)
    if (m) { path_ = m[1]; continue }
    m = /^    (get|post|put|patch|delete):\s*$/.exec(l)
    if (m) { method = m[1].toUpperCase(); out.push({ path: path_, method, tag: '', summary: '' }); continue }
    const cur = out[out.length - 1]
    if (!cur) continue
    if ((m = /^      tags: \[([^\]]+)\]/.exec(l))) cur.tag = m[1].split(',')[0].trim()
    if ((m = /^      summary: (.+)$/.exec(l))) cur.summary = m[1].replace(/^["']|["']$/g, '').replace(/\|/g, '/')
  }
  const tags = [...new Set(out.map((e) => e.tag))].sort()
  const blocks = tags.map((t) => {
    const rows = out.filter((e) => e.tag === t).map((e) => `| ${e.method} | \`${e.path}\` | ${e.summary} |`)
    return `**${t || '(tanpa tag)'}** (${rows.length})\n\n| Method | Path | Ringkasan |\n|---|---|---|\n${rows.join('\n')}`
  })
  return { count: out.length, text: blocks.join('\n\n') }
}

// ---- tabel database ----
function tables() {
  const s = rd('schema.sql')
  const rows = []
  const re = /CREATE TABLE (\w+) \(([\s\S]*?)\n\)\s*ENGINE=[^;]*?(?:COMMENT='([^']*)')?;/g
  let m
  while ((m = re.exec(s))) {
    const [, name, body, comment] = m
    const cols = body.split('\n').filter((l) => /^\s{2,}`?\w+`?\s+(BIGINT|INT|VARCHAR|CHAR|TEXT|DATETIME|TINYINT|DECIMAL|JSON|DOUBLE|MEDIUMTEXT|LONGTEXT|SMALLINT|VARBINARY|BLOB|ENUM|DATE|TIMESTAMP|FLOAT)/i.test(l)).length
    rows.push(`| \`${name}\` | ${cols} | ${(comment ?? '').replace(/\|/g, '/').slice(0, 140) || '—'} |`)
  }
  return { count: rows.length, text: ['| Tabel | Kolom | Keterangan |', '|---|---:|---|', ...rows].join('\n') }
}

// ---- frontend ----
function frontend() {
  const app = rd('frontend/src/App.tsx')
  const routes = [...app.matchAll(/<Route path="([^"]+)" element=\{<(\w+)/g)].map((m) => `| \`${m[1]}\` | ${m[2]} |`)
  const comps = ls('frontend/src/components').filter((e) => e.isFile() && /\.tsx$/.test(e.name) && !/\.test\./.test(e.name)).map((e) => e.name.replace(/\.tsx$/, ''))
  const libs = ls('frontend/src/lib').filter((e) => e.isFile() && /\.tsx?$/.test(e.name) && !/\.test\./.test(e.name)).map((e) => e.name.replace(/\.tsx?$/, ''))
  const tests = walk('frontend/src', (p) => /\.test\.tsx?$/.test(p)).length
  return [
    '**Rute Console** (`frontend/src/App.tsx`)\n',
    '| Path | Halaman |\n|---|---|',
    routes.join('\n'),
    `\n**Komponen** (\`frontend/src/components\`, ${comps.length}): ${comps.join(', ')}`,
    `\n**Lib/hooks** (\`frontend/src/lib\`, ${libs.length}): ${libs.join(', ')}`,
    `\nBerkas tes frontend: ${tests}.`,
  ].join('\n')
}

// ---- e2e & migrasi ----
function scenarios() {
  const s = rd('cmd/e2e/main.go')
  return [...s.matchAll(/\{"(S\d+-[^"]+)", (\w+)\}/g)].map((m) => `| ${m[1]} | \`${m[2]}\` |`)
}
function migrations() {
  const files = ls('migrations').filter((e) => e.name.endsWith('.up.sql')).map((e) => e.name.replace('.up.sql', ''))
  return files
}

const eps = endpoints()
const tbl = tables()
const mig = migrations()
const scn = scenarios()
const now = process.env.SOURCE_DATE_EPOCH ? new Date(Number(process.env.SOURCE_DATE_EPOCH) * 1000) : null

const md = `# Peta kode ACS

> **Dibuat otomatis** oleh \`node scripts/gen-code-map.mjs\` — jangan diedit tangan. Tujuan: sesi baru (manusia atau asisten AI) langsung tahu
> "apa ada di mana" tanpa menjelajah ulang repo. Konteks bisnis: \`docs/produk/PRD.md\`; keputusan arsitektur: \`docs/arsitektur/TECH.md\`;
> aturan kerja & konvensi: \`CLAUDE.md\`. Perbarui setelah menambah paket/endpoint/tabel/halaman (\`--check\` untuk memeriksa).

Ringkasan: ${eps.count} operasi REST • ${tbl.count} tabel • ${mig.length} migrasi (terakhir \`${mig[mig.length - 1]}\`) • ${scn.length} skenario e2e.

## Alur & lapisan (Clean Architecture)
\`domain\` (entitas + antarmuka) → \`usecase/*\` (aturan bisnis) → \`repository/mysql\` (query sqlx) / \`delivery/{cwmp,http,ws,usp}\` (adapter masuk).
Handler hanya parsing/validasi; keputusan akses & bisnis di usecase; repository hanya query. Entrypoint: \`cmd/acsd\` (server), \`cmd/migrate\`,
\`cmd/e2e\` (uji end-to-end vs simulator CPE \`internal/simcpe\`), \`cmd/cpesim\`, \`cmd/seed-admin\`, \`cmd/seal-secrets\`.

## Paket Go
${goPackages()}

## Endpoint REST (dari \`openapi.yaml\`, dikelompokkan per tag)
${eps.text}

## Tabel database (dari \`schema.sql\`)
${tbl.text}

## Frontend
${frontend()}

## Skenario e2e (\`cmd/e2e\`)
| Skenario | Fungsi |
|---|---|
${scn.join('\n')}

## Migrasi (\`migrations/\`)
${mig.map((m) => `\`${m}\``).join(' · ')}

## Perintah penting
- Go: \`go vet ./...\`, \`go test ./...\`; race + integrasi MariaDB via Docker \`golang:1.26.9\` (lihat \`docs/pengujian/PENGUJIAN_E2E.md\`).
- Frontend (\`frontend/\`): \`npx tsc -b\`, \`npx oxlint\`, \`npx vitest run\`, \`npm run build\`, \`npm run audit:ui\`; paritas API: \`node scripts/api-parity.mjs\`.
- Spec: \`npx @redocly/cli@2 lint openapi.yaml\`. Peta ini: \`node scripts/gen-code-map.mjs\`.
${now ? `\nDibuat: ${now.toISOString()}\n` : ''}`

if (process.argv.includes('--check')) {
  const cur = exists('docs/arsitektur/PETA_KODE.md') ? rd('docs/arsitektur/PETA_KODE.md') : ''
  if (cur !== md) {
    console.error('PETA_KODE.md usang — jalankan: node scripts/gen-code-map.mjs')
    process.exit(1)
  }
  console.log('PETA_KODE.md mutakhir')
} else {
  fs.writeFileSync(OUT, md)
  console.log(`ditulis ${path.relative(ROOT, OUT)} (${md.split('\n').length} baris)`)
}
