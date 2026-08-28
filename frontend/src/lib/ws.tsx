import { createContext, useContext, useEffect, useRef, useState } from 'react'
import { getAuthToken } from './api'
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

  useEffect(() => {
    const connect = () => {
      const token = getAuthToken()
      if (!token) return // Don't connect if not authenticated

      // Gunakan WSS jika HTTPS, WS jika HTTP
      const protocol = window.location.protocol === 'https:' ? 'wss:' : 'ws:'
      // Fallback localhost jika VITE_API_BASE_URL mengarah ke API (misal http://localhost:18080/api/v1)
      const host = import.meta.env.VITE_API_BASE_URL 
        ? new URL(import.meta.env.VITE_API_BASE_URL as string).host 
        : window.location.host
        
      const wsUrl = `${protocol}//${host}/api/v1/ws?token=${token}`
      const ws = new WebSocket(wsUrl)
      
      // Catatan: Karena Websocket API bawaan browser tidak mendukung custom headers, 
      // cookie atau token URL param digunakan. Karena kita pakai token auth di Bearer,
      // ini bisa menyulitkan koneksi WS murni. Solusi alternatif adalah kita bisa mengirim 
      // auth token pada message pertama, ATAU di backend menerima token via query params.
      // Modifikasi ws_handler di backend untuk mengekstrak token dari query param diperlukan
      // jika `getAuthToken()` digunakan. 

      ws.onopen = () => {
        setConnected(true)
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
          } else if (data.type === 'DEVICE_ONLINE') {
             // Invalidate daftar device
             queryClient.invalidateQueries({ queryKey: ['devices'] })
             if (data.payload?.device_id) {
                // Invalidate device detail
                queryClient.invalidateQueries({ queryKey: ['device', data.payload.device_id] })
             }
          }
        } catch (e) {
          console.error("WS Parse error", e)
        }
      }

      ws.onclose = () => {
        setConnected(false)
        // Auto reconnect
        reconnectTimeout.current = window.setTimeout(connect, 3000)
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

    return () => {
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
