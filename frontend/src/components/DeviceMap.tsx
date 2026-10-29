import { useEffect, useRef } from 'react'
import L from '../lib/leafletGlobal'
import 'leaflet.markercluster'
import 'leaflet/dist/leaflet.css'
import 'leaflet.markercluster/dist/MarkerCluster.css'
import { bboxParam, clusterColor, countStatuses, DEFAULT_CENTER, DEFAULT_ZOOM, statusStyle } from '../lib/deviceMap'
import { formatRelativeTime } from '../lib/format'
import { TILE_ATTRIBUTION, TILE_URL } from '../lib/mapConfig'
import type { DeviceMapPoint } from '../lib/types'

// Peta perangkat (Leaflet + OpenStreetMap, open source). Dimuat lazy oleh DevicesMapView supaya pustaka peta tidak membebani bundel awal.
//
// Sumber ubin: lib/mapConfig.ts (dapat diganti lewat VITE_MAP_TILE_URL / VITE_MAP_ATTRIBUTION; bawaan OSM publik, bukan untuk produksi berat).

interface DeviceMarkerOptions extends L.CircleMarkerOptions {
  status?: string
}

interface Props {
  points: DeviceMapPoint[]
  // Berubah (mis. filter diganti) = peta menyesuaikan diri ke titik yang ada. Penyegaran data biasa TIDAK mengubah pandangan pengguna.
  // undefined = data belum sesuai filter terbaru (masih data lama sementara data baru dimuat): jangan menyesuaikan dulu.
  fitKey?: string
  onOpen: (deviceId: number) => void
  // Dipanggil (debounced) setelah pengguna menggeser/zoom; bbox undefined = seluruh dunia.
  onViewportChange?: (bbox: string | undefined) => void
}

// popupContent membangun isi popup dengan DOM + textContent: serial/product class berasal dari CPE (tak tepercaya), jadi tidak boleh lewat innerHTML.
function popupContent(p: DeviceMapPoint, onOpen: (id: number) => void): HTMLElement {
  const style = statusStyle(p.status)
  const root = document.createElement('div')
  root.style.minWidth = '180px'

  const title = document.createElement('div')
  title.style.fontWeight = '600'
  title.textContent = p.serial_number
  root.appendChild(title)

  if (p.product_class) {
    const pc = document.createElement('div')
    pc.style.opacity = '0.75'
    pc.textContent = p.product_class
    root.appendChild(pc)
  }

  const status = document.createElement('div')
  status.style.display = 'flex'
  status.style.alignItems = 'center'
  status.style.gap = '6px'
  status.style.margin = '6px 0'
  const dot = document.createElement('span')
  dot.style.cssText = `display:inline-block;width:8px;height:8px;border-radius:9999px;background:${style.color}`
  const label = document.createElement('span')
  label.textContent = p.last_inform_at ? `${style.label} · Inform ${formatRelativeTime(p.last_inform_at)}` : `${style.label} · belum pernah Inform`
  status.append(dot, label)
  root.appendChild(status)

  const open = document.createElement('button')
  open.type = 'button'
  open.textContent = 'Buka detail perangkat'
  open.style.cssText = 'color:#4f46e5;font-weight:500;cursor:pointer;background:none;border:0;padding:0;text-decoration:underline'
  open.addEventListener('click', () => onOpen(p.id))
  root.appendChild(open)
  return root
}

function clusterIcon(cluster: L.MarkerCluster): L.DivIcon {
  const children = cluster.getAllChildMarkers() as L.Marker[]
  const counts = countStatuses(children.map((m) => ({ status: (m.options as DeviceMarkerOptions).status ?? '' })))
  const n = children.length
  const size = n < 10 ? 34 : n < 100 ? 40 : 48
  return L.divIcon({
    html: `<div style="width:${size}px;height:${size}px;line-height:${size - 6}px;border-radius:9999px;background:${clusterColor(counts)};color:#fff;font-weight:600;font-size:13px;text-align:center;border:3px solid rgba(255,255,255,.9);box-shadow:0 1px 4px rgba(0,0,0,.35)">${n}</div>`,
    className: '',
    iconSize: [size, size],
  })
}

export default function DeviceMap({ points, fitKey, onOpen, onViewportChange }: Props) {
  const containerRef = useRef<HTMLDivElement>(null)
  const mapRef = useRef<L.Map | null>(null)
  const clusterRef = useRef<L.MarkerClusterGroup | null>(null)
  const fittedKey = useRef<string | null>(null)
  // Callback terbaru tanpa membuat ulang peta/efek tiap render induk.
  const onOpenRef = useRef(onOpen)
  const onViewportRef = useRef(onViewportChange)
  useEffect(() => {
    onOpenRef.current = onOpen
    onViewportRef.current = onViewportChange
  })

  // Peta dibuat sekali.
  useEffect(() => {
    if (!containerRef.current || mapRef.current) return
    // Renderer SVG (bawaan), BUKAN Canvas: renderer Canvas masih menjadwalkan gambar ulang saat peta dilepas (kembali ke tampilan Tabel) dan
    // melempar "Cannot read properties of undefined (reading 'clearRect')" (terlihat di browser nyata). Klaster hanya memasang penanda yang
    // terlihat, jadi SVG cukup untuk 5000 titik.
    const map = L.map(containerRef.current, { worldCopyJump: true, minZoom: 2 }).setView(DEFAULT_CENTER, DEFAULT_ZOOM)
    L.tileLayer(TILE_URL, { attribution: TILE_ATTRIBUTION, maxZoom: 19 }).addTo(map)
    const cluster = L.markerClusterGroup({ chunkedLoading: true, maxClusterRadius: 50, showCoverageOnHover: false, iconCreateFunction: clusterIcon })
    map.addLayer(cluster)
    mapRef.current = map
    clusterRef.current = cluster

    let timer: number | undefined
    const report = () => {
      window.clearTimeout(timer)
      timer = window.setTimeout(() => {
        const b = map.getBounds()
        onViewportRef.current?.(bboxParam({ west: b.getWest(), south: b.getSouth(), east: b.getEast(), north: b.getNorth() }))
      }, 400)
    }
    map.on('moveend', report)
    return () => {
      window.clearTimeout(timer)
      map.off('moveend', report)
      map.remove()
      mapRef.current = null
      clusterRef.current = null
    }
  }, [])

  // Titik diganti seluruhnya tiap data berubah (maks 5000; klaster bekerja bertahap).
  useEffect(() => {
    const map = mapRef.current
    const cluster = clusterRef.current
    if (!map || !cluster) return
    cluster.clearLayers()
    const markers = points.map((p) => {
      const style = statusStyle(p.status)
      const m = L.circleMarker([p.latitude, p.longitude], {
        radius: 8, color: '#ffffff', weight: 2, fillColor: style.color, fillOpacity: 0.95, status: p.status,
      } as DeviceMarkerOptions)
      m.bindPopup(() => popupContent(p, (id) => onOpenRef.current(id)))
      m.bindTooltip(p.serial_number)
      return m
    })
    cluster.addLayers(markers)
    if (points.length > 0 && fitKey !== undefined && fittedKey.current !== fitKey) {
      fittedKey.current = fitKey
      map.fitBounds(cluster.getBounds().pad(0.2), { maxZoom: 15, animate: false })
    }
  }, [points, fitKey])

  return <div ref={containerRef} className="h-[60vh] min-h-[360px] w-full overflow-hidden rounded-xl border border-slate-200 dark:border-slate-800" data-testid="device-map" />
}
