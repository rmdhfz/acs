import {
  BarChart3,
  Bell,
  Bug,
  Building2,
  Cable,
  CreditCard,
  FileSliders,
  FunctionSquare,
  FolderArchive,
  HardDrive,
  History,
  LayoutDashboard,
  LayoutGrid,
  ListChecks,
  TriangleAlert,
  Radio,
  ScrollText,
  SlidersHorizontal,
  Tag,
  type LucideIcon,
} from 'lucide-react'

// Katalog menu sidebar + logika susunan (grup/sub menu, urutan, sembunyikan). Susunan disimpan per pengguna di server
// (GET/PUT /auth/menu-layout) dan HANYA mengatur tampilan: hak akses menu tetap ditentukan peran (requireRole).

export interface MenuItemDef {
  key: string
  to: string
  labelKey: string
  icon: LucideIcon
  requireRole?: string
}

export const MENU_CATALOG: MenuItemDef[] = [
  { key: 'my-wifi', to: '/self-service', labelKey: 'nav.myWifi', icon: Radio, requireRole: 'ENDUSER' },
  { key: 'dashboard', to: '/dashboard', labelKey: 'nav.dashboard', icon: LayoutDashboard, requireRole: 'VIEWER' },
  { key: 'devices', to: '/devices', labelKey: 'nav.devices', icon: LayoutGrid, requireRole: 'VIEWER' },
  { key: 'tasks', to: '/tasks', labelKey: 'nav.tasks', icon: ListChecks, requireRole: 'VIEWER' },
  { key: 'faults', to: '/faults', labelKey: 'nav.faults', icon: TriangleAlert, requireRole: 'VIEWER' },
  { key: 'virtual-parameters', to: '/virtual-parameters', labelKey: 'nav.virtualParameters', icon: FunctionSquare, requireRole: 'VIEWER' },
  { key: 'provisioning', to: '/provisioning', labelKey: 'nav.provisioning', icon: FileSliders, requireRole: 'VIEWER' },
  { key: 'presets', to: '/presets', labelKey: 'nav.presets', icon: SlidersHorizontal, requireRole: 'ADMIN' },
  { key: 'firmware', to: '/firmware', labelKey: 'nav.firmware', icon: HardDrive, requireRole: 'VIEWER' },
  { key: 'files', to: '/files', labelKey: 'nav.files', icon: FolderArchive, requireRole: 'VIEWER' },
  { key: 'tags', to: '/tags', labelKey: 'nav.tags', icon: Tag, requireRole: 'ADMIN' },
  { key: 'webhooks', to: '/webhooks', labelKey: 'nav.webhooks', icon: Bell, requireRole: 'ADMIN' },
  { key: 'audit', to: '/audit', labelKey: 'nav.auditTrail', icon: ScrollText, requireRole: 'ADMIN' },
  { key: 'data-transfers', to: '/data-transfers', labelKey: 'nav.dataTransfers', icon: History, requireRole: 'VIEWER' },
  { key: 'catalog', to: '/catalog', labelKey: 'nav.catalog', icon: Cable, requireRole: 'SUPERADMIN' },
  { key: 'reports', to: '/reports', labelKey: 'nav.reports', icon: BarChart3, requireRole: 'SUPERADMIN' },
  { key: 'bug-reports', to: '/bug-reports', labelKey: 'nav.bugReports', icon: Bug, requireRole: 'VIEWER' },
  { key: 'billing', to: '/billing', labelKey: 'nav.billing', icon: CreditCard, requireRole: 'ADMIN' },
  { key: 'administration', to: '/administration', labelKey: 'nav.administration', icon: Building2, requireRole: 'ADMIN' },
]

const BY_KEY = new Map(MENU_CATALOG.map((m) => [m.key, m]))
export function menuItemByKey(key: string): MenuItemDef | undefined {
  return BY_KEY.get(key)
}

// ---- bentuk layout (sama dengan kontrak server: usecase/console.MenuLayout) ----

export interface MenuLayoutNode {
  type: 'item' | 'group'
  key?: string // item
  id?: string // grup
  label?: string // grup
  collapsed?: boolean // grup
  children?: string[] // grup: kunci item
}

export interface MenuLayout {
  version: 1
  nodes: MenuLayoutNode[]
  hidden: string[]
}

export const MENU_LIMITS = { groups: 12, label: 32, nodes: 60, children: 40 } as const

// Susunan bawaan: menu harian di atas, sisanya dikelompokkan agar sidebar ringkas.
export const DEFAULT_LAYOUT: MenuLayout = {
  version: 1,
  nodes: [
    { type: 'item', key: 'my-wifi' },
    { type: 'item', key: 'dashboard' },
    { type: 'item', key: 'devices' },
    { type: 'item', key: 'tasks' },
    { type: 'group', id: 'konfigurasi', label: 'Konfigurasi', children: ['provisioning', 'presets', 'firmware', 'files', 'tags'] },
    { type: 'group', id: 'integrasi', label: 'Integrasi & riwayat', collapsed: true, children: ['webhooks', 'audit', 'data-transfers'] },
    { type: 'group', id: 'platform', label: 'Platform', collapsed: true, children: ['catalog', 'reports', 'bug-reports'] },
    { type: 'item', key: 'billing' },
    { type: 'item', key: 'administration' },
  ],
  hidden: [],
}

// ---- hasil resolusi untuk render ----

export type ResolvedNode =
  | { type: 'item'; def: MenuItemDef }
  | { type: 'group'; id: string; label: string; collapsed: boolean; items: MenuItemDef[] }

// resolveLayout menerapkan layout (tersimpan atau bawaan) ke menu yang BOLEH dilihat pengguna: kunci tak dikenal/tak
// berizin dibuang, item tersembunyi dibuang, grup kosong tidak ditampilkan, dan menu baru yang belum ada di layout
// ditambahkan di akhir (supaya fitur baru tidak hilang bagi pengguna yang sudah menyimpan susunan sendiri).
export function resolveLayout(saved: MenuLayout | null | undefined, allowed: MenuItemDef[]): ResolvedNode[] {
  const layout = saved ?? DEFAULT_LAYOUT
  const ok = new Map(allowed.map((m) => [m.key, m]))
  const hidden = new Set(layout.hidden)
  const placed = new Set<string>()
  const out: ResolvedNode[] = []
  for (const n of layout.nodes) {
    if (n.type === 'item' && n.key) {
      const def = ok.get(n.key)
      if (def && !hidden.has(n.key) && !placed.has(n.key)) {
        placed.add(n.key)
        out.push({ type: 'item', def })
      }
    } else if (n.type === 'group' && n.id) {
      const items: MenuItemDef[] = []
      for (const k of n.children ?? []) {
        const def = ok.get(k)
        if (def && !hidden.has(k) && !placed.has(k)) {
          placed.add(k)
          items.push(def)
        }
      }
      if (items.length > 0) out.push({ type: 'group', id: n.id, label: n.label ?? '', collapsed: !!n.collapsed, items })
    }
  }
  // menu yang belum tercantum di layout mana pun (termasuk yang disembunyikan eksplisit TIDAK ditambahkan)
  const listed = new Set<string>([...hidden, ...layout.nodes.flatMap((n) => (n.type === 'item' ? [n.key ?? ''] : (n.children ?? [])))])
  for (const def of allowed) {
    if (!placed.has(def.key) && !listed.has(def.key)) out.push({ type: 'item', def })
  }
  return out
}

// ---- operasi draft (murni; dipakai MenuManagerModal & diuji) ----

export type Draft = MenuLayout

export function cloneDraft(d: Draft): Draft {
  return { version: 1, nodes: d.nodes.map((n) => ({ ...n, children: n.children ? [...n.children] : undefined })), hidden: [...d.hidden] }
}

// draftFrom membangun draft yang LENGKAP untuk menu yang diizinkan: semua item ada tepat sekali (di root, di grup, atau
// di hidden). Item baru yang belum ada ditambahkan di akhir root.
export function draftFrom(saved: MenuLayout | null | undefined, allowed: MenuItemDef[]): Draft {
  const base = cloneDraft(saved ?? DEFAULT_LAYOUT)
  const ok = new Set(allowed.map((m) => m.key))
  const seen = new Set<string>()
  const keep = (k: string) => ok.has(k) && !seen.has(k) && (seen.add(k), true)
  base.nodes = base.nodes
    .map((n) => (n.type === 'item' ? (n.key && keep(n.key) ? n : null) : { ...n, children: (n.children ?? []).filter(keep) }))
    .filter((n): n is MenuLayoutNode => n !== null)
  base.hidden = base.hidden.filter(keep)
  for (const m of allowed) if (!seen.has(m.key)) base.nodes.push({ type: 'item', key: m.key })
  return base
}

function removeKey(d: Draft, key: string): void {
  d.nodes = d.nodes.filter((n) => !(n.type === 'item' && n.key === key))
  for (const n of d.nodes) if (n.type === 'group') n.children = (n.children ?? []).filter((k) => k !== key)
  d.hidden = d.hidden.filter((k) => k !== key)
}

export type Container = 'root' | 'hidden' | string // string = id grup

// moveItem memindahkan item ke container (root/hidden/grup) pada indeks tertentu (indeks dihitung SETELAH item dicabut).
export function moveItem(d: Draft, key: string, to: Container, index?: number): Draft {
  const next = cloneDraft(d)
  removeKey(next, key)
  if (to === 'hidden') {
    next.hidden.push(key)
  } else if (to === 'root') {
    const node: MenuLayoutNode = { type: 'item', key }
    next.nodes.splice(index ?? next.nodes.length, 0, node)
  } else {
    const g = next.nodes.find((n) => n.type === 'group' && n.id === to)
    if (!g) return d
    g.children = g.children ?? []
    g.children.splice(index ?? g.children.length, 0, key)
  }
  return next
}

// moveNode memindahkan simpul root (item atau grup) ke indeks root baru (indeks dihitung SETELAH dicabut).
export function moveNode(d: Draft, from: number, to: number): Draft {
  if (from < 0 || from >= d.nodes.length) return d
  const next = cloneDraft(d)
  const [n] = next.nodes.splice(from, 1)
  next.nodes.splice(Math.max(0, Math.min(to, next.nodes.length)), 0, n)
  return next
}

export function newGroupId(d: Draft): string {
  const used = new Set(d.nodes.filter((n) => n.type === 'group').map((n) => n.id))
  for (let i = 1; i < 1000; i++) {
    const id = `grup-${i}`
    if (!used.has(id)) return id
  }
  return `grup-${Date.now().toString(36)}`
}

export function addGroup(d: Draft, label = 'Grup baru'): Draft {
  if (d.nodes.filter((n) => n.type === 'group').length >= MENU_LIMITS.groups) return d
  const next = cloneDraft(d)
  next.nodes.push({ type: 'group', id: newGroupId(d), label: label.slice(0, MENU_LIMITS.label), children: [] })
  return next
}

export function renameGroup(d: Draft, id: string, label: string): Draft {
  const next = cloneDraft(d)
  const g = next.nodes.find((n) => n.type === 'group' && n.id === id)
  if (g) g.label = label.slice(0, MENU_LIMITS.label)
  return next
}

export function toggleGroupCollapsed(d: Draft, id: string): Draft {
  const next = cloneDraft(d)
  const g = next.nodes.find((n) => n.type === 'group' && n.id === id)
  if (g) g.collapsed = !g.collapsed
  return next
}

// removeGroup menghapus grup; isinya naik ke root pada posisi grup tersebut (tidak ada menu yang hilang).
export function removeGroup(d: Draft, id: string): Draft {
  const next = cloneDraft(d)
  const i = next.nodes.findIndex((n) => n.type === 'group' && n.id === id)
  if (i < 0) return d
  const kids = (next.nodes[i].children ?? []).map<MenuLayoutNode>((k) => ({ type: 'item', key: k }))
  next.nodes.splice(i, 1, ...kids)
  return next
}

export function toggleHidden(d: Draft, key: string): Draft {
  if (d.hidden.includes(key)) return moveItem(d, key, 'root')
  return moveItem(d, key, 'hidden')
}

// validateDraft mencerminkan aturan server; mengembalikan pesan galat atau null.
export function validateDraft(d: Draft): string | null {
  const groups = d.nodes.filter((n) => n.type === 'group')
  if (groups.length > MENU_LIMITS.groups) return `Maksimal ${MENU_LIMITS.groups} grup.`
  if (d.nodes.length > MENU_LIMITS.nodes) return 'Terlalu banyak simpul menu.'
  for (const g of groups) {
    const label = (g.label ?? '').trim()
    if (label.length < 1 || label.length > MENU_LIMITS.label) return `Nama grup wajib diisi (1–${MENU_LIMITS.label} karakter).`
    // eslint-disable-next-line no-control-regex
    if (/[\u0000-\u001f\u007f]/.test(label)) return 'Nama grup tidak boleh memuat karakter kontrol.'
    if ((g.children ?? []).length > MENU_LIMITS.children) return 'Isi grup terlalu banyak.'
  }
  return null
}

// toLayout menormalkan draft ke bentuk kiriman (nama grup dipangkas).
export function toLayout(d: Draft): MenuLayout {
  return {
    version: 1,
    nodes: d.nodes.map((n) =>
      n.type === 'item'
        ? { type: 'item', key: n.key }
        : { type: 'group', id: n.id, label: (n.label ?? '').trim(), ...(n.collapsed ? { collapsed: true } : {}), children: n.children ?? [] },
    ),
    hidden: d.hidden,
  }
}
