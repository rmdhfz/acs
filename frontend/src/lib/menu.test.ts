import { describe, expect, it } from 'vitest'
import {
  DEFAULT_LAYOUT,
  MENU_CATALOG,
  addGroup,
  draftFrom,
  moveItem,
  moveNode,
  removeGroup,
  renameGroup,
  resolveLayout,
  toLayout,
  toggleHidden,
  validateDraft,
  type MenuItemDef,
} from './menu'

const allKeys = (d: ReturnType<typeof draftFrom>) => [...d.nodes.flatMap((n) => (n.type === 'item' ? [n.key!] : n.children ?? [])), ...d.hidden]
const admin = MENU_CATALOG.filter((m) => !m.requireRole || ['VIEWER', 'ADMIN'].includes(m.requireRole))

describe('resolveLayout', () => {
  it('tanpa layout tersimpan memakai susunan bawaan dan hanya menu yang diizinkan', () => {
    const out = resolveLayout(null, admin)
    const keys = out.flatMap((n) => (n.type === 'item' ? [n.def.key] : n.items.map((i) => i.key)))
    expect(keys).toContain('dashboard')
    expect(keys).not.toContain('catalog') // SUPERADMIN-only tidak muncul untuk ADMIN
    expect(keys).not.toContain('my-wifi') // ENDUSER-only
    expect(new Set(keys).size).toBe(keys.length) // tanpa duplikat
    // grup "Platform" hanya berisi bug-reports (catalog/reports tak diizinkan) => tetap tampil karena tidak kosong
    expect(out.some((n) => n.type === 'group' && n.id === 'platform' && n.items.length === 1)).toBe(true)
  })

  it('grup yang seluruh isinya tak diizinkan tidak ditampilkan', () => {
    const only = MENU_CATALOG.filter((m) => m.key === 'dashboard')
    const out = resolveLayout(DEFAULT_LAYOUT, only)
    expect(out).toHaveLength(1)
    expect(out[0].type).toBe('item')
  })

  it('layout tersimpan: item hidden hilang, kunci asing dibuang, menu baru ditambahkan di akhir', () => {
    const saved = {
      version: 1 as const,
      nodes: [
        { type: 'item' as const, key: 'devices' },
        { type: 'item' as const, key: 'kunci-lama-yang-sudah-dihapus' },
        { type: 'group' as const, id: 'g1', label: 'Favorit', children: ['tasks', 'tags'] },
      ],
      hidden: ['tags'],
    }
    const out = resolveLayout(saved, admin)
    expect(out[0]).toMatchObject({ type: 'item', def: { key: 'devices' } })
    expect(out[1]).toMatchObject({ type: 'group', id: 'g1', label: 'Favorit' })
    const g = out[1] as Extract<(typeof out)[number], { type: 'group' }>
    expect(g.items.map((i) => i.key)).toEqual(['tasks']) // tags disembunyikan
    const rest = out.slice(2).map((n) => (n.type === 'item' ? n.def.key : n.id))
    expect(rest).toContain('billing') // menu yang belum ada di layout tetap muncul
    expect(rest).not.toContain('tags')
    expect(rest).not.toContain('devices')
  })

  it('kunci yang sama di dua tempat hanya dirender sekali', () => {
    const saved = {
      version: 1 as const,
      nodes: [
        { type: 'item' as const, key: 'devices' },
        { type: 'group' as const, id: 'g', label: 'G', children: ['devices'] },
      ],
      hidden: [],
    }
    const keys = resolveLayout(saved, admin).flatMap((n) => (n.type === 'item' ? [n.def.key] : n.items.map((i) => i.key)))
    expect(keys.filter((k) => k === 'devices')).toHaveLength(1)
  })
})

describe('operasi draft', () => {
  const base = () => draftFrom(null, admin)

  it('draftFrom memuat setiap menu yang diizinkan tepat satu kali', () => {
    const d = base()
    const keys = allKeys(d)
    expect(new Set(keys).size).toBe(keys.length)
    expect(new Set(keys)).toEqual(new Set(admin.map((m) => m.key)))
  })

  it('moveItem: root -> grup -> hidden -> root, tanpa kehilangan/duplikasi', () => {
    let d = base()
    d = moveItem(d, 'devices', 'konfigurasi', 0)
    const g = d.nodes.find((n) => n.type === 'group' && n.id === 'konfigurasi')!
    expect(g.children![0]).toBe('devices')
    d = moveItem(d, 'devices', 'hidden')
    expect(d.hidden).toContain('devices')
    expect(allKeys(d).filter((k) => k === 'devices')).toHaveLength(1)
    d = moveItem(d, 'devices', 'root', 0)
    expect(d.nodes[0]).toMatchObject({ type: 'item', key: 'devices' })
    expect(d.hidden).not.toContain('devices')
    expect(new Set(allKeys(d)).size).toBe(allKeys(d).length)
  })

  it('moveItem ke grup yang tidak ada tidak mengubah apa pun', () => {
    const d = base()
    expect(moveItem(d, 'devices', 'tidak-ada')).toBe(d)
  })

  it('moveNode mengurutkan ulang simpul root', () => {
    let d = base()
    const first = d.nodes[0]
    d = moveNode(d, 0, 3)
    expect(d.nodes[3]).toEqual(first)
    expect(moveNode(d, -1, 0)).toBe(d)
  })

  it('addGroup/renameGroup/removeGroup: isi grup naik ke root saat grup dihapus', () => {
    let d = base()
    d = addGroup(d, 'Favorit')
    const id = d.nodes[d.nodes.length - 1].id!
    d = moveItem(d, 'devices', id)
    d = renameGroup(d, id, 'X'.repeat(50))
    expect(d.nodes.find((n) => n.id === id)!.label!.length).toBe(32)
    d = removeGroup(d, id)
    expect(d.nodes.some((n) => n.id === id)).toBe(false)
    expect(allKeys(d)).toContain('devices')
    expect(new Set(allKeys(d)).size).toBe(allKeys(d).length)
  })

  it('addGroup dibatasi 12 grup', () => {
    let d = base()
    for (let i = 0; i < 20; i++) d = addGroup(d)
    expect(d.nodes.filter((n) => n.type === 'group').length).toBeLessThanOrEqual(12)
  })

  it('toggleHidden menyembunyikan lalu menampilkan lagi di root', () => {
    let d = toggleHidden(base(), 'tags')
    expect(d.hidden).toContain('tags')
    d = toggleHidden(d, 'tags')
    expect(d.hidden).not.toContain('tags')
    expect(allKeys(d).filter((k) => k === 'tags')).toHaveLength(1)
  })

  it('validateDraft menolak nama grup kosong/kontrol/terlalu panjang', () => {
    let d = addGroup(base(), 'Ok')
    const id = d.nodes[d.nodes.length - 1].id!
    expect(validateDraft(d)).toBeNull()
    d = renameGroup(d, id, '   ')
    expect(validateDraft(d)).toMatch(/Nama grup/)
    d = renameGroup(d, id, 'a\u0000b')
    expect(validateDraft(d)).toMatch(/kontrol/)
  })

  it('toLayout menghasilkan bentuk kiriman yang bersih', () => {
    const l = toLayout(addGroup(base(), '  Favorit  '))
    expect(l.version).toBe(1)
    const g = l.nodes[l.nodes.length - 1]
    expect(g).toMatchObject({ type: 'group', label: 'Favorit', children: [] })
    expect(Object.keys(l.nodes[0]).sort()).toEqual(['key', 'type'])
  })

  it('katalog: kunci unik dan sesuai pola server', () => {
    const keys = MENU_CATALOG.map((m: MenuItemDef) => m.key)
    expect(new Set(keys).size).toBe(keys.length)
    for (const k of keys) expect(k).toMatch(/^[a-z][a-z0-9._-]{0,47}$/)
  })
})
