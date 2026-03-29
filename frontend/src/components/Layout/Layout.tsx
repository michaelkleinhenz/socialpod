import type { ReactNode } from 'react';
import { Link, useLocation } from 'react-router-dom';
import { useAuth } from '../../contexts/AuthContext';
import { Calendar, Settings, Users, Share2, LogOut, User, Zap } from 'lucide-react';
import './Layout.css';

export function Layout({ children }: { children: ReactNode }) {
  const { user, logout } = useAuth();
  const location = useLocation();

  const navItems = [
    { path: '/', icon: Calendar, label: 'Calendar' },
    ...(user?.isAdmin ? [
      { path: '/admin', icon: Zap, label: 'Dashboard' },
      { path: '/admin/accounts', icon: Share2, label: 'Accounts' },
      { path: '/admin/users', icon: Users, label: 'Users' },
      { path: '/admin/settings', icon: Settings, label: 'Settings' },
    ] : []),
  ];

  return (
    <div className="layout">
      <aside className="sidebar">
        <div className="sidebar-header">
          <div className="logo">
            <div className="logo-icon">
              <Zap size={20} />
            </div>
            <span className="logo-text">SocialPod</span>
          </div>
        </div>

        <nav className="sidebar-nav">
          {navItems.map(item => (
            <Link
              key={item.path}
              to={item.path}
              className={`nav-item ${location.pathname === item.path ? 'active' : ''}`}
            >
              <item.icon size={18} />
              <span>{item.label}</span>
            </Link>
          ))}
        </nav>

        <div className="sidebar-footer">
          <Link to="/profile" className="nav-item user-item">
            <User size={18} />
            <span>{user?.name}</span>
          </Link>
          <button className="nav-item logout-btn" onClick={logout}>
            <LogOut size={18} />
            <span>Logout</span>
          </button>
        </div>
      </aside>

      <main className="main-content">
        {children}
      </main>
    </div>
  );
}
