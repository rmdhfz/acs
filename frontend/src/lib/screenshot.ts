// Tangkapan layar untuk laporan bug: ambil dari layar/tab (getDisplayMedia), tempel (Ctrl+V), atau pilih berkas — lalu
// perkecil & kodekan ke PNG/JPEG di browser. Server TETAP memvalidasi ulang isi berkas (jenis, dimensi, ukuran); ini hanya
// supaya unggahan kecil dan formatnya selalu yang diterima server.

export const SCREENSHOT_MAX_BYTES = 2 * 1024 * 1024 // = batas server
export const SCREENSHOT_MAX_SIDE = 1920

/** Ukuran hasil perkecilan: sisi terpanjang <= max, rasio dipertahankan, tidak pernah membesarkan. */
export function fitSize(w: number, h: number, max = SCREENSHOT_MAX_SIDE): { width: number; height: number } {
  if (w <= 0 || h <= 0) return { width: 0, height: 0 }
  const scale = Math.min(1, max / Math.max(w, h))
  return { width: Math.max(1, Math.round(w * scale)), height: Math.max(1, Math.round(h * scale)) }
}

export class ScreenshotError extends Error {}

const ACCEPT = new Set(['image/png', 'image/jpeg', 'image/webp', 'image/gif', 'image/bmp'])

function canvasToBlob(c: HTMLCanvasElement, type: string, quality?: number): Promise<Blob> {
  return new Promise((resolve, reject) => c.toBlob((b) => (b ? resolve(b) : reject(new ScreenshotError('Gagal mengubah gambar'))), type, quality))
}

/** Mengubah sumber gambar apa pun (Blob/File/bitmap) menjadi PNG (atau JPEG bila PNG > batas) yang aman diunggah. */
export async function prepareScreenshot(src: Blob | ImageBitmap | HTMLVideoElement): Promise<File> {
  let bitmap: ImageBitmap | HTMLVideoElement
  if (src instanceof Blob) {
    if (!ACCEPT.has(src.type)) throw new ScreenshotError('Pilih berkas gambar (PNG, JPEG, WEBP, GIF, atau BMP).')
    if (src.size > 20 * 1024 * 1024) throw new ScreenshotError('Gambar terlalu besar (maks. 20 MB sebelum diperkecil).')
    try {
      bitmap = await createImageBitmap(src)
    } catch {
      throw new ScreenshotError('Berkas gambar rusak atau tidak dapat dibaca.')
    }
  } else {
    bitmap = src
  }
  const w = 'videoWidth' in bitmap ? bitmap.videoWidth : bitmap.width
  const h = 'videoHeight' in bitmap ? bitmap.videoHeight : bitmap.height
  const { width, height } = fitSize(w, h)
  if (width < 16 || height < 16) throw new ScreenshotError('Gambar terlalu kecil.')
  const canvas = document.createElement('canvas')
  canvas.width = width
  canvas.height = height
  const ctx = canvas.getContext('2d')
  if (!ctx) throw new ScreenshotError('Browser tidak mendukung pengolahan gambar.')
  ctx.drawImage(bitmap, 0, 0, width, height)
  if ('close' in bitmap) bitmap.close()

  let blob = await canvasToBlob(canvas, 'image/png')
  let name = 'tangkapan-layar.png'
  for (const q of [0.9, 0.8, 0.65, 0.5]) {
    if (blob.size <= SCREENSHOT_MAX_BYTES) break
    blob = await canvasToBlob(canvas, 'image/jpeg', q) // PNG terlalu besar -> JPEG berkualitas bertahap
    name = 'tangkapan-layar.jpg'
  }
  if (blob.size > SCREENSHOT_MAX_BYTES) throw new ScreenshotError('Gambar masih terlalu besar setelah diperkecil. Pilih gambar lain.')
  return new File([blob], name, { type: blob.type })
}

/** Menangkap tab/jendela/layar yang dipilih pengguna (izin dari browser). `hide` menyembunyikan dialog sesaat agar tidak ikut tertangkap. */
export async function captureScreen(hide: (hidden: boolean) => void): Promise<File> {
  if (!navigator.mediaDevices?.getDisplayMedia) throw new ScreenshotError('Browser ini tidak mendukung tangkapan layar otomatis. Pakai tombol "Pilih gambar" atau tempel dengan Ctrl+V.')
  let stream: MediaStream
  try {
    stream = await navigator.mediaDevices.getDisplayMedia({ video: true, audio: false })
  } catch {
    throw new ScreenshotError('Izin tangkapan layar dibatalkan.')
  }
  try {
    const video = document.createElement('video')
    video.muted = true
    video.srcObject = stream
    await video.play()
    hide(true)
    await new Promise((r) => setTimeout(r, 450)) // beri waktu dialog tersembunyi & frame baru
    return await prepareScreenshot(video)
  } finally {
    hide(false)
    stream.getTracks().forEach((t) => t.stop())
  }
}
