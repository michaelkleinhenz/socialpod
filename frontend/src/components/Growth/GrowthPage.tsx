import { useCallback, useEffect, useState } from 'react';
import { Link } from 'react-router-dom';
import { addWeeks, format, getISOWeek, parseISO, startOfWeek, subWeeks } from 'date-fns';
import {
  TrendingUp, RefreshCw, ChevronLeft, ChevronRight, AlertTriangle, CheckCircle2, Clock, Megaphone,
  Target, Settings, ExternalLink, Trophy,
} from 'lucide-react';
import toast from 'react-hot-toast';
import { api } from '../../services/api';
import { useAuth } from '../../contexts/AuthContext';
import type { GrowthOverview, PostPerformance } from '../../types';
import { FORMAT_LABELS, percent } from '../../services/instagramGrowth';
import { FollowerChart } from './FollowerChart';
import './Growth.css';

function PostRow({ p, rank }: { p: PostPerformance; rank?: number }) {
  const apiUrl = import.meta.env.VITE_API_URL || '';
  return (
    <div className="perf-row">
      {rank !== undefined && <span className="perf-rank">{rank}</span>}
      {p.imageUrl && !/\.(mp4|mov)$/i.test(p.imageUrl)
        ? <img src={p.imageUrl.startsWith('/') ? apiUrl + p.imageUrl : p.imageUrl} alt="" />
        : <span className="perf-thumb" />}
      <div className="perf-main">
        <div className="perf-caption">{p.caption || '(no caption)'}</div>
        <div className="perf-meta">
          {FORMAT_LABELS[p.format]} · {format(parseISO(p.postedAt), 'EEE d MMM, HH:mm')}
          {p.adCandidate && <span className="ad-flag" title="Top 20% of the last 30 days by sends + saves"><Megaphone size={11} /> Ad candidate</span>}
          {p.permalink && <a href={p.permalink} target="_blank" rel="noreferrer" title="Open on Instagram"><ExternalLink size={11} /></a>}
        </div>
      </div>
      {p.hasInsights ? (
        <div className="perf-numbers">
          <span><strong>{p.reach}</strong> reach</span>
          <span><strong>{p.shares}</strong> sends <em>{percent(p.sendsPerReach)}</em></span>
          <span><strong>{p.saved}</strong> saves <em>{percent(p.savesPerReach)}</em></span>
          <span><strong>{p.follows}</strong> follows</span>
        </div>
      ) : (
        <div className="perf-numbers muted" title={p.insightsError}>{p.insightsError ? 'Insights unavailable' : 'No numbers yet'}</div>
      )}
    </div>
  );
}

export function GrowthPage() {
  const { user } = useAuth();
  const canEdit = !!(user?.isTeamAdmin || user?.isAdmin);
  const [data, setData] = useState<GrowthOverview | null>(null);
  const [error, setError] = useState('');
  const [accountId, setAccountId] = useState<string | undefined>();
  // The Monday review looks at the week that just ended.
  const [weekStart, setWeekStart] = useState(() => subWeeks(startOfWeek(new Date(), { weekStartsOn: 1 }), 1));
  const [refreshing, setRefreshing] = useState(false);

  const load = useCallback(() => {
    api.getGrowthOverview({
      accountId,
      start: weekStart.toISOString(),
      end: addWeeks(weekStart, 1).toISOString(),
    }).then(d => { setData(d); setError(''); })
      .catch(err => setError(err.message || 'Failed to load growth data'));
  }, [accountId, weekStart]);

  useEffect(() => { load(); }, [load]);

  const refresh = async () => {
    setRefreshing(true);
    try {
      const res = await api.refreshGrowth();
      if (res.errors.length > 0) toast.error(res.errors[0], { duration: 8000 });
      else toast.success(`Updated followers and ${res.refreshed} posts`);
      load();
    } catch (err) {
      toast.error(err instanceof Error ? err.message : 'Refresh failed');
    } finally {
      setRefreshing(false);
    }
  };

  if (error) {
    return (
      <div className="page">
        <div className="page-header"><h1><TrendingUp size={24} /> Instagram Growth</h1></div>
        <div className="empty-state"><p>{error}</p></div>
      </div>
    );
  }
  if (!data) {
    return <div className="page"><div className="loading-screen"><div className="spinner" /></div></div>;
  }

  const goal = data.goal;
  const report = data.report;
  const thisWeek = startOfWeek(new Date(), { weekStartsOn: 1 });

  return (
    <div className="page growth-page">
      <div className="page-header">
        <h1><TrendingUp size={24} /> Instagram Growth</h1>
        <div style={{ display: 'flex', gap: 8, alignItems: 'center' }}>
          {data.accounts.length > 1 && (
            <select className="select" value={data.accountId} onChange={e => setAccountId(e.target.value)}>
              {data.accounts.map(a => <option key={a.id} value={a.id}>@{a.accountName}</option>)}
            </select>
          )}
          {canEdit && <Link className="btn btn-ghost btn-sm" to="/admin/growth"><Settings size={14} /> Settings</Link>}
          <button className="btn btn-secondary btn-sm" onClick={refresh} disabled={refreshing} title="Read today's follower count and the Insights of the last 30 days from Instagram">
            <RefreshCw size={14} className={refreshing ? 'post-editor-spin' : ''} /> {refreshing ? 'Refreshing…' : 'Refresh'}
          </button>
        </div>
      </div>

      {data.accounts.length === 0 && (
        <div className="card growth-notice">No Instagram account is connected to this team.</div>
      )}

      {/* ── Goal tracker ─────────────────────────────────────── */}
      <section className="growth-section">
        <h2><Target size={18} /> Follower goal</h2>
        {!goal ? (
          <div className="card growth-notice">
            No follower goal set yet.{' '}
            {canEdit ? <Link to="/admin/growth">Set one in the growth settings.</Link> : 'Ask a team admin to set one.'}
            {data.latestSnapshot && <> Current followers: <strong>{data.latestSnapshot.followersCount}</strong>.</>}
          </div>
        ) : (
          <>
            {goal.halfway === 'below' && (
              <div className="growth-alert danger">
                <AlertTriangle size={16} />
                <div>
                  <strong>Halfway review: below {percent(goal.goal.halfwayThreshold ?? 0.4, 0)} of the way</strong>
                  <span>
                    On {format(parseISO(goal.halfwayDate), 'd MMM')} the account had covered {percent(goal.halfwayProgress ?? 0, 0)} of the way.
                    The plan: put the ad budget on the best ad and add a second collab reel per week.
                  </span>
                </div>
              </div>
            )}
            {goal.halfway === 'ok' && (
              <div className="growth-alert ok">
                <CheckCircle2 size={16} />
                <div><strong>Halfway review passed</strong><span>{percent(goal.halfwayProgress ?? 0, 0)} of the way on {format(parseISO(goal.halfwayDate), 'd MMM')}.</span></div>
              </div>
            )}
            {goal.halfway === 'pending' && goal.halfwayDate && (
              <div className="growth-alert neutral">
                <Clock size={16} />
                <div>
                  <strong>Halfway review on {format(parseISO(goal.halfwayDate), 'EEE d MMM')}</strong>
                  <span>Needs {percent(goal.goal.halfwayThreshold ?? 0.4, 0)} of the way by then{goal.progress !== null ? ` — ${percent(goal.progress, 0)} so far` : ''}.</span>
                </div>
              </div>
            )}

            <div className="stat-tiles">
              <div className="stat-tile">
                <span className="stat-label">Followers now</span>
                <span className="stat-value">{goal.currentFollowers ?? '—'}</span>
                <span className="stat-sub">{goal.currentDate ? format(parseISO(goal.currentDate), 'd MMM') : 'no snapshot yet'}</span>
              </div>
              <div className="stat-tile">
                <span className="stat-label">Target</span>
                <span className="stat-value">{goal.goal.target}</span>
                <span className="stat-sub">by {format(parseISO(goal.goal.endDate), 'd MMM yyyy')}</span>
              </div>
              <div className="stat-tile">
                <span className="stat-label">Progress</span>
                <span className="stat-value">{goal.progress !== null ? percent(goal.progress, 0) : '—'}</span>
                <span className="stat-sub">from {goal.startFollowers ?? '—'} on {format(parseISO(goal.goal.startDate), 'd MMM')}</span>
              </div>
              <div className="stat-tile">
                <span className="stat-label">Needed per week</span>
                <span className="stat-value">{goal.startFollowers !== null ? Math.ceil(goal.requiredPerWeek) : '—'}</span>
                <span className="stat-sub">over {goal.totalWeeks} weeks</span>
              </div>
              <div className="stat-tile">
                <span className="stat-label">Plan for today</span>
                <span className="stat-value">{goal.expectedToday ?? '—'}</span>
                <span className="stat-sub">
                  {goal.expectedToday !== null && goal.currentFollowers !== null
                    ? `${goal.currentFollowers - goal.expectedToday >= 0 ? '+' : ''}${goal.currentFollowers - goal.expectedToday} vs. plan`
                    : ''}
                </span>
              </div>
            </div>

            {goal.series.length > 0 ? (
              <div className="card"><FollowerChart goal={goal} /></div>
            ) : (
              <div className="card growth-notice">Follower counts are recorded hourly from now on; the chart fills in from the first one.</div>
            )}

            {goal.weeks.length > 0 && (
              <div className="card goal-weeks">
                <table>
                  <thead><tr><th>Week</th><th>Net followers</th><th>Needed</th><th /></tr></thead>
                  <tbody>
                    {goal.weeks.map((w, i) => (
                      <tr key={w.start} className={w.current ? 'current' : ''}>
                        <td>{i + 1}. {format(parseISO(w.start), 'd MMM')} – {format(parseISO(w.end), 'd MMM')}</td>
                        <td>{w.net === null ? '—' : `${w.net >= 0 ? '+' : ''}${w.net}`}</td>
                        <td>{w.required}</td>
                        <td>
                          {w.net === null || w.current
                            ? (w.current ? <span className="muted">in progress</span> : null)
                            : w.net >= w.required
                              ? <span className="status ok"><CheckCircle2 size={13} /> on track</span>
                              : <span className="status behind"><AlertTriangle size={13} /> behind</span>}
                        </td>
                      </tr>
                    ))}
                  </tbody>
                </table>
              </div>
            )}
          </>
        )}
      </section>

      {/* ── Weekly report ────────────────────────────────────── */}
      <section className="growth-section">
        <div className="section-head">
          <h2><Trophy size={18} /> Weekly report</h2>
          <div className="week-picker">
            <button className="btn btn-ghost btn-sm" onClick={() => setWeekStart(subWeeks(weekStart, 1))}><ChevronLeft size={16} /></button>
            <span>Week {getISOWeek(weekStart)} · {format(weekStart, 'd MMM')} – {format(addWeeks(weekStart, 1).getTime() - 1, 'd MMM')}</span>
            <button className="btn btn-ghost btn-sm" onClick={() => setWeekStart(addWeeks(weekStart, 1))} disabled={weekStart >= thisWeek}><ChevronRight size={16} /></button>
          </div>
        </div>

        {report.posts.length === 0 ? (
          <div className="card growth-notice">No Instagram feed posts or reels went out this week{report.stories ? ` (${report.stories} stories)` : ''}.</div>
        ) : (
          <>
            <div className="stat-tiles">
              <div className="stat-tile">
                <span className="stat-label">Net followers</span>
                <span className="stat-value">{report.netFollowers !== undefined ? `${report.netFollowers >= 0 ? '+' : ''}${report.netFollowers}` : '—'}</span>
                <span className="stat-sub">{report.followersEnd !== undefined ? `${report.followersEnd} at week's end` : 'no snapshots for this week'}</span>
              </div>
              <div className="stat-tile">
                <span className="stat-label">Posts</span>
                <span className="stat-value">{report.posts.length}</span>
                <span className="stat-sub">{report.stories} stories{report.missingInsights ? ` · ${report.missingInsights} without numbers` : ''}</span>
              </div>
              <div className={`stat-tile${report.weakestFormat ? ' warn' : ''}`}>
                <span className="stat-label">Weakest format</span>
                <span className="stat-value small">{report.weakestFormat ? FORMAT_LABELS[report.weakestFormat] : '—'}</span>
                <span className="stat-sub">{report.weakestFormat ? 'lowest sends + saves per reach — cut it back' : 'needs two formats with numbers'}</span>
              </div>
            </div>

            <div className="card">
              <h3 className="card-title">Top 3 posts by sends + saves</h3>
              {report.topPosts.length === 0
                ? <p className="muted">No numbers for this week's posts yet.</p>
                : report.topPosts.map((p, i) => <PostRow key={p.postId} p={p} rank={i + 1} />)}
            </div>

            <div className="card format-table">
              <table>
                <thead>
                  <tr><th>Format</th><th>Posts</th><th>Avg. reach</th><th>Sends / reach</th><th>Saves / reach</th><th>Follows</th></tr>
                </thead>
                <tbody>
                  {report.formats.map(f => (
                    <tr key={f.format} className={f.format === report.weakestFormat ? 'weakest' : ''}>
                      <td>{FORMAT_LABELS[f.format]}</td>
                      <td>{f.posts}</td>
                      <td>{f.withInsights ? f.avgReach : '—'}</td>
                      <td>{f.withInsights ? percent(f.sendsPerReach, 2) : '—'}</td>
                      <td>{f.withInsights ? percent(f.savesPerReach, 2) : '—'}</td>
                      <td>{f.withInsights ? f.follows : '—'}</td>
                    </tr>
                  ))}
                </tbody>
              </table>
            </div>

            <details className="card all-posts">
              <summary>All {report.posts.length} posts of the week</summary>
              {report.posts.map(p => <PostRow key={p.postId} p={p} />)}
            </details>
          </>
        )}
      </section>

      {/* ── Ad candidates ────────────────────────────────────── */}
      <section className="growth-section">
        <h2><Megaphone size={18} /> Ad candidates</h2>
        <p className="section-intro">
          Reels and carousels of the last 30 days in the top 20% by sends plus saves — the plan promotes only what already works organically.
        </p>
        {data.adCandidates.length === 0
          ? <div className="card growth-notice">No candidates yet: they need reels or carousels with Insights.</div>
          : <div className="card">{data.adCandidates.map(p => <PostRow key={p.postId} p={p} />)}</div>}
      </section>
    </div>
  );
}
