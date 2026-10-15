import { useState, useEffect, useMemo, type ReactNode } from 'react';
import { Link, useLocation } from 'react-router-dom';
import { useAuth } from '../../contexts/AuthContext';
import { api } from '../../services/api';
import {
  Calendar, Settings, Users, Share2, LogOut, User, Zap,
  ScrollText, UsersRound, Signature, LayoutGrid,
  AtSign, Tent, ChevronDown, Layers, SlidersHorizontal, Shield,
  Newspaper, Mic, TrendingUp, Target, Menu, X, FolderOpen, Bot,
} from 'lucide-react';
import './Layout.css';

type LucideIcon = React.ComponentType<{ size?: number; className?: string }>;
type NavLeaf = { kind: 'leaf'; path: string; icon: LucideIcon; label: string };
type NavGroup = { kind: 'group'; id: string; icon: LucideIcon; label: string; children: NavLeaf[] };
type NavEntry = NavLeaf | NavGroup;

export function Layout({ children }: { children: ReactNode }) {
  const { user, logout } = useAuth();
  const location = useLocation();
  const [newsPluginEnabled, setNewsPluginEnabled] = useState(false);
  const [episodePluginEnabled, setEpisodePluginEnabled] = useState(false);

  useEffect(() => {
    if (user?.teamId) {
      api.getTeamSettings().then(s => {
        if (s.enabledPlugins?.includes('news_creator') && s.newsCreatorUrl && s.hasNewsCreatorBearerToken) {
          setNewsPluginEnabled(true);
        }
        if (s.enabledPlugins?.includes('episode_creator') && s.episodeCreatorUrl && s.hasEpisodeCreatorBearerToken) {
          setEpisodePluginEnabled(true);
        }
      }).catch(() => {});
    }
  }, [user?.teamId]);

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
        ...(user?.teamId ? [{ kind: 'leaf' as const, path: '/growth', icon: TrendingUp, label: 'Growth' }] : []),
        { kind: 'leaf' as const, path: '/log', icon: ScrollText, label: 'Post Log' },
        { kind: 'leaf' as const, path: '/convention', icon: Tent, label: 'Convention' },
        ...(newsPluginEnabled ? [{ kind: 'leaf' as const, path: '/news', icon: Newspaper, label: 'News' }] : []),
        ...(episodePluginEnabled ? [{ kind: 'leaf' as const, path: '/episodes', icon: Mic, label: 'Episodes' }] : []),
      ],
    },
    {
      kind: 'group' as const,
      id: 'tools',
      icon: SlidersHorizontal,
      label: 'Post Tools',
      children: [
        { kind: 'leaf' as const, path: '/footers', icon: Signature, label: 'Footers' },
        { kind: 'leaf' as const, path: '/mentions', icon: AtSign, label: 'Mentions' },
        { kind: 'leaf' as const, path: '/media', icon: FolderOpen, label: 'Media Library' },
        { kind: 'leaf' as const, path: '/agents', icon: Bot, label: 'Agents' },
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
        ] : [
          { kind: 'leaf' as const, path: '/admin/my-team', icon: UsersRound, label: 'Team' },
          { kind: 'leaf' as const, path: '/admin/team-accounts', icon: Share2, label: 'Accounts' },
        ]),
        ...(user?.teamId ? [{ kind: 'leaf' as const, path: '/admin/growth', icon: Target, label: 'Growth Settings' }] : []),
      ],
    }] : []),
  ], [user?.isAdmin, user?.isTeamAdmin, user?.teamId, newsPluginEnabled, episodePluginEnabled]);

  // On phones the sidebar is an off-canvas drawer behind a hamburger in a
  // slim top bar, so it takes no width from the page while closed. It
  // remembers the path it was opened on, so navigating closes it.
  const [drawerPath, setDrawerPath] = useState<string | null>(null);
  const drawerOpen = drawerPath === location.pathname;
  const closeDrawer = () => setDrawerPath(null);

  useEffect(() => {
    if (!drawerOpen) return;
    const onKey = (e: KeyboardEvent) => { if (e.key === 'Escape') setDrawerPath(null); };
    document.addEventListener('keydown', onKey);
    return () => document.removeEventListener('keydown', onKey);
  }, [drawerOpen]);

  const [openGroups, setOpenGroups] = useState<Set<string>>(() => {
    const initial = new Set<string>(['content']);
    for (const entry of navEntries) {
      if (entry.kind === 'group' && entry.children.some(c => c.path === location.pathname)) {
        initial.add(entry.id);
      }
    }
    return initial;
  });

  // Expand the group holding the current page whenever the page changes.
  const [groupsPath, setGroupsPath] = useState(location.pathname);
  if (groupsPath !== location.pathname) {
    setGroupsPath(location.pathname);
    const active = navEntries.find(e => e.kind === 'group' && e.children.some(c => c.path === location.pathname));
    if (active && active.kind === 'group' && !openGroups.has(active.id)) {
      setOpenGroups(new Set([...openGroups, active.id]));
    }
  }

  const toggleGroup = (id: string) => {
    setOpenGroups(prev => {
      const next = new Set(prev);
      if (next.has(id)) next.delete(id); else next.add(id);
      return next;
    });
  };

  const isActive = (path: string) => location.pathname === path;

  const currentLabel = useMemo(() => {
    for (const entry of navEntries) {
      if (entry.kind === 'leaf' && entry.path === location.pathname) return entry.label;
      if (entry.kind === 'group') {
        const child = entry.children.find(c => c.path === location.pathname);
        if (child) return child.label;
      }
    }
    return location.pathname === '/profile' ? 'Profile' : '';
  }, [navEntries, location.pathname]);

  return (
    <div className="layout">
      <header className="mobile-topbar">
        <button
          className="mobile-menu-btn"
          onClick={() => setDrawerPath(location.pathname)}
          aria-label="Open menu"
          aria-expanded={drawerOpen}
        >
          <Menu size={22} />
        </button>
        <div className="logo-icon mobile-topbar-logo">
          <Zap size={16} />
        </div>
        <span className="mobile-topbar-title">{currentLabel || 'SocialPod'}</span>
      </header>

      {drawerOpen && (
        <div className="sidebar-backdrop" onClick={() => closeDrawer()} />
      )}

      <aside className={`sidebar${drawerOpen ? ' open' : ''}`}>
        <div className="sidebar-header">
          <div className="logo">
            <div className="logo-icon">
              <Zap size={20} />
            </div>
            <span className="logo-text">SocialPod</span>
          </div>
          <button
            className="sidebar-close-btn"
            onClick={() => closeDrawer()}
            aria-label="Close menu"
          >
            <X size={20} />
          </button>
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
                  className={`nav-item nav-group-header ${hasActive ? 'active' : ''}`}
                  onClick={() => toggleGroup(entry.id)}
                >
                  <entry.icon size={18} />
                  <span>{entry.label}</span>
                  <ChevronDown size={14} className={`nav-group-chevron ${isOpen ? 'open' : ''}`} />
                </button>
                <div className={`nav-group-children${isOpen ? '' : ' collapsed'}`}>
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
