import { useState, useEffect } from 'react';
import { Link } from 'react-router-dom';
import { api } from '../../services/api';
import { useAuth } from '../../contexts/AuthContext';
import type { Post } from '../../types';
import {
  Calendar, Share2, Users, UsersRound, Settings, TrendingUp, Clock,
  CheckCircle, AlertCircle, Image, Sparkles, ArrowUp, ArrowDown, Minus,
  RefreshCw, AlertTriangle, Lightbulb, MessageCircle, Mail, Heart, BarChart2,
  Flame, TrendingDown,
} from 'lucide-react';
import './Admin.css';

// ─── Types ────────────────────────────────────────────────────────────────────

type InsightsData = {
  stats: { label: string; value: string; trend: 'up' | 'down' | 'neutral' }[];
  recommendations: { title: string; description: string; priority: 'high' | 'medium' | 'low' }[];
};

interface DashStats {
  postsByDay: { date: string; count: number }[];
  platformBreakdown: Record<string, number>;
  postTypeBreakdown: Record<string, number>;
  statusBreakdown: Record<string, number>;
  hourDistribution: { hour: number; count: number }[];
  weekdayDistribution: { day: string; count: number }[];
  thisWeekPosts: number;
  lastWeekPosts: number;
  successRate: number;
  inboxStats: { comments: number; dms: number; unread: number; liked: number };
}

interface FeedItem {
  id: string;
  caption: string;
  mediaType: string;
  mediaUrl: string;
  thumbnailUrl: string;
  permalink: string;
  timestamp: string;
  likeCount: number;
  commentsCount: number;
}

// ─── Mini chart primitives ─────────────────────────────────────────────────────

function BarChart({ data, color = 'var(--accent)', height = 72 }: {
  data: { value: number; label?: string }[];
  color?: string;
  height?: number;
}) {
  const max = Math.max(...data.map(d => d.value), 1);
  const bw = 100 / data.length;
  return (
    <svg
      viewBox={`0 0 100 ${height}`}
      preserveAspectRatio="none"
      style={{ width: '100%', height, display: 'block' }}
    >
      {data.map((d, i) => {
        const h = (d.value / max) * height;
        const opacity = d.value === 0 ? 0.15 : 0.55 + 0.45 * (d.value / max);
        return (
          <rect
            key={i}
            x={i * bw + 0.4}
            y={height - h}
            width={bw - 0.8}
            height={Math.max(h, d.value === 0 ? 1 : 0)}
            fill={color}
            opacity={opacity}
            rx={0.8}
          />
        );
      })}
    </svg>
  );
}

function HorizontalBar({ pct, color }: { pct: number; color: string }) {
  return (
    <div className="hbar-track">
      <div className="hbar-fill" style={{ width: `${Math.max(pct, 2)}%`, background: color }} />
    </div>
  );
}

// ─── Main component ────────────────────────────────────────────────────────────

export function AdminPage() {
  const { user } = useAuth();
  const [stats, setStats] = useState({
    total: 0, scheduled: 0, published: 0, failed: 0,
    accounts: 0, users: 0, teams: 0,
  });
  const [dashStats, setDashStats] = useState<DashStats | null>(null);
  const [feedItems, setFeedItems] = useState<FeedItem[]>([]);
  const [feedLoading, setFeedLoading] = useState(false);
  const [activeAccounts, setActiveAccounts] = useState<any[]>([]);

  const [insights, setInsights] = useState<InsightsData | null>(null);
  const [insightsLoading, setInsightsLoading] = useState(false);
  const [insightsError, setInsightsError] = useState('');

  useEffect(() => {
    Promise.all([
      api.getPosts({}),
      api.getDashboardStats(),
      api.getActiveAccounts(),
    ]).then(([posts, ds, active]) => {
      setStats(s => ({
        ...s,
        total: posts.length,
        scheduled: posts.filter((p: Post) => p.status === 'scheduled').length,
        published: posts.filter((p: Post) => p.status === 'published').length,
        failed: posts.filter((p: Post) => p.status === 'failed').length,
        accounts: active.length,
      }));
      setDashStats(ds);
      setActiveAccounts(active);
    });

    if (user?.isAdmin) {
      Promise.all([
        api.getAccounts(),
        api.getUsers(),
        api.getTeams(),
      ]).then(([adminAccounts, users, teams]) => {
        setStats(s => ({
          ...s,
          accounts: adminAccounts.length,
          users: users.length,
          teams: teams.length,
        }));
      });
    }
  }, []); // eslint-disable-line react-hooks/exhaustive-deps

  // Load feed engagement data for all active accounts
  useEffect(() => {
    if (activeAccounts.length === 0) return;
    setFeedLoading(true);
    Promise.all(
      activeAccounts.map((a: any) => api.getFeed(a.id, a.platform).catch(() => []))
    )
      .then(results => {
        const all: FeedItem[] = (results as FeedItem[][]).flat();
        setFeedItems(all);
      })
      .finally(() => setFeedLoading(false));
  }, [activeAccounts]);

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

  // ─── Derived engagement metrics ──────────────────────────────────────────────
  const sortedByLikes = [...feedItems].sort((a, b) => b.likeCount - a.likeCount);
  const mostPopular = sortedByLikes[0] ?? null;
  const leastPopular = sortedByLikes.length > 1 ? sortedByLikes[sortedByLikes.length - 1] : null;
  const totalFeedLikes = feedItems.reduce((s, f) => s + f.likeCount, 0);
  const totalFeedComments = feedItems.reduce((s, f) => s + f.commentsCount, 0);
  const avgEngagement = feedItems.length > 0
    ? Math.round((totalFeedLikes + totalFeedComments) / feedItems.length)
    : 0;

  // ─── Derived chart helpers ───────────────────────────────────────────────────
  const bestHour = dashStats
    ? dashStats.hourDistribution.reduce((best, h) => h.count > best.count ? h : best, { hour: 0, count: 0 })
    : null;

  const bestDay = dashStats
    ? dashStats.weekdayDistribution.reduce((best, d) => d.count > best.count ? d : best, { day: '-', count: 0 })
    : null;

  const weekChange = dashStats && dashStats.lastWeekPosts > 0
    ? Math.round(((dashStats.thisWeekPosts - dashStats.lastWeekPosts) / dashStats.lastWeekPosts) * 100)
    : null;

  const platformTotal = dashStats
    ? Object.values(dashStats.platformBreakdown).reduce((s, n) => s + n, 0) || 1
    : 1;

  const typeTotal = dashStats
    ? Object.values(dashStats.postTypeBreakdown).reduce((s, n) => s + n, 0) || 1
    : 1;

  const platformColors: Record<string, string> = {
    instagram: '#e1306c',
    bluesky: '#0085ff',
  };

  const typeColors: Record<string, string> = {
    post: 'var(--accent)',
    story: '#f59e0b',
    reel: '#8b5cf6',
  };

  // ─── Render ──────────────────────────────────────────────────────────────────

  return (
    <div className="page">
      <div className="page-header">
        <h1>Dashboard</h1>
      </div>

      {/* ── Top stats ── */}
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

      {/* ── Analytics ── */}
      {dashStats && (
        <div className="analytics-section">
          <div className="analytics-section-title">
            <BarChart2 size={18} />
            <h2>Analytics</h2>
          </div>

          {/* Row 1: posts over time + weekly comparison + inbox overview */}
          <div className="analytics-row">
            {/* Posts over time */}
            <div className="analytics-card analytics-card-wide">
              <div className="analytics-card-header">
                <span className="analytics-card-label">Posts — last 30 days</span>
                <span className="analytics-card-meta">{dashStats.postsByDay.reduce((s, d) => s + d.count, 0)} total</span>
              </div>
              <BarChart
                data={dashStats.postsByDay.map(d => ({ value: d.count }))}
                color="var(--accent)"
                height={72}
              />
              <div className="chart-axis-labels">
                <span>{dashStats.postsByDay[0]?.date.slice(5)}</span>
                <span>{dashStats.postsByDay[14]?.date.slice(5)}</span>
                <span>{dashStats.postsByDay[29]?.date.slice(5)}</span>
              </div>
            </div>

            {/* This week vs last week */}
            <div className="analytics-card">
              <div className="analytics-card-label" style={{ marginBottom: 16 }}>Weekly activity</div>
              <div className="week-compare">
                <div className="week-compare-item">
                  <div className="week-compare-value">{dashStats.thisWeekPosts}</div>
                  <div className="week-compare-sub">This week</div>
                </div>
                <div className="week-compare-sep">vs</div>
                <div className="week-compare-item">
                  <div className="week-compare-value" style={{ color: 'var(--text-muted)' }}>
                    {dashStats.lastWeekPosts}
                  </div>
                  <div className="week-compare-sub">Last week</div>
                </div>
              </div>
              {weekChange !== null && (
                <div className={`week-change ${weekChange >= 0 ? 'positive' : 'negative'}`}>
                  {weekChange >= 0 ? <ArrowUp size={13} /> : <ArrowDown size={13} />}
                  {Math.abs(weekChange)}% vs last week
                </div>
              )}
              {weekChange === null && dashStats.lastWeekPosts === 0 && dashStats.thisWeekPosts > 0 && (
                <div className="week-change positive">
                  <ArrowUp size={13} />
                  New activity this week
                </div>
              )}
            </div>

            {/* Inbox overview */}
            <div className="analytics-card">
              <div className="analytics-card-label" style={{ marginBottom: 16 }}>Inbox overview</div>
              <div className="inbox-overview-rows">
                <div className="inbox-overview-row">
                  <MessageCircle size={15} style={{ color: 'var(--accent)' }} />
                  <span>Comments</span>
                  <span className="inbox-overview-val">{dashStats.inboxStats.comments}</span>
                </div>
                <div className="inbox-overview-row">
                  <Mail size={15} style={{ color: '#f59e0b' }} />
                  <span>DMs</span>
                  <span className="inbox-overview-val">{dashStats.inboxStats.dms}</span>
                </div>
                <div className="inbox-overview-row">
                  <AlertCircle size={15} style={{ color: 'var(--danger)' }} />
                  <span>Unread</span>
                  <span className="inbox-overview-val">{dashStats.inboxStats.unread}</span>
                </div>
                <div className="inbox-overview-row">
                  <Heart size={15} style={{ color: '#e84040' }} />
                  <span>Liked by us</span>
                  <span className="inbox-overview-val">{dashStats.inboxStats.liked}</span>
                </div>
              </div>
            </div>
          </div>

          {/* Row 2: platform, post types, success rate */}
          <div className="analytics-row">
            {/* Platform breakdown */}
            <div className="analytics-card">
              <div className="analytics-card-label" style={{ marginBottom: 14 }}>Platform distribution</div>
              {Object.entries(dashStats.platformBreakdown).length === 0 ? (
                <div className="analytics-empty">No data yet</div>
              ) : (
                Object.entries(dashStats.platformBreakdown).map(([platform, count]) => (
                  <div key={platform} className="breakdown-row">
                    <div className="breakdown-row-top">
                      <span className={`breakdown-label platform-${platform}`}>{platform}</span>
                      <span className="breakdown-count">{count}</span>
                    </div>
                    <HorizontalBar
                      pct={(count / platformTotal) * 100}
                      color={platformColors[platform] || 'var(--accent)'}
                    />
                  </div>
                ))
              )}
            </div>

            {/* Post type breakdown */}
            <div className="analytics-card">
              <div className="analytics-card-label" style={{ marginBottom: 14 }}>Post types</div>
              {Object.entries(dashStats.postTypeBreakdown).length === 0 ? (
                <div className="analytics-empty">No data yet</div>
              ) : (
                Object.entries(dashStats.postTypeBreakdown).map(([type, count]) => (
                  <div key={type} className="breakdown-row">
                    <div className="breakdown-row-top">
                      <span className="breakdown-label">{type}</span>
                      <span className="breakdown-count">{count}</span>
                    </div>
                    <HorizontalBar
                      pct={(count / typeTotal) * 100}
                      color={typeColors[type] || 'var(--accent)'}
                    />
                  </div>
                ))
              )}
            </div>

            {/* Publishing success rate */}
            <div className="analytics-card">
              <div className="analytics-card-label" style={{ marginBottom: 14 }}>Publishing success</div>
              <div className="success-rate-circle">
                <svg viewBox="0 0 36 36" className="success-donut">
                  <circle cx="18" cy="18" r="15.9" fill="none"
                    stroke="var(--bg-tertiary)" strokeWidth="3.2" />
                  <circle cx="18" cy="18" r="15.9" fill="none"
                    stroke={dashStats.successRate >= 80 ? 'var(--success)' : dashStats.successRate >= 50 ? '#f59e0b' : 'var(--danger)'}
                    strokeWidth="3.2"
                    strokeDasharray={`${dashStats.successRate} ${100 - dashStats.successRate}`}
                    strokeDashoffset="25"
                    strokeLinecap="round"
                  />
                </svg>
                <div className="success-rate-label">
                  <span className="success-rate-pct">{Math.round(dashStats.successRate)}%</span>
                  <span className="success-rate-sub">success rate</span>
                </div>
              </div>
              <div className="success-breakdown">
                <span style={{ color: 'var(--success)' }}>
                  <CheckCircle size={12} /> {dashStats.statusBreakdown.published ?? 0} published
                </span>
                <span style={{ color: 'var(--danger)' }}>
                  <AlertCircle size={12} /> {dashStats.statusBreakdown.failed ?? 0} failed
                </span>
              </div>
            </div>
          </div>

          {/* Row 3: best hours + best days */}
          <div className="analytics-row">
            {/* Best hours */}
            <div className="analytics-card analytics-card-wide">
              <div className="analytics-card-header">
                <span className="analytics-card-label">Publishing hours (published posts)</span>
                {bestHour && bestHour.count > 0 && (
                  <span className="analytics-card-meta">
                    Peak: {bestHour.hour}:00–{bestHour.hour + 1}:00
                  </span>
                )}
              </div>
              <BarChart
                data={dashStats.hourDistribution.map(h => ({ value: h.count }))}
                color="#8b5cf6"
                height={56}
              />
              <div className="chart-axis-labels">
                {[0, 4, 8, 12, 16, 20, 23].map(h => (
                  <span key={h}>{h}h</span>
                ))}
              </div>
            </div>

            {/* Best days */}
            <div className="analytics-card">
              <div className="analytics-card-header">
                <span className="analytics-card-label">Day of week</span>
                {bestDay && bestDay.count > 0 && (
                  <span className="analytics-card-meta">Peak: {bestDay.day}</span>
                )}
              </div>
              <BarChart
                data={dashStats.weekdayDistribution.map(d => ({ value: d.count }))}
                color="#f59e0b"
                height={56}
              />
              <div className="chart-axis-labels">
                {dashStats.weekdayDistribution.map(d => (
                  <span key={d.day}>{d.day}</span>
                ))}
              </div>
            </div>
          </div>

          {/* Row 4: engagement from feed */}
          {feedLoading ? (
            <div className="analytics-row">
              <div className="analytics-card" style={{ alignItems: 'center', justifyContent: 'center', minHeight: 80 }}>
                <div className="spinner" style={{ width: 20, height: 20 }} />
                <span style={{ fontSize: 13, color: 'var(--text-muted)', marginLeft: 10 }}>
                  Loading engagement data…
                </span>
              </div>
            </div>
          ) : feedItems.length > 0 ? (
            <div className="analytics-row">
              {/* Engagement summary */}
              <div className="analytics-card">
                <div className="analytics-card-label" style={{ marginBottom: 14 }}>Live engagement</div>
                <div className="inbox-overview-rows">
                  <div className="inbox-overview-row">
                    <Heart size={15} style={{ color: '#e84040' }} />
                    <span>Total likes</span>
                    <span className="inbox-overview-val">{totalFeedLikes.toLocaleString()}</span>
                  </div>
                  <div className="inbox-overview-row">
                    <MessageCircle size={15} style={{ color: 'var(--accent)' }} />
                    <span>Total comments</span>
                    <span className="inbox-overview-val">{totalFeedComments.toLocaleString()}</span>
                  </div>
                  <div className="inbox-overview-row">
                    <TrendingUp size={15} style={{ color: 'var(--success)' }} />
                    <span>Avg per post</span>
                    <span className="inbox-overview-val">{avgEngagement}</span>
                  </div>
                  <div className="inbox-overview-row">
                    <BarChart2 size={15} style={{ color: '#f59e0b' }} />
                    <span>Posts tracked</span>
                    <span className="inbox-overview-val">{feedItems.length}</span>
                  </div>
                </div>
              </div>

              {/* Most popular */}
              {mostPopular && (
                <div className="analytics-card popular-post-card">
                  <div className="popular-post-badge most">
                    <Flame size={13} /> Most popular
                  </div>
                  <div className="popular-post-caption">
                    {mostPopular.caption
                      ? mostPopular.caption.slice(0, 100) + (mostPopular.caption.length > 100 ? '…' : '')
                      : <em style={{ color: 'var(--text-muted)' }}>No caption</em>}
                  </div>
                  <div className="popular-post-stats">
                    <span><Heart size={12} /> {mostPopular.likeCount.toLocaleString()} likes</span>
                    <span><MessageCircle size={12} /> {mostPopular.commentsCount.toLocaleString()} comments</span>
                  </div>
                  {mostPopular.permalink && (
                    <a
                      href={mostPopular.permalink}
                      target="_blank"
                      rel="noopener noreferrer"
                      className="popular-post-link"
                      onClick={e => e.stopPropagation()}
                    >
                      View post →
                    </a>
                  )}
                </div>
              )}

              {/* Least popular */}
              {leastPopular && leastPopular.id !== mostPopular?.id && (
                <div className="analytics-card popular-post-card">
                  <div className="popular-post-badge least">
                    <TrendingDown size={13} /> Least popular
                  </div>
                  <div className="popular-post-caption">
                    {leastPopular.caption
                      ? leastPopular.caption.slice(0, 100) + (leastPopular.caption.length > 100 ? '…' : '')
                      : <em style={{ color: 'var(--text-muted)' }}>No caption</em>}
                  </div>
                  <div className="popular-post-stats">
                    <span><Heart size={12} /> {leastPopular.likeCount.toLocaleString()} likes</span>
                    <span><MessageCircle size={12} /> {leastPopular.commentsCount.toLocaleString()} comments</span>
                  </div>
                  {leastPopular.permalink && (
                    <a
                      href={leastPopular.permalink}
                      target="_blank"
                      rel="noopener noreferrer"
                      className="popular-post-link"
                    >
                      View post →
                    </a>
                  )}
                </div>
              )}

              {/* Likes over time (last 10 feed posts) */}
              <div className="analytics-card">
                <div className="analytics-card-header">
                  <span className="analytics-card-label">Likes — recent posts</span>
                  <span className="analytics-card-meta">{feedItems.length} posts</span>
                </div>
                <BarChart
                  data={feedItems.slice(0, 20).map(f => ({ value: f.likeCount }))}
                  color="#e84040"
                  height={64}
                />
                <div className="chart-axis-labels">
                  <span>oldest</span>
                  <span>newest</span>
                </div>
              </div>
            </div>
          ) : null}
        </div>
      )}

      {/* ── AI Insights ── */}
      {user?.isAdmin && <div className="insights-section">
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
      </div>}

      {/* ── Navigation links ── */}
      <div className="admin-links">
        {user?.isAdmin ? (
          <>
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

            <Link to="/admin/settings" className="admin-link-card">
              <Settings size={24} />
              <div>
                <h3>Settings</h3>
                <p>App configuration</p>
              </div>
            </Link>
          </>
        ) : (
          <Link to="/team/manage" className="admin-link-card">
            <UsersRound size={24} />
            <div>
              <h3>Team Admin</h3>
              <p>Manage accounts &amp; members</p>
            </div>
          </Link>
        )}

        <Link to="/watermarks" className="admin-link-card">
          <Image size={24} />
          <div>
            <h3>Watermarks</h3>
            <p>Image editor gallery</p>
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
