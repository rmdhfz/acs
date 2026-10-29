// Sumber ubin peta bersama (peta perangkat dan pemilih lokasi). Dapat diganti saat build lewat VITE_MAP_TILE_URL / VITE_MAP_ATTRIBUTION.
// Bawaan = server ubin publik OpenStreetMap yang kebijakannya melarang pemakaian berat: untuk produksi gunakan penyedia ubin sendiri atau self-host.
const env = import.meta.env as Record<string, string | undefined>
export const TILE_URL = env.VITE_MAP_TILE_URL || 'https://tile.openstreetmap.org/{z}/{x}/{y}.png'
export const TILE_ATTRIBUTION = env.VITE_MAP_ATTRIBUTION || '&copy; <a href="https://www.openstreetmap.org/copyright">OpenStreetMap</a> contributors'
