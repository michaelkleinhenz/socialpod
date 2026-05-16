import type { ReactNode } from 'react';
import { Link, useLocation } from 'react-router-dom';
import { useAuth } from '../../contexts/AuthContext';
import { Calendar, Settings, Users, Share2, LogOut, User, Zap, ScrollText, UsersRound, Signature, Image, MessageCircle, Mail, LayoutGrid } from 'lucide-react';
import './Layout.css';

export function Layout({ children }: { children: ReactNode }) {
  const { user, logout } = useAuth();
  const location = useLocation();

  const navItems = [
    ...(user?.isAdmin ? [{ path: '/admin', icon: Zap, label: 'Dashboard' }] : []),
    { path: '/', icon: Calendar, label: 'Calendar' },
    { path: '/inbox/comments', icon: MessageCircle, label: 'Comments' },
    { path: '/inbox/dms', icon: Mail, label: 'DMs' },
    { path: '/feed', icon: LayoutGrid, label: 'Feed' },
    { path: '/log', icon: ScrollText, label: 'Post Log' },
    ...(user?.isAdmin ? [
      { path: '/admin/accounts', icon: Share2, label: 'Accounts' },
    ] : []),
    ...(!user?.isAdmin && user?.isTeamAdmin ? [
      { path: '/team/manage', icon: UsersRound, label: 'Team Admin' },
    ] : []),
    { path: '/suffixes', icon: Signature, label: 'Suffixes' },
    ...(user?.isAdmin ? [
      { path: '/admin/watermarks', icon: Image, label: 'Watermarks' },
      { path: '/admin/users', icon: Users, label: 'Users' },
      { path: '/admin/teams', icon: UsersRound, label: 'Teams' },
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
