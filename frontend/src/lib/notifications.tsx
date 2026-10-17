import { createContext, useCallback, useContext, useEffect, useMemo, type ReactNode } from 'react'
import { useQueryClient } from '@tanstack/react-query'
import { useNotificationList, useUnreadCount, useMarkNotifications, type ServerNotification } from './hooks'
import { useToast } from './toast'

// Notifikasi SERVER (persisten, per pengguna, in-app + email) menggantikan inferensi client-side dari polling statistik.
// Sumber kebenaran: GET /notifications. Pembaruan instan lewat WebSocket (event "NOTIFICATION" -> window event
// 'acs:notification', lihat lib/ws.tsx); polling pelan tetap berjalan sebagai cadangan bila WebSocket terputus.

export const NOTIFICATION_EVENT = 'acs:notification'

export interface NotificationEventDetail {
  id: string
  type: string
  severity: 'INFO' | 'WARNING' | 'CRITICAL'
  title: string
  link?: string | null
}

interface NotificationContextValue {
  items: ServerNotification[]
  unreadCount: number
  isLoading: boolean
  markRead: (ids: string[]) => void
  markAllRead: () => void
}

const NotificationContext = createContext<NotificationContextValue | null>(null)

export function NotificationProvider({ children }: { children: ReactNode }) {
  const qc = useQueryClient()
  const toast = useToast()
  const list = useNotificationList({ page: 1, pageSize: 15 })
  const unread = useUnreadCount()
  const mark = useMarkNotifications()

  // Notifikasi baru lewat WebSocket: segarkan daftar + hitungan dan tampilkan toast singkat (CRITICAL = merah).
  useEffect(() => {
    function onNotify(e: Event) {
      const d = (e as CustomEvent<NotificationEventDetail>).detail
      qc.invalidateQueries({ queryKey: ['notifications'] })
      if (!d?.title) return
      if (d.severity === 'CRITICAL') toast.error(d.title, 'Notifikasi penting')
      else if (d.severity === 'WARNING') toast.warning(d.title, 'Perlu perhatian')
      else toast.info(d.title, 'Notifikasi baru')
    }
    window.addEventListener(NOTIFICATION_EVENT, onNotify)
    return () => window.removeEventListener(NOTIFICATION_EVENT, onNotify)
  }, [qc, toast])

  // Jumlah belum dibaca di judul tab — terlihat walau tab sedang di latar.
  const count = unread.data ?? 0
  useEffect(() => {
    const base = document.title.replace(/^\(\d+\+?\)\s/, '')
    document.title = count > 0 ? `(${count > 99 ? '99+' : count}) ${base}` : base
    return () => {
      document.title = document.title.replace(/^\(\d+\+?\)\s/, '')
    }
  }, [count])

  const markRead = useCallback((ids: string[]) => mark.mutate({ ids }), [mark])
  const markAllRead = useCallback(() => mark.mutate({ all: true }), [mark])

  const value = useMemo<NotificationContextValue>(
    () => ({ items: list.data?.data ?? [], unreadCount: count, isLoading: list.isLoading, markRead, markAllRead }),
    [list.data, list.isLoading, count, markRead, markAllRead],
  )
  return <NotificationContext.Provider value={value}>{children}</NotificationContext.Provider>
}

export function useNotifications() {
  const ctx = useContext(NotificationContext)
  if (!ctx) throw new Error('useNotifications harus dipakai di dalam NotificationProvider')
  return ctx
}
