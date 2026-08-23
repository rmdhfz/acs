import { Fragment, useMemo, useState, type FormEvent } from 'react'
import { ChevronLeft, ChevronRight, ListChecks, Plus, X } from 'lucide-react'
import { StatusBadge } from '../components/StatusBadge'
import { EmptyState } from '../components/EmptyState'
import { Modal } from '../components/Modal'
import { useAuth } from '../lib/auth'
import { ApiError } from '../lib/api'
import { findRefById, useCancelTask, useCreateTask, useRefs, useTasks, type TaskFilters } from '../lib/hooks'
import { formatJSONField, formatDateTime, formatRelativeTime } from '../lib/format'

const PAGE_SIZE = 25
const inputCls =
  'w-full rounded-lg border border-slate-300 px-3 py-2 text-sm text-slate-900 outline-none transition-colors focus:border-slate-500 focus:ring-1 focus:ring-slate-500 dark:border-slate-700 dark:bg-slate-900 dark:text-slate-100'
const primaryBtnCls =
  'flex items-center justify-center gap-2 rounded-lg bg-slate-900 px-3 py-2 text-sm font-medium text-white transition-colors hover:bg-slate-800 disabled:cursor-not-allowed disabled:opacity-60 dark:bg-slate-100 dark:text-slate-900 dark:hover:bg-white'

export function TasksPage() {
  const { hasRole } = useAuth()
  const canManage = hasRole('ADMIN', 'NOC')

  const [statusFilter, setStatusFilter] = useState('')
  const [typeFilter, setTypeFilter] = useState('')
  const [deviceIdFilter, setDeviceIdFilter] = useState('')
  const [page, setPage] = useState(1)
  const [showCreate, setShowCreate] = useState(false)
  const [expanded, setExpanded] = useState<number | null>(null)

  const { data: statusRefs } = useRefs('ref_task_status')
  const { data: typeRefs } = useRefs('ref_task_types')

  const filters: TaskFilters = useMemo(
    () => ({
      device_id: deviceIdFilter ? Number(deviceIdFilter) : undefined,
      task_status_id: statusFilter ? Number(statusFilter) : undefined,
      task_type: typeFilter || undefined,
      page,
      page_size: PAGE_SIZE,
    }),
    [deviceIdFilter, statusFilter, typeFilter, page],
  )

  const { data, isLoading } = useTasks(filters)
  const tasks = data?.data ?? []
  const total = data?.total ?? 0
  const totalPages = Math.max(1, Math.ceil(total / PAGE_SIZE))

  const cancelMutation = useCancelTask()

  return (
    <div className="mx-auto max-w-7xl px-6 py-8">
      <div className="mb-6 flex flex-wrap items-center justify-between gap-3">
        <div>
          <h1 className="text-xl font-semibold text-slate-900 dark:text-slate-100">Tasks</h1>
          <p className="mt-0.5 text-sm text-slate-500 dark:text-slate-400">Antrean RPC CWMP (SetParameterValues, Reboot, dst) ke seluruh device</p>
        </div>
        {canManage && (
          <button onClick={() => setShowCreate(true)} className={primaryBtnCls}>
            <Plus className="h-4 w-4" /> Buat Task
          </button>
        )}
      </div>

      <div className="mb-4 flex flex-wrap items-center gap-3">
        <input
          type="number"
          placeholder="Filter Device ID..."
          value={deviceIdFilter}
          onChange={(e) => {
            setDeviceIdFilter(e.target.value)
            setPage(1)
          }}
          className={`max-w-[180px] ${inputCls}`}
        />
        <select
          value={statusFilter}
          onChange={(e) => {
            setStatusFilter(e.target.value)
            setPage(1)
          }}
          className={`max-w-[200px] ${inputCls}`}
        >
          <option value="">Semua status</option>
          {statusRefs?.map((s) => (
            <option key={s.id} value={s.id}>
              {s.name}
            </option>
          ))}
        </select>
        <select
          value={typeFilter}
          onChange={(e) => {
            setTypeFilter(e.target.value)
            setPage(1)
          }}
          className={`max-w-[220px] ${inputCls}`}
        >
          <option value="">Semua tipe task</option>
          {typeRefs?.map((t) => (
            <option key={t.id} value={t.code}>
              {t.name}
            </option>
          ))}
        </select>
      </div>

      <div className="overflow-hidden rounded-xl border border-slate-200 bg-white shadow-sm dark:border-slate-800 dark:bg-slate-900">
        {isLoading ? (
          <div className="divide-y divide-slate-100 dark:divide-slate-800">
            {Array.from({ length: 6 }).map((_, i) => (
              <div key={i} className="flex items-center gap-4 px-5 py-4">
                <div className="h-5 w-20 animate-pulse rounded-full bg-slate-100" />
                <div className="h-4 flex-1 animate-pulse rounded bg-slate-100" />
              </div>
            ))}
          </div>
        ) : tasks.length === 0 ? (
          <EmptyState icon={ListChecks} title="Belum ada task" description="Task RPC yang diantrekan ke device akan muncul di sini." />
        ) : (
          <>
            <table className="w-full text-left text-sm">
              <thead>
                <tr className="border-b border-slate-200 bg-slate-50 text-xs font-medium uppercase tracking-wide text-slate-500 dark:border-slate-800 dark:bg-slate-800/50 dark:text-slate-400">
                  <th className="px-5 py-3">Status</th>
                  <th className="px-5 py-3">Tipe</th>
                  <th className="px-5 py-3">Device ID</th>
                  <th className="px-5 py-3">Prioritas</th>
                  <th className="px-5 py-3">Retry</th>
                  <th className="px-5 py-3">Dibuat</th>
                  <th className="px-5 py-3"></th>
                </tr>
              </thead>
              <tbody className="divide-y divide-slate-100 dark:divide-slate-800">
                {tasks.map((t) => {
                  const status = findRefById(statusRefs, t.task_status_id)
                  const type = findRefById(typeRefs, t.task_type_id)
                  const isCancellable = status?.code === 'PENDING' || status?.code === 'QUEUED'
                  const isExpanded = expanded === t.id
                  return (
                    <Fragment key={t.id}>
                      <tr
                        onClick={() => setExpanded(isExpanded ? null : t.id)}
                        className="cursor-pointer transition-colors hover:bg-slate-50 dark:hover:bg-slate-800/50"
                      >
                        <td className="px-5 py-3.5">
                          <StatusBadge code={status?.code} label={status?.name ?? '-'} />
                        </td>
                        <td className="px-5 py-3.5 text-slate-700 dark:text-slate-300">{type?.name ?? type?.code ?? '-'}</td>
                        <td className="px-5 py-3.5 font-mono text-xs text-slate-500 dark:text-slate-400">#{t.device_id}</td>
                        <td className="px-5 py-3.5 text-slate-500 dark:text-slate-400">{t.priority}</td>
                        <td className="px-5 py-3.5 text-slate-500 dark:text-slate-400">
                          {t.retry_count}/{t.max_retries}
                        </td>
                        <td className="px-5 py-3.5 text-slate-500 dark:text-slate-400">{formatRelativeTime(t.created_at)}</td>
                        <td className="px-5 py-3.5 text-right">
                          {canManage && isCancellable && (
                            <button
                              onClick={(e) => {
                                e.stopPropagation()
                                if (confirm('Batalkan task ini?')) cancelMutation.mutate(t.id)
                              }}
                              className="rounded-md p-1.5 text-slate-400 dark:text-slate-500 transition-colors hover:bg-red-50 hover:text-red-600"
                              title="Batalkan"
                            >
                              <X className="h-4 w-4" />
                            </button>
                          )}
                        </td>
                      </tr>
                      {isExpanded && (
                        <tr className="bg-slate-50">
                          <td colSpan={7} className="px-5 py-4">
                            <dl className="grid grid-cols-2 gap-4 text-xs sm:grid-cols-4">
                              <div>
                                <dt className="text-slate-400 dark:text-slate-500">Task UUID</dt>
                                <dd className="font-mono text-slate-700 dark:text-slate-300">{t.task_uuid}</dd>
                              </div>
                              <div>
                                <dt className="text-slate-400 dark:text-slate-500">Terkirim</dt>
                                <dd className="text-slate-700 dark:text-slate-300">{formatDateTime(t.sent_at)}</dd>
                              </div>
                              <div>
                                <dt className="text-slate-400 dark:text-slate-500">Selesai</dt>
                                <dd className="text-slate-700 dark:text-slate-300">{formatDateTime(t.completed_at)}</dd>
                              </div>
                              <div>
                                <dt className="text-slate-400 dark:text-slate-500">Error</dt>
                                <dd className="text-red-600">{t.error_message ?? '-'}</dd>
                              </div>
                            </dl>
                            {formatJSONField(t.parameters) && (
                              <div className="mt-3">
                                <p className="text-xs text-slate-400 dark:text-slate-500">Parameters</p>
                                <pre className="mt-1 max-h-40 overflow-auto rounded-lg bg-slate-900 p-3 text-xs text-slate-200">
                                  {formatJSONField(t.parameters)}
                                </pre>
                              </div>
                            )}
                          </td>
                        </tr>
                      )}
                    </Fragment>
                  )
                })}
              </tbody>
            </table>

            <div className="flex items-center justify-between border-t border-slate-200 px-5 py-3 text-sm text-slate-500 dark:text-slate-400">
              <span>
                Menampilkan {tasks.length} dari {total} task
              </span>
              <div className="flex items-center gap-1">
                <button
                  onClick={() => setPage((p) => Math.max(1, p - 1))}
                  disabled={page <= 1}
                  className="flex h-8 w-8 items-center justify-center rounded-lg border border-slate-200 text-slate-500 dark:text-slate-400 transition-colors hover:bg-slate-50 dark:hover:bg-slate-800/50 disabled:cursor-not-allowed disabled:opacity-40"
                >
                  <ChevronLeft className="h-4 w-4" />
                </button>
                <span className="px-2 text-xs tabular-nums">
                  {page} / {totalPages}
                </span>
                <button
                  onClick={() => setPage((p) => Math.min(totalPages, p + 1))}
                  disabled={page >= totalPages}
                  className="flex h-8 w-8 items-center justify-center rounded-lg border border-slate-200 text-slate-500 dark:text-slate-400 transition-colors hover:bg-slate-50 dark:hover:bg-slate-800/50 disabled:cursor-not-allowed disabled:opacity-40"
                >
                  <ChevronRight className="h-4 w-4" />
                </button>
              </div>
            </div>
          </>
        )}
      </div>

      {showCreate && <CreateTaskModal typeRefs={typeRefs} onClose={() => setShowCreate(false)} />}
    </div>
  )
}

function CreateTaskModal({
  typeRefs,
  onClose,
}: {
  typeRefs: ReturnType<typeof useRefs>['data']
  onClose: () => void
}) {
  const [deviceId, setDeviceId] = useState('')
  const [taskType, setTaskType] = useState('')
  const [priority, setPriority] = useState('5')
  const [maxRetries, setMaxRetries] = useState('3')
  const [parametersJson, setParametersJson] = useState('{}')
  const [error, setError] = useState<string | null>(null)
  const createMutation = useCreateTask()

  async function handleSubmit(e: FormEvent) {
    e.preventDefault()
    setError(null)
    let parameters: Record<string, unknown> | undefined
    try {
      parameters = parametersJson.trim() ? JSON.parse(parametersJson) : undefined
    } catch {
      setError('Parameters harus JSON valid, mis. {"InternetGatewayDevice.X":"value"}')
      return
    }
    try {
      await createMutation.mutateAsync({
        device_id: Number(deviceId),
        task_type: taskType,
        priority: Number(priority),
        max_retries: Number(maxRetries),
        parameters,
      })
      onClose()
    } catch (err) {
      setError(err instanceof ApiError ? err.message : 'Gagal membuat task')
    }
  }

  return (
    <Modal title="Buat Task Baru" onClose={onClose}>
      <form onSubmit={handleSubmit} className="space-y-3">
        <div>
          <label className="mb-1 block text-xs font-medium text-slate-600 dark:text-slate-400">Device ID</label>
          <input type="number" required value={deviceId} onChange={(e) => setDeviceId(e.target.value)} className={inputCls} />
        </div>
        <div>
          <label className="mb-1 block text-xs font-medium text-slate-600 dark:text-slate-400">Tipe Task</label>
          <select required value={taskType} onChange={(e) => setTaskType(e.target.value)} className={inputCls}>
            <option value="">Pilih tipe...</option>
            {typeRefs?.map((t) => (
              <option key={t.id} value={t.code}>
                {t.name}
              </option>
            ))}
          </select>
        </div>
        <div className="grid grid-cols-2 gap-3">
          <div>
            <label className="mb-1 block text-xs font-medium text-slate-600 dark:text-slate-400">Prioritas (1=tertinggi)</label>
            <input type="number" min={1} max={9} value={priority} onChange={(e) => setPriority(e.target.value)} className={inputCls} />
          </div>
          <div>
            <label className="mb-1 block text-xs font-medium text-slate-600 dark:text-slate-400">Max Retries</label>
            <input type="number" min={0} value={maxRetries} onChange={(e) => setMaxRetries(e.target.value)} className={inputCls} />
          </div>
        </div>
        <div>
          <label className="mb-1 block text-xs font-medium text-slate-600 dark:text-slate-400">Parameters (JSON)</label>
          <textarea
            value={parametersJson}
            onChange={(e) => setParametersJson(e.target.value)}
            rows={4}
            className={`${inputCls} font-mono text-xs`}
            placeholder='{"logical.key": "value"}'
          />
        </div>
        {error && <p className="text-sm text-red-600">{error}</p>}
        <button type="submit" disabled={createMutation.isPending} className={`${primaryBtnCls} w-full`}>
          Buat Task
        </button>
      </form>
    </Modal>
  )
}
