import { createContext, useCallback, useContext, useEffect, useState, type ReactNode } from 'react'

// i18n ringan tanpa dependency. `t(key)` mencari di kamus bahasa aktif, jatuh
// ke bahasa 'id' bila key belum diterjemahkan, lalu ke key mentah. Cakupan awal:
// navigasi, top bar, halaman Login & Profile, istilah umum. Body tiap halaman
// diterjemahkan bertahap — string yang belum di-wrap t() tetap Bahasa Indonesia.

export type Lang = 'id' | 'en'
const STORAGE_KEY = 'acs_lang'

type Dict = Record<string, string>

const ID: Dict = {
  // nav
  'nav.myWifi': 'WiFi Saya',
  'nav.dashboard': 'Dashboard',
  'nav.devices': 'Perangkat',
  'nav.tasks': 'Task',
  'nav.provisioning': 'Provisioning',
  'nav.presets': 'Preset',
  'nav.firmware': 'Firmware',
  'nav.files': 'File',
  'nav.tags': 'Tag',
  'nav.webhooks': 'Webhook',
  'nav.catalog': 'Katalog Vendor',
  'nav.administration': 'Administrasi',
  'nav.auditTrail': 'Audit Trail',
  'nav.profile': 'Profil',
  // top bar / user
  'top.changePassword': 'Ganti password',
  'top.logout': 'Keluar',
  'top.search': 'Cari…',
  'top.theme.light': 'Terang',
  'top.theme.dark': 'Gelap',
  'top.theme.system': 'Sistem',
  'top.language': 'Bahasa',
  // common
  'common.cancel': 'Batal',
  'common.save': 'Simpan',
  'common.delete': 'Hapus',
  'common.close': 'Tutup',
  'common.confirm': 'Lanjutkan',
  'common.loading': 'Memuat…',
  'common.saving': 'Menyimpan…',
  'common.search': 'Cari',
  'common.actions': 'Aksi',
  'common.status': 'Status',
  'common.active': 'Aktif',
  'common.inactive': 'Nonaktif',
  'common.yes': 'Ya',
  'common.no': 'Tidak',
  'common.none': '—',
  'common.roles': 'Peran',
  'common.tenant': 'Tenant',
  'common.username': 'Username',
  'common.email': 'Email',
  'common.password': 'Password',
  // login
  'login.title': 'ACS Console',
  'login.subtitle': 'Masuk untuk memonitor perangkat CPE',
  'login.submit': 'Masuk',
  'login.checking': 'Memeriksa…',
  'login.or': 'Atau',
  'login.sso': 'Masuk dengan SSO',
  'login.ssoHint': 'SSO hanya aktif bila operator sudah mengkonfigurasi Identity Provider.',
  'login.forgot': 'Lupa password? Hubungi administrator tenant Anda untuk reset.',
  'login.error': 'Gagal login, coba lagi',
  'login.showPassword': 'Tampilkan password',
  'login.hidePassword': 'Sembunyikan password',
  // profile
  'profile.title': 'Profil Saya',
  'profile.subtitle': 'Informasi akun, keamanan, dan preferensi.',
  'profile.account': 'Akun',
  'profile.security': 'Keamanan',
  'profile.fullName': 'Nama lengkap',
  'profile.lastLogin': 'Login terakhir',
  'profile.userId': 'ID pengguna',
  'profile.changePasswordDesc': 'Ganti password login Anda. Minimal 8 karakter.',
  'profile.change': 'Ganti',
  'profile.2fa': 'Autentikasi Dua Faktor (2FA)',
  'profile.2faDesc': 'Lapisan keamanan tambahan dengan kode dari aplikasi authenticator.',
  'profile.2faSoon': 'Belum tersedia — membutuhkan dukungan backend (TOTP). Direncanakan pada rilis berikutnya.',
  'profile.language': 'Bahasa antarmuka',
  'profile.theme': 'Tema',
  // password strength
  'pw.veryWeak': 'Sangat lemah',
  'pw.weak': 'Lemah',
  'pw.fair': 'Sedang',
  'pw.strong': 'Kuat',
  'pw.veryStrong': 'Sangat kuat',
  // audit
  'audit.title': 'Audit Trail',
  'audit.subtitle': 'Jejak perubahan data & aksi admin di tenant Anda.',
  'audit.who': 'Pengguna',
  'audit.action': 'Aksi',
  'audit.entity': 'Objek',
  'audit.when': 'Waktu',
  'audit.ip': 'IP',
  'audit.empty': 'Belum ada aktivitas tercatat.',
  'audit.filterAction': 'Semua aksi',
  'audit.filterEntity': 'Semua objek',
}

const EN: Dict = {
  'nav.myWifi': 'My WiFi',
  'nav.dashboard': 'Dashboard',
  'nav.devices': 'Devices',
  'nav.tasks': 'Tasks',
  'nav.provisioning': 'Provisioning',
  'nav.presets': 'Presets',
  'nav.firmware': 'Firmware',
  'nav.files': 'Files',
  'nav.tags': 'Tags',
  'nav.webhooks': 'Webhooks',
  'nav.catalog': 'Vendor Catalog',
  'nav.administration': 'Administration',
  'nav.auditTrail': 'Audit Trail',
  'nav.profile': 'Profile',
  'top.changePassword': 'Change password',
  'top.logout': 'Sign out',
  'top.search': 'Search…',
  'top.theme.light': 'Light',
  'top.theme.dark': 'Dark',
  'top.theme.system': 'System',
  'top.language': 'Language',
  'common.cancel': 'Cancel',
  'common.save': 'Save',
  'common.delete': 'Delete',
  'common.close': 'Close',
  'common.confirm': 'Continue',
  'common.loading': 'Loading…',
  'common.saving': 'Saving…',
  'common.search': 'Search',
  'common.actions': 'Actions',
  'common.status': 'Status',
  'common.active': 'Active',
  'common.inactive': 'Inactive',
  'common.yes': 'Yes',
  'common.no': 'No',
  'common.none': '—',
  'common.roles': 'Roles',
  'common.tenant': 'Tenant',
  'common.username': 'Username',
  'common.email': 'Email',
  'common.password': 'Password',
  'login.title': 'ACS Console',
  'login.subtitle': 'Sign in to monitor your CPE fleet',
  'login.submit': 'Sign in',
  'login.checking': 'Checking…',
  'login.or': 'Or',
  'login.sso': 'Sign in with SSO',
  'login.ssoHint': 'SSO only works once an operator has configured an Identity Provider.',
  'login.forgot': 'Forgot your password? Ask your tenant administrator to reset it.',
  'login.error': 'Sign-in failed, please try again',
  'login.showPassword': 'Show password',
  'login.hidePassword': 'Hide password',
  'profile.title': 'My Profile',
  'profile.subtitle': 'Account details, security, and preferences.',
  'profile.account': 'Account',
  'profile.security': 'Security',
  'profile.fullName': 'Full name',
  'profile.lastLogin': 'Last login',
  'profile.userId': 'User ID',
  'profile.changePasswordDesc': 'Change your login password. Minimum 8 characters.',
  'profile.change': 'Change',
  'profile.2fa': 'Two-Factor Authentication (2FA)',
  'profile.2faDesc': 'An extra security layer using a code from an authenticator app.',
  'profile.2faSoon': 'Not available yet — needs backend support (TOTP). Planned for a future release.',
  'profile.language': 'Interface language',
  'profile.theme': 'Theme',
  'pw.veryWeak': 'Very weak',
  'pw.weak': 'Weak',
  'pw.fair': 'Fair',
  'pw.strong': 'Strong',
  'pw.veryStrong': 'Very strong',
  'audit.title': 'Audit Trail',
  'audit.subtitle': 'Trail of data changes and admin actions in your tenant.',
  'audit.who': 'User',
  'audit.action': 'Action',
  'audit.entity': 'Object',
  'audit.when': 'Time',
  'audit.ip': 'IP',
  'audit.empty': 'No activity recorded yet.',
  'audit.filterAction': 'All actions',
  'audit.filterEntity': 'All objects',
}

const DICTS: Record<Lang, Dict> = { id: ID, en: EN }

interface I18nValue {
  lang: Lang
  setLang: (l: Lang) => void
  t: (key: string, vars?: Record<string, string | number>) => string
}

const I18nContext = createContext<I18nValue | null>(null)

function readStored(): Lang {
  if (typeof window === 'undefined') return 'id'
  const raw = localStorage.getItem(STORAGE_KEY)
  return raw === 'en' || raw === 'id' ? raw : 'id'
}

export function I18nProvider({ children }: { children: ReactNode }) {
  const [lang, setLangState] = useState<Lang>(readStored)

  useEffect(() => {
    localStorage.setItem(STORAGE_KEY, lang)
    document.documentElement.lang = lang
  }, [lang])

  const setLang = useCallback((l: Lang) => setLangState(l), [])

  const t = useCallback(
    (key: string, vars?: Record<string, string | number>) => {
      let str = DICTS[lang][key] ?? DICTS.id[key] ?? key
      if (vars) for (const [k, v] of Object.entries(vars)) str = str.replace(new RegExp(`\\{${k}\\}`, 'g'), String(v))
      return str
    },
    [lang],
  )

  return <I18nContext.Provider value={{ lang, setLang, t }}>{children}</I18nContext.Provider>
}

// eslint-disable-next-line react-refresh/only-export-components
export function useI18n(): I18nValue {
  const ctx = useContext(I18nContext)
  if (!ctx) throw new Error('useI18n harus dipakai di dalam I18nProvider')
  return ctx
}
