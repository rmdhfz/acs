import { Navigate, Route, Routes } from 'react-router-dom'
import { Layout } from './components/Layout'
import { ProtectedRoute } from './components/ProtectedRoute'
import { LoginPage } from './pages/LoginPage'
import { DevicesPage } from './pages/DevicesPage'
import { DeviceDetailPage } from './pages/DeviceDetailPage'
import { TasksPage } from './pages/TasksPage'
import { ProvisioningPage } from './pages/ProvisioningPage'
import { FirmwarePage } from './pages/FirmwarePage'
import { CatalogPage } from './pages/CatalogPage'
import { AdministrationPage } from './pages/AdministrationPage'

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
        <Route path="/devices" element={<DevicesPage />} />
        <Route path="/devices/:id" element={<DeviceDetailPage />} />
        <Route path="/tasks" element={<TasksPage />} />
        <Route path="/provisioning" element={<ProvisioningPage />} />
        <Route path="/firmware" element={<FirmwarePage />} />
        <Route path="/catalog" element={<CatalogPage />} />
        <Route path="/administration" element={<AdministrationPage />} />
        <Route path="/" element={<Navigate to="/devices" replace />} />
      </Route>

      <Route path="*" element={<Navigate to="/devices" replace />} />
    </Routes>
  )
}

export default App
