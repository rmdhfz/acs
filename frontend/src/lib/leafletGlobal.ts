import L from 'leaflet'

// Plugin leaflet.markercluster (UMD) mencari `L` global; pada bundel ESM global itu tidak ada. Modul ini harus diimpor SEBELUM
// 'leaflet.markercluster' (urutan evaluasi modul dijaga oleh urutan import di pemakai).
;(window as unknown as { L: typeof L }).L = L

export default L
