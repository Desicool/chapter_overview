import { BrowserRouter, Routes, Route, NavLink } from 'react-router-dom'
import UploadPage from './pages/UploadPage'
import TaskPage from './pages/TaskPage'
import TaskListPage from './pages/TaskListPage'
import ViewerPage from './pages/ViewerPage'
import StatsPage from './pages/StatsPage'

function Layout({ children }: { children: React.ReactNode }) {
  return (
    <div className="min-h-screen bg-bg">
      <nav className="bg-surface border-b border-border">
        <div className="max-w-7xl mx-auto px-8 h-12 flex items-center gap-6">
          <span className="font-serif text-amber text-sm tracking-wide">Chapter</span>
          <div className="flex items-center gap-1">
            <NavLink
              to="/"
              end
              className={({ isActive }) =>
                `font-mono text-xs px-3 py-1.5 rounded transition-colors ${
                  isActive ? 'text-amber bg-amber/10' : 'text-muted hover:text-text'
                }`
              }
            >
              Upload
            </NavLink>
            <NavLink
              to="/tasks"
              className={({ isActive }) =>
                `font-mono text-xs px-3 py-1.5 rounded transition-colors ${
                  isActive ? 'text-amber bg-amber/10' : 'text-muted hover:text-text'
                }`
              }
            >
              Tasks
            </NavLink>
            <NavLink
              to="/stats"
              className={({ isActive }) =>
                `font-mono text-xs px-3 py-1.5 rounded transition-colors ${
                  isActive ? 'text-amber bg-amber/10' : 'text-muted hover:text-text'
                }`
              }
            >
              Stats
            </NavLink>
          </div>
        </div>
      </nav>
      {children}
    </div>
  )
}

export default function App() {
  return (
    <BrowserRouter>
      <Layout>
        <Routes>
          <Route path="/" element={<UploadPage />} />
          <Route path="/tasks" element={<TaskListPage />} />
          <Route path="/tasks/:id" element={<TaskPage />} />
          <Route path="/tasks/:id/view/:page" element={<ViewerPage />} />
          <Route path="/stats" element={<StatsPage />} />
        </Routes>
      </Layout>
    </BrowserRouter>
  )
}
