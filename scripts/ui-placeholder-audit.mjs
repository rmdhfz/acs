#!/usr/bin/env node
// ui-placeholder-audit.mjs — periksa apakah setiap kontrol input di frontend
// punya petunjuk isi yang memadai untuk user.
//
// KENAPA BUKAN "semua input wajib placeholder": placeholder BUKAN pengganti
// label (teksnya hilang begitu user mengetik, dan pembaca layar tidak
// memperlakukannya sebagai nama field). Jadi yang dilaporkan agent ini adalah
// kontrol yang benar-benar TIDAK punya petunjuk apa pun — tanpa label DAN
// tanpa placeholder — plus <select> yang tidak punya opsi kosong sebagai
// prompt pilihan.
//
// Pemakaian:
//   node scripts/ui-placeholder-audit.mjs           # ringkasan + temuan
//   node scripts/ui-placeholder-audit.mjs --all     # + daftar yang sudah OK
//   node scripts/ui-placeholder-audit.mjs --json
//
// Exit code 1 bila ada kontrol tanpa petunjuk sama sekali.

import { readFileSync, readdirSync, statSync } from 'node:fs'
import { join, extname, relative } from 'node:path'

const SRC = 'frontend/src'
const asJson = process.argv.includes('--json')
const showAll = process.argv.includes('--all')

// Tipe input yang placeholder-nya TIDAK dirender browser sama sekali, atau
// yang memang tidak butuh petunjuk teks.
const NO_PLACEHOLDER_TYPES = new Set([
  'checkbox', 'radio', 'color', 'file', 'range', 'submit', 'button', 'reset', 'hidden', 'image',
  'date', 'datetime-local', 'month', 'week', 'time',
])

function walk(dir, out = []) {
  for (const name of readdirSync(dir)) {
    const full = join(dir, name)
    if (statSync(full).isDirectory()) walk(full, out)
    else if (['.tsx', '.ts'].includes(extname(name))) out.push(full)
  }
  return out
}

/**
 * Cari akhir tag JSX yang dimulai di `start`. Tidak bisa pakai regex sampai '>'
 * karena atribut JSX memuat ekspresi seperti onChange={(e) => ...} yang
 * mengandung '>' sendiri. Jadi ditelusuri manual sambil melacak kedalaman
 * kurung kurawal dan string.
 */
function findTagEnd(src, start) {
  let depth = 0
  let quote = null
  for (let i = start; i < src.length; i++) {
    const ch = src[i]
    if (quote) {
      if (ch === quote && src[i - 1] !== '\\') quote = null
      continue
    }
    if (ch === '"' || ch === "'" || ch === '`') { quote = ch; continue }
    if (ch === '{') { depth++; continue }
    if (ch === '}') { depth--; continue }
    if (ch === '>' && depth === 0) return i
  }
  return -1
}

const findings = []
const okList = []

for (const file of walk(SRC)) {
  const src = readFileSync(file, 'utf8')
  const rel = relative('.', file).split('\\').join('/')

  for (const tagName of ['input', 'textarea', 'select']) {
    const re = new RegExp(`<${tagName}\\b`, 'g')
    let m
    while ((m = re.exec(src)) !== null) {
      const end = findTagEnd(src, m.index)
      if (end === -1) continue
      const tag = src.slice(m.index, end + 1)
      const line = src.slice(0, m.index).split('\n').length

      const typeMatch = /\btype=["']([a-z-]+)["']/.exec(tag)
      const type = typeMatch ? typeMatch[1] : tagName === 'input' ? 'text' : tagName
      if (NO_PLACEHOLDER_TYPES.has(type)) continue

      const hasPlaceholder = /\bplaceholder[=\s]/.test(tag)

      // Label dianggap ada bila muncul dalam ~400 karakter sebelum tag ini
      // (pola proyek: <label ...>Nama</label> lalu <input .../> di div yang sama).
      const before = src.slice(Math.max(0, m.index - 400), m.index)
      const hasLabel = /<label\b/.test(before)

      // <select> tidak mengenal placeholder; prompt-nya berupa opsi kosong.
      let hasEmptyOption = false
      if (tagName === 'select') {
        const after = src.slice(end, end + 600)
        hasEmptyOption = /<option[^>]*value=["']{2}/.test(after) || /<option[^>]*value=\{?["']{2}/.test(after)
      }

      const entry = { file: rel, line, tag: tagName, type, hasPlaceholder, hasLabel, hasEmptyOption }

      if (tagName === 'select') {
        if (!hasEmptyOption && !hasLabel) findings.push({ ...entry, issue: 'select tanpa opsi prompt DAN tanpa label' })
        else okList.push(entry)
      } else if (!hasPlaceholder && !hasLabel) {
        findings.push({ ...entry, issue: 'tanpa placeholder DAN tanpa label' })
      } else {
        okList.push(entry)
      }
    }
  }
}

const total = findings.length + okList.length
const withPlaceholder = okList.filter((e) => e.hasPlaceholder).length
const labelOnly = okList.filter((e) => !e.hasPlaceholder && e.hasLabel && e.tag !== 'select').length

if (asJson) {
  console.log(JSON.stringify({ summary: { total, withPlaceholder, labelOnly, findings: findings.length }, findings, ok: showAll ? okList : undefined }, null, 2))
} else {
  console.log('\n=== Audit petunjuk input (placeholder / label) ===')
  console.log(`Kontrol relevan       : ${total}  (checkbox/color/file/date dikecualikan)`)
  console.log(`Punya placeholder     : ${withPlaceholder}`)
  console.log(`Label saja (tanpa ph) : ${labelOnly}  <- sah, tapi kandidat perbaikan UX`)
  console.log(`TANPA petunjuk apa pun: ${findings.length}\n`)

  if (findings.length) {
    console.log('--- Kontrol tanpa label DAN tanpa placeholder (perbaiki) ---')
    for (const f of findings) console.log(`  ${f.file}:${f.line}  <${f.tag} type=${f.type}>  ${f.issue}`)
    console.log('')
  }
  if (showAll && labelOnly) {
    console.log('--- Punya label tapi belum ada placeholder (opsional) ---')
    for (const e of okList.filter((x) => !x.hasPlaceholder && x.hasLabel && x.tag !== 'select')) {
      console.log(`  ${e.file}:${e.line}  <${e.tag} type=${e.type}>`)
    }
    console.log('')
  }
}

process.exit(findings.length > 0 ? 1 : 0)
