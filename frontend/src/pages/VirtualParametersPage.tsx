import { useState, type FormEvent } from 'react'
import { AlertCircle, FunctionSquare, Globe2, Pencil, Play, Plus, Trash2 } from 'lucide-react'
import { useI18n } from '../lib/i18n'
import { useAuth } from '../lib/auth'
import { ApiError } from '../lib/api'
import { useToast } from '../lib/toast'
import { useConfirm } from '../lib/confirm'
import { Modal } from '../components/Modal'
import { EmptyState } from '../components/EmptyState'
import { PageSpinner } from '../components/Spinner'
import {
  useCreateVirtualParameter,
  useDeleteVirtualParameter,
  useTestVirtualParameter,
  useUpdateVirtualParameter,
  useVirtualParameters,
} from '../lib/hooks'
import type { VirtualParameter, VirtualParameterTestResult } from '../lib/types'

const inputCls =
  'w-full rounded-lg border border-slate-300 px-3 py-2 text-sm text-slate-900 outline-none transition-colors focus:border-indigo-600 focus:ring-1 focus:ring-indigo-600 dark:border-slate-700 dark:bg-slate-900 dark:text-slate-100'
const labelCls = 'mb-1 block text-sm font-medium text-slate-600 dark:text-slate-400'
const primaryBtnCls =
  'flex items-center justify-center gap-2 rounded-full bg-indigo-600 px-3 py-2 text-sm font-medium text-white transition-colors hover:bg-indigo-700 disabled:cursor-not-allowed disabled:opacity-60 dark:bg-indigo-300 dark:text-slate-900 dark:hover:bg-indigo-200'
const secondaryBtnCls =
  'flex items-center justify-center gap-2 rounded-full border border-slate-300 px-3 py-2 text-sm font-medium text-slate-700 transition-colors hover:bg-slate-50 disabled:cursor-not-allowed disabled:opacity-60 dark:border-slate-700 dark:text-slate-300 dark:hover:bg-slate-800'

// Contoh yang bisa disalin; semuanya hanya memakai fungsi yang disediakan pkg/vexpr (tanpa skrip).
const EXAMPLES: { title: string; expr: string }[] = [
  { title: 'Redaman dBm dari satuan 0,001', expr: 'round({InternetGatewayDevice.X_ZTE.OpticalRx} / 1000, 1)' },
  { title: 'Uptime dalam hari (TR-098 atau TR-181)', expr: 'round(first({InternetGatewayDevice.DeviceInfo.UpTime}, {Device.DeviceInfo.UpTime}) / 86400, 2)' },
  { title: 'Uptime terbaca manusia', expr: 'duration({InternetGatewayDevice.DeviceInfo.UpTime})' },
  { title: 'Status redaman', expr: "if({InternetGatewayDevice.X_ZTE.OpticalRx} < -27000, 'kritis', 'normal')" },
]

export default function VirtualParametersPage() {
  const { t: tl } = useI18n()
  const { hasRole } = useAuth()
  const canManage = hasRole('ADMIN')
  const isSuper = hasRole('SUPERADMIN')
  // Definisi GLOBAL dipakai semua tenant: hanya SUPERADMIN yang boleh mengubah/menghapus (backend menolak ADMIN dengan 403).
  const canMutate = (v: VirtualParameter) => canManage && (v.tenant_id != null || isSuper)
  const { data, isLoading } = useVirtualParameters()
  const rows = data?.data ?? []
  const toast = useToast()
  const confirm = useConfirm()
  const del = useDeleteVirtualParameter()
  const toggle = useUpdateVirtualParameter()
  const [editor, setEditor] = useState<{ vp: VirtualParameter | null } | null>(null)

  async function handleDelete(v: VirtualParameter) {
    const ok = await confirm({
      title: 'Hapus virtual parameter',
      message: `"${v.name}" tidak akan dihitung lagi. Nilai turunan tidak disimpan, jadi tidak ada data perangkat yang hilang.`,
      confirmLabel: 'Hapus',
      tone: 'danger',
    })
    if (!ok) return
    del.mutate(v.id, {
      onSuccess: () => toast.success(`Virtual parameter "${v.name}" dihapus.`),
      onError: (e) => toast.error(e instanceof ApiError ? e.message : 'Gagal menghapus'),
    })
  }

  return (
    <div className="mx-auto max-w-7xl px-6 py-8">
      <div className="mb-6 flex flex-wrap items-center justify-between gap-3">
        <div>
          <h1 className="text-xl font-semibold text-slate-900 dark:text-slate-100">{tl('nav.virtualParameters')}</h1>
          <p className="mt-0.5 max-w-2xl text-sm text-slate-500 dark:text-slate-400">
            Nilai turunan yang dihitung dari parameter perangkat lewat ekspresi (mis. redaman dBm, uptime hari). Dihitung saat dibaca — tidak ada skrip dan
            tidak ada data yang disimpan.
          </p>
        </div>
        {canManage && (
          <button onClick={() => setEditor({ vp: null })} className={primaryBtnCls}>
            <Plus className="h-4 w-4" /> Virtual Parameter Baru
          </button>
        )}
      </div>

      <div className="overflow-hidden rounded-xl border border-slate-200 bg-white shadow-sm dark:border-slate-800 dark:bg-slate-900">
        {isLoading ? (
          <PageSpinner />
        ) : rows.length === 0 ? (
          <EmptyState
            icon={FunctionSquare}
            title="Belum ada virtual parameter"
            description="Buat satu untuk menampilkan nilai turunan (mis. redaman dBm) di detail perangkat dan di daftar via vparams=."
          />
        ) : (
          <div className="table-scroll">
            <table className="w-full text-left text-sm">
              <thead>
                <tr className="border-b border-slate-200 bg-slate-50 text-xs font-medium uppercase tracking-wide text-slate-500 dark:border-slate-800 dark:bg-slate-800/50 dark:text-slate-400">
                  <th className="px-5 py-3">Nama</th>
                  <th className="px-5 py-3">Ekspresi</th>
                  <th className="px-5 py-3">Satuan</th>
                  <th className="px-5 py-3">Lingkup</th>
                  <th className="px-5 py-3">Aktif</th>
                  <th className="px-5 py-3"></th>
                </tr>
              </thead>
              <tbody className="divide-y divide-slate-100 dark:divide-slate-800">
                {rows.map((v) => (
                  <tr key={v.id} className="align-top">
                    <td className="px-5 py-3.5">
                      <p className="font-medium text-slate-900 dark:text-slate-100">{v.label}</p>
                      <p className="font-mono text-xs text-slate-500 dark:text-slate-400">{v.name}</p>
                    </td>
                    <td className="max-w-md px-5 py-3.5">
                      <code className="break-all rounded bg-slate-100 px-1.5 py-0.5 text-xs text-slate-700 dark:bg-slate-800 dark:text-slate-300">{v.expression}</code>
                    </td>
                    <td className="px-5 py-3.5 text-slate-600 dark:text-slate-400">{v.unit ?? '-'}</td>
                    <td className="px-5 py-3.5">
                      {v.tenant_id == null ? (
                        <span className="inline-flex items-center gap-1 rounded-full bg-indigo-50 px-2 py-0.5 text-xs font-medium text-indigo-700 dark:bg-indigo-950/40 dark:text-indigo-300">
                          <Globe2 className="h-3 w-3" /> Global
                        </span>
                      ) : (
                        <span className="text-xs text-slate-500 dark:text-slate-400">Tenant</span>
                      )}
                    </td>
                    <td className="px-5 py-3.5">
                      {canMutate(v) ? (
                        <input
                          type="checkbox"
                          checked={v.is_active}
                          aria-label={`Aktif: ${v.name}`}
                          disabled={toggle.isPending}
                          onChange={(e) =>
                            toggle.mutate(
                              { id: v.id, input: { is_active: e.target.checked } },
                              { onError: (err) => toast.error(err instanceof ApiError ? err.message : 'Gagal mengubah') },
                            )
                          }
                          className="h-4 w-4 rounded border-slate-300 accent-indigo-600"
                        />
                      ) : (
                        <span className="text-xs text-slate-500 dark:text-slate-400">{v.is_active ? 'Ya' : 'Tidak'}</span>
                      )}
                    </td>
                    <td className="px-5 py-3.5 text-right">
                      {canMutate(v) && (
                        <div className="flex justify-end gap-1">
                          <button
                            onClick={() => setEditor({ vp: v })}
                            title="Edit"
                            aria-label={`Edit ${v.name}`}
                            className="rounded-md p-1.5 text-slate-500 transition-colors hover:bg-slate-100 hover:text-slate-800 dark:text-slate-400 dark:hover:bg-slate-800"
                          >
                            <Pencil className="h-4 w-4" />
                          </button>
                          <button
                            onClick={() => void handleDelete(v)}
                            title="Hapus"
                            aria-label={`Hapus ${v.name}`}
                            className="rounded-md p-1.5 text-slate-500 transition-colors hover:bg-red-50 hover:text-red-600 dark:text-slate-400"
                          >
                            <Trash2 className="h-4 w-4" />
                          </button>
                        </div>
                      )}
                    </td>
                  </tr>
                ))}
              </tbody>
            </table>
          </div>
        )}
      </div>

      {editor && <VirtualParameterModal vp={editor.vp} isSuper={isSuper} onClose={() => setEditor(null)} />}
    </div>
  )
}

function VirtualParameterModal({ vp, isSuper, onClose }: { vp: VirtualParameter | null; isSuper: boolean; onClose: () => void }) {
  const editing = vp != null
  const [name, setName] = useState(vp?.name ?? '')
  const [label, setLabel] = useState(vp?.label ?? '')
  const [unit, setUnit] = useState(vp?.unit ?? '')
  const [description, setDescription] = useState(vp?.description ?? '')
  const [expression, setExpression] = useState(vp?.expression ?? '')
  const [deviceId, setDeviceId] = useState('')
  const [result, setResult] = useState<VirtualParameterTestResult | null>(null)
  const [error, setError] = useState<string | null>(null)
  const create = useCreateVirtualParameter()
  const update = useUpdateVirtualParameter()
  const test = useTestVirtualParameter()
  const toast = useToast()
  const pending = create.isPending || update.isPending

  async function runTest() {
    setResult(null)
    setError(null)
    const id = Number(deviceId)
    if (!Number.isInteger(id) || id <= 0) {
      setError('Isi ID perangkat untuk mencoba ekspresi.')
      return
    }
    try {
      setResult(await test.mutateAsync({ expression, device_id: id }))
    } catch (err) {
      setError(err instanceof ApiError ? err.message : 'Gagal menguji ekspresi')
    }
  }

  async function handleSubmit(e: FormEvent) {
    e.preventDefault()
    setError(null)
    try {
      if (editing) {
        await update.mutateAsync({ id: vp.id, input: { name, label, unit, description, expression } })
      } else {
        // Tanpa tenant_id: ADMIN = tenant sendiri (ditentukan server dari akun); SUPERADMIN = definisi global.
        await create.mutateAsync({ name, label, unit: unit || null, description: description || null, expression })
      }
      toast.success(`Virtual parameter "${name}" disimpan.`)
      onClose()
    } catch (err) {
      setError(err instanceof ApiError ? err.message : 'Gagal menyimpan')
    }
  }

  return (
    <Modal title={editing ? `Edit — ${vp.name}` : 'Virtual Parameter Baru'} onClose={onClose} wide>
      <form onSubmit={handleSubmit} className="space-y-4">
        <div className="grid gap-4 sm:grid-cols-2">
          <div>
            <label htmlFor="vp-name" className={labelCls}>Nama (kunci)</label>
            <input
              id="vp-name"
              required
              maxLength={64}
              pattern="[a-z][a-z0-9_]{0,63}"
              title="Huruf kecil, angka, underscore; diawali huruf"
              value={name}
              onChange={(e) => setName(e.target.value)}
              placeholder="rx_dbm"
              className={`${inputCls} font-mono`}
            />
          </div>
          <div>
            <label htmlFor="vp-label" className={labelCls}>Label</label>
            <input id="vp-label" required pattern=".*\S.*" title="Tidak boleh kosong atau hanya spasi" maxLength={255} value={label} onChange={(e) => setLabel(e.target.value)} placeholder="Redaman RX" className={inputCls} />
          </div>
          <div>
            <label htmlFor="vp-unit" className={labelCls}>Satuan (opsional)</label>
            <input id="vp-unit" maxLength={32} value={unit} onChange={(e) => setUnit(e.target.value)} placeholder="dBm" className={inputCls} />
          </div>
          <div>
            <label htmlFor="vp-desc" className={labelCls}>Deskripsi (opsional)</label>
            <input id="vp-desc" maxLength={1024} value={description} onChange={(e) => setDescription(e.target.value)} className={inputCls} />
          </div>
        </div>

        <div>
          <label htmlFor="vp-expr" className={labelCls}>Ekspresi</label>
          <textarea
            id="vp-expr"
            required
            minLength={1}
            rows={3}
            maxLength={1000}
            spellCheck={false}
            value={expression}
            onChange={(e) => {
              setExpression(e.target.value)
              setResult(null)
            }}
            placeholder="round({InternetGatewayDevice.X_ZTE.OpticalRx} / 1000, 1)"
            className={`${inputCls} font-mono text-xs`}
          />
          <p className="mt-1 text-xs text-slate-500 dark:text-slate-400">
            <code>{'{Nama.Parameter}'}</code> = nilai parameter · operator <code>+ - * / %</code> <code>== != &lt; &gt;</code> <code>and or not</code> · fungsi{' '}
            <code>first if round floor ceil abs min max num str concat upper lower trim len contains startswith endswith replace substr duration isnull</code>.
            Parameter rahasia (password/PSK) tidak boleh dirujuk.
          </p>
          <div className="mt-2 flex flex-wrap gap-1.5">
            {EXAMPLES.map((ex) => (
              <button
                key={ex.title}
                type="button"
                onClick={() => {
                  setExpression(ex.expr)
                  setResult(null)
                }}
                className="rounded-full border border-slate-200 px-2.5 py-1 text-xs text-slate-600 hover:bg-slate-50 dark:border-slate-700 dark:text-slate-400 dark:hover:bg-slate-800"
                title={ex.expr}
              >
                {ex.title}
              </button>
            ))}
          </div>
        </div>

        {!editing && isSuper && (
          <p className="rounded-lg bg-indigo-50 px-3 py-2 text-xs text-indigo-800 dark:bg-indigo-950/40 dark:text-indigo-200">
            Dibuat sebagai definisi <b>global</b>: berlaku untuk semua tenant; definisi tenant bernama sama menimpanya.
          </p>
        )}

        <div className="rounded-xl border border-slate-200 p-3 dark:border-slate-800">
          <p className="text-sm font-medium text-slate-700 dark:text-slate-300">Coba pada perangkat</p>
          <div className="mt-2 flex flex-wrap items-center gap-2">
            <input
              type="number"
              min={1}
              max={999999999999}
              aria-label="ID perangkat untuk uji"
              placeholder="ID perangkat"
              value={deviceId}
              onChange={(e) => setDeviceId(e.target.value)}
              className={`max-w-[160px] ${inputCls}`}
            />
            <button type="button" onClick={() => void runTest()} disabled={test.isPending || !expression.trim()} className={secondaryBtnCls}>
              <Play className="h-4 w-4" /> {test.isPending ? 'Menguji…' : 'Uji'}
            </button>
          </div>
          {result && (
            <div className="mt-3 text-sm" role="status">
              {result.ok ? (
                <>
                  <p className="text-slate-700 dark:text-slate-300">
                    Hasil:{' '}
                    {result.value == null ? (
                      <span className="text-amber-700 dark:text-amber-400">belum dapat dihitung (parameter sumber belum dilaporkan)</span>
                    ) : (
                      <span className="font-mono font-semibold text-emerald-700 dark:text-emerald-400">
                        {result.value}
                        {unit ? ` ${unit}` : ''}
                      </span>
                    )}
                  </p>
                  {result.refs.length > 0 && (
                    <ul className="mt-1.5 space-y-0.5 text-xs text-slate-500 dark:text-slate-400">
                      {result.refs.map((r) => (
                        <li key={r.name} className="break-all">
                          <code>{r.name}</code> = {r.value == null ? <em>belum dilaporkan</em> : <span className="font-mono">{r.value}</span>}
                        </li>
                      ))}
                    </ul>
                  )}
                </>
              ) : (
                <p className="text-red-600 dark:text-red-400">
                  <AlertCircle className="mr-1 inline h-4 w-4" />
                  {result.error?.message ?? 'Ekspresi tidak valid'}
                  {result.error && result.error.pos > 0 ? ` (posisi ${result.error.pos})` : ''}
                </p>
              )}
            </div>
          )}
        </div>

        {error && (
          <p role="alert" className="text-sm text-red-600 dark:text-red-400">
            {error}
          </p>
        )}
        <div className="pt-2">
          <button type="submit" disabled={pending} className={`${primaryBtnCls} w-full`}>
            {pending ? 'Menyimpan...' : editing ? 'Simpan Perubahan' : 'Simpan'}
          </button>
        </div>
      </form>
    </Modal>
  )
}
