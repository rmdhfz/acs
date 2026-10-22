import { useState, type FormEvent } from 'react'
import { AlertCircle, CheckCircle2, History, Pencil, Play, Plus, RotateCcw, Trash2, Workflow, XCircle } from 'lucide-react'
import { useI18n } from '../lib/i18n'
import { useAuth } from '../lib/auth'
import { ApiError } from '../lib/api'
import { useToast } from '../lib/toast'
import { useConfirm } from '../lib/confirm'
import { Modal } from '../components/Modal'
import { EmptyState } from '../components/EmptyState'
import { PageSpinner } from '../components/Spinner'
import {
  useAutomationRules,
  useAutomationRuns,
  useCreateAutomationRule,
  useDeleteAutomationRule,
  useResetAutomationRule,
  useTags,
  useTestAutomationRule,
  useUpdateAutomationRule,
} from '../lib/hooks'
import { formatRelativeTime } from '../lib/format'
import type { AutomationAction, AutomationActionType, AutomationRule, AutomationTestResult, AutomationTrigger } from '../lib/types'

const inputCls =
  'w-full rounded-lg border border-slate-300 px-3 py-2 text-sm text-slate-900 outline-none transition-colors focus:border-indigo-600 focus:ring-1 focus:ring-indigo-600 dark:border-slate-700 dark:bg-slate-900 dark:text-slate-100'
const labelCls = 'mb-1 block text-sm font-medium text-slate-600 dark:text-slate-400'
const primaryBtnCls =
  'flex items-center justify-center gap-2 rounded-full bg-indigo-600 px-3 py-2 text-sm font-medium text-white transition-colors hover:bg-indigo-700 disabled:cursor-not-allowed disabled:opacity-60 dark:bg-indigo-300 dark:text-slate-900 dark:hover:bg-indigo-200'
const secondaryBtnCls =
  'flex items-center justify-center gap-2 rounded-full border border-slate-300 px-3 py-2 text-sm font-medium text-slate-700 transition-colors hover:bg-slate-50 disabled:cursor-not-allowed disabled:opacity-60 dark:border-slate-700 dark:text-slate-300 dark:hover:bg-slate-800'

const TRIGGERS: { value: AutomationTrigger; label: string }[] = [
  { value: 'EVERY_INFORM', label: 'Setiap Inform' },
  { value: 'BOOTSTRAP_OR_BOOT', label: 'Saat boot / bootstrap' },
  { value: 'BOOTSTRAP_ONLY', label: 'Hanya bootstrap (pertama kali)' },
]
const ACTION_LABEL: Record<AutomationActionType, string> = {
  set_parameter: 'Atur parameter',
  reboot: 'Reboot',
  get_parameter_values: 'Baca parameter',
  add_tag: 'Beri tag',
  webhook: 'Kirim webhook',
}
const MAX_ACTIONS = 10

function describeAction(a: AutomationAction, tagName?: (id: number) => string): string {
  switch (a.type) {
    case 'set_parameter':
      return `${ACTION_LABEL.set_parameter}: ${a.name} = ${a.value}`
    case 'get_parameter_values':
      return `${ACTION_LABEL.get_parameter_values} (${a.names?.length ?? 0})`
    case 'add_tag':
      return `${ACTION_LABEL.add_tag}: ${a.tag_id != null && tagName ? tagName(a.tag_id) : a.tag_id}`
  }
  return ACTION_LABEL[a.type]
}

export default function AutomationPage() {
  const { t: tl } = useI18n()
  const { hasRole } = useAuth()
  const canManage = hasRole('ADMIN')
  const { data, isLoading } = useAutomationRules()
  const rows = data?.data ?? []
  const toast = useToast()
  const confirm = useConfirm()
  const del = useDeleteAutomationRule()
  const toggle = useUpdateAutomationRule()
  const { data: tagsResp } = useTags({ pageSize: 100 })
  const tagName = (id: number) => tagsResp?.data?.find((t) => t.id === id)?.name ?? String(id)
  const [editor, setEditor] = useState<{ rule: AutomationRule | null } | null>(null)
  const [history, setHistory] = useState<AutomationRule | null>(null)

  async function handleDelete(r: AutomationRule) {
    const ok = await confirm({
      title: 'Hapus aturan otomasi',
      message: `Aturan "${r.name}" berhenti dievaluasi. Task yang sudah diantre tetap berjalan.`,
      confirmLabel: 'Hapus',
      tone: 'danger',
    })
    if (!ok) return
    del.mutate(r.id, {
      onSuccess: () => toast.success(`Aturan "${r.name}" dihapus.`),
      onError: (e) => toast.error(e instanceof ApiError ? e.message : 'Gagal menghapus'),
    })
  }

  return (
    <div className="mx-auto max-w-7xl px-6 py-8">
      <div className="mb-6 flex flex-wrap items-center justify-between gap-3">
        <div>
          <h1 className="text-xl font-semibold text-slate-900 dark:text-slate-100">{tl('nav.automation')}</h1>
          <p className="mt-0.5 max-w-2xl text-sm text-slate-500 dark:text-slate-400">
            “Jika kondisi, maka aksi” yang dievaluasi saat perangkat Inform — tanpa skrip. Tiap aturan punya jeda (cooldown), berhenti sendiri bila tak kunjung
            konvergen, dan tidak memicu perangkat yang masih punya task tertunda.
          </p>
        </div>
        {canManage && (
          <button onClick={() => setEditor({ rule: null })} className={primaryBtnCls}>
            <Plus className="h-4 w-4" /> Aturan Baru
          </button>
        )}
      </div>

      <div className="overflow-hidden rounded-xl border border-slate-200 bg-white shadow-sm dark:border-slate-800 dark:bg-slate-900">
        {isLoading ? (
          <PageSpinner />
        ) : rows.length === 0 ? (
          <EmptyState
            icon={Workflow}
            title="Belum ada aturan otomasi"
            description="Contoh: jika redaman < -27 dBm maka beri tag “redaman-buruk”; atau seragamkan SSID saat perangkat pertama kali terhubung."
          />
        ) : (
          <div className="table-scroll">
            <table className="w-full text-left text-sm">
              <thead>
                <tr className="border-b border-slate-200 bg-slate-50 text-xs font-medium uppercase tracking-wide text-slate-500 dark:border-slate-800 dark:bg-slate-800/50 dark:text-slate-400">
                  <th className="px-5 py-3">Aturan</th>
                  <th className="px-5 py-3">Kondisi</th>
                  <th className="px-5 py-3">Aksi</th>
                  <th className="px-5 py-3">Jeda</th>
                  <th className="px-5 py-3">Aktif</th>
                  <th className="px-5 py-3"></th>
                </tr>
              </thead>
              <tbody className="divide-y divide-slate-100 dark:divide-slate-800">
                {rows.map((r) => (
                  <tr key={r.id} className="align-top">
                    <td className="px-5 py-3.5">
                      <p className="font-medium text-slate-900 dark:text-slate-100">{r.name}</p>
                      <p className="text-xs text-slate-500 dark:text-slate-400">{TRIGGERS.find((t) => t.value === r.trigger_event)?.label ?? r.trigger_event}</p>
                    </td>
                    <td className="max-w-xs px-5 py-3.5">
                      {r.condition ? (
                        <code className="break-all rounded bg-slate-100 px-1.5 py-0.5 text-xs text-slate-700 dark:bg-slate-800 dark:text-slate-300">{r.condition}</code>
                      ) : (
                        <span className="text-xs text-slate-500 dark:text-slate-400">selalu</span>
                      )}
                    </td>
                    <td className="max-w-sm px-5 py-3.5">
                      <ul className="space-y-0.5 text-xs text-slate-700 dark:text-slate-300">
                        {r.actions.map((a, i) => (
                          <li key={i} className="break-all">
                            {describeAction(a, tagName)}
                          </li>
                        ))}
                      </ul>
                    </td>
                    <td className="px-5 py-3.5 text-xs text-slate-600 dark:text-slate-400">{Math.round(r.cooldown_seconds / 60)} mnt</td>
                    <td className="px-5 py-3.5">
                      {canManage ? (
                        <input
                          type="checkbox"
                          checked={r.is_active}
                          aria-label={`Aktif: ${r.name}`}
                          disabled={toggle.isPending}
                          onChange={(e) =>
                            toggle.mutate(
                              { id: r.id, input: { is_active: e.target.checked } },
                              { onError: (err) => toast.error(err instanceof ApiError ? err.message : 'Gagal mengubah') },
                            )
                          }
                          className="h-4 w-4 rounded border-slate-300 accent-indigo-600"
                        />
                      ) : (
                        <span className="text-xs text-slate-500 dark:text-slate-400">{r.is_active ? 'Ya' : 'Tidak'}</span>
                      )}
                    </td>
                    <td className="px-5 py-3.5 text-right">
                      <div className="flex justify-end gap-1">
                        <button
                          onClick={() => setHistory(r)}
                          title="Riwayat"
                          aria-label={`Riwayat ${r.name}`}
                          className="rounded-md p-1.5 text-slate-500 transition-colors hover:bg-slate-100 hover:text-slate-800 dark:text-slate-400 dark:hover:bg-slate-800"
                        >
                          <History className="h-4 w-4" />
                        </button>
                        {canManage && (
                          <>
                            <button
                              onClick={() => setEditor({ rule: r })}
                              title="Edit"
                              aria-label={`Edit ${r.name}`}
                              className="rounded-md p-1.5 text-slate-500 transition-colors hover:bg-slate-100 hover:text-slate-800 dark:text-slate-400 dark:hover:bg-slate-800"
                            >
                              <Pencil className="h-4 w-4" />
                            </button>
                            <button
                              onClick={() => void handleDelete(r)}
                              title="Hapus"
                              aria-label={`Hapus ${r.name}`}
                              className="rounded-md p-1.5 text-slate-500 transition-colors hover:bg-red-50 hover:text-red-600 dark:text-slate-400"
                            >
                              <Trash2 className="h-4 w-4" />
                            </button>
                          </>
                        )}
                      </div>
                    </td>
                  </tr>
                ))}
              </tbody>
            </table>
          </div>
        )}
      </div>

      {editor && <RuleModal rule={editor.rule} onClose={() => setEditor(null)} />}
      {history && <HistoryModal rule={history} canManage={canManage} onClose={() => setHistory(null)} />}
    </div>
  )
}

function RuleModal({ rule, onClose }: { rule: AutomationRule | null; onClose: () => void }) {
  const editing = rule != null
  const [name, setName] = useState(rule?.name ?? '')
  const [description, setDescription] = useState(rule?.description ?? '')
  const [trigger, setTrigger] = useState<AutomationTrigger>(rule?.trigger_event ?? 'EVERY_INFORM')
  const [condition, setCondition] = useState(rule?.condition ?? '')
  const [actions, setActions] = useState<AutomationAction[]>(rule?.actions ?? [{ type: 'reboot' }])
  const [cooldownMin, setCooldownMin] = useState(String(Math.round((rule?.cooldown_seconds ?? 3600) / 60)))
  const [maxFires, setMaxFires] = useState(String(rule?.max_consecutive_fires ?? 3))
  const [deviceId, setDeviceId] = useState('')
  const [result, setResult] = useState<AutomationTestResult | null>(null)
  const [error, setError] = useState<string | null>(null)
  const create = useCreateAutomationRule()
  const update = useUpdateAutomationRule()
  const test = useTestAutomationRule()
  const { data: tagsResp } = useTags({ pageSize: 100 })
  const tags = tagsResp?.data ?? []
  const toast = useToast()
  const pending = create.isPending || update.isPending

  const patchAction = (i: number, patch: Partial<AutomationAction>) => {
    setActions((prev) => prev.map((a, j) => (j === i ? { ...a, ...patch } : a)))
    setResult(null)
  }
  const changeType = (i: number, type: AutomationActionType) => {
    setActions((prev) => prev.map((a, j) => (j === i ? { type } : a)))
    setResult(null)
  }

  async function runTest() {
    setResult(null)
    setError(null)
    const id = Number(deviceId)
    if (!Number.isInteger(id) || id <= 0) {
      setError('Isi ID perangkat untuk mencoba aturan.')
      return
    }
    try {
      setResult(await test.mutateAsync({ condition, actions, device_id: id }))
    } catch (err) {
      setError(err instanceof ApiError ? err.message : 'Gagal menguji aturan')
    }
  }

  async function handleSubmit(e: FormEvent) {
    e.preventDefault()
    setError(null)
    const input = {
      name,
      description: description || null,
      trigger_event: trigger,
      condition,
      actions,
      cooldown_seconds: Math.round(Number(cooldownMin) * 60),
      max_consecutive_fires: Number(maxFires),
    }
    try {
      if (editing) await update.mutateAsync({ id: rule.id, input })
      else await create.mutateAsync(input)
      toast.success(`Aturan "${name}" disimpan.`)
      onClose()
    } catch (err) {
      setError(err instanceof ApiError ? err.message : 'Gagal menyimpan')
    }
  }

  return (
    <Modal title={editing ? `Edit — ${rule.name}` : 'Aturan Otomasi Baru'} onClose={onClose} wide>
      <form onSubmit={handleSubmit} className="space-y-4">
        <div className="grid gap-4 sm:grid-cols-2">
          <div>
            <label htmlFor="au-name" className={labelCls}>Nama</label>
            <input id="au-name" required pattern=".*\S.*" title="Tidak boleh kosong atau hanya spasi" maxLength={128} value={name} onChange={(e) => setName(e.target.value)} className={inputCls} />
          </div>
          <div>
            <label htmlFor="au-trigger" className={labelCls}>Kapan dievaluasi</label>
            <select id="au-trigger" value={trigger} onChange={(e) => setTrigger(e.target.value as AutomationTrigger)} className={inputCls}>
              {TRIGGERS.map((t) => (
                <option key={t.value} value={t.value}>
                  {t.label}
                </option>
              ))}
            </select>
          </div>
        </div>
        <div>
          <label htmlFor="au-desc" className={labelCls}>Deskripsi (opsional)</label>
          <input id="au-desc" maxLength={1024} value={description} onChange={(e) => setDescription(e.target.value)} className={inputCls} />
        </div>

        <div>
          <label htmlFor="au-cond" className={labelCls}>Kondisi (kosong = selalu)</label>
          <textarea
            id="au-cond"
            rows={2}
            maxLength={1000}
            spellCheck={false}
            value={condition}
            onChange={(e) => {
              setCondition(e.target.value)
              setResult(null)
            }}
            placeholder="{InternetGatewayDevice.X_ZTE.OpticalRx} < -27000 and startswith({@serial_number}, 'ZTE')"
            className={`${inputCls} font-mono text-xs`}
          />
          <p className="mt-1 text-xs text-slate-500 dark:text-slate-400">
            Parameter: <code>{'{Nama.Parameter}'}</code> · konteks: <code>{'{@serial_number}'}</code> <code>{'{@oui}'}</code> <code>{'{@product_class}'}</code>{' '}
            <code>{'{@software_version}'}</code> <code>{'{@hardware_version}'}</code> <code>{'{@ip_address}'}</code> <code>{'{@events}'}</code>. Fungsi dan operator sama dengan
            virtual parameter. Parameter rahasia tidak boleh dirujuk.
          </p>
        </div>

        <div>
          <p className={labelCls}>Aksi ({actions.length}/{MAX_ACTIONS})</p>
          <div className="space-y-2">
            {actions.map((a, i) => (
              <div key={i} className="rounded-lg border border-slate-200 p-3 dark:border-slate-800">
                <div className="flex items-center gap-2">
                  <select aria-label={`Jenis aksi ${i + 1}`} value={a.type} onChange={(e) => changeType(i, e.target.value as AutomationActionType)} className={`max-w-[220px] ${inputCls}`}>
                    {(Object.keys(ACTION_LABEL) as AutomationActionType[]).map((t) => (
                      <option key={t} value={t}>
                        {ACTION_LABEL[t]}
                      </option>
                    ))}
                  </select>
                  <button
                    type="button"
                    aria-label={`Hapus aksi ${i + 1}`}
                    onClick={() => {
                      setActions((prev) => prev.filter((_, j) => j !== i))
                      setResult(null)
                    }}
                    className="ml-auto rounded-md p-1.5 text-slate-500 hover:bg-red-50 hover:text-red-600 dark:text-slate-400"
                  >
                    <Trash2 className="h-4 w-4" />
                  </button>
                </div>
                {a.type === 'set_parameter' && (
                  <div className="mt-2 grid gap-2 sm:grid-cols-2">
                    <input
                      aria-label={`Parameter aksi ${i + 1}`}
                      maxLength={256}
                      value={a.name ?? ''}
                      onChange={(e) => patchAction(i, { name: e.target.value })}
                      placeholder="wifi.ssid atau InternetGatewayDevice...SSID"
                      className={`${inputCls} font-mono text-xs`}
                    />
                    <input
                      aria-label={`Nilai aksi ${i + 1}`}
                      maxLength={1000}
                      value={a.value ?? ''}
                      onChange={(e) => patchAction(i, { value: e.target.value })}
                      placeholder="concat('ACS-', {@serial_number})  atau  'teks tetap'"
                      className={`${inputCls} font-mono text-xs`}
                    />
                  </div>
                )}
                {a.type === 'get_parameter_values' && (
                  <NamesInput label={`Nama parameter aksi ${i + 1}`} names={a.names ?? []} onChange={(names) => patchAction(i, { names })} />
                )}
                {a.type === 'add_tag' && (
                  <select
                    aria-label={`Tag aksi ${i + 1}`}
                    value={a.tag_id ?? ''}
                    onChange={(e) => patchAction(i, { tag_id: e.target.value ? Number(e.target.value) : undefined })}
                    className={`mt-2 max-w-xs ${inputCls}`}
                  >
                    <option value="">Pilih tag…</option>
                    {tags.map((t) => (
                      <option key={t.id} value={t.id}>
                        {t.name}
                      </option>
                    ))}
                  </select>
                )}
                {a.type === 'set_parameter' && (
                  <p className="mt-1 text-xs text-slate-500 dark:text-slate-400">Nilai adalah ekspresi: teks tetap ditulis dalam kutip. Parameter rahasia diatur lewat provisioning profile.</p>
                )}
              </div>
            ))}
          </div>
          {actions.length < MAX_ACTIONS && (
            <button
              type="button"
              onClick={() => {
                setActions((prev) => [...prev, { type: 'reboot' }])
                setResult(null)
              }}
              className="mt-2 flex items-center gap-1.5 text-sm font-medium text-indigo-700 hover:underline dark:text-indigo-300"
            >
              <Plus className="h-4 w-4" /> Tambah aksi
            </button>
          )}
        </div>

        <div className="grid gap-4 sm:grid-cols-2">
          <div>
            <label htmlFor="au-cool" className={labelCls}>Jeda antar-pemicuan (menit, min 5)</label>
            <input id="au-cool" type="number" min={5} max={43200} required value={cooldownMin} onChange={(e) => setCooldownMin(e.target.value)} className={inputCls} />
          </div>
          <div>
            <label htmlFor="au-max" className={labelCls}>Berhenti setelah pemicuan berturut-turut</label>
            <input id="au-max" type="number" min={1} max={20} required value={maxFires} onChange={(e) => setMaxFires(e.target.value)} className={inputCls} />
            <p className="mt-1 text-xs text-slate-500 dark:text-slate-400">Tanpa kondisi pernah salah/konvergen di antaranya; setelah itu aturan di-suspend untuk perangkat tsb.</p>
          </div>
        </div>

        <div className="rounded-xl border border-slate-200 p-3 dark:border-slate-800">
          <p className="text-sm font-medium text-slate-700 dark:text-slate-300">Coba pada perangkat (tidak mengeksekusi apa pun)</p>
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
            <button type="button" onClick={() => void runTest()} disabled={test.isPending} className={secondaryBtnCls}>
              <Play className="h-4 w-4" /> {test.isPending ? 'Menguji…' : 'Uji'}
            </button>
          </div>
          {result && (
            <div className="mt-3 text-sm" role="status">
              {!result.ok ? (
                <p className="text-red-600 dark:text-red-400">
                  <AlertCircle className="mr-1 inline h-4 w-4" />
                  {result.error ?? 'Definisi tidak valid'}
                </p>
              ) : (
                <>
                  <p className="text-slate-700 dark:text-slate-300">
                    Kondisi: <b>{result.matched ? 'terpenuhi' : 'tidak terpenuhi'}</b> ·{' '}
                    {result.would_fire ? (
                      <span className="font-medium text-emerald-700 dark:text-emerald-400">aksi AKAN dijalankan pada Inform berikutnya</span>
                    ) : (
                      <span className="text-slate-500 dark:text-slate-400">tidak ada aksi yang dijalankan</span>
                    )}
                  </p>
                  <ul className="mt-1.5 space-y-0.5 text-xs text-slate-600 dark:text-slate-400">
                    {result.steps.map((st, i) => (
                      <li key={i} className="break-all">
                        {st.change ? '▶' : '•'} {st.detail}
                        {st.type === 'set_parameter' && st.path ? (
                          <>
                            {' '}
                            — saat ini <code>{st.current ?? 'belum ada'}</code>, target <code>{st.value}</code>
                            {!st.change ? ' (sudah sesuai)' : ''}
                          </>
                        ) : null}
                        {st.skip ? <em className="text-amber-700 dark:text-amber-400"> — dilewati: {st.skip}</em> : null}
                        {!st.change && st.type === 'add_tag' ? ' (sudah terpasang)' : ''}
                      </li>
                    ))}
                  </ul>
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

// Teks mentah disimpan lokal: menormalkan (split/trim) di tiap ketukan membuat koma yang baru diketik langsung hilang.
function NamesInput({ label, names, onChange }: { label: string; names: string[]; onChange: (names: string[]) => void }) {
  const [text, setText] = useState(names.join(', '))
  return (
    <input
      aria-label={label}
      maxLength={6000}
      value={text}
      onChange={(e) => {
        setText(e.target.value)
        onChange(e.target.value.split(',').map((x) => x.trim()).filter(Boolean))
      }}
      placeholder="nama parameter, dipisah koma (maks 20)"
      className={`mt-2 ${inputCls} font-mono text-xs`}
    />
  )
}

function HistoryModal({ rule, canManage, onClose }: { rule: AutomationRule; canManage: boolean; onClose: () => void }) {
  const { data, isLoading } = useAutomationRuns(rule.id)
  const rows = data?.data ?? []
  const reset = useResetAutomationRule()
  const toast = useToast()
  const confirm = useConfirm()

  async function handleReset() {
    const ok = await confirm({
      title: 'Buka kembali aturan',
      message: 'Semua perangkat yang di-suspend oleh aturan ini akan dievaluasi lagi pada Inform berikutnya.',
      confirmLabel: 'Buka kembali',
    })
    if (!ok) return
    reset.mutate(
      { id: rule.id },
      {
        onSuccess: (r) => toast.success(r.reset > 0 ? `${r.reset} perangkat dibuka kembali.` : 'Tidak ada perangkat yang sedang di-suspend.'),
        onError: (e) => toast.error(e instanceof ApiError ? e.message : 'Gagal membuka kembali'),
      },
    )
  }

  return (
    <Modal title={`Riwayat — ${rule.name}`} onClose={onClose} wide>
      <div className="space-y-3">
        {canManage && (
          <button onClick={() => void handleReset()} disabled={reset.isPending} className={secondaryBtnCls}>
            <RotateCcw className="h-4 w-4" /> Buka kembali yang di-suspend
          </button>
        )}
        {isLoading ? (
          <PageSpinner />
        ) : rows.length === 0 ? (
          <p className="text-sm text-slate-500 dark:text-slate-400">Belum pernah memicu. Riwayat disimpan 30 hari.</p>
        ) : (
          <ul className="divide-y divide-slate-100 dark:divide-slate-800">
            {rows.map((r) => (
              <li key={r.id} className="flex items-start gap-3 py-2.5 text-sm">
                {r.fired ? (
                  <CheckCircle2 className="mt-0.5 h-4 w-4 shrink-0 text-emerald-600" aria-label="Berhasil" />
                ) : (
                  <XCircle className="mt-0.5 h-4 w-4 shrink-0 text-red-600" aria-label={r.suspended ? 'Di-suspend' : 'Gagal'} />
                )}
                <div className="min-w-0 flex-1">
                  <p className="text-slate-800 dark:text-slate-200">
                    <span className="font-mono text-xs">{r.device_serial}</span>
                    {r.suspended && <span className="ml-2 rounded-full bg-amber-50 px-2 py-0.5 text-xs text-amber-800 dark:bg-amber-950/40 dark:text-amber-300">di-suspend</span>}
                  </p>
                  {r.summary && <p className="break-all text-xs text-slate-600 dark:text-slate-400">{r.summary}</p>}
                  {r.error && <p className="break-words text-xs text-red-600 dark:text-red-400">{r.error}</p>}
                </div>
                <span className="shrink-0 text-xs text-slate-500 dark:text-slate-400">{formatRelativeTime(r.created_at)}</span>
              </li>
            ))}
          </ul>
        )}
      </div>
    </Modal>
  )
}
