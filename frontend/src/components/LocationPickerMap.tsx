import { useEffect, useRef } from 'react'
import L from '../lib/leafletGlobal'
import 'leaflet/dist/leaflet.css'
import { DEFAULT_CENTER, DEFAULT_ZOOM } from '../lib/deviceMap'
import { TILE_ATTRIBUTION, TILE_URL } from '../lib/mapConfig'
import type { Coordinates } from '../lib/coordinates'

// Peta pemilih lokasi: klik peta atau seret penanda untuk menetapkan koordinat. Dimuat lazy oleh LocationPickerModal.
interface Props {
  value: Coordinates | null
  onChange: (c: Coordinates) => void
}

// Penanda berupa divIcon (bukan ikon gambar bawaan Leaflet: jalur gambarnya rusak pada bundel ESM).
const pin = () =>
  L.divIcon({
    className: '',
    html: '<div style="width:22px;height:22px;border-radius:9999px;background:#4f46e5;border:3px solid #fff;box-shadow:0 1px 5px rgba(0,0,0,.5)"></div>',
    iconSize: [22, 22],
    iconAnchor: [11, 11],
  })

// Enam desimal (±0,1 m) dan bujur dibungkus ke -180..180 (peta boleh digeser melewati salinan dunia).
const toCoords = (ll: L.LatLng): Coordinates => {
  const w = ll.wrap()
  return { lat: Number(w.lat.toFixed(6)), lon: Number(w.lng.toFixed(6)) }
}

export default function LocationPickerMap({ value, onChange }: Props) {
  const containerRef = useRef<HTMLDivElement>(null)
  const mapRef = useRef<L.Map | null>(null)
  const markerRef = useRef<L.Marker | null>(null)
  const onChangeRef = useRef(onChange)
  useEffect(() => {
    onChangeRef.current = onChange
  })
  const initial = useRef(value)

  useEffect(() => {
    if (!containerRef.current || mapRef.current) return
    const v = initial.current
    const map = L.map(containerRef.current, { worldCopyJump: true, minZoom: 2 }).setView(v ? [v.lat, v.lon] : DEFAULT_CENTER, v ? 16 : DEFAULT_ZOOM)
    L.tileLayer(TILE_URL, { attribution: TILE_ATTRIBUTION, maxZoom: 19 }).addTo(map)
    mapRef.current = map

    const place = (ll: L.LatLngExpression) => {
      if (markerRef.current) {
        markerRef.current.setLatLng(ll)
        return
      }
      const m = L.marker(ll, { draggable: true, icon: pin(), keyboard: false }).addTo(map)
      m.on('dragend', () => onChangeRef.current(toCoords(m.getLatLng())))
      markerRef.current = m
    }
    if (v) place([v.lat, v.lon])
    map.on('click', (e: L.LeafletMouseEvent) => {
      place(e.latlng)
      onChangeRef.current(toCoords(e.latlng))
    })
    return () => {
      map.remove()
      mapRef.current = null
      markerRef.current = null
    }
  }, [])

  // Nilai diubah dari luar (mis. koordinat ditempel): pindahkan penanda dan peta, tanpa memicu onChange lagi.
  useEffect(() => {
    const map = mapRef.current
    if (!map || !value) return
    const ll = L.latLng(value.lat, value.lon)
    if (markerRef.current) markerRef.current.setLatLng(ll)
    else {
      const m = L.marker(ll, { draggable: true, icon: pin(), keyboard: false }).addTo(map)
      m.on('dragend', () => onChangeRef.current(toCoords(m.getLatLng())))
      markerRef.current = m
    }
    map.setView(ll, Math.max(map.getZoom(), 14))
  }, [value])

  return <div ref={containerRef} className="h-[50vh] min-h-[300px] w-full overflow-hidden rounded-lg border border-slate-200 dark:border-slate-800" data-testid="location-picker-map" />
}
