import { createContext, useContext, useEffect, useRef, useState } from 'react'
import { API_BASE, getAuthToken } from './api'
import { useQueryClient } from '@tanstack/react-query'

interface WSEvent {
  type: string
  payload: any
}

interface WSContextValue {
  connected: boolean
}

const WSContext = createContext<WSContextValue | null>(null)

export function WebSocketProvider({ children }: { children: React.ReactNode }) {
  const [connected, setConnected] = useState(false)
  const queryClient = useQueryClient()
  const wsRef = useRef<WebSocket | null>(null)
  const reconnectTimeout = useRef<number | null>(null)
  const backoffRef = useRef(3000)
  const tokenAtConnect = useRef<string | null>(null)

  useEffect(() => {
    const connect = () => {
      const token = getAuthToken()
      if (!token) {
        // Belum login — coba lagi nanti supaya WS tetap tersambung setelah login.
        reconnectTimeout.current = window.setTimeout(connect, 5000)
        return
      }

      // URL WebSocket diturunkan dari alamat API yang sama dengan REST (http→ws,
      // https→wss). Peramban tidak bisa mengirim header Authorization pada
      // WebSocket, jadi token ditawarkan sebagai subprotocol ("acs.bearer", token);
      // dengan begitu token tidak muncul di URL/access log.
      const wsUrl = API_BASE.replace(/^http/i, 'ws').replace(/\/+$/, '') + '/ws'
      const ws = new WebSocket(wsUrl, ['acs.bearer', token])
      tokenAtConnect.current = token

      ws.onopen = () => {
        setConnected(true)
        backoffRef.current = 3000
        if (reconnectTimeout.current) {
          clearTimeout(reconnectTimeout.current)
          reconnectTimeout.current = null
        }
      }

      ws.onmessage = (event) => {
        try {
          const data: WSEvent = JSON.parse(event.data)
          if (data.type === 'TASK_CREATED' || data.type === 'TASK_STATUS_CHANGED') {
             // Invalidate query daftar task
             queryClient.invalidateQueries({ queryKey: ['tasks'] })
             if (data.payload?.device_id) {
               // Invalidate device specific tasks
               queryClient.invalidateQueries({ queryKey: ['tasks', data.payload.device_id] })
             }
          } else if (data.type === 'NOTIFICATION') {
             // Notifikasi persisten baru untuk pengguna ini: NotificationProvider menyegarkan daftar & menampilkan toast.
             window.dispatchEvent(new CustomEvent('acs:notification', { detail: data.payload }))
          } else if (data.type === 'DEVICE_ONLINE' || data.type === 'DEVICE_OFFLINE') {
             // Status perangkat berubah: segarkan daftar, peta, ringkasan, dan (bila terbuka) detail + diagnosisnya.
             queryClient.invalidateQueries({ queryKey: ['devices'] })
             queryClient.invalidateQueries({ queryKey: ['device-map'] })
             queryClient.invalidateQueries({ queryKey: ['device-stats'] })
             if (data.payload?.device_id) {
                queryClient.invalidateQueries({ queryKey: ['device', data.payload.device_id] })
                queryClient.invalidateQueries({ queryKey: ['device-diagnosis', data.payload.device_id] })
             }
          }
        } catch (e) {
          console.error("WS Parse error", e)
        }
      }

      ws.onclose = () => {
        setConnected(false)
        // Auto reconnect dengan exponential backoff (maks 30 dtk) supaya tidak
        // membanjiri server saat backend WS memang tidak tersedia.
        reconnectTimeout.current = window.setTimeout(connect, backoffRef.current)
        backoffRef.current = Math.min(backoffRef.current * 2, 30000)
      }

      ws.onerror = (e) => {
        console.error("WS Error", e)
        ws.close()
      }

      wsRef.current = ws
    }

    // Connect immediately (assuming user is logged in, or we wait for them to log in)
    // The provider should ideally be wrapped inside AuthProvider so token is available
    connect()

    // Token berubah (login ulang / logout / ganti password) → tutup koneksi lama;
    // onclose menyambung kembali dengan token terbaru (atau menunggu login).
    const tokenWatch = window.setInterval(() => {
      if (wsRef.current && tokenAtConnect.current !== getAuthToken()) wsRef.current.close()
    }, 10000)

    return () => {
      window.clearInterval(tokenWatch)
      if (reconnectTimeout.current) clearTimeout(reconnectTimeout.current)
      if (wsRef.current) wsRef.current.close()
    }
  }, [queryClient])

  return (
    <WSContext.Provider value={{ connected }}>
      {children}
    </WSContext.Provider>
  )
}

export function useWebSocket() {
  const ctx = useContext(WSContext)
  if (!ctx) throw new Error('useWebSocket must be used within WebSocketProvider')
  return ctx
}
