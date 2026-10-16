import { beforeEach, describe, expect, it } from 'vitest'
import { screen, waitFor } from '@testing-library/react'
import userEvent from '@testing-library/user-event'
import { LoginPage } from './LoginPage'
import { mockFetch, renderApp } from '../test/utils'

const user = { id: 1, uuid: 'u', username: 'ani', roles: ['NOC'], tenant_id: 1 }
const CODE_LABEL = /kode verifikasi|verification code/i

describe('LoginPage — login dua tahap (MFA)', () => {
  beforeEach(() => {
    localStorage.clear()
  })

  it('akun tanpa MFA: satu langkah, tidak ada form kode', async () => {
    const calls = mockFetch(({ url, method }) => {
      if (url.endsWith('/auth/config')) return { json: { sso_enabled: false, mfa_available: true } }
      if (url.endsWith('/auth/login') && method === 'POST') return { json: { access_token: 'JWT', token_type: 'Bearer', user } }
    })
    renderApp(<LoginPage />)
    const u = userEvent.setup()
    await u.type(document.getElementById('email') as HTMLInputElement, 'ani@isp.id')
    await u.type(document.getElementById('password') as HTMLInputElement, 'rahasia123')
    await u.click(screen.getByRole('button', { name: /masuk|sign in/i }))
    await waitFor(() => expect(calls.some((c) => c.url.endsWith('/auth/login'))).toBe(true))
    expect(screen.queryByLabelText(CODE_LABEL)).toBeNull()
    await waitFor(() => expect(localStorage.getItem('acs_token')).toBe('JWT'))
  })

  it('akun ber-MFA: muncul langkah kode, kode salah diberi pesan jelas, lalu sukses', async () => {
    let verifyAttempt = 0
    const calls = mockFetch(({ url, method, body }) => {
      if (url.endsWith('/auth/config')) return { json: { sso_enabled: false, mfa_available: true } }
      if (url.endsWith('/auth/login') && method === 'POST') return { json: { mfa_required: true, mfa_token: 'TAHAP2' } }
      if (url.endsWith('/auth/mfa/verify') && method === 'POST') {
        verifyAttempt++
        const b = body as { mfa_token: string; code: string }
        if (b.mfa_token !== 'TAHAP2') return { status: 401, json: { message: 'auth: token tidak valid' } }
        if (b.code !== '123456') return { status: 401, json: { message: 'auth: kode verifikasi salah' } }
        return { json: { access_token: 'SESI', token_type: 'Bearer', user } }
      }
    })
    renderApp(<LoginPage />)
    const u = userEvent.setup()
    await u.type(document.getElementById('email') as HTMLInputElement, 'ani@isp.id')
    await u.type(document.getElementById('password') as HTMLInputElement, 'rahasia123')
    await u.click(screen.getByRole('button', { name: /masuk|sign in/i }))

    // langkah 2 tampil; token tahap-2 BUKAN sesi (belum ada token tersimpan) dan field password hilang
    const code = await screen.findByLabelText(CODE_LABEL)
    expect(localStorage.getItem('acs_token')).toBeNull()
    expect(document.getElementById('password')).toBeNull()

    // kode salah -> pesan jelas, tetap di langkah 2, belum ada sesi
    await u.type(code, '000000')
    await u.click(screen.getByRole('button', { name: /verifikasi|verify/i }))
    expect(await screen.findByRole('alert')).toBeTruthy()
    expect(localStorage.getItem('acs_token')).toBeNull()

    // kode benar -> sesi tersimpan; setiap request membawa mfa_token yang sama
    await u.type(screen.getByLabelText(CODE_LABEL), '123456')
    await u.click(screen.getByRole('button', { name: /verifikasi|verify/i }))
    await waitFor(() => expect(localStorage.getItem('acs_token')).toBe('SESI'))
    expect(verifyAttempt).toBe(2)
    const verifyCalls = calls.filter((c) => c.url.endsWith('/auth/mfa/verify'))
    expect(verifyCalls.every((c) => (c.body as { mfa_token: string }).mfa_token === 'TAHAP2')).toBe(true)
  })

  it('token tahap-2 kedaluwarsa: kembali ke form login dengan pesan', async () => {
    mockFetch(({ url, method }) => {
      if (url.endsWith('/auth/config')) return { json: { sso_enabled: false, mfa_available: true } }
      if (url.endsWith('/auth/login') && method === 'POST') return { json: { mfa_required: true, mfa_token: 'X' } }
      if (url.endsWith('/auth/mfa/verify')) return { status: 401, json: { message: 'auth: token tidak valid' } }
    })
    renderApp(<LoginPage />)
    const u = userEvent.setup()
    await u.type(document.getElementById('email') as HTMLInputElement, 'ani@isp.id')
    await u.type(document.getElementById('password') as HTMLInputElement, 'rahasia123')
    await u.click(screen.getByRole('button', { name: /masuk|sign in/i }))
    await u.type(await screen.findByLabelText(CODE_LABEL), '123456')
    await u.click(screen.getByRole('button', { name: /verifikasi|verify/i }))
    await waitFor(() => expect(document.getElementById('email')).not.toBeNull()) // kembali ke langkah 1
    expect((await screen.findByRole('alert')).textContent).toMatch(/kedaluwarsa|expired/i)
  })
})
