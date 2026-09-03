import { Navigate, Route, Routes } from 'react-router-dom'
import { Layout } from './components/Layout'
import { ProtectedRoute } from './components/ProtectedRoute'
import { LoginPage } from './pages/LoginPage'
import { DashboardPage } from './pages/DashboardPage'
import { DevicesPage } from './pages/DevicesPage'
import { DeviceDetailPage } from './pages/DeviceDetailPage'
import { TasksPage } from './pages/TasksPage'
import { ProvisioningPage } from './pages/ProvisioningPage'
import { FirmwarePage } from './pages/FirmwarePage'
import { CatalogPage } from './pages/CatalogPage'
import { AdministrationPage } from './pages/AdministrationPage'
import { TenantOnboardingWizard } from './pages/TenantOnboardingWizard'
import { WebhooksPage } from './pages/WebhooksPage'
import FilesPage from './pages/FilesPage'
import TagsPage from './pages/TagsPage'
import PresetsPage from './pages/PresetsPage'
import ProfilePage from './pages/ProfilePage'
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

function App() {
  return (
    <Routes>
      <Route path="/login" element={<LoginPage />} />

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
        <Route path="/provisioning" element={<ProvisioningPage />} />
        <Route path="/firmware" element={<FirmwarePage />} />
        <Route path="/files" element={<FilesPage />} />
        <Route path="/tags" element={<TagsPage />} />
        <Route path="/presets" element={<PresetsPage />} />
        <Route path="/audit" element={<AuditPage />} />
        <Route path="/profile" element={<ProfilePage />} />
        <Route path="/self-service" element={<SelfServicePage />} />
        <Route path="/webhooks" element={<WebhooksPage />} />
        <Route path="/catalog" element={<CatalogPage />} />
        <Route path="/administration" element={<AdministrationPage />} />
        <Route path="/administration/onboarding" element={<TenantOnboardingWizard />} />
        <Route path="/" element={<HomeRedirect />} />
      </Route>

      <Route path="*" element={<Navigate to="/" replace />} />
    </Routes>
  )
}

export default App
