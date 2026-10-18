import { afterEach, describe, expect, it } from 'vitest'
import { fireEvent, screen, waitFor } from '@testing-library/react'
import userEvent from '@testing-library/user-event'
import { BugReportModal } from './BugReportModal'
import { setAuthToken } from '../lib/api'
import { mockFetch, renderApp } from '../test/utils'

afterEach(() => setAuthToken(null))

function setup() {
  setAuthToken('JWT')
  let closed = 0
  const calls = mockFetch(({ url, method }) => {
    if (url.endsWith('/bug-reports') && method === 'POST') return { status: 201, json: { id: '11111111-2222-3333-4444-555555555555', has_screenshot: false } }
  })
  renderApp(<BugReportModal onClose={() => (closed += 1)} />)
  return { calls, closed: () => closed }
}

describe('BugReportModal', () => {
  it('"apa yang terjadi" dan "apa yang diharapkan" adalah dua isian terpisah dan keduanya wajib', async () => {
    const { calls } = setup()
    const u = userEvent.setup()
    expect(screen.getByLabelText('Apa yang terjadi?')).not.toBe(screen.getByLabelText('Apa yang Anda harapkan?'))
    await u.type(screen.getByLabelText('Judul singkat'), 'Tombol simpan tidak merespons')
    await u.type(screen.getByLabelText('Apa yang terjadi?'), 'Klik simpan tidak ada reaksi sama sekali')
    // harapan masih kosong -> ditolak di klien, tanpa request
    fireEvent.submit(screen.getByRole('button', { name: /kirim laporan/i }).closest('form')!)
    expect((await screen.findByRole('alert')).textContent).toMatch(/harapkan/i)
    expect(calls.some((c) => c.method === 'POST')).toBe(false)
  })

  it('mengirim multipart dengan dua kolom terpisah + konteks otomatis, lalu menutup dialog', async () => {
    const { calls, closed } = setup()
    const u = userEvent.setup()
    await u.type(screen.getByLabelText('Judul singkat'), 'Tombol simpan tidak merespons')
    await u.type(screen.getByLabelText('Apa yang terjadi?'), 'Klik simpan tidak ada reaksi sama sekali')
    await u.type(screen.getByLabelText('Apa yang Anda harapkan?'), 'Seharusnya data tersimpan')
    await u.selectOptions(screen.getByLabelText('Dampak'), 'MAJOR')
    await u.click(screen.getByRole('button', { name: /kirim laporan/i }))
    await waitFor(() => expect(calls.some((c) => c.method === 'POST')).toBe(true))
    const b = calls.find((c) => c.method === 'POST')!.body as Record<string, unknown>
    expect(b.description).toBe('Klik simpan tidak ada reaksi sama sekali')
    expect(b.expected).toBe('Seharusnya data tersimpan')
    expect(b.severity).toBe('MAJOR')
    expect(typeof b.page_path).toBe('string')
    expect(b.app_version).toBeTruthy()
    expect(b.screenshot).toBeUndefined() // tanpa gambar bila tidak dilampirkan
    await waitFor(() => expect(closed()).toBe(1))
  })

  it('berkas yang bukan gambar ditolak dengan pesan jelas dan tidak dilampirkan', async () => {
    setup()
    const input = screen.getByLabelText('Pilih berkas gambar tangkapan layar') as HTMLInputElement
    const file = new File(['<svg><script>1</script></svg>'], 'x.svg', { type: 'image/svg+xml' })
    fireEvent.change(input, { target: { files: [file] } })
    expect((await screen.findByRole('alert')).textContent).toMatch(/berkas gambar/i)
    expect(screen.queryByAltText('Pratinjau tangkapan layar')).toBeNull()
  })
})
