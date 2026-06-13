import { BrowserRouter, Routes, Route, Navigate } from 'react-router'
import BookingPage from './pages/BookingPage'
import CancelPage from './pages/CancelPage'
import ViewBookingPage from './pages/ViewBookingPage'
import TermsPage from './pages/TermsPage'
import PrivacyPage from './pages/PrivacyPage'
import AdminLoginPage from './pages/AdminLoginPage'
import AdminDashboard from './pages/AdminDashboard'
import AdminBookings from './pages/AdminBookings'
import AdminClosures from './pages/AdminClosures'
import AdminSettings from './pages/AdminSettings'
import AdminInsights from './pages/AdminInsights'
import AdminUsers from './pages/AdminUsers'
import { AdminLayout } from './components/AdminLayout'

export default function App() {
  return (
    <BrowserRouter>
      <Routes>
        {/* Public */}
        <Route path="/" element={<BookingPage />} />
        <Route path="/booking/:token" element={<ViewBookingPage />} />
        <Route path="/cancel/:token" element={<CancelPage />} />
        <Route path="/terms" element={<TermsPage />} />
        <Route path="/privacy" element={<PrivacyPage />} />

        {/* Admin */}
        <Route path="/admin/login" element={<AdminLoginPage />} />
        <Route path="/admin" element={<AdminLayout />}>
          <Route index element={<AdminDashboard />} />
          <Route path="bookings" element={<AdminBookings />} />
          <Route path="closures" element={<AdminClosures />} />
          <Route path="insights" element={<AdminInsights />} />
          <Route path="users" element={<AdminUsers />} />
          <Route path="settings" element={<AdminSettings />} />
        </Route>

        <Route path="*" element={<Navigate to="/" replace />} />
      </Routes>
    </BrowserRouter>
  )
}
