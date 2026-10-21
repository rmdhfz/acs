import { RotateCw, Send } from 'lucide-react'
import { useAuth } from '../lib/auth'
import { useToast } from '../lib/toast'
import { ApiError } from '../lib/api'
import { findRefById, useRefs, useRetryTask } from '../lib/hooks'
import { NOC_MAY_RETRY, retryKind, summarizeTask } from '../lib/taskSummary'
import type { Task } from '../lib/types'

// Potongan UI bersama untuk daftar task: ringkasan "apa yang dikerjakan" dan tombol coba-ulang.

/** Judul (apa yang dilakukan) + baris rincian di bawahnya, dari jenis & parameter task. */
export function TaskSummaryView({ task }: { task: Task }) {
  const { data: typeRefs } = useRefs('ref_task_types')
  const code = findRefById(typeRefs, task.task_type_id)?.code
  const s = summarizeTask(code, task.parameters)
  return (
    <div className="min-w-0">
      <p className="font-medium text-slate-900 dark:text-slate-100">{s.title}</p>
      {s.details.length > 0 && (
        <ul className="mt-0.5 space-y-0.5 text-xs text-slate-500 dark:text-slate-400">
          {s.details.map((d, i) => (
            <li key={i} className="break-words">
              {d}
            </li>
          ))}
        </ul>
      )}
    </div>
  )
}

/**
 * "Coba lagi" (task gagal/dibatalkan/timeout) atau "Kirim sekarang" (menunggu jadwal retry) — memakai task yang SAMA,
 * bukan membuat task baru. Tak tampil bila tidak relevan atau role tidak berhak (NOC hanya untuk baca & reboot).
 */
export function RetryTaskButton({ task, compact }: { task: Task; compact?: boolean }) {
  const { hasRole } = useAuth()
  const toast = useToast()
  const { data: statusRefs } = useRefs('ref_task_status')
  const { data: typeRefs } = useRefs('ref_task_types')
  const retry = useRetryTask()

  const kind = retryKind(findRefById(statusRefs, task.task_status_id)?.code, task.retry_count, task.scheduled_at)
  const typeCode = findRefById(typeRefs, task.task_type_id)?.code
  const allowed = hasRole('ADMIN') || (hasRole('NOC') && !!typeCode && NOC_MAY_RETRY.has(typeCode))
  if (!kind || !allowed) return null

  const label = kind === 'retry' ? 'Coba lagi' : 'Kirim sekarang'
  const Icon = kind === 'retry' ? RotateCw : Send
  const onClick = (e: React.MouseEvent) => {
    e.stopPropagation()
    retry.mutate(task.id, {
      onSuccess: () => toast.success(kind === 'retry' ? 'Task diantre ulang. Dikirim saat perangkat Inform berikutnya.' : 'Task akan dikirim pada Inform berikutnya tanpa menunggu jeda retry.', 'Berhasil'),
      onError: (err) => toast.error(err instanceof ApiError ? err.message : 'Gagal mengulang task'),
    })
  }
  return (
    <button
      type="button"
      onClick={onClick}
      disabled={retry.isPending}
      aria-label={`${label} task ${task.id}`}
      className="inline-flex items-center gap-1 whitespace-nowrap rounded-full border border-slate-300 px-2.5 py-1 text-xs font-medium text-slate-700 transition-colors hover:bg-slate-50 disabled:opacity-50 dark:border-slate-700 dark:text-slate-300 dark:hover:bg-slate-800"
    >
      <Icon className="h-3.5 w-3.5" />
      {compact ? null : label}
    </button>
  )
}
