import { useState, useEffect } from 'react';
import { Link } from 'react-router-dom';
import { api } from '../../services/api';
import type { Post } from '../../types';
import { Calendar, Share2, Users, Settings, TrendingUp, Clock, CheckCircle, AlertCircle } from 'lucide-react';
import './Admin.css';

export function AdminPage() {
  const [stats, setStats] = useState({ total: 0, scheduled: 0, published: 0, failed: 0, accounts: 0, users: 0 });

  useEffect(() => {
    Promise.all([
      api.getPosts({}),
      api.getAccounts(),
      api.getUsers(),
    ]).then(([posts, accounts, users]) => {
      setStats({
        total: posts.length,
        scheduled: posts.filter((p: Post) => p.status === 'scheduled').length,
        published: posts.filter((p: Post) => p.status === 'published').length,
        failed: posts.filter((p: Post) => p.status === 'failed').length,
        accounts: accounts.length,
        users: users.length,
      });
    });
  }, []);

  return (
    <div className="page">
      <div className="page-header">
        <h1>Dashboard</h1>
      </div>

      <div className="stats-grid">
        <div className="stat-card">
          <div className="stat-icon" style={{ background: 'var(--accent-muted)', color: 'var(--accent)' }}>
            <Calendar size={24} />
          </div>
          <div>
            <div className="stat-value">{stats.total}</div>
            <div className="stat-label">Total Posts</div>
          </div>
        </div>

        <div className="stat-card">
          <div className="stat-icon" style={{ background: 'rgba(99,102,241,0.15)', color: 'var(--accent)' }}>
            <Clock size={24} />
          </div>
          <div>
            <div className="stat-value">{stats.scheduled}</div>
            <div className="stat-label">Scheduled</div>
          </div>
        </div>

        <div className="stat-card">
          <div className="stat-icon" style={{ background: 'rgba(34,197,94,0.15)', color: 'var(--success)' }}>
            <CheckCircle size={24} />
          </div>
          <div>
            <div className="stat-value">{stats.published}</div>
            <div className="stat-label">Published</div>
          </div>
        </div>

        <div className="stat-card">
          <div className="stat-icon" style={{ background: 'rgba(239,68,68,0.15)', color: 'var(--danger)' }}>
            <AlertCircle size={24} />
          </div>
          <div>
            <div className="stat-value">{stats.failed}</div>
            <div className="stat-label">Failed</div>
          </div>
        </div>
      </div>

      <div className="admin-links">
        <Link to="/admin/accounts" className="admin-link-card">
          <Share2 size={24} />
          <div>
            <h3>Social Accounts</h3>
            <p>{stats.accounts} connected</p>
          </div>
        </Link>

        <Link to="/admin/users" className="admin-link-card">
          <Users size={24} />
          <div>
            <h3>Users</h3>
            <p>{stats.users} registered</p>
          </div>
        </Link>

        <Link to="/admin/settings" className="admin-link-card">
          <Settings size={24} />
          <div>
            <h3>Settings</h3>
            <p>App configuration</p>
          </div>
        </Link>

        <Link to="/" className="admin-link-card">
          <TrendingUp size={24} />
          <div>
            <h3>Calendar</h3>
            <p>View schedule</p>
          </div>
        </Link>
      </div>
    </div>
  );
}
