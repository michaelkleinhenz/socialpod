import { BrowserRouter, Routes, Route, Navigate } from 'react-router-dom';
import { Toaster } from 'react-hot-toast';
import { AuthProvider, useAuth } from './contexts/AuthContext';
import { Layout } from './components/Layout/Layout';
import { LoginPage } from './components/Auth/LoginPage';
import { RegisterPage } from './components/Auth/RegisterPage';
import { CalendarPage } from './components/Calendar/CalendarPage';
import { AdminPage } from './components/Admin/AdminPage';
import { AccountsPage } from './components/Admin/AccountsPage';
import { SettingsPage } from './components/Admin/SettingsPage';
import { UsersPage } from './components/Admin/UsersPage';
import { TeamsPage } from './components/Admin/TeamsPage';
import { WatermarksPage } from './components/Admin/WatermarksPage';
import { ProfilePage } from './components/Auth/ProfilePage';
import { LogPage } from './components/Log/LogPage';
import { SuffixesPage } from './components/Suffixes/SuffixesPage';
import { MentionsPage } from './components/Mentions/MentionsPage';
import { ShareTargetPage } from './components/ShareTarget/ShareTargetPage';
import { TeamManagePage } from './components/TeamAdmin/TeamManagePage';
import { ConventionPage } from './components/Convention/ConventionPage';
import { QueueDetailPage } from './components/Convention/QueueDetailPage';
import './styles/global.css';

// Register service worker for PWA / Web Share Target
if ('serviceWorker' in navigator) {
  window.addEventListener('load', () => {
    navigator.serviceWorker.register('/sw.js').catch(() => {});
  });
}

function ProtectedRoute({
  children,
  adminOnly = false,
  teamAdminOnly = false,
}: {
  children: React.ReactNode;
  adminOnly?: boolean;
  teamAdminOnly?: boolean;
}) {
  const { user, loading } = useAuth();
  if (loading) return <div className="loading-screen"><div className="spinner" /></div>;
  if (!user) return <Navigate to="/login" />;
  if (adminOnly && !user.isAdmin) return <Navigate to="/" />;
  if (teamAdminOnly && !user.isAdmin && !user.isTeamAdmin) return <Navigate to="/" />;
  return <>{children}</>;
}

function AppRoutes() {
  const { user, loading } = useAuth();
  if (loading) return <div className="loading-screen"><div className="spinner" /></div>;

  return (
    <Routes>
      <Route path="/login" element={user ? <Navigate to="/" /> : <LoginPage />} />
      <Route path="/register" element={user ? <Navigate to="/" /> : <RegisterPage />} />
      <Route path="/" element={<ProtectedRoute><Layout><CalendarPage /></Layout></ProtectedRoute>} />
      <Route path="/log" element={<ProtectedRoute><Layout><LogPage /></Layout></ProtectedRoute>} />
      <Route path="/suffixes" element={<ProtectedRoute><Layout><SuffixesPage /></Layout></ProtectedRoute>} />
      <Route path="/mentions" element={<ProtectedRoute><Layout><MentionsPage /></Layout></ProtectedRoute>} />
      <Route path="/profile" element={<ProtectedRoute><Layout><ProfilePage /></Layout></ProtectedRoute>} />
      <Route path="/admin" element={<ProtectedRoute teamAdminOnly><Layout><AdminPage /></Layout></ProtectedRoute>} />
      <Route path="/admin/accounts" element={<ProtectedRoute adminOnly><Layout><AccountsPage /></Layout></ProtectedRoute>} />
      <Route path="/admin/settings" element={<ProtectedRoute adminOnly><Layout><SettingsPage /></Layout></ProtectedRoute>} />
      <Route path="/admin/users" element={<ProtectedRoute adminOnly><Layout><UsersPage /></Layout></ProtectedRoute>} />
      <Route path="/admin/teams" element={<ProtectedRoute adminOnly><Layout><TeamsPage /></Layout></ProtectedRoute>} />
      <Route path="/watermarks" element={<ProtectedRoute><Layout><WatermarksPage /></Layout></ProtectedRoute>} />
      {/* Convention mode */}
      <Route path="/convention" element={<ProtectedRoute><Layout><ConventionPage /></Layout></ProtectedRoute>} />
      <Route path="/convention/:id" element={<ProtectedRoute><Layout><QueueDetailPage /></Layout></ProtectedRoute>} />
      {/* Team admin routes */}
      <Route path="/team/manage" element={<ProtectedRoute teamAdminOnly><Layout><TeamManagePage /></Layout></ProtectedRoute>} />
      {/* PWA Web Share Target — standalone mobile UI, no sidebar */}
      <Route path="/share-target" element={<ShareTargetPage />} />
      <Route path="*" element={<Navigate to="/" />} />
    </Routes>
  );
}

export default function App() {
  return (
    <BrowserRouter>
      <AuthProvider>
        <AppRoutes />
        <Toaster
          position="top-right"
          toastOptions={{
            style: { background: '#1e293b', color: '#f1f5f9', border: '1px solid #334155' },
            success: { iconTheme: { primary: '#22c55e', secondary: '#f1f5f9' } },
            error: { iconTheme: { primary: '#ef4444', secondary: '#f1f5f9' } },
          }}
        />
      </AuthProvider>
    </BrowserRouter>
  );
}
