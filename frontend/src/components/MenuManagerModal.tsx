import { useMemo, useState } from 'react'
import { ArrowDown, ArrowUp, ChevronDown, Eye, EyeOff, FolderPlus, GripVertical, RotateCcw, Trash2 } from 'lucide-react'
import { Modal } from './Modal'
import { ApiError } from '../lib/api'
import { useI18n } from '../lib/i18n'
import { useResetMenuLayout, useSaveMenuLayout } from '../lib/hooks'
import { useToast } from '../lib/toast'
import {
  DEFAULT_LAYOUT,
  MENU_LIMITS,
  addGroup,
  draftFrom,
  menuItemByKey,
  moveItem,
  moveNode,
  removeGroup,
  renameGroup,
  toLayout,
  toggleGroupCollapsed,
  toggleHidden,
  validateDraft,
  type Container,
  type Draft,
  type MenuItemDef,
  type MenuLayout,
} from '../lib/menu'

// Pengelola menu: atur urutan, buat grup (sub menu), pindahkan item antar grup, dan sembunyikan item — dengan drag &
// drop. Setiap baris juga punya tombol naik/turun dan pemilih tujuan, jadi tetap bisa dipakai tanpa mouse/di layar
// sentuh (HTML5 drag & drop tidak jalan di sentuh). Hak akses menu tetap ditentukan peran, bukan susunan ini.

type Drag = { kind: 'item'; key: string } | { kind: 'group'; id: string } | null

interface Props {
  saved: MenuLayout | null | undefined
  allowed: MenuItemDef[]
  onClose: () => void
}

const field =
  'rounded-lg border border-slate-300 bg-white px-2 py-1 text-sm text-slate-900 outline-none focus:border-indigo-500 focus:ring-2 focus:ring-indigo-500/20 dark:border-slate-700 dark:bg-slate-900 dark:text-slate-100'
const iconBtn =
  'inline-flex h-8 w-8 items-center justify-center rounded-full text-slate-600 hover:bg-slate-100 disabled:opacity-30 dark:text-slate-300 dark:hover:bg-slate-800'

export function MenuManagerModal({ saved, allowed, onClose }: Props) {
  const { t } = useI18n()
  const toast = useToast()
  const save = useSaveMenuLayout()
  const reset = useResetMenuLayout()
  const [draft, setDraft] = useState<Draft>(() => draftFrom(saved, allowed))
  const [drag, setDrag] = useState<Drag>(null)
  const [over, setOver] = useState<string | null>(null)
  const [error, setError] = useState<string | null>(null)
  const busy = save.isPending || reset.isPending

  const groups = useMemo(() => draft.nodes.filter((n) => n.type === 'group'), [draft])
  const labelOf = (key: string) => {
    const d = menuItemByKey(key)
    return d ? t(d.labelKey) : key
  }

  // ---- drop ----
  function indexIn(container: Container, key: string): number {
    if (container === 'root') return draft.nodes.findIndex((n) => n.type === 'item' && n.key === key)
    if (container === 'hidden') return draft.hidden.indexOf(key)
    return (draft.nodes.find((n) => n.type === 'group' && n.id === container)?.children ?? []).indexOf(key)
  }

  function dropItemAt(container: Container, index: number) {
    if (drag?.kind !== 'item') return
    const from = indexIn(container, drag.key)
    // indeks tujuan dihitung SETELAH item dicabut: bila asalnya di container yang sama dan lebih awal, geser satu.
    const adjusted = from >= 0 && from < index ? index - 1 : index
    setDraft((d) => moveItem(d, drag.key, container, container === 'hidden' ? undefined : adjusted))
  }

  function dropGroupAt(index: number) {
    if (drag?.kind !== 'group') return
    const from = draft.nodes.findIndex((n) => n.type === 'group' && n.id === drag.id)
    if (from < 0) return
    setDraft((d) => moveNode(d, from, from < index ? index - 1 : index))
  }

  function onDropZone(container: Container, index: number) {
    if (drag?.kind === 'group') {
      if (container === 'root') dropGroupAt(index)
    } else dropItemAt(container, index)
    setDrag(null)
    setOver(null)
  }

  const zoneProps = (id: string, container: Container, index: number) => ({
    onDragOver: (e: React.DragEvent) => {
      if (!drag) return
      if (drag.kind === 'group' && container !== 'root') return
      e.preventDefault()
      if (over !== id) setOver(id)
    },
    onDragLeave: () => setOver((o) => (o === id ? null : o)),
    onDrop: (e: React.DragEvent) => {
      e.preventDefault()
      onDropZone(container, index)
    },
  })

  const dragProps = (d: Drag) => ({
    draggable: true,
    onDragStart: (e: React.DragEvent) => {
      setDrag(d)
      e.dataTransfer.effectAllowed = 'move'
      e.dataTransfer.setData('text/plain', d?.kind === 'item' ? d.key : (d?.id ?? ''))
    },
    onDragEnd: () => {
      setDrag(null)
      setOver(null)
    },
  })

  const zone = (id: string, container: Container, index: number) => (
    <div
      data-testid={`zone-${id}`}
      aria-hidden="true"
      className={`rounded transition-all ${over === id ? 'h-8 bg-indigo-100 ring-2 ring-indigo-400 dark:bg-indigo-500/20' : drag ? 'h-3' : 'h-1'}`}
      {...zoneProps(id, container, index)}
    />
  )

  // ---- pemindahan tanpa mouse ----
  function move(key: string, to: string) {
    setDraft((d) => moveItem(d, key, to as Container))
  }
  function shift(container: Container, key: string, delta: number) {
    const i = indexIn(container, key)
    if (i < 0) return
    setDraft((d) => moveItem(d, key, container, Math.max(0, i + delta)))
  }

  const destOptions = (
    <>
      <option value="root">Menu utama</option>
      {groups.map((g) => (
        <option key={g.id} value={g.id}>
          Grup: {g.label}
        </option>
      ))}
      <option value="hidden">Sembunyikan</option>
    </>
  )

  const itemRow = (k: string, container: Container, index: number, count: number) => {
    const def = menuItemByKey(k)
    const Icon = def?.icon
    const isHidden = container === 'hidden'
    return (
      <div
        data-testid={`item-${k}`}
        {...dragProps({ kind: 'item', key: k })}
        className={`flex items-center gap-2 rounded-xl border border-slate-200 bg-white px-2 py-1.5 dark:border-slate-700 dark:bg-slate-900 ${isHidden ? 'opacity-60' : ''}`}
      >
        <GripVertical className="h-4 w-4 shrink-0 cursor-grab text-slate-400" aria-hidden="true" />
        {Icon && <Icon className="h-4 w-4 shrink-0 text-slate-600 dark:text-slate-300" strokeWidth={1.75} aria-hidden="true" />}
        <span className="min-w-0 flex-1 truncate text-sm text-slate-800 dark:text-slate-200">{labelOf(k)}</span>
        <select aria-label={`Pindahkan ${labelOf(k)} ke`} value={container} onChange={(e) => move(k, e.target.value)} className={`${field} max-w-[9.5rem]`}>
          {destOptions}
        </select>
        {!isHidden && (
          <>
            <button type="button" aria-label={`Naikkan ${labelOf(k)}`} disabled={index === 0} onClick={() => shift(container, k, -1)} className={iconBtn}>
              <ArrowUp className="h-4 w-4" />
            </button>
            <button type="button" aria-label={`Turunkan ${labelOf(k)}`} disabled={index === count - 1} onClick={() => shift(container, k, 1)} className={iconBtn}>
              <ArrowDown className="h-4 w-4" />
            </button>
          </>
        )}
        <button type="button" aria-label={`${isHidden ? 'Tampilkan' : 'Sembunyikan'} ${labelOf(k)}`} onClick={() => setDraft((d) => toggleHidden(d, k))} className={iconBtn}>
          {isHidden ? <Eye className="h-4 w-4" /> : <EyeOff className="h-4 w-4" />}
        </button>
      </div>
    )
  }

  function submit() {
    setError(null)
    const msg = validateDraft(draft)
    if (msg) return setError(msg)
    save.mutate(toLayout(draft), {
      onSuccess: () => {
        toast.success('Susunan menu disimpan.', 'Menu')
        onClose()
      },
      onError: (err) => setError(err instanceof ApiError ? err.message : 'Gagal menyimpan susunan menu'),
    })
  }

  function resetToDefault() {
    reset.mutate(undefined, {
      onSuccess: () => {
        setDraft(draftFrom(DEFAULT_LAYOUT, allowed))
        toast.success('Menu dikembalikan ke susunan bawaan.', 'Menu')
        onClose()
      },
      onError: (err) => setError(err instanceof ApiError ? err.message : 'Gagal mengembalikan menu'),
    })
  }

  return (
    <Modal title="Atur menu" onClose={onClose} wide dismissable={!busy}>
      <p className="mb-3 text-sm text-slate-600 dark:text-slate-400">
        Seret menu (atau pakai tombol &amp; pemilih tujuan) untuk mengubah urutan, memasukkannya ke grup/sub menu, atau menyembunyikannya. Susunan ini
        hanya untuk akun Anda dan tidak mengubah hak akses.
      </p>

      <div className="space-y-1" role="group" aria-label="Susunan menu">
        {draft.nodes.map((n, i) =>
          n.type === 'item' ? (
            <div key={`i-${n.key}`}>
              {zone(`root-${i}`, 'root', i)}
              {itemRow(n.key!, 'root', i, draft.nodes.length)}
            </div>
          ) : (
            <div key={`g-${n.id}`}>
              {zone(`root-${i}`, 'root', i)}
              <div
                data-testid={`group-${n.id}`}
                className={`rounded-2xl border bg-slate-50 p-2 dark:bg-slate-800/40 ${over === `grp-${n.id}` ? 'border-indigo-400 ring-2 ring-indigo-300' : 'border-slate-200 dark:border-slate-700'}`}
                onDragOver={(e) => {
                  if (drag?.kind === 'item') {
                    e.preventDefault()
                    if (over !== `grp-${n.id}`) setOver(`grp-${n.id}`)
                  }
                }}
                onDragLeave={() => setOver((o) => (o === `grp-${n.id}` ? null : o))}
                onDrop={(e) => {
                  e.preventDefault()
                  if (drag?.kind === 'item') dropItemAt(n.id!, (n.children ?? []).length)
                  setDrag(null)
                  setOver(null)
                }}
              >
                <div className="flex items-center gap-2" {...dragProps({ kind: 'group', id: n.id! })}>
                  <GripVertical className="h-4 w-4 shrink-0 cursor-grab text-slate-400" aria-hidden="true" />
                  <input
                    aria-label={`Nama grup ${groups.findIndex((g) => g.id === n.id) + 1}`}
                    value={n.label ?? ''}
                    onChange={(e) => setDraft((d) => renameGroup(d, n.id!, e.target.value))}
                    maxLength={MENU_LIMITS.label}
                    minLength={1}
                    pattern=".*\S.*"
                    required
                    autoComplete="off"
                    draggable={false}
                    className={`${field} min-w-0 flex-1 font-medium`}
                  />
                  <button
                    type="button"
                    aria-pressed={!!n.collapsed}
                    aria-label={`Grup ${n.label} ${n.collapsed ? 'tertutup' : 'terbuka'} secara bawaan`}
                    title="Tertutup/terbuka secara bawaan"
                    onClick={() => setDraft((d) => toggleGroupCollapsed(d, n.id!))}
                    className={iconBtn}
                  >
                    <ChevronDown className={`h-4 w-4 transition-transform ${n.collapsed ? '-rotate-90' : ''}`} />
                  </button>
                  <button type="button" aria-label={`Hapus grup ${n.label}`} onClick={() => setDraft((d) => removeGroup(d, n.id!))} className={`${iconBtn} text-red-600 dark:text-red-400`}>
                    <Trash2 className="h-4 w-4" />
                  </button>
                </div>
                <div className="mt-1 space-y-1 pl-6">
                  {(n.children ?? []).length === 0 && <p className="px-2 py-2 text-xs text-slate-500 dark:text-slate-400">Grup kosong — seret menu ke sini.</p>}
                  {(n.children ?? []).map((k, ci) => (
                    <div key={k}>
                      {zone(`${n.id}-${ci}`, n.id!, ci)}
                      {itemRow(k, n.id!, ci, (n.children ?? []).length)}
                    </div>
                  ))}
                  {zone(`${n.id}-end`, n.id!, (n.children ?? []).length)}
                </div>
              </div>
            </div>
          ),
        )}
        {zone('root-end', 'root', draft.nodes.length)}
      </div>

      <div className="mt-4">
        <h3 className="mb-1 text-xs font-semibold uppercase tracking-wide text-slate-600 dark:text-slate-400">Disembunyikan ({draft.hidden.length})</h3>
        <div className={`min-h-10 space-y-1 rounded-2xl border border-dashed p-2 ${over === 'hidden' ? 'border-indigo-400 bg-indigo-50 dark:bg-indigo-500/10' : 'border-slate-300 dark:border-slate-700'}`} {...zoneProps('hidden', 'hidden', 0)}>
          {draft.hidden.length === 0 && <p className="px-2 py-1 text-xs text-slate-500 dark:text-slate-400">Seret menu ke sini untuk menyembunyikannya.</p>}
          {draft.hidden.map((k, i) => (
            <div key={k}>{itemRow(k, 'hidden', i, draft.hidden.length)}</div>
          ))}
        </div>
      </div>

      {error && (
        <p role="alert" className="mt-3 text-sm text-red-600 dark:text-red-400">
          {error}
        </p>
      )}

      <div className="mt-4 flex flex-wrap items-center justify-between gap-2">
        <div className="flex gap-2">
          <button type="button" onClick={() => setDraft((d) => addGroup(d))} disabled={groups.length >= MENU_LIMITS.groups} className="inline-flex items-center gap-1.5 rounded-full border border-slate-300 px-3 py-2 text-sm font-medium text-slate-700 hover:bg-slate-50 disabled:opacity-50 dark:border-slate-700 dark:text-slate-300 dark:hover:bg-slate-800">
            <FolderPlus className="h-4 w-4" /> Tambah grup
          </button>
          <button type="button" onClick={resetToDefault} disabled={busy} className="inline-flex items-center gap-1.5 rounded-full px-3 py-2 text-sm font-medium text-slate-700 hover:bg-slate-100 disabled:opacity-50 dark:text-slate-300 dark:hover:bg-slate-800">
            <RotateCcw className="h-4 w-4" /> Kembalikan bawaan
          </button>
        </div>
        <div className="flex gap-2">
          <button type="button" onClick={onClose} disabled={busy} className="rounded-full border border-slate-300 px-3 py-2 text-sm text-slate-700 hover:bg-slate-50 dark:border-slate-700 dark:text-slate-300 dark:hover:bg-slate-800">
            Batal
          </button>
          <button type="button" onClick={submit} disabled={busy} className="rounded-full bg-indigo-600 px-4 py-2 text-sm font-semibold text-white hover:bg-indigo-500 disabled:opacity-50">
            {save.isPending ? 'Menyimpan…' : 'Simpan'}
          </button>
        </div>
      </div>
    </Modal>
  )
}
