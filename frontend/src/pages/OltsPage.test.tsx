import { afterEach, beforeEach, describe, expect, it } from 'vitest'
import { screen, waitFor, within } from '@testing-library/react'
import userEvent from '@testing-library/user-event'
import { OltsPage } from './OltsPage'
import { ConfirmProvider } from '../lib/confirm'
import { setAuthToken } from '../lib/api'
import { mockFetch, renderApp } from '../test/utils'

function login(roles: string[]) {
  setAuthToken('JWT')
  localStorage.setItem('acs_user', JSON.stringify({ id: 1, uuid: 'u', username: 'adm', roles, tenant_id: 7 }))
}

const OLT = {
  id: 'olt-1', tenant_id: 7, name: 'OLT Pusat', driver_code: 'sim', host: '10.255.0.4', snmp_port: 161, ssh_port: 22, ssh_username: 'operator',
  notes: null, is_active: true, poll_status: 'OK', last_poll_at: new Date().toISOString(), last_poll_error: null,
  reported_name: 'SIM-OLT-10.255.0.4', reported_version: 'sim-1.0', has_snmp_community: true, has_ssh_password: true,
}
const ONU = (over: Record<string, unknown> = {}) => ({
  id: 'onu-1', olt_id: 'olt-1', olt_name: 'OLT Pusat', tenant_id: 7, pon_port: 'sim/1/1', serial_number: 'SIMONU0001', onu_type: 'SIM-ONU',
  status: 'PENDING', first_seen_at: new Date(Date.now() - 3600_000).toISOString(), last_seen_at: new Date().toISOString(), resolved_at: null, ...over,
})

const ui = (
  <ConfirmProvider>
    <OltsPage />
  </ConfirmProvider>
)

// Handler dasar: satu OLT, driver sim, satu ONU PENDING. `extra` menimpa/menambah untuk tes tertentu.
function backend(extra?: (r: { url: string; method: string; body: unknown }) => { status?: number; json?: unknown } | undefined) {
  return (r: { url: string; method: string; body: unknown }) => {
    const e = extra?.(r)
    if (e) return e
    if (r.method !== 'GET') return { status: 204 }
    if (r.url.includes('/olt-drivers')) return { json: { data: ['sim'], total: 1 } }
    if (r.url.includes('/olt-onus')) return { json: { data: [ONU()], total: 1 } }
    if (r.url.includes('/olts')) return { json: { data: [OLT], total: 1 } }
    return { json: [] }
  }
}

beforeEach(() => localStorage.clear())
afterEach(() => {
  setAuthToken(null)
  localStorage.clear()
})

describe('OltsPage', () => {
  it('VIEWER: melihat OLT (status, alamat, penanda kredensial) dan antrean ONU, tanpa tombol aksi', async () => {
    login(['VIEWER'])
    mockFetch(backend())
    renderApp(ui)
    expect((await screen.findAllByText('OLT Pusat')).length).toBeGreaterThan(0) // tabel OLT, opsi filter, dan kolom antrean
    expect(screen.getByText('Terhubung')).toBeTruthy()
    expect(screen.getAllByText(/10\.255\.0\.4/).length).toBeGreaterThan(0) // sel alamat (nama yang dilaporkan OLT juga memuatnya)
    expect(screen.getByText(/kredensial tersimpan: community, password SSH/)).toBeTruthy()
    expect(screen.getByText('SIM-OLT-10.255.0.4')).toBeTruthy()
    expect(await screen.findByText('SIMONU0001')).toBeTruthy()
    expect(screen.getAllByText('Menunggu').some((el) => el.tagName !== 'OPTION')).toBe(true) // lencana status (opsi filter punya teks sama)
    for (const name of [/Tambah OLT/, /Poll /, /Uji koneksi/, /Ubah /, /Hapus /, /Abaikan /]) expect(screen.queryByRole('button', { name })).toBeNull()
  })

  it('antrean diminta dengan status=PENDING bawaan; memilih OLT memakai parameter `olt` (bukan olt_id)', async () => {
    login(['VIEWER'])
    const calls = mockFetch(backend())
    renderApp(ui)
    await screen.findByText('SIMONU0001')
    expect(calls.some((c) => c.url.includes('/olt-onus') && c.url.includes('status=PENDING'))).toBe(true)
    await userEvent.selectOptions(screen.getByLabelText('Filter OLT'), 'olt-1')
    await waitFor(() => expect(calls.some((c) => c.url.includes('/olt-onus') && c.url.includes('olt=olt-1'))).toBe(true))
    expect(calls.some((c) => c.url.includes('olt_id='))).toBe(false) // QueryGuard menolak *_id non-numerik
  })

  it('NOC: dapat poll dan mengabaikan ONU, tetapi tidak melihat tambah/uji/ubah/hapus', async () => {
    login(['NOC'])
    const calls = mockFetch(
      backend((r) => {
        if (r.method === 'POST' && r.url.endsWith('/olts/olt-1/poll')) return { json: { seen: 3, new: 3, reappeared: 0, resolved: 0, skipped: 0, queue_length: 3 } }
        if (r.method === 'PATCH' && r.url.endsWith('/olt-onus/onu-1')) return { json: ONU({ status: 'IGNORED' }) }
        return undefined
      }),
    )
    renderApp(ui)
    expect(screen.queryByRole('button', { name: /Tambah OLT/ })).toBeNull()
    await userEvent.click(await screen.findByRole('button', { name: 'Poll OLT Pusat' }))
    await waitFor(() => expect(calls.some((c) => c.method === 'POST' && c.url.endsWith('/olts/olt-1/poll'))).toBe(true))
    expect(await screen.findByText(/Polling OLT Pusat selesai: 3 baru/)).toBeTruthy()
    for (const name of [/Uji koneksi/, /Ubah /, /Hapus /]) expect(screen.queryByRole('button', { name })).toBeNull()

    await userEvent.click(await screen.findByRole('button', { name: 'Abaikan SIMONU0001' }))
    await waitFor(() => expect(calls.some((c) => c.method === 'PATCH' && c.url.endsWith('/olt-onus/onu-1') && (c.body as { ignored: boolean }).ignored === true)).toBe(true))
  })

  it('entri IGNORED menawarkan "Pulihkan", RESOLVED tanpa aksi', async () => {
    login(['ADMIN'])
    const calls = mockFetch(
      backend((r) => {
        if (r.method === 'GET' && r.url.includes('/olt-onus'))
          return { json: { data: [ONU({ id: 'onu-i', serial_number: 'IGN1', status: 'IGNORED' }), ONU({ id: 'onu-r', serial_number: 'RES1', status: 'RESOLVED' })], total: 2 } }
        if (r.method === 'PATCH') return { json: ONU({ status: 'PENDING' }) }
        return undefined
      }),
    )
    renderApp(ui)
    await userEvent.click(await screen.findByRole('button', { name: 'Pulihkan IGN1' }))
    await waitFor(() => expect(calls.some((c) => c.method === 'PATCH' && c.url.endsWith('/olt-onus/onu-i') && (c.body as { ignored: boolean }).ignored === false)).toBe(true))
    expect(screen.queryByRole('button', { name: /RES1/ })).toBeNull()
  })

  it('ADMIN menambah OLT: kredensial hanya dikirim bila diisi, lalu koneksi diuji otomatis', async () => {
    login(['ADMIN'])
    const calls = mockFetch(backend((r) => (r.method === 'POST' && r.url.endsWith('/olts') ? { status: 201, json: OLT } : undefined)))
    renderApp(ui)
    await userEvent.click(await screen.findByRole('button', { name: /Tambah OLT/ }))
    const dlg = await screen.findByRole('dialog')
    await userEvent.type(within(dlg).getByLabelText(/^Nama/), 'OLT Baru')
    await userEvent.type(within(dlg).getByLabelText(/^Host/), '10.1.2.3')
    await userEvent.type(within(dlg).getByLabelText('SNMP community'), 'rahasia')
    await userEvent.click(within(dlg).getByRole('button', { name: 'Tambah OLT' }))
    await waitFor(() => expect(calls.some((c) => c.method === 'POST' && c.url.endsWith('/olts'))).toBe(true))
    const body = calls.find((c) => c.method === 'POST' && c.url.endsWith('/olts'))!.body as Record<string, unknown>
    expect(body).toMatchObject({ name: 'OLT Baru', driver_code: 'sim', host: '10.1.2.3', snmp_port: 161, ssh_port: 22, snmp_community: 'rahasia', is_active: true })
    expect('ssh_password' in body).toBe(false) // tidak diisi -> tidak dikirim
    expect('tenant_id' in body).toBe(false) // ADMIN: tenant dari token
    // kotak "Uji koneksi setelah disimpan" aktif bawaan pada OLT baru
    await waitFor(() => expect(calls.some((c) => c.method === 'POST' && c.url.endsWith('/olts/olt-1/test'))).toBe(true))
  })

  it('"Uji koneksi setelah disimpan" dapat dimatikan: OLT tersimpan tanpa memanggil /test', async () => {
    login(['ADMIN'])
    const calls = mockFetch(backend((r) => (r.method === 'POST' && r.url.endsWith('/olts') ? { status: 201, json: OLT } : undefined)))
    renderApp(ui)
    await userEvent.click(await screen.findByRole('button', { name: /Tambah OLT/ }))
    const dlg = await screen.findByRole('dialog')
    await userEvent.type(within(dlg).getByLabelText(/^Nama/), 'OLT Baru')
    await userEvent.type(within(dlg).getByLabelText(/^Host/), '10.1.2.3')
    await userEvent.click(within(dlg).getByLabelText('Uji koneksi setelah disimpan'))
    await userEvent.click(within(dlg).getByRole('button', { name: 'Tambah OLT' }))
    await waitFor(() => expect(screen.queryByRole('dialog')).toBeNull()) // modal menutup setelah tersimpan
    expect(calls.some((c) => c.method === 'POST' && c.url.endsWith('/olts'))).toBe(true)
    expect(calls.some((c) => c.url.endsWith('/test'))).toBe(false)
  })

  it('uji koneksi setelah simpan gagal: OLT tetap tersimpan dan galatnya dijelaskan', async () => {
    login(['ADMIN'])
    const calls = mockFetch(
      backend((r) => {
        if (r.method === 'POST' && r.url.endsWith('/olts')) return { status: 201, json: OLT }
        if (r.method === 'POST' && r.url.endsWith('/olts/olt-1/test')) return { status: 502, json: { message: 'OLT tidak dapat dijangkau' } }
        return undefined
      }),
    )
    renderApp(ui)
    await userEvent.click(await screen.findByRole('button', { name: /Tambah OLT/ }))
    const dlg = await screen.findByRole('dialog')
    await userEvent.type(within(dlg).getByLabelText(/^Nama/), 'OLT Baru')
    await userEvent.type(within(dlg).getByLabelText(/^Host/), '10.1.2.3')
    await userEvent.click(within(dlg).getByRole('button', { name: 'Tambah OLT' }))
    expect(await screen.findByText(/OLT tersimpan, tetapi uji koneksi gagal: OLT tidak dapat dijangkau/)).toBeTruthy()
    expect(calls.some((c) => c.method === 'POST' && c.url.endsWith('/olts'))).toBe(true)
    await waitFor(() => expect(screen.queryByRole('dialog')).toBeNull())
  })

  it('validasi: field wajib kosong dan port di luar 1–65535 ditolak validasi bawaan browser, tanpa memanggil API', async () => {
    login(['ADMIN'])
    const calls = mockFetch(backend())
    renderApp(ui)
    await userEvent.click(await screen.findByRole('button', { name: /Tambah OLT/ }))
    const dlg = await screen.findByRole('dialog')
    const form = dlg.querySelector('form') as HTMLFormElement

    // nama dan host kosong -> form tidak valid (required)
    expect(form.checkValidity()).toBe(false)
    await userEvent.click(within(dlg).getByRole('button', { name: 'Tambah OLT' }))
    expect(calls.some((c) => c.method === 'POST')).toBe(false)

    // terisi -> valid; port 70000 -> tidak valid lagi (min/max)
    await userEvent.type(within(dlg).getByLabelText(/^Nama/), 'X')
    await userEvent.type(within(dlg).getByLabelText(/^Host/), '10.1.2.3')
    expect(form.checkValidity()).toBe(true)
    const port = within(dlg).getByLabelText('Port SNMP')
    await userEvent.clear(port)
    await userEvent.type(port, '70000')
    expect(form.checkValidity()).toBe(false)
    await userEvent.click(within(dlg).getByRole('button', { name: 'Tambah OLT' }))
    expect(calls.some((c) => c.method === 'POST')).toBe(false)
  })

  it('tanpa driver terdaftar: modal menjelaskan dan tombol simpan dinonaktifkan', async () => {
    login(['ADMIN'])
    mockFetch((r) => {
      if (r.url.includes('/olt-drivers')) return { json: { data: [], total: 0 } }
      if (r.url.includes('/olt-onus') || r.url.includes('/olts')) return { json: { data: [], total: 0 } }
      return { json: [] }
    })
    renderApp(ui)
    await userEvent.click(await screen.findByRole('button', { name: /Tambah OLT/ }))
    const dlg = await screen.findByRole('dialog')
    expect(within(dlg).getByText(/Belum ada driver OLT di server/)).toBeTruthy()
    expect((within(dlg).getByRole('button', { name: 'Tambah OLT' }) as HTMLButtonElement).disabled).toBe(true)
  })

  it('memilih driver menampilkan label dan petunjuknya (host khusus simulator)', async () => {
    login(['ADMIN'])
    mockFetch(backend())
    renderApp(ui)
    await userEvent.click(await screen.findByRole('button', { name: /Tambah OLT/ }))
    const dlg = await screen.findByRole('dialog')
    expect(within(dlg).getByRole('option', { name: 'Simulator (uji coba)' })).toBeTruthy()
    expect(within(dlg).getByText(/10\.255\.0\.4 antrean berputar/)).toBeTruthy()
  })

  it('Batal menutup modal tanpa menyimpan', async () => {
    login(['ADMIN'])
    const calls = mockFetch(backend())
    renderApp(ui)
    await userEvent.click(await screen.findByRole('button', { name: /Tambah OLT/ }))
    const dlg = await screen.findByRole('dialog')
    await userEvent.type(within(dlg).getByLabelText(/^Nama/), 'Tidak jadi')
    await userEvent.click(within(dlg).getByRole('button', { name: 'Batal' }))
    await waitFor(() => expect(screen.queryByRole('dialog')).toBeNull())
    expect(calls.some((c) => c.method !== 'GET')).toBe(false)
  })

  it('ADMIN mengubah OLT: hanya field yang berubah dikirim; "hapus password" mengirim string kosong', async () => {
    login(['ADMIN'])
    const calls = mockFetch(backend((r) => (r.method === 'PATCH' && r.url.endsWith('/olts/olt-1') ? { json: OLT } : undefined)))
    renderApp(ui)
    await userEvent.click(await screen.findByRole('button', { name: 'Ubah OLT Pusat' }))
    const dlg = await screen.findByRole('dialog')
    const name = within(dlg).getByLabelText(/^Nama/)
    await userEvent.clear(name)
    await userEvent.type(name, 'OLT Pusat 2')
    await userEvent.click(within(dlg).getByLabelText('Hapus password tersimpan'))
    await userEvent.click(within(dlg).getByRole('button', { name: 'Simpan perubahan' }))
    await waitFor(() => expect(calls.some((c) => c.method === 'PATCH')).toBe(true))
    expect(calls.find((c) => c.method === 'PATCH')!.body).toEqual({ name: 'OLT Pusat 2', ssh_password: '' })
  })

  it('ADMIN menghapus OLT: konfirmasi lalu DELETE /olts/:id', async () => {
    login(['ADMIN'])
    const calls = mockFetch(backend())
    renderApp(ui)
    await userEvent.click(await screen.findByRole('button', { name: 'Hapus OLT Pusat' }))
    await userEvent.click(await screen.findByRole('button', { name: 'Hapus OLT' }))
    await waitFor(() => expect(calls.some((c) => c.method === 'DELETE' && c.url.endsWith('/olts/olt-1'))).toBe(true))
  })

  it('ADMIN menguji koneksi: POST /olts/:id/test; galat 502 ditampilkan', async () => {
    login(['ADMIN'])
    const calls = mockFetch(backend((r) => (r.method === 'POST' && r.url.endsWith('/olts/olt-1/test') ? { status: 502, json: { message: 'OLT tidak dapat dijangkau' } } : undefined)))
    renderApp(ui)
    await userEvent.click(await screen.findByRole('button', { name: 'Uji koneksi OLT Pusat' }))
    await waitFor(() => expect(calls.some((c) => c.method === 'POST' && c.url.endsWith('/olts/olt-1/test'))).toBe(true))
    expect(await screen.findByText(/OLT tidak dapat dijangkau/)).toBeTruthy()
  })

  it('OLT nonaktif: tombol poll dinonaktifkan; status galat tampil dengan pesannya', async () => {
    login(['NOC'])
    mockFetch(
      backend((r) =>
        r.method === 'GET' && r.url.includes('/olts') && !r.url.includes('/olt-')
          ? { json: { data: [{ ...OLT, is_active: false, poll_status: 'UNREACHABLE', last_poll_error: 'OLT tidak dapat dijangkau' }], total: 1 } }
          : undefined,
      ),
    )
    renderApp(ui)
    const poll = await screen.findByRole('button', { name: 'Poll OLT Pusat' })
    expect((poll as HTMLButtonElement).disabled).toBe(true)
    expect(screen.getByText('Tak terjangkau')).toBeTruthy()
    expect(screen.getByText('OLT tidak dapat dijangkau')).toBeTruthy()
    expect(screen.getByText('nonaktif')).toBeTruthy()
  })

  it('keadaan kosong, dan peringatan bila server belum punya driver OLT', async () => {
    login(['ADMIN'])
    mockFetch((r) => {
      if (r.url.includes('/olt-drivers')) return { json: { data: [], total: 0 } }
      if (r.url.includes('/olt-onus') || r.url.includes('/olts')) return { json: { data: [], total: 0 } }
      return { json: [] }
    })
    renderApp(ui)
    expect(await screen.findByText('Belum ada OLT')).toBeTruthy()
    expect(await screen.findByText('Tidak ada ONU')).toBeTruthy()
    expect(await screen.findByText(/Belum ada driver OLT yang terdaftar/)).toBeTruthy()
  })

  it('SUPERADMIN memilih tenant dari daftar (bukan mengetik ID) saat menambah OLT', async () => {
    login(['SUPERADMIN'])
    const TENANT = { id: 9, tenant_uuid: 't-9', code: 'ISP', name: 'ISP Sejahtera', is_active: true }
    const calls = mockFetch(
      backend((r) => {
        if (r.method === 'GET' && r.url.includes('/tenants')) return { json: { data: [TENANT, { ...TENANT, id: 10, code: 'OFF', name: 'ISP Lama', is_active: false }], total: 2 } }
        if (r.method === 'POST' && r.url.endsWith('/olts')) return { status: 201, json: OLT }
        return undefined
      }),
    )
    renderApp(ui)
    await userEvent.click(await screen.findByRole('button', { name: /Tambah OLT/ }))
    const dlg = await screen.findByRole('dialog')
    await userEvent.type(within(dlg).getByLabelText(/^Nama/), 'OLT S')
    await userEvent.type(within(dlg).getByLabelText(/^Host/), '10.1.2.3')
    const tenant = await within(dlg).findByLabelText(/^Tenant/)
    await within(dlg).findByRole('option', { name: /ISP Sejahtera \(ISP\)/ })
    expect(within(dlg).getByRole('option', { name: /ISP Lama \(OFF\) — nonaktif/ })).toBeTruthy()

    // tenant belum dipilih -> form tidak valid (required), tidak ada POST
    expect((dlg.querySelector('form') as HTMLFormElement).checkValidity()).toBe(false)
    await userEvent.click(within(dlg).getByRole('button', { name: 'Tambah OLT' }))
    expect(calls.some((c) => c.method === 'POST')).toBe(false)

    await userEvent.selectOptions(tenant, '9')
    await userEvent.click(within(dlg).getByRole('button', { name: 'Tambah OLT' }))
    await waitFor(() => expect(calls.some((c) => c.method === 'POST' && c.url.endsWith('/olts'))).toBe(true))
    expect((calls.find((c) => c.method === 'POST' && c.url.endsWith('/olts'))!.body as { tenant_id: number }).tenant_id).toBe(9)
  })

  it('ADMIN tidak melihat pemilih tenant (OLT selalu milik tenant-nya)', async () => {
    login(['ADMIN'])
    mockFetch(backend())
    renderApp(ui)
    await userEvent.click(await screen.findByRole('button', { name: /Tambah OLT/ }))
    const dlg = await screen.findByRole('dialog')
    expect(within(dlg).queryByLabelText(/^Tenant/)).toBeNull()
  })
})
