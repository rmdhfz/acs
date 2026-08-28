import React, { useEffect, useState } from 'react'
import { MapContainer, TileLayer, Marker, Popup } from 'react-leaflet'
import 'leaflet/dist/leaflet.css'
import L from 'leaflet'
import { Link } from 'react-router-dom'
import type { Device } from '../lib/types'
import { StatusBadge } from './StatusBadge'
import { useRefs } from '../lib/hooks'

// Fix missing marker icons in react-leaflet
import markerIcon2x from 'leaflet/dist/images/marker-icon-2x.png'
import markerIcon from 'leaflet/dist/images/marker-icon.png'
import markerShadow from 'leaflet/dist/images/marker-shadow.png'

delete (L.Icon.Default.prototype as any)._getIconUrl
L.Icon.Default.mergeOptions({
  iconUrl: markerIcon,
  iconRetinaUrl: markerIcon2x,
  shadowUrl: markerShadow,
})

interface DeviceMapProps {
  devices: Device[]
}

export function DeviceMap({ devices }: DeviceMapProps) {
  const { data: statusRefs } = useRefs('ref_device_status')
  const geoDevices = devices.filter((d) => d.latitude != null && d.longitude != null)

  const center: [number, number] = geoDevices.length > 0 
    ? [geoDevices[0].latitude!, geoDevices[0].longitude!] 
    : [-6.200000, 106.816666] // Jakarta default

  if (geoDevices.length === 0) {
    return (
      <div className="h-[400px] w-full rounded-xl border border-slate-200 bg-slate-50 flex items-center justify-center text-slate-400">
        Belum ada perangkat dengan koordinat (latitude/longitude)
      </div>
    )
  }

  return (
    <div className="h-[400px] w-full rounded-xl overflow-hidden border border-slate-200 shadow-sm relative z-0">
      <MapContainer center={center} zoom={11} scrollWheelZoom={true} style={{ height: '100%', width: '100%' }}>
        <TileLayer
          attribution='&copy; <a href="https://www.openstreetmap.org/copyright">OpenStreetMap</a> contributors'
          url="https://{s}.tile.openstreetmap.org/{z}/{x}/{y}.png"
        />
        {geoDevices.map((d) => {
          const status = statusRefs?.find(s => s.id === d.device_status_id)
          return (
            <Marker key={d.id} position={[d.latitude!, d.longitude!]}>
              <Popup>
                <div className="text-sm">
                  <div className="font-semibold mb-1">{d.serial_number}</div>
                  <div className="mb-2"><StatusBadge code={status?.code} label={status?.name || '-'} /></div>
                  <div className="text-xs text-slate-500 mb-2">{d.ip_address}</div>
                  <Link to={`/devices/${d.id}`} className="text-blue-600 hover:underline text-xs">Lihat Detail &rarr;</Link>
                </div>
              </Popup>
            </Marker>
          )
        })}
      </MapContainer>
    </div>
  )
}
