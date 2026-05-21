import { useState, useEffect, useMemo, type ReactNode } from 'react';
import { Link, useLocation } from 'react-router-dom';
import { useAuth } from '../../contexts/AuthContext';
import {
  Calendar, Settings, Users, Share2, LogOut, User, Zap,
  ScrollText, UsersRound, Signature, Image, LayoutGrid,
  AtSign, Tent, ChevronDown, Layers, SlidersHorizontal, Shield,
} from 'lucide-react';
import './Layout.css';

type LucideIcon = React.ComponentType<{ size?: number; className?: string }>;
type NavLeaf = { kind: 'leaf'; path: string; icon: LucideIcon; label: string };
type NavGroup = { kind: 'group'; id: string; icon: LucideIcon; label: string; children: NavLeaf[] };
type NavEntry = NavLeaf | NavGroup;

export function Layout({ children }: { children: ReactNode }) {
  const { user, logout } = useAuth();
  const location = useLocation();

  const navEntries = useMemo<NavEntry[]>(() => [
    ...((user?.isAdmin || user?.isTeamAdmin) ? [
      { kind: 'leaf' as const, path: '/admin', icon: Zap, label: 'Dashboard' },
    ] : []),
    {
      kind: 'group' as const,
      id: 'content',
      icon: Layers,
      label: 'Content',
      children: [
        { kind: 'leaf' as const, path: '/', icon: Calendar, label: 'Calendar' },
        { kind: 'leaf' as const, path: '/feed', icon: LayoutGrid, label: 'Feed' },
        { kind: 'leaf' as const, path: '/log', icon: ScrollText, label: 'Post Log' },
        { kind: 'leaf' as const, path: '/convention', icon: Tent, label: 'Convention' },
      ],
    },
    {
      kind: 'group' as const,
      id: 'tools',
      icon: SlidersHorizontal,
      label: 'Post Tools',
      children: [
        { kind: 'leaf' as const, path: '/suffixes', icon: Signature, label: 'Suffixes' },
        { kind: 'leaf' as const, path: '/mentions', icon: AtSign, label: 'Mentions' },
        { kind: 'leaf' as const, path: '/watermarks', icon: Image, label: 'Watermarks' },
      ],
    },
    ...((user?.isAdmin || user?.isTeamAdmin) ? [{
      kind: 'group' as const,
      id: 'admin',
      icon: Shield,
      label: 'Administration',
      children: [
        ...(user?.isAdmin ? [
          { kind: 'leaf' as const, path: '/admin/accounts', icon: Share2, label: 'Accounts' },
          { kind: 'leaf' as const, path: '/admin/users', icon: Users, label: 'Users' },
          { kind: 'leaf' as const, path: '/admin/teams', icon: UsersRound, label: 'Teams' },
          { kind: 'leaf' as const, path: '/admin/settings', icon: Settings, label: 'Settings' },
        ] : []),
        ...(!user?.isAdmin && user?.isTeamAdmin ? [
          { kind: 'leaf' as const, path: '/team/manage', icon: UsersRound, label: 'Team Admin' },
        ] : []),
      ],
    }] : []),
  ], [user?.isAdmin, user?.isTeamAdmin]);

  const [openGroups, setOpenGroups] = useState<Set<string>>(() => {
    const initial = new Set<string>(['content']);
    for (const entry of navEntries) {
      if (entry.kind === 'group' && entry.children.some(c => c.path === location.pathname)) {
        initial.add(entry.id);
      }
    }
    return initial;
  });

  useEffect(() => {
    for (const entry of navEntries) {
      if (entry.kind === 'group' && entry.children.some(c => c.path === location.pathname)) {
        setOpenGroups(prev => prev.has(entry.id) ? prev : new Set([...prev, entry.id]));
        break;
      }
    }
  }, [location.pathname, navEntries]);

  const toggleGroup = (id: string) => {
    setOpenGroups(prev => {
      const next = new Set(prev);
      if (next.has(id)) next.delete(id); else next.add(id);
      return next;
    });
  };

  const isActive = (path: string) => location.pathname === path;

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
          {navEntries.map(entry => {
            if (entry.kind === 'leaf') {
              return (
                <Link
                  key={entry.path}
                  to={entry.path}
                  className={`nav-item ${isActive(entry.path) ? 'active' : ''}`}
                >
                  <entry.icon size={18} />
                  <span>{entry.label}</span>
                </Link>
              );
            }

            const isOpen = openGroups.has(entry.id);
            const hasActive = entry.children.some(c => isActive(c.path));

            return (
              <div key={entry.id} className="nav-group">
                <button
                  className={`nav-item nav-group-header ${hasActive && !isOpen ? 'active' : ''}`}
                  onClick={() => toggleGroup(entry.id)}
                >
                  <entry.icon size={18} />
                  <span>{entry.label}</span>
                  <ChevronDown size={14} className={`nav-group-chevron ${isOpen ? 'open' : ''}`} />
                </button>
                {isOpen && (
                  <div className="nav-group-children">
                    {entry.children.map(child => (
                      <Link
                        key={child.path}
                        to={child.path}
                        className={`nav-item nav-child ${isActive(child.path) ? 'active' : ''}`}
                      >
                        <child.icon size={16} />
                        <span>{child.label}</span>
                      </Link>
                    ))}
                  </div>
                )}
              </div>
            );
          })}
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
