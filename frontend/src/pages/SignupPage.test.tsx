import { beforeEach, describe, expect, it } from 'vitest'
import { screen, waitFor } from '@testing-library/react'
import userEvent from '@testing-library/user-event'
import { SignupPage } from './SignupPage'
import { VerifySignupPage } from './VerifySignupPage'
import { mockFetch, renderApp } from '../test/utils'

describe('SignupPage', () => {
  it('ditutup bila backend melaporkan self-signup mati', async () => {
    mockFetch(({ url }) => (url.endsWith('/auth/config') ? { json: { self_signup_enabled: false } } : undefined))
    renderApp(<SignupPage />)
    expect(await screen.findByText(/pendaftaran belum dibuka/i)).toBeTruthy()
    expect(screen.queryByLabelText(/nama perusahaan/i)).toBeNull()
  })

  it('validasi klien, honeypot tidak terisi, kirim, lalu layar "cek email"', async () => {
    const calls = mockFetch(({ url, method }) => {
      if (url.endsWith('/auth/config')) return { json: { self_signup_enabled: true } }
      if (url.endsWith('/auth/signup') && method === 'POST') return { status: 202, json: { message: 'ok' } }
    })
    renderApp(<SignupPage />)
    const u = userEvent.setup()
    const submit = (await screen.findByRole('button', { name: /daftar & kirim verifikasi/i })) as HTMLButtonElement
    expect(submit.disabled).toBe(true) // form kosong

    await u.type(screen.getByLabelText(/nama perusahaan/i), 'PT Contoh Jaringan')
    await u.type(screen.getByLabelText(/nama lengkap/i), 'Budi Santoso')
    await u.type(screen.getByLabelText(/^email/i), 'budi@contoh.id')
    await u.type(document.getElementById('su-pass') as HTMLInputElement, 'Kuda-Laut-7788-xyz')
    await u.type(document.getElementById('su-confirm') as HTMLInputElement, 'Kuda-Laut-0000-xyz')
    expect(await screen.findByText(/konfirmasi password tidak cocok/i)).toBeTruthy()
    expect(submit.disabled).toBe(true)

    await u.clear(document.getElementById('su-confirm') as HTMLInputElement)
    await u.type(document.getElementById('su-confirm') as HTMLInputElement, 'Kuda-Laut-7788-xyz')
    expect(submit.disabled).toBe(true) // belum setuju ketentuan
    await u.click(screen.getByRole('checkbox'))
    await waitFor(() => expect(submit.disabled).toBe(false))
    await u.click(submit)

    expect(await screen.findByText(/cek email anda/i)).toBeTruthy()
    const body = calls.find((c) => c.url.endsWith('/auth/signup'))!.body as Record<string, unknown>
    expect(body).toMatchObject({ company_name: 'PT Contoh Jaringan', email: 'budi@contoh.id', accept_terms: true, website: '' })
  })

  it('password lemah ditolak di klien sebelum request', async () => {
    const calls = mockFetch(({ url }) => (url.endsWith('/auth/config') ? { json: { self_signup_enabled: true } } : undefined))
    renderApp(<SignupPage />)
    const u = userEvent.setup()
    await screen.findByRole('button', { name: /daftar & kirim verifikasi/i })
    await u.type(screen.getByLabelText(/nama perusahaan/i), 'PT Contoh Jaringan')
    await u.type(screen.getByLabelText(/nama lengkap/i), 'Budi')
    await u.type(screen.getByLabelText(/^email/i), 'budi@contoh.id')
    await u.type(document.getElementById('su-pass') as HTMLInputElement, 'aaaaaaaaaaaa')
    await u.type(document.getElementById('su-confirm') as HTMLInputElement, 'aaaaaaaaaaaa')
    await u.click(screen.getByRole('checkbox'))
    await u.click(screen.getByRole('button', { name: /daftar & kirim verifikasi/i }))
    expect((await screen.findByRole('alert')).textContent).toMatch(/terlalu lemah/i)
    expect(calls.some((c) => c.url.endsWith('/auth/signup'))).toBe(false)
  })
})

describe('VerifySignupPage', () => {
  beforeEach(() => {
    window.location.hash = ''
  })

  it('token di fragmen: dikirim SEKALI, dihapus dari URL, layar sukses', async () => {
    window.location.hash = '#token=abc123abc123abc123abc123'
    const calls = mockFetch(({ url, method }) => {
      if (url.endsWith('/auth/signup/verify') && method === 'POST') return { json: { tenant_code: 'pt-contoh-1a2b3c', tenant_name: 'PT Contoh', email: 'budi@contoh.id' } }
    })
    renderApp(<VerifySignupPage />)
    expect(await screen.findByText(/akun aktif/i)).toBeTruthy()
    expect(calls.filter((c) => c.url.endsWith('/auth/signup/verify'))).toHaveLength(1)
    expect((calls[0].body as { token: string }).token).toBe('abc123abc123abc123abc123')
    expect(window.location.hash).toBe('') // token tidak tertinggal di URL/riwayat
  })

  it('tanpa token: pesan jelas tanpa request', async () => {
    const calls = mockFetch(() => undefined)
    renderApp(<VerifySignupPage />)
    expect((await screen.findByRole('alert')).textContent).toMatch(/tidak lengkap/i)
    expect(calls).toHaveLength(0)
  })

  it('token ditolak server: pesan galat', async () => {
    window.location.hash = '#token=kedaluwarsakedaluwarsakedaluwarsa'
    mockFetch(({ url }) => (url.endsWith('/auth/signup/verify') ? { status: 400, json: { message: 'signup: tautan verifikasi tidak valid atau sudah kedaluwarsa' } } : undefined))
    renderApp(<VerifySignupPage />)
    expect((await screen.findByRole('alert')).textContent).toMatch(/tidak valid atau sudah kedaluwarsa/i)
  })
})
