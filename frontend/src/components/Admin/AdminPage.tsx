import { useState, useEffect } from 'react';
import { Link } from 'react-router-dom';
import { api } from '../../services/api';
import type { Post } from '../../types';
import { Calendar, Share2, Users, UsersRound, Settings, TrendingUp, Clock, CheckCircle, AlertCircle, Image, Sparkles, ArrowUp, ArrowDown, Minus, RefreshCw, AlertTriangle, Lightbulb } from 'lucide-react';
import './Admin.css';

type InsightsData = {
  stats: { label: string; value: string; trend: 'up' | 'down' | 'neutral' }[];
  recommendations: { title: string; description: string; priority: 'high' | 'medium' | 'low' }[];
};

export function AdminPage() {
  const [stats, setStats] = useState({ total: 0, scheduled: 0, published: 0, failed: 0, accounts: 0, users: 0, teams: 0 });
  const [insights, setInsights] = useState<InsightsData | null>(null);
  const [insightsLoading, setInsightsLoading] = useState(false);
  const [insightsError, setInsightsError] = useState('');

  useEffect(() => {
    Promise.all([
      api.getPosts({}),
      api.getAccounts(),
      api.getUsers(),
      api.getTeams(),
    ]).then(([posts, accounts, users, teams]) => {
      setStats({
        total: posts.length,
        scheduled: posts.filter((p: Post) => p.status === 'scheduled').length,
        published: posts.filter((p: Post) => p.status === 'published').length,
        failed: posts.filter((p: Post) => p.status === 'failed').length,
        accounts: accounts.length,
        users: users.length,
        teams: teams.length,
      });
    });
  }, []);

  const loadInsights = () => {
    setInsightsLoading(true);
    setInsightsError('');
    api.getDashboardInsights()
      .then(data => setInsights(data))
      .catch(err => setInsightsError(err.message || 'Failed to generate insights'))
      .finally(() => setInsightsLoading(false));
  };

  const trendIcon = (trend: string) => {
    if (trend === 'up') return <ArrowUp size={14} />;
    if (trend === 'down') return <ArrowDown size={14} />;
    return <Minus size={14} />;
  };

  const priorityConfig = {
    high: { color: 'var(--danger)', label: 'High' },
    medium: { color: 'var(--warning)', label: 'Medium' },
    low: { color: 'var(--success)', label: 'Low' },
  };

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

      {/* AI Insights Section */}
      <div className="insights-section">
        <div className="insights-header">
          <div className="insights-title">
            <Sparkles size={20} />
            <h2>AI Insights</h2>
          </div>
          <button
            className="btn btn-ghost btn-sm"
            onClick={loadInsights}
            disabled={insightsLoading}
          >
            <RefreshCw size={14} className={insightsLoading ? 'spinning' : ''} />
            {insights ? 'Refresh' : 'Generate insights'}
          </button>
        </div>

        {insightsLoading && (
          <div className="insights-loading">
            <div className="spinner" />
            <p>Analyzing your posting data...</p>
          </div>
        )}

        {insightsError && (
          <div className="insights-error">
            <AlertTriangle size={16} />
            {insightsError}
          </div>
        )}

        {insights && !insightsLoading && (
          <>
            {insights.stats && insights.stats.length > 0 && (
              <div className="ai-stats-grid">
                {insights.stats.map((s, i) => (
                  <div key={i} className="ai-stat-card">
                    <div className="ai-stat-value">{s.value}</div>
                    <div className="ai-stat-label">{s.label}</div>
                    <div className={`ai-stat-trend ${s.trend}`}>
                      {trendIcon(s.trend)}
                    </div>
                  </div>
                ))}
              </div>
            )}

            {insights.recommendations && insights.recommendations.length > 0 && (
              <div className="recommendations-list">
                <h3><Lightbulb size={16} /> Recommendations</h3>
                {insights.recommendations.map((r, i) => {
                  const pc = priorityConfig[r.priority] || priorityConfig.medium;
                  return (
                    <div key={i} className="recommendation-card">
                      <div className="recommendation-priority" style={{ background: pc.color }} title={pc.label} />
                      <div className="recommendation-content">
                        <div className="recommendation-title">{r.title}</div>
                        <div className="recommendation-desc">{r.description}</div>
                      </div>
                    </div>
                  );
                })}
              </div>
            )}
          </>
        )}

        {!insights && !insightsLoading && !insightsError && (
          <div className="insights-placeholder">
            <Sparkles size={32} />
            <p>Get AI-powered insights about your posting patterns and content strategy.</p>
            <button className="btn btn-primary" onClick={loadInsights}>
              Generate insights
            </button>
          </div>
        )}
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

        <Link to="/admin/teams" className="admin-link-card">
          <UsersRound size={24} />
          <div>
            <h3>Teams</h3>
            <p>{stats.teams} team{stats.teams !== 1 ? 's' : ''}</p>
          </div>
        </Link>

        <Link to="/admin/watermarks" className="admin-link-card">
          <Image size={24} />
          <div>
            <h3>Watermarks</h3>
            <p>Image editor gallery</p>
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
