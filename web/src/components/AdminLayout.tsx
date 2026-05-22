import { useEffect, useState } from 'react'
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
  const [sidebarOpen, setSidebarOpen] = useState(false)

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

  const closeSidebar = () => setSidebarOpen(false)

  return (
    <div className="min-h-screen flex">
      {/* Mobile top bar */}
      <header className="md:hidden fixed top-0 inset-x-0 h-12 bg-gray-900 text-white flex items-center gap-3 px-4 z-40">
        <button
          type="button"
          onClick={() => setSidebarOpen(true)}
          className="p-1 -ml-1 rounded-md text-gray-300 hover:text-white hover:bg-gray-800 transition-colors"
          aria-label="Open menu"
        >
          <span className="text-xl leading-none">☰</span>
        </button>
        <span className="font-semibold text-sm tracking-tight">BookLab Admin</span>
      </header>

      {/* Backdrop */}
      {sidebarOpen && (
        <div
          className="fixed inset-0 bg-black/40 z-40 md:hidden"
          onClick={closeSidebar}
          aria-hidden
        />
      )}

      {/* Sidebar */}
      <aside
        className={`fixed inset-y-0 left-0 z-50 w-56 bg-gray-900 text-white flex flex-col transform transition-transform duration-200 ease-in-out md:translate-x-0 md:static md:transform-none ${
          sidebarOpen ? 'translate-x-0' : '-translate-x-full'
        }`}
      >
        <div className="px-5 py-6 border-b border-gray-700">
          <span className="font-semibold text-lg tracking-tight">BookLab Admin</span>
        </div>
        <nav className="flex-1 px-3 py-4 space-y-1">
          {navItems.map((item) => (
            <NavLink
              key={item.to}
              to={item.to}
              end={item.end}
              onClick={closeSidebar}
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
      <main className="flex-1 overflow-auto bg-gray-50 pt-12 md:pt-0 min-w-0">
        <Outlet />
      </main>
    </div>
  )
}
