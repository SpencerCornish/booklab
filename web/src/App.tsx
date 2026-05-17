import { BrowserRouter, Routes, Route, Navigate } from 'react-router'
import BookingPage from './pages/BookingPage'
import CancelPage from './pages/CancelPage'
import ViewBookingPage from './pages/ViewBookingPage'
import AdminLoginPage from './pages/AdminLoginPage'
import AdminDashboard from './pages/AdminDashboard'
import AdminBookings from './pages/AdminBookings'
import AdminClosures from './pages/AdminClosures'
import AdminSettings from './pages/AdminSettings'
import { AdminLayout } from './components/AdminLayout'

export default function App() {
  return (
    <BrowserRouter>
      <Routes>
        {/* Public */}
        <Route path="/" element={<BookingPage />} />
        <Route path="/booking/:token" element={<ViewBookingPage />} />
        <Route path="/cancel/:token" element={<CancelPage />} />

        {/* Admin */}
        <Route path="/admin/login" element={<AdminLoginPage />} />
        <Route path="/admin" element={<AdminLayout />}>
          <Route index element={<AdminDashboard />} />
          <Route path="bookings" element={<AdminBookings />} />
          <Route path="closures" element={<AdminClosures />} />
          <Route path="settings" element={<AdminSettings />} />
        </Route>

        <Route path="*" element={<Navigate to="/" replace />} />
      </Routes>
    </BrowserRouter>
  )
}
