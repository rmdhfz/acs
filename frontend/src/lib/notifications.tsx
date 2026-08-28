import { createContext, useContext, useEffect, useRef, useState, type ReactNode } from 'react'
import { findRefIdByCode, useDeviceStats, useRefs, useTaskStats, useWebhookFailedCount } from './hooks'

// Notifikasi di sini berbasis POLLING (refetchInterval 5 detik yang sudah
// dipakai useTaskStats/useDeviceStats), BUKAN push/WebSocket sungguhan —
// belum ada infrastruktur realtime di backend (lihat ROADMAP.md). Ini
// deteksi perubahan client-side dari snapshot agregat yang sudah di-poll:
// cukup utk "terasa real-time" pada skala NOC kecil-menengah, tapi bukan
// substitusi event stream sungguhan bila kebutuhannya jadi lebih berat.

export interface AppNotification {
  id: string
  message: string
  tone: 'error' | 'warning'
  createdAt: number
}

const PENDING_BACKLOG_THRESHOLD = 20

interface NotificationContextValue {
  notifications: AppNotification[]
  unreadCount: number
  markAllRead: () => void
  dismiss: (id: string) => void
}

const NotificationContext = createContext<NotificationContextValue | null>(null)

export function NotificationProvider({ children }: { children: ReactNode }) {
  const [notifications, setNotifications] = useState<AppNotification[]>([])
  const [unreadCount, setUnreadCount] = useState(0)

  const { data: taskStatusRefs } = useRefs('ref_task_status')
  const { data: deviceStatusRefs } = useRefs('ref_device_status')
  const { data: taskStats } = useTaskStats()
  const { data: deviceStats } = useDeviceStats()
  const { data: webhookStats } = useWebhookFailedCount()

  const failedId = findRefIdByCode(taskStatusRefs, 'FAILED')
  const pendingId = findRefIdByCode(taskStatusRefs, 'PENDING')
  const offlineId = findRefIdByCode(deviceStatusRefs, 'OFFLINE')

  const prevFailed = useRef<number | null>(null)
  const prevOffline = useRef<number | null>(null)
  const prevWebhookFailed = useRef<number | null>(null)
  const wasOverBacklog = useRef(false)

  function push(message: string, tone: AppNotification['tone']) {
    const item: AppNotification = { id: `${Date.now()}-${Math.random().toString(36).slice(2, 8)}`, message, tone, createdAt: Date.now() }
    setNotifications((prev) => [item, ...prev].slice(0, 30))
    setUnreadCount((c) => c + 1)
  }

  useEffect(() => {
    if (failedId === undefined || !taskStats) return
    const failedCount = taskStats.find((t) => t.task_status_id === failedId)?.count ?? 0
    if (prevFailed.current !== null && failedCount > prevFailed.current) {
      const delta = failedCount - prevFailed.current
      push(`${delta} task baru gagal (total ${failedCount} FAILED)`, 'error')
    }
    prevFailed.current = failedCount
  }, [taskStats, failedId])

  useEffect(() => {
    if (pendingId === undefined || !taskStats) return
    const pendingCount = taskStats.find((t) => t.task_status_id === pendingId)?.count ?? 0
    const overNow = pendingCount > PENDING_BACKLOG_THRESHOLD
    if (overNow && !wasOverBacklog.current) {
      push(`Backlog task menumpuk: ${pendingCount} task PENDING (ambang ${PENDING_BACKLOG_THRESHOLD})`, 'warning')
    }
    wasOverBacklog.current = overNow
  }, [taskStats, pendingId])

  useEffect(() => {
    if (offlineId === undefined || !deviceStats) return
    const offlineCount = deviceStats.by_status?.find((s) => s.device_status_id === offlineId)?.count ?? 0
    if (prevOffline.current !== null && offlineCount > prevOffline.current) {
      const delta = offlineCount - prevOffline.current
      push(`${delta} device baru offline (total ${offlineCount} offline)`, 'warning')
    }
    prevOffline.current = offlineCount
  }, [deviceStats, offlineId])

  useEffect(() => {
    if (!webhookStats) return
    const failedCount = webhookStats.count ?? 0
    if (prevWebhookFailed.current !== null && failedCount > prevWebhookFailed.current) {
      const delta = failedCount - prevWebhookFailed.current
      push(`${delta} webhook delivery gagal (total ${failedCount} FAILED)`, 'error')
    }
    prevWebhookFailed.current = failedCount
  }, [webhookStats])

  function markAllRead() {
    setUnreadCount(0)
  }

  function dismiss(id: string) {
    setNotifications((prev) => prev.filter((n) => n.id !== id))
  }

  return (
    <NotificationContext.Provider value={{ notifications, unreadCount, markAllRead, dismiss }}>
      {children}
    </NotificationContext.Provider>
  )
}

export function useNotifications() {
  const ctx = useContext(NotificationContext)
  if (!ctx) throw new Error('useNotifications harus dipakai di dalam NotificationProvider')
  return ctx
}

