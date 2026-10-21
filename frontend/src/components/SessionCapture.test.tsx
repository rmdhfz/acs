import { afterEach, beforeEach, describe, expect, it } from 'vitest'
import { screen, waitFor } from '@testing-library/react'
import userEvent from '@testing-library/user-event'
import { SessionCapture, prettyXml } from './SessionCapture'
import { setAuthToken } from '../lib/api'
import { mockFetch, renderApp } from '../test/utils'

const msg = (over: Record<string, unknown>) => ({
  id: 'm1', session_ref: 'ab12cd34', from_cpe: true, rpc_name: 'Inform', http_status: null, body_bytes: 1234, body_truncated: false,
  created_at: '2026-10-21T08:00:00Z', ...over,
})

function setup(opts: { active: boolean; rows: unknown[] }) {
  setAuthToken('JWT')
  localStorage.setItem('acs_user', JSON.stringify({ id: 1, uuid: 'u', username: 'u', roles: ['ADMIN'], tenant_id: 7 }))
  const until = new Date(Date.now() + 10 * 60_000).toISOString()
  return mockFetch((r) => {
    if (r.url.endsWith('/devices/5/capture') && r.method === 'GET') return { json: { active: opts.active, until: opts.active ? until : null } }
    if (r.url.endsWith('/devices/5/capture') && r.method === 'POST') return { json: { active: true, until } }
    if (r.url.endsWith('/devices/5/capture') && r.method === 'DELETE') return { status: 204 }
    if (r.url.includes('/devices/5/captures/m1')) return { json: msg({ body: '<Envelope><Body><Inform><Name>A</Name><Value>***REDACTED***</Value></Inform></Body></Envelope>' }) }
    if (r.url.includes('/devices/5/captures')) return { json: { data: opts.rows, total: opts.rows.length } }
    return undefined
  })
}

beforeEach(() => localStorage.clear())
afterEach(() => {
  setAuthToken(null)
  localStorage.clear()
})

describe('SessionCapture', () => {
  it('menyalakan capture mengirim minutes yang dipilih', async () => {
    const calls = setup({ active: false, rows: [] })
    renderApp(<SessionCapture deviceId={5} />)
    const u = userEvent.setup()
    expect(await screen.findByText('Tidak aktif')).toBeTruthy()
    expect(screen.getByText(/belum ada pesan terekam/i)).toBeTruthy()
    await u.selectOptions(screen.getByLabelText(/lama capture/i), '30')
    await u.click(screen.getByRole('button', { name: /nyalakan/i }))
    await waitFor(() => expect(calls.some((c) => c.method === 'POST' && c.url.endsWith('/devices/5/capture'))).toBe(true))
    expect(calls.find((c) => c.method === 'POST')?.body).toEqual({ minutes: 30 })
  })

  it('aktif: tampil sisa waktu, daftar pesan dua arah, dan isi pesan tersamarkan', async () => {
    setup({ active: true, rows: [msg({}), msg({ id: 'm2', from_cpe: false, rpc_name: 'InformResponse', http_status: 200, session_ref: null })] })
    renderApp(<SessionCapture deviceId={5} />)
    const u = userEvent.setup()
    expect(await screen.findByText(/aktif · sisa/i)).toBeTruthy()
    expect(screen.getByRole('button', { name: /matikan/i })).toBeTruthy()
    expect(await screen.findByText('CPE → ACS')).toBeTruthy()
    expect(screen.getByText('ACS → CPE')).toBeTruthy()
    expect(screen.getByText('InformResponse')).toBeTruthy()
    await u.click(screen.getAllByRole('button', { name: /^lihat$/i })[0])
    expect(await screen.findByText(/\*\*\*REDACTED\*\*\*/)).toBeTruthy()
  })

  it('mematikan capture memanggil DELETE', async () => {
    const calls = setup({ active: true, rows: [] })
    renderApp(<SessionCapture deviceId={5} />)
    const u = userEvent.setup()
    await u.click(await screen.findByRole('button', { name: /matikan/i }))
    await waitFor(() => expect(calls.some((c) => c.method === 'DELETE')).toBe(true))
  })
})

describe('prettyXml', () => {
  it('memberi indentasi, elemen daun tetap satu baris, teks tidak berubah', () => {
    expect(prettyXml('<a><b>x</b><c/><d></d><e><f>1</f></e></a>')).toBe('<a>\n  <b>x</b>\n  <c/>\n  <d></d>\n  <e>\n    <f>1</f>\n  </e>\n</a>')
  })
})
