import { render, screen } from '@testing-library/react'
import userEvent from '@testing-library/user-event'
import { useState } from 'react'
import { describe, expect, it, vi } from 'vitest'
import { Modal } from './Modal'

function Harness({ onClose = () => {}, dismissable = true }: { onClose?: () => void; dismissable?: boolean }) {
  const [open, setOpen] = useState(false)
  return (
    <div>
      <button onClick={() => setOpen(true)}>Buka</button>
      {open && (
        <Modal
          title="Judul uji"
          dismissable={dismissable}
          onClose={() => {
            onClose()
            setOpen(false)
          }}
        >
          <input aria-label="Nama" />
          <button>Simpan</button>
        </Modal>
      )}
    </div>
  )
}

describe('Modal — aksesibilitas', () => {
  it('memberi nama dialog dari judul dan memindahkan fokus ke kontrol pertama di badan', async () => {
    const user = userEvent.setup()
    render(<Harness />)
    await user.click(screen.getByText('Buka'))
    expect(screen.getByRole('dialog', { name: 'Judul uji' })).toBeTruthy()
    expect(document.activeElement).toBe(screen.getByLabelText('Nama'))
  })

  it('Tab dan Shift+Tab berputar di dalam dialog (jebakan fokus)', async () => {
    const user = userEvent.setup()
    render(<Harness />)
    await user.click(screen.getByText('Buka'))
    // urutan fokus: Nama -> Simpan -> (putar) Tutup -> Nama
    await user.tab()
    expect(document.activeElement).toBe(screen.getByText('Simpan'))
    await user.tab()
    expect(document.activeElement).toBe(screen.getByRole('button', { name: 'Tutup' }))
    await user.tab()
    expect(document.activeElement).toBe(screen.getByLabelText('Nama'))
    await user.tab({ shift: true })
    expect(document.activeElement).toBe(screen.getByRole('button', { name: 'Tutup' }))
    // fokus tidak pernah keluar dari dialog
    expect(screen.getByRole('dialog').contains(document.activeElement)).toBe(true)
  })

  it('Esc menutup dan fokus kembali ke tombol pemicu', async () => {
    const onClose = vi.fn()
    const user = userEvent.setup()
    render(<Harness onClose={onClose} />)
    const opener = screen.getByText('Buka')
    await user.click(opener)
    await user.keyboard('{Escape}')
    expect(onClose).toHaveBeenCalledTimes(1)
    expect(screen.queryByRole('dialog')).toBeNull()
    expect(document.activeElement).toBe(opener)
  })

  it('tidak bisa ditutup dengan Esc saat dismissable=false (operasi berjalan)', async () => {
    const onClose = vi.fn()
    const user = userEvent.setup()
    render(<Harness onClose={onClose} dismissable={false} />)
    await user.click(screen.getByText('Buka'))
    await user.keyboard('{Escape}')
    expect(onClose).not.toHaveBeenCalled()
    expect(screen.getByRole('dialog')).toBeTruthy()
  })
})
