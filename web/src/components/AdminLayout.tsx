import { useEffect } from 'react'
import { Outlet, NavLink, useNavigate } from 'react-router'
import { adminGetSettings, adminLogout, ApiError } from '../lib/api'

const navItems = [
  { to: '/admin', label: 'Dashboard', end: true },
  { to: '/admin/bookings', label: 'Bookings' },
  { to: '/admin/closures', label: 'Closures' },
  { to: '/admin/settings', label: 'Settings' },
]

export function AdminLayout() {
  const navigate = useNavigate()

  useEffect(() => {
    adminGetSettings().catch((err) => {
      if (err instanceof ApiError && err.status === 401) {
        navigate('/admin/login', { replace: true })
      }
    })
  }, [navigate])

  const handleLogout = async () => {
    await adminLogout().catch(() => {})
    navigate('/admin/login')
  }

  return (
    <div className="min-h-screen flex">
      {/* Sidebar */}
      <aside className="w-56 bg-gray-900 text-white flex flex-col">
        <div className="px-5 py-6 border-b border-gray-700">
          <span className="font-semibold text-lg tracking-tight">BookLab Admin</span>
        </div>
        <nav className="flex-1 px-3 py-4 space-y-1">
          {navItems.map((item) => (
            <NavLink
              key={item.to}
              to={item.to}
              end={item.end}
              className={({ isActive }) =>
                `block px-3 py-2 rounded-md text-sm font-medium transition-colors ${
                  isActive
                    ? 'bg-gray-700 text-white'
                    : 'text-gray-300 hover:bg-gray-800 hover:text-white'
                }`
              }
            >
              {item.label}
            </NavLink>
          ))}
        </nav>
        <div className="px-3 py-4 border-t border-gray-700">
          <button
            onClick={handleLogout}
            className="w-full text-left px-3 py-2 rounded-md text-sm text-gray-400 hover:text-white hover:bg-gray-800 transition-colors"
          >
            Sign out
          </button>
        </div>
      </aside>

      {/* Main content */}
      <main className="flex-1 overflow-auto bg-gray-50">
        <Outlet />
      </main>
    </div>
  )
}
