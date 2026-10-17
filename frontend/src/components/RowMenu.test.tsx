import { describe, expect, it, vi } from 'vitest'
import { fireEvent, render, screen } from '@testing-library/react'
import userEvent from '@testing-library/user-event'
import { Trash2 } from 'lucide-react'
import { RowMenu } from './RowMenu'

function setup(onReset = vi.fn(), onDelete = vi.fn()) {
  render(
    <div>
      <RowMenu
        label="Aksi untuk budi"
        items={[
          { label: 'Reset password', onClick: onReset },
          { label: 'Lepas MFA', onClick: vi.fn(), hidden: true },
          { label: 'Hapus user', icon: Trash2, tone: 'danger', onClick: onDelete },
        ]}
      />
      <button>setelah</button>
    </div>,
  )
  return { onReset, onDelete }
}

describe('RowMenu', () => {
  it('tertutup bawaan; dibuka lewat tombol dengan atribut aksesibilitas yang benar', async () => {
    setup()
    const trigger = screen.getByRole('button', { name: 'Aksi untuk budi' })
    expect(trigger.getAttribute('aria-haspopup')).toBe('menu')
    expect(trigger.getAttribute('aria-expanded')).toBe('false')
    expect(screen.queryByRole('menu')).toBeNull()
    await userEvent.click(trigger)
    expect(screen.getByRole('menu')).toBeTruthy()
    expect(trigger.getAttribute('aria-expanded')).toBe('true')
  })

  it('item hidden tidak tampil; fokus awal ke item pertama; klik item memanggil handler dan menutup', async () => {
    const { onReset } = setup()
    await userEvent.click(screen.getByRole('button', { name: 'Aksi untuk budi' }))
    const items = screen.getAllByRole('menuitem')
    expect(items.map((i) => i.textContent)).toEqual(['Reset password', 'Hapus user'])
    expect(document.activeElement).toBe(items[0])
    await userEvent.click(items[0])
    expect(onReset).toHaveBeenCalledTimes(1)
    expect(screen.queryByRole('menu')).toBeNull()
  })

  it('navigasi keyboard: panah berputar, Esc menutup dan mengembalikan fokus ke pemicu', async () => {
    setup()
    const trigger = screen.getByRole('button', { name: 'Aksi untuk budi' })
    await userEvent.click(trigger)
    const items = screen.getAllByRole('menuitem')
    fireEvent.keyDown(items[0], { key: 'ArrowDown' })
    expect(document.activeElement).toBe(items[1])
    fireEvent.keyDown(items[1], { key: 'ArrowDown' })
    expect(document.activeElement).toBe(items[0]) // berputar
    fireEvent.keyDown(items[0], { key: 'ArrowUp' })
    expect(document.activeElement).toBe(items[1])
    fireEvent.keyDown(items[1], { key: 'Escape' })
    expect(screen.queryByRole('menu')).toBeNull()
    expect(document.activeElement).toBe(trigger)
  })

  it('klik di luar menutup; semua item hidden = tidak merender apa pun', async () => {
    setup()
    await userEvent.click(screen.getByRole('button', { name: 'Aksi untuk budi' }))
    await userEvent.click(screen.getByRole('button', { name: 'setelah' }))
    expect(screen.queryByRole('menu')).toBeNull()
  })

  it('tanpa item terlihat tidak ada tombol sama sekali', () => {
    const { container } = render(<RowMenu items={[{ label: 'x', onClick: () => {}, hidden: true }]} />)
    expect(container.innerHTML).toBe('')
  })
})
