import { StrictMode } from 'react'
import { QueryClient, QueryClientProvider } from '@tanstack/react-query'
import { render } from '@testing-library/react'
import { afterEach, beforeEach, describe, expect, it, vi } from 'vitest'
import { setAuthToken } from './api'
import { WebSocketProvider } from './ws'

// WebSocket tiruan: mencatat setiap koneksi yang dibuat dan apakah ia sempat dibuka sebelum ditutup.
class FakeWS {
  static instances: FakeWS[] = []
  onopen: (() => void) | null = null
  onclose: (() => void) | null = null
  onerror: ((e: unknown) => void) | null = null
  onmessage: ((e: { data: string }) => void) | null = null
  closed = false
  url: string
  protocols: string[]
  constructor(url: string, protocols: string[]) {
    this.url = url
    this.protocols = protocols
    FakeWS.instances.push(this)
  }
  close() {
    if (this.closed) return
    this.closed = true
    // Peramban: menutup sebelum terhubung memicu error lalu close.
    this.onerror?.({ type: 'error' })
    this.onclose?.()
  }
}

const mount = (strict: boolean) => {
  const qc = new QueryClient()
  const tree = (
    <QueryClientProvider client={qc}>
      <WebSocketProvider>
        <p>x</p>
      </WebSocketProvider>
    </QueryClientProvider>
  )
  return render(strict ? <StrictMode>{tree}</StrictMode> : tree)
}

beforeEach(() => {
  vi.useFakeTimers()
  FakeWS.instances = []
  vi.stubGlobal('WebSocket', FakeWS)
  setAuthToken('JWT')
})
afterEach(() => {
  vi.useRealTimers()
  vi.unstubAllGlobals()
  vi.restoreAllMocks()
  setAuthToken(null)
})

describe('WebSocketProvider', () => {
  it('menyambung dengan token sebagai subprotocol (bukan di URL)', () => {
    mount(false)
    expect(FakeWS.instances).toHaveLength(1)
    expect(FakeWS.instances[0].protocols).toEqual(['acs.bearer', 'JWT'])
    expect(FakeWS.instances[0].url).not.toContain('JWT')
  })

  it('setelah unmount tidak menyambung ulang dan tidak mencatat galat', () => {
    const err = vi.spyOn(console, 'error').mockImplementation(() => {})
    const { unmount } = mount(false)
    unmount()
    vi.advanceTimersByTime(120_000)
    expect(FakeWS.instances).toHaveLength(1)
    expect(err.mock.calls.some((c) => String(c[0]).includes('WS Error'))).toBe(false)
  })

  // Regresi: StrictMode memasang efek dua kali; koneksi pertama ditutup sebelum terhubung. Dulu onclose-nya menjadwalkan sambung ulang
  // sehingga koneksi ganda menumpuk, dan penutupan itu dicatat sebagai "WS Error".
  it('StrictMode: tidak ada koneksi ganda yang menumpuk dan tidak ada galat palsu', () => {
    const err = vi.spyOn(console, 'error').mockImplementation(() => {})
    mount(true)
    vi.advanceTimersByTime(120_000)
    const alive = FakeWS.instances.filter((w) => !w.closed)
    expect(alive).toHaveLength(1)
    expect(FakeWS.instances.length).toBeLessThanOrEqual(2)
    expect(err.mock.calls.some((c) => String(c[0]).includes('WS Error'))).toBe(false)
  })

  it('koneksi yang putus sungguhan (provider masih terpasang) tetap menyambung ulang', () => {
    mount(false)
    const first = FakeWS.instances[0]
    first.closed = true
    first.onclose?.()
    vi.advanceTimersByTime(3500)
    expect(FakeWS.instances).toHaveLength(2)
  })
})
