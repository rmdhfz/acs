import type { DeviceParameter } from './types'

// Parser WiFi murni (tanpa React) dari parameter TR-069 yang sudah tersinkron: mengelompokkan per instance
// (SSID #1..#N) untuk TR-098 (InternetGatewayDevice.LANDevice.1.WLANConfiguration.N.*) dan TR-181
// (Device.WiFi.SSID.N.* + Device.WiFi.AccessPoint.N.* + Device.WiFi.Radio.M.*).
// Path dipakai apa adanya dari yang dilaporkan perangkat (bukan ditebak), jadi aman lintas vendor.

export type WifiModel = 'TR098' | 'TR181'

export interface WifiClient {
  mac: string
  ip: string | null
  name: string | null
  /** Kekuatan sinyal dBm bila dilaporkan. */
  rssi: string | null
}

export interface WifiRow {
  index: number
  ssid: string
  /** true/false bila Enable terbaca, null bila belum. */
  enabled: boolean | null
  status: string | null
  band: string | null
  security: string | null
  channel: string | null
  clients: string | null
  /** Nilai password apa adanya ('[redacted]' bagi non-ADMIN, '' bila perangkat tidak mengembalikannya, null bila belum terbaca). */
  passphrase: string | null
  /** Path TR-069 untuk menulis; dipakai persis seperti yang dilaporkan perangkat bila ada. */
  ssidPath: string
  passphrasePath: string
  /** Path Enable (aktif/nonaktif SSID); dibentuk dari instance bila perangkat belum melaporkannya. */
  enablePath: string
  clientList: WifiClient[]
  /** true = SSID disembunyikan; null bila belum terbaca / ONT tak melaporkannya. */
  hidden: boolean | null
  /** Path parameter hide yang DILAPORKAN ONT: SSIDAdvertisementEnabled (standar, terbalik) atau *SSIDHide / *HideSSID (vendor, langsung). null = tak dilaporkan. */
  hidePath: string | null
  /** true bila nilai parameter berarti kebalikan dari "disembunyikan" (SSIDAdvertisementEnabled). */
  hideInverted: boolean
  /** Pengaturan opsional — HANYA ada bila ONT melaporkannya (nama field dikenali lewat akhiran; tidak ditebak). */
  maxClients: string | null
  maxClientsPath: string | null
  isolation: boolean | null
  isolationPath: string | null
  priority: string | null
  priorityPath: string | null
  updatedAt: string | null
}

const TR098_PREFIX = 'InternetGatewayDevice.LANDevice.1.WLANConfiguration.'

export function detectModel(params: DeviceParameter[] | undefined): WifiModel {
  return (params ?? []).some((p) => p.parameter_name.startsWith('Device.')) ? 'TR181' : 'TR098'
}

/** Nama parameter yang dibaca satu task GetParameterValues untuk memuat SEMUA SSID (path parsial = seluruh cabang). */
export function wifiReadNames(model: WifiModel): string[] {
  return model === 'TR181' ? ['Device.WiFi.SSID.', 'Device.WiFi.AccessPoint.', 'Device.WiFi.Radio.'] : [TR098_PREFIX]
}

const bool = (v: string | undefined): boolean | null => {
  if (v === undefined) return null
  const s = v.trim().toLowerCase()
  if (s === '1' || s === 'true') return true
  if (s === '0' || s === 'false') return false
  return null
}

const invert = (v: boolean | null): boolean | null => (v === null ? null : !v)

function bandFromText(freq: string): string {
  const f = freq.replace(/\s/g, '')
  if (f.startsWith('2.4')) return '2,4 GHz'
  if (f.startsWith('5')) return '5 GHz'
  if (f.startsWith('6')) return '6 GHz'
  return f
}

const firstInt = (v: string | undefined): number => {
  const m = (v ?? '').match(/\d+/)
  return m ? Number(m[0]) : 0
}

interface BandHints {
  /** OperatingFrequencyBand (standar) */
  freq?: string
  /** field vendor *RFBand — hanya dipakai bila berisi teks GHz (nilai numerik 0/1 beda arti per vendor) */
  rfband?: string
  channel?: string
  /** ChannelsInUse: channel aktual saat Channel=0 (auto) */
  inUse?: string
  possible?: string
  /** Standard / OperatingStandards, mis. "b,g,n" (2,4 GHz) atau "a,n,ac" (5 GHz) */
  standard?: string
}

/** Band dari petunjuk yang dilaporkan ONT, berurutan dari yang paling pasti; null bila tak ada petunjuk. */
function inferBand(h: BandHints): string | null {
  if (h.freq) return bandFromText(h.freq)
  if (h.rfband && /ghz/i.test(h.rfband)) return bandFromText(h.rfband)
  for (const n of [Number(h.channel), firstInt(h.inUse), firstInt(h.possible)]) {
    if (Number.isFinite(n) && n > 0) return n <= 14 ? '2,4 GHz' : '5 GHz'
  }
  const std = (h.standard ?? '').toLowerCase().split(/[,\s/]+/).filter(Boolean)
  if (std.includes('b') || std.includes('g')) return '2,4 GHz'
  if (std.includes('a') || std.includes('ac') || std.includes('ax')) return '5 GHz'
  return null
}

/** Channel yang ditampilkan: nilai Channel; bila 0/kosong (auto) pakai ChannelsInUse sebagai "N (auto)". */
function channelLabel(channel: string | undefined, inUse: string | undefined): string | null {
  if (channel && channel !== '0') return channel
  const u = firstInt(inUse)
  if (u > 0) return `${u} (auto)`
  return channel || null
}

function security098(f: Map<string, string>): string | null {
  const beacon = f.get('BeaconType')
  if (!beacon) return null
  if (beacon === 'None') return 'Terbuka'
  const mode = f.get('IEEE11iAuthenticationMode') || f.get('WPAAuthenticationMode')
  return [beacon, mode].filter(Boolean).join(' · ')
}

/** Klien tersambung: AssociatedDevice.K.* (TR-098) atau AccessPoint.N.AssociatedDevice.K.* (TR-181); nama field dicocokkan lewat akhiran agar lintas vendor. */
function clientsOf(fields: Map<string, string>): WifiClient[] {
  const byK = new Map<number, Map<string, string>>()
  for (const [rest, value] of fields) {
    const m = rest.match(/^AssociatedDevice\.(\d+)\.(.+)$/)
    if (!m) continue
    const k = Number(m[1])
    if (!byK.has(k)) byK.set(k, new Map())
    byK.get(k)?.set(m[2], value)
  }
  const pick = (f: Map<string, string>, re: RegExp) => {
    for (const [k, v] of f) if (re.test(k) && v !== '') return v
    return null
  }
  const out: WifiClient[] = []
  for (const [, f] of [...byK].sort((a, b) => a[0] - b[0])) {
    const mac = pick(f, /(?:MACAddress|MAC)$/)
    if (!mac) continue
    out.push({ mac, ip: pick(f, /IPAddress$/), name: pick(f, /(?:DeviceName|HostName)$/), rssi: pick(f, /(?:RSSI|SignalStrength)$/) })
  }
  return out
}

/** Field yang bukan konfigurasi SSID (statistik, klien, WPS, kunci WEP) tidak dicari sebagai pengaturan. */
const NOT_SETTING = /^(?:AssociatedDevice|Stats|WPS|WEPKey)\./

const MAX_CLIENTS_RE = /(?:^|[._])(?:MaxUserNum|MaxAssociatedDevices|MaxStations|MaxClients)$/
const ISOLATION_RE = /(?:Isolat\w*|ClientIsolation\w*)$/i
const PRIORITY_RE = /(?:^|[._])Priority$/

interface Found {
  value: string
  path: string
}

/** Cari field pada satu atau beberapa kelompok (SSID/AccessPoint) yang akhiran namanya cocok; path dipakai apa adanya. */
function findSetting(groups: ({ fields: Map<string, string>; names: Map<string, string> } | undefined)[], re: RegExp): Found | null {
  for (const g of groups) {
    if (!g) continue
    for (const [k, v] of g.fields) {
      if (NOT_SETTING.test(k) || !re.test(k)) continue
      const path = g.names.get(k)
      if (path) return { value: v, path }
    }
  }
  return null
}

const ADVERTISE_RE = /(?:^|[._])SSIDAdvertisementEnabled$/
const HIDE_RE = /(?:SSIDHide|HideSSID)$/i

function hideInfo(groups: ({ fields: Map<string, string>; names: Map<string, string> } | undefined)[]) {
  const adv = findSetting(groups, ADVERTISE_RE)
  if (adv) return { hidden: invert(bool(adv.value)), hidePath: adv.path, hideInverted: true }
  const hide = findSetting(groups, HIDE_RE)
  if (hide) return { hidden: bool(hide.value), hidePath: hide.path, hideInverted: false }
  return { hidden: null, hidePath: null, hideInverted: true }
}

function extras(groups: ({ fields: Map<string, string>; names: Map<string, string> } | undefined)[]) {
  const max = findSetting(groups, MAX_CLIENTS_RE)
  const iso = findSetting(groups, ISOLATION_RE)
  const pri = findSetting(groups, PRIORITY_RE)
  return {
    ...hideInfo(groups),
    maxClients: max?.value ?? null,
    maxClientsPath: max?.path ?? null,
    isolation: iso ? bool(iso.value) : null,
    isolationPath: iso?.path ?? null,
    priority: pri?.value ?? null,
    priorityPath: pri?.path ?? null,
  }
}

function fieldsByIndex(params: DeviceParameter[], re: RegExp) {
  const out = new Map<number, { fields: Map<string, string>; names: Map<string, string>; updated: string }>()
  for (const p of params) {
    const m = p.parameter_name.match(re)
    if (!m) continue
    const idx = Number(m[1])
    const rest = m[2]
    let e = out.get(idx)
    if (!e) {
      e = { fields: new Map(), names: new Map(), updated: '' }
      out.set(idx, e)
    }
    e.fields.set(rest, p.parameter_value ?? '')
    e.names.set(rest, p.parameter_name)
    if (p.updated_at > e.updated) e.updated = p.updated_at
  }
  return out
}

export function parseWifi(params: DeviceParameter[] | undefined): { model: WifiModel; rows: WifiRow[] } {
  const list = params ?? []
  const model = detectModel(list)
  const rows: WifiRow[] = []

  if (model === 'TR098') {
    const groups = fieldsByIndex(list, /^InternetGatewayDevice\.LANDevice\.1\.WLANConfiguration\.(\d+)\.(.+)$/)
    for (const [index, g] of groups) {
      if (!g.fields.has('SSID')) continue
      const passKey = ['PreSharedKey.1.KeyPassphrase', 'KeyPassphrase', 'PreSharedKey.1.PreSharedKey'].find((k) => g.fields.has(k))
      rows.push({
        index,
        ssid: g.fields.get('SSID') ?? '',
        enabled: bool(g.fields.get('Enable')),
        status: g.fields.get('Status') ?? null,
        band: inferBand({
          freq: g.fields.get('OperatingFrequencyBand'),
          rfband: findSetting([g], /RFBand$/)?.value,
          channel: g.fields.get('Channel'),
          inUse: g.fields.get('ChannelsInUse'),
          possible: g.fields.get('PossibleChannels'),
          standard: g.fields.get('Standard'),
        }),
        security: security098(g.fields),
        channel: channelLabel(g.fields.get('Channel'), g.fields.get('ChannelsInUse')),
        clients: g.fields.get('TotalAssociations') ?? null,
        passphrase: passKey ? (g.fields.get(passKey) ?? '') : null,
        ssidPath: g.names.get('SSID') ?? `${TR098_PREFIX}${index}.SSID`,
        passphrasePath: passKey ? (g.names.get(passKey) as string) : `${TR098_PREFIX}${index}.PreSharedKey.1.KeyPassphrase`,
        enablePath: g.names.get('Enable') ?? `${TR098_PREFIX}${index}.Enable`,
        clientList: clientsOf(g.fields),
        ...extras([g]),
        updatedAt: g.updated || null,
      })
    }
  } else {
    const ssids = fieldsByIndex(list, /^Device\.WiFi\.SSID\.(\d+)\.(.+)$/)
    const aps = fieldsByIndex(list, /^Device\.WiFi\.AccessPoint\.(\d+)\.(.+)$/)
    const radios = fieldsByIndex(list, /^Device\.WiFi\.Radio\.(\d+)\.(.+)$/)
    for (const [index, g] of ssids) {
      if (!g.fields.has('SSID')) continue
      const ap = aps.get(index)
      const lower = g.fields.get('LowerLayers') ?? ''
      const radioIdx = Number(lower.match(/Radio\.(\d+)/)?.[1])
      const radio = Number.isFinite(radioIdx) ? radios.get(radioIdx) : undefined
      rows.push({
        index,
        ssid: g.fields.get('SSID') ?? '',
        enabled: bool(g.fields.get('Enable')),
        status: g.fields.get('Status') ?? null,
        band: inferBand({
          freq: radio?.fields.get('OperatingFrequencyBand'),
          channel: radio?.fields.get('Channel'),
          inUse: radio?.fields.get('ChannelsInUse'),
          possible: radio?.fields.get('PossibleChannels'),
          standard: radio?.fields.get('OperatingStandards'),
        }),
        security: ap?.fields.get('Security.ModeEnabled') ?? null,
        channel: channelLabel(radio?.fields.get('Channel'), radio?.fields.get('ChannelsInUse')),
        clients: ap?.fields.get('AssociatedDeviceNumberOfEntries') ?? null,
        passphrase: ap?.fields.has('Security.KeyPassphrase') ? (ap.fields.get('Security.KeyPassphrase') ?? '') : null,
        ssidPath: g.names.get('SSID') ?? `Device.WiFi.SSID.${index}.SSID`,
        passphrasePath: ap?.names.get('Security.KeyPassphrase') ?? `Device.WiFi.AccessPoint.${index}.Security.KeyPassphrase`,
        enablePath: g.names.get('Enable') ?? `Device.WiFi.SSID.${index}.Enable`,
        clientList: ap ? clientsOf(ap.fields) : [],
        ...extras([ap, g]),
        updatedAt: g.updated || null,
      })
    }
  }
  rows.sort((a, b) => a.index - b.index)
  return { model, rows }
}
