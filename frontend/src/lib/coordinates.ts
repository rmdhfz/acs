// Parser koordinat untuk kolom "tempel koordinat" pada pemilih lokasi. Operator biasanya menyalin dari Google Maps/ponsel: "-6.2088, 106.8456",
// kadang dengan tanda derajat, minus Unicode, pemisah titik koma, atau koma desimal gaya Indonesia ("-6,2088; 106,8456").

export interface Coordinates {
  lat: number
  lon: number
}

const NUM = /^-?\d+(\.\d+)?$/

// toNumber: teks desimal -> angka, atau null. Hanya tanda minus dan titik desimal; tanpa eksponen/heksa/spasi sisa.
function toNumber(raw: string): number | null {
  const t = raw.trim()
  if (!NUM.test(t)) return null
  const n = Number(t)
  return Number.isFinite(n) ? n : null
}

export function validCoordinates(lat: number, lon: number): boolean {
  return Number.isFinite(lat) && Number.isFinite(lon) && lat >= -90 && lat <= 90 && lon >= -180 && lon <= 180
}

// parseCoordinates: "lat, lon" | "lat lon" | "lat; lon" (koma desimal diizinkan bila dipisah titik koma/spasi). null bila tak dapat dibaca
// dengan pasti atau di luar rentang (lintang ±90, bujur ±180). Pemisah koma TUNGGAL tanpa spasi dipakai hanya bila kedua bagian memakai titik
// desimal: "−6,2088,106,8456" ambigu, jadi ditolak, bukan ditebak.
export function parseCoordinates(input: string): Coordinates | null {
  const text = input
    .replace(/[−–—]/g, '-') // minus Unicode / en-dash / em-dash
    .replace(/[°º]/g, '')
    .replace(/\s+/g, ' ')
    .trim()
  if (!text) return null

  let parts: string[] | null = null
  if (text.includes(';')) {
    parts = text.split(';')
  } else if (text.includes(',') && (text.match(/,/g) ?? []).length === 1) {
    // satu koma: pemisah ("-6.2088, 106.8456"), kecuali bila tampak sebagai koma desimal dipisah spasi ("-6,2088 106,8456" punya dua koma).
    parts = text.split(',')
  } else if (!text.includes(',')) {
    parts = text.split(' ')
  } else {
    // beberapa koma tanpa titik koma: hanya "-6,2088 106,8456" (dua token berspasi, masing-masing satu koma desimal) yang tak ambigu.
    const tokens = text.split(' ')
    if (tokens.length === 2 && tokens.every((t) => (t.match(/,/g) ?? []).length === 1)) {
      parts = tokens.map((t) => t.replace(',', '.'))
    }
  }
  if (!parts || parts.length !== 2) return null

  // Koma desimal ("-6,2088") hanya diterima bila pemisah antarbagian BUKAN koma.
  const usedCommaSeparator = !text.includes(';') && text.includes(',') && (text.match(/,/g) ?? []).length === 1
  const norm = (p: string) => (usedCommaSeparator ? p : p.replace(',', '.'))
  const lat = toNumber(norm(parts[0]))
  const lon = toNumber(norm(parts[1]))
  if (lat == null || lon == null || !validCoordinates(lat, lon)) return null
  return { lat, lon }
}

// formatCoordinates: lima desimal (±1 m), tanpa nol berlebih yang menyesatkan soal ketelitian.
export function formatCoordinates(lat: number, lon: number): string {
  return `${lat.toFixed(5)}, ${lon.toFixed(5)}`
}
