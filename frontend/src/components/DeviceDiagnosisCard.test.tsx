import { afterEach, beforeEach, describe, expect, it, vi } from 'vitest'
import { screen, waitFor } from '@testing-library/react'
import userEvent from '@testing-library/user-event'
import { DeviceDiagnosisCard } from './DeviceDiagnosisCard'
import { ConfirmProvider } from '../lib/confirm'
import { setAuthToken } from '../lib/api'
import { mockFetch, renderApp } from '../test/utils'
import type { Device, DeviceDiagnosis } from '../lib/types'

const device = {
  id: 42, device_uuid: 'u', connection_request_url: 'http://203.0.113.9:58000/x', connection_request_credentials_set: true,
} as Device

const DIAG: DeviceDiagnosis = {
  device_id: 42, status: 'OFFLINE', verdict: 'OFFLINE',
  summary: 'Dugaan utama: Redaman optik terakhir kritis. RX -29.1 dBm pada 2026-10-29 08:00 UTC.',
  last_inform_at: '2026-10-29T08:00:00Z', silent_seconds: 3 * 3600 + 20 * 60, inform_interval_seconds: null,
  inform_interval_parameter: 'InternetGatewayDevice.ManagementServer.PeriodicInformInterval',
  olt: null,
  offline_threshold_seconds: 900,
  findings: [
    { code: 'OPTICAL_BAD', severity: 'critical', title: 'Redaman optik terakhir kritis', evidence: 'RX -29.1 dBm pada 2026-10-29 08:00 UTC.', hint: 'Bersihkan konektor.' },
    { code: 'INTERVAL_UNKNOWN', severity: 'info', title: 'Interval Inform perangkat belum diketahui', evidence: 'ACS belum mengetahui interval.', hint: 'Baca PeriodicInformInterval.' },
  ],
  actions: ['CONNECTION_REQUEST', 'READ_INFORM_INTERVAL', 'OPEN_OPTICAL'],
  limits: 'Diagnosis ini hanya memakai bukti dari sisi ACS.', generated_at: '2026-10-29T11:20:00Z',
}

function login(roles: string[]) {
  setAuthToken('JWT')
  localStorage.setItem('acs_user', JSON.stringify({ id: 1, uuid: 'u', username: 'u', roles, tenant_id: 7 }))
}

function renderCard(statusCode: string, canAct: boolean, onGoTo = vi.fn()) {
  renderApp(
    <ConfirmProvider>
      <DeviceDiagnosisCard device={device} statusCode={statusCode} canAct={canAct} onEdit={() => {}} onGoTo={onGoTo} />
    </ConfirmProvider>,
  )
  return onGoTo
}

beforeEach(() => localStorage.clear())
afterEach(() => {
  setAuthToken(null)
  localStorage.clear()
})

describe('DeviceDiagnosisCard', () => {
  it('perangkat offline: terbuka otomatis, menampilkan ringkasan, temuan berkeparahan, ambang, dan batas diagnosis', async () => {
    login(['NOC'])
    mockFetch((r) => (r.url.includes('/devices/42/diagnosis') ? { json: DIAG } : undefined))
    renderCard('OFFLINE', true)
    expect(await screen.findByText(DIAG.summary)).toBeTruthy()
    expect(screen.getByText('Offline')).toBeTruthy()
    expect(screen.getByText('Kritis')).toBeTruthy()
    expect(screen.getByText('Redaman optik terakhir kritis')).toBeTruthy()
    expect(screen.getByText('Saran: Bersihkan konektor.')).toBeTruthy()
    expect(screen.getByText(/Diam 3 jam 20 menit · ambang offline 15 menit · interval Inform belum diketahui/)).toBeTruthy()
    expect(screen.getByText(DIAG.limits)).toBeTruthy()
  })

  it('perangkat yang belum pernah Inform: menulis "Belum pernah Inform", bukan "Diam -"', async () => {
    login(['NOC'])
    mockFetch((r) =>
      r.url.includes('/diagnosis')
        ? { json: { ...DIAG, verdict: 'NEVER_CONNECTED', silent_seconds: null, last_inform_at: null, summary: 'Perangkat belum pernah terhubung ke ACS.' } }
        : undefined,
    )
    renderCard('UNREGISTERED', true)
    expect(await screen.findByText(/Belum pernah Inform · ambang offline 15 menit/)).toBeTruthy()
    expect(screen.queryByText(/Diam -/)).toBeNull()
  })

  it('perangkat online: tidak memuat apa pun sampai operator menekan "Periksa koneksi"', async () => {
    login(['NOC'])
    const calls = mockFetch((r) => (r.url.includes('/diagnosis') ? { json: { ...DIAG, verdict: 'REACHABLE', status: 'ONLINE', summary: 'Perangkat terjangkau.' } } : undefined))
    renderCard('ONLINE', true)
    await userEvent.setup().click(screen.getByRole('button', { name: /Periksa koneksi/ }))
    expect(await screen.findByText('Perangkat terjangkau.')).toBeTruthy()
    expect(screen.getByText('Terjangkau')).toBeTruthy()
    expect(calls.filter((c) => c.url.includes('/diagnosis')).length).toBe(1)
  })

  it('status belum dimuat (undefined): jangan memuat diagnosis', () => {
    login(['NOC'])
    const calls = mockFetch(() => undefined)
    renderCard(undefined as unknown as string, true)
    expect(calls.length).toBe(0)
    expect(screen.getByRole('button', { name: /Periksa koneksi/ })).toBeTruthy()
  })

  it('aksi menurut peran: NOC melihat semua, VIEWER hanya yang bersifat tampilan', async () => {
    login(['NOC'])
    mockFetch((r) => (r.url.includes('/diagnosis') ? { json: DIAG } : undefined))
    renderCard('OFFLINE', true)
    expect(await screen.findByRole('button', { name: /Hubungi sekarang/ })).toBeTruthy()
    expect(screen.getByRole('button', { name: /Baca interval Inform/ })).toBeTruthy()
    expect(screen.getByRole('button', { name: /Lihat redaman optik/ })).toBeTruthy()
  })

  it('VIEWER (canAct=false) tidak diberi tombol aksi tulis, hanya melihat redaman optik', async () => {
    login(['VIEWER'])
    mockFetch((r) => (r.url.includes('/diagnosis') ? { json: DIAG } : undefined))
    renderCard('OFFLINE', false)
    expect(await screen.findByText(DIAG.summary)).toBeTruthy()
    expect(screen.queryByRole('button', { name: /Hubungi sekarang/ })).toBeNull()
    expect(screen.queryByRole('button', { name: /Baca interval Inform/ })).toBeNull()
    expect(screen.getByRole('button', { name: /Lihat redaman optik/ })).toBeTruthy()
  })

  it('"Lihat redaman optik" membuka tab optik', async () => {
    login(['NOC'])
    mockFetch((r) => (r.url.includes('/diagnosis') ? { json: DIAG } : undefined))
    const onGoTo = renderCard('OFFLINE', true)
    await userEvent.setup().click(await screen.findByRole('button', { name: /Lihat redaman optik/ }))
    expect(onGoTo).toHaveBeenCalledWith('optical')
  })

  it('"Baca interval Inform" mengantre GetParameterValues dengan nama parameter dari backend (bukan tebakan root)', async () => {
    login(['NOC'])
    const calls = mockFetch((r) => {
      if (r.url.includes('/diagnosis')) return { json: { ...DIAG, inform_interval_parameter: 'Device.ManagementServer.PeriodicInformInterval' } }
      if (r.url.includes('/tasks/get-parameter-values')) return { status: 202, json: { message: 'ok' } }
      return undefined
    })
    renderCard('OFFLINE', true)
    await userEvent.setup().click(await screen.findByRole('button', { name: /Baca interval Inform/ }))
    await waitFor(() => expect(calls.some((c) => c.url.includes('/devices/42/tasks/get-parameter-values'))).toBe(true))
    const post = calls.find((c) => c.url.includes('/tasks/get-parameter-values'))!
    expect(post.method).toBe('POST')
    expect(post.body).toEqual({ names: ['Device.ManagementServer.PeriodicInformInterval'] })
  })

  it('tanpa nama parameter dari backend, tombol baca interval tidak muncul walau aksinya terdaftar', async () => {
    login(['NOC'])
    mockFetch((r) => (r.url.includes('/diagnosis') ? { json: { ...DIAG, inform_interval_parameter: '' } } : undefined))
    renderCard('OFFLINE', true)
    expect(await screen.findByText(DIAG.summary)).toBeTruthy()
    expect(screen.queryByRole('button', { name: /Baca interval Inform/ })).toBeNull()
  })

  it('"Hubungi sekarang" memicu Connection Request perangkat itu', async () => {
    login(['NOC'])
    const calls = mockFetch((r) => {
      if (r.url.includes('/diagnosis')) return { json: DIAG }
      if (r.url.includes('/connection-request')) return { json: { message: 'ok' } }
      return undefined
    })
    renderCard('OFFLINE', true)
    await userEvent.setup().click(await screen.findByRole('button', { name: /Hubungi sekarang/ }))
    await waitFor(() => expect(calls.some((c) => c.url.includes('/devices/42/connection-request') && c.method === 'POST')).toBe(true))
  })

  it('"Periksa ulang" mengambil diagnosis lagi', async () => {
    login(['NOC'])
    const calls = mockFetch((r) => (r.url.includes('/diagnosis') ? { json: DIAG } : undefined))
    renderCard('OFFLINE', true)
    await screen.findByText(DIAG.summary)
    await userEvent.setup().click(screen.getByRole('button', { name: /Periksa ulang/ }))
    await waitFor(() => expect(calls.filter((c) => c.url.includes('/diagnosis')).length).toBe(2))
  })
})
