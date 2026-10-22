import { useEffect } from 'react'
import { Navigate, Route, Routes, useLocation } from 'react-router-dom'
import { LandingPage } from './pages/LandingPage'
import { META_TITLE } from './landing/content'
import { Layout } from './components/Layout'
import { ProtectedRoute } from './components/ProtectedRoute'
import { LoginPage } from './pages/LoginPage'
import { ForgotPasswordPage } from './pages/ForgotPasswordPage'
import { ResetPasswordPage } from './pages/ResetPasswordPage'
import { SsoCallbackPage } from './pages/SsoCallbackPage'
import { DashboardPage } from './pages/DashboardPage'
import { DevicesPage } from './pages/DevicesPage'
import { DeviceDetailPage } from './pages/DeviceDetailPage'
import { TasksPage } from './pages/TasksPage'
import { FaultsPage } from './pages/FaultsPage'
import { ProvisioningPage } from './pages/ProvisioningPage'
import { FirmwarePage } from './pages/FirmwarePage'
import { CatalogPage } from './pages/CatalogPage'
import { AdministrationPage } from './pages/AdministrationPage'
import { ReportsPage } from './pages/ReportsPage'
import { BillingPage } from './pages/BillingPage'
import { BugReportsPage } from './pages/BugReportsPage'
import { DataTransfersPage } from './pages/DataTransfersPage'
import { NotFoundPage } from './pages/NotFoundPage'
import { SignupPage } from './pages/SignupPage'
import { VerifySignupPage } from './pages/VerifySignupPage'
import { PrivacyPage, TermsPage } from './pages/LegalPages'
import { TenantOnboardingWizard } from './pages/TenantOnboardingWizard'
import { WebhooksPage } from './pages/WebhooksPage'
import FilesPage from './pages/FilesPage'
import TagsPage from './pages/TagsPage'
import PresetsPage from './pages/PresetsPage'
import ProfilePage from './pages/ProfilePage'
import { NotificationsPage } from './pages/NotificationsPage'
import AuditPage from './pages/AuditPage'
import SelfServicePage from './pages/SelfServicePage'
import { useAuth } from './lib/auth'

// Role ENDUSER (portal pelanggan) tidak punya akses ke /dashboard dkk —
// arahkan ke portal self-service. Role lain ke dashboard seperti biasa.
function HomeRedirect() {
  const { hasRole } = useAuth()
  const onlyEndUser = hasRole('ENDUSER') && !hasRole('VIEWER', 'NOC', 'ADMIN', 'SUPERADMIN')
  return <Navigate to={onlyEndUser ? '/self-service' : '/dashboard'} replace />
}

// Beranda publik: pengunjung anonim melihat landing page (SEO); yang sudah login langsung ke aplikasi.
function PublicHome() {
  const { isAuthenticated, isLoading } = useAuth()
  if (isLoading || !isAuthenticated) return <LandingPage />
  return <HomeRedirect />
}

// Judul tab per halaman (aplikasi di-noindex, tetapi judul yang jelas membantu pengguna & riwayat peramban).
const PAGE_TITLES: Record<string, string> = {
  '/login': 'Masuk — ACS Console',
  '/signup': 'Daftar — ACS Console',
  '/forgot-password': 'Lupa password — ACS Console',
  '/reset-password': 'Buat password baru — ACS Console',
  '/terms': 'Persyaratan layanan — ACS Console',
  '/privacy': 'Kebijakan privasi — ACS Console',
  '/dashboard': 'Dashboard — ACS Console',
  '/devices': 'Perangkat — ACS Console',
  '/tasks': 'Task — ACS Console',
  '/faults': 'Fault — ACS Console',
  '/provisioning': 'Provisioning — ACS Console',
  '/presets': 'Preset — ACS Console',
  '/firmware': 'Firmware — ACS Console',
  '/files': 'File — ACS Console',
  '/tags': 'Tag — ACS Console',
  '/webhooks': 'Webhook — ACS Console',
  '/audit': 'Audit Trail — ACS Console',
  '/catalog': 'Katalog Vendor — ACS Console',
  '/reports': 'Laporan SaaS — ACS Console',
  '/billing': 'Langganan & Tagihan — ACS Console',
  '/bug-reports': 'Laporan Bug — ACS Console',
  '/data-transfers': 'Riwayat Impor/Ekspor — ACS Console',
  '/administration': 'Administrasi — ACS Console',
  '/profile': 'Profil — ACS Console',
  '/notifications': 'Notifikasi — ACS Console',
  '/self-service': 'WiFi Saya — ACS Console',
}

// Hanya beranda yang boleh diindeks mesin pencari; halaman aplikasi di-noindex (selain di robots.txt).
function useRouteSeo() {
  const { pathname } = useLocation()
  useEffect(() => {
    const robots = document.querySelector<HTMLMetaElement>('meta[name="robots"]')
    if (pathname === '/') {
      robots?.setAttribute('content', 'index,follow,max-image-preview:large,max-snippet:-1')
      document.title = META_TITLE
    } else {
      robots?.setAttribute('content', 'noindex,nofollow')
      document.title = PAGE_TITLES[pathname] ?? 'ACS Console'
    }
  }, [pathname])
}

function App() {
  useRouteSeo()
  return (
    <Routes>
      <Route path="/login" element={<LoginPage />} />
      <Route path="/forgot-password" element={<ForgotPasswordPage />} />
      <Route path="/reset-password" element={<ResetPasswordPage />} />
      <Route path="/sso" element={<SsoCallbackPage />} />
      <Route path="/signup" element={<SignupPage />} />
      <Route path="/verify-signup" element={<VerifySignupPage />} />
      <Route path="/terms" element={<TermsPage />} />
      <Route path="/privacy" element={<PrivacyPage />} />

      <Route
        element={
          <ProtectedRoute>
            <Layout />
          </ProtectedRoute>
        }
      >
        <Route path="/dashboard" element={<DashboardPage />} />
        <Route path="/devices" element={<DevicesPage />} />
        <Route path="/devices/:id" element={<DeviceDetailPage />} />
        <Route path="/tasks" element={<TasksPage />} />
        <Route path="/faults" element={<FaultsPage />} />
        <Route path="/provisioning" element={<ProvisioningPage />} />
        <Route path="/firmware" element={<FirmwarePage />} />
        <Route path="/files" element={<FilesPage />} />
        <Route path="/tags" element={<TagsPage />} />
        <Route path="/presets" element={<PresetsPage />} />
        <Route path="/audit" element={<AuditPage />} />
        <Route path="/profile" element={<ProfilePage />} />
        <Route path="/notifications" element={<NotificationsPage />} />
        <Route path="/self-service" element={<SelfServicePage />} />
        <Route path="/webhooks" element={<WebhooksPage />} />
        <Route path="/catalog" element={<CatalogPage />} />
        <Route path="/reports" element={<ReportsPage />} />
        <Route path="/billing" element={<BillingPage />} />
        <Route path="/bug-reports" element={<BugReportsPage />} />
        <Route path="/data-transfers" element={<DataTransfersPage />} />
        <Route path="/administration" element={<AdministrationPage />} />
        <Route path="/administration/onboarding" element={<TenantOnboardingWizard />} />
      </Route>
      <Route path="/" element={<PublicHome />} />

      <Route path="*" element={<NotFoundPage />} />
    </Routes>
  )
}

export default App
