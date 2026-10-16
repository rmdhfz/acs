import { describe, expect, it, vi } from 'vitest'
import { screen, waitFor } from '@testing-library/react'
import userEvent from '@testing-library/user-event'
import { MfaCard } from './MfaCard'
import { mockFetch, renderApp } from '../test/utils'

// qrcode memakai canvas (tidak ada di jsdom); yang diuji di sini alur & gating UI, bukan gambar QR.
vi.mock('qrcode', () => ({ default: { toDataURL: async () => 'data:image/png;base64,AAAA' } }))

describe('MfaCard', () => {
  it('fitur tidak tersedia: tidak ada tombol aktifkan', async () => {
    mockFetch(({ url }) => (url.endsWith('/auth/mfa') ? { json: { available: false, enabled: false, recovery_codes_left: 0 } } : undefined))
    renderApp(<MfaCard />)
    expect(await screen.findByText(/belum diaktifkan di instalasi/i)).toBeTruthy()
    expect(screen.queryByRole('button', { name: 'Aktifkan' })).toBeNull()
  })

  it('enrol: setup -> kode salah ditolak -> kode benar menampilkan kode pemulihan; Selesai butuh konfirmasi simpan', async () => {
    const calls = mockFetch(({ url, method, body }) => {
      if (url.endsWith('/auth/mfa') && method === 'GET') return { json: { available: true, enabled: false, recovery_codes_left: 0 } }
      if (url.endsWith('/auth/mfa/setup')) return { json: { secret: 'JBSWY3DPEHPK3PXP', otpauth_uri: 'otpauth://totp/ACS:ani?secret=JBSWY3DPEHPK3PXP' } }
      if (url.endsWith('/auth/mfa/enable')) {
        return (body as { code: string }).code === '654321'
          ? { json: { recovery_codes: ['AAAAA-BBBBB', 'CCCCC-DDDDD'] } }
          : { status: 400, json: { message: 'auth: kode verifikasi salah' } }
      }
    })
    renderApp(<MfaCard />)
    const u = userEvent.setup()
    await u.click(await screen.findByRole('button', { name: 'Aktifkan' }))
    expect(await screen.findByText('JBSWY3DPEHPK3PXP')).toBeTruthy() // kunci manual tampil
    expect(await screen.findByAltText(/kode qr/i)).toBeTruthy()
    expect(calls.filter((c) => c.url.endsWith('/auth/mfa/setup'))).toHaveLength(1) // rahasia dibuat SEKALI

    const input = screen.getByLabelText('Kode 6 digit') as HTMLInputElement
    const submit = screen.getAllByRole('button', { name: 'Aktifkan' }).pop() as HTMLButtonElement
    expect(submit.disabled).toBe(true) // kode belum 6 digit
    await u.type(input, '12ab34') // huruf disaring
    expect(input.value).toBe('1234')
    await u.clear(input)
    await u.type(input, '111111')
    await u.click(submit)
    expect((await screen.findByRole('alert')).textContent).toMatch(/kode verifikasi salah/i)

    await u.clear(input)
    await u.type(input, '654321')
    await u.click(submit)
    expect(await screen.findByText('AAAAA-BBBBB')).toBeTruthy()
    const done = screen.getByRole('button', { name: 'Selesai' }) as HTMLButtonElement
    expect(done.disabled).toBe(true) // belum konfirmasi menyimpan kode
    await u.click(screen.getByRole('checkbox'))
    await waitFor(() => expect(done.disabled).toBe(false))
  })

  it('nonaktifkan: password & kode wajib; 401 diterjemahkan jadi "Password salah"', async () => {
    mockFetch(({ url, method }) => {
      if (url.endsWith('/auth/mfa') && method === 'GET') return { json: { available: true, enabled: true, recovery_codes_left: 1 } }
      if (url.endsWith('/auth/mfa/disable')) return { status: 401, json: { message: 'auth: email atau password salah' } }
    })
    renderApp(<MfaCard />)
    const u = userEvent.setup()
    expect(await screen.findByText(/hampir habis/i)).toBeTruthy() // sisa kode pemulihan <= 2
    await u.click(await screen.findByRole('button', { name: 'Nonaktifkan' }))
    const submit = screen.getAllByRole('button', { name: 'Nonaktifkan' }).pop() as HTMLButtonElement
    expect(submit.disabled).toBe(true)
    await u.type(screen.getByLabelText('Password'), 'salah-banget')
    await u.type(screen.getByLabelText(/kode authenticator/i), '123456')
    await u.click(submit)
    expect((await screen.findByRole('alert')).textContent).toBe('Password salah.')
  })
})
