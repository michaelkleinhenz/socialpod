import { useState, useEffect } from 'react';
import { api } from '../../services/api';
import type { Team, User, Watermark, TeamSettings, TeamInvite } from '../../types';
import { AVAILABLE_PLUGINS } from '../../types';
import { Trash2, Plus, X, Key, Copy, RefreshCw, Settings, Puzzle, Mail, Clock } from 'lucide-react';
import { format } from 'date-fns';
import toast from 'react-hot-toast';
import './Admin.css';

export function TeamsPage() {
  const [teams, setTeams] = useState<Team[]>([]);
  const [users, setUsers] = useState<User[]>([]);
  const [watermarks, setWatermarks] = useState<Watermark[]>([]);
  const [showForm, setShowForm] = useState(false);
  const [teamName, setTeamName] = useState('');
  const [creating, setCreating] = useState(false);
  const [editTeam, setEditTeam] = useState<Team | null>(null);
  const [selectedUserIds, setSelectedUserIds] = useState<string[]>([]);
  const [savingMembers, setSavingMembers] = useState(false);
  const [bggTeam, setBggTeam] = useState<Team | null>(null);
  const [bggSettings, setBggSettings] = useState<TeamSettings>({});
  const [episodeNewsBearerToken, setEpisodeNewsBearerToken] = useState('');
  const [newsCreatorUrl, setNewsCreatorUrl] = useState('');
  const [newsCreatorWatermarkId, setNewsCreatorWatermarkId] = useState('');
  const [newsCreatorBearerToken, setNewsCreatorBearerToken] = useState('');
  const [episodeCreatorUrl, setEpisodeCreatorUrl] = useState('');
  const [episodeCreatorWatermarkId, setEpisodeCreatorWatermarkId] = useState('');
  const [episodeOverlayNewsId, setEpisodeOverlayNewsId] = useState('');
  const [episodeOverlayNewsOffsetX, setEpisodeOverlayNewsOffsetX] = useState(0);
  const [episodeOverlayNewsOffsetY, setEpisodeOverlayNewsOffsetY] = useState(0);
  const [episodeOverlayReviewId, setEpisodeOverlayReviewId] = useState('');
  const [episodeOverlayReviewOffsetX, setEpisodeOverlayReviewOffsetX] = useState(0);
  const [episodeOverlayReviewOffsetY, setEpisodeOverlayReviewOffsetY] = useState(0);
  const [episodeOverlaySpecialId, setEpisodeOverlaySpecialId] = useState('');
  const [episodeOverlaySpecialOffsetX, setEpisodeOverlaySpecialOffsetX] = useState(0);
  const [episodeOverlaySpecialOffsetY, setEpisodeOverlaySpecialOffsetY] = useState(0);
  const [episodeCreatorBearerToken, setEpisodeCreatorBearerToken] = useState('');
  const [savingBgg, setSavingBgg] = useState(false);

  const [pluginsTeam, setPluginsTeam] = useState<Team | null>(null);
  const [enabledPlugins, setEnabledPlugins] = useState<string[]>([]);
  const [savingPlugins, setSavingPlugins] = useState(false);

  const [invites, setInvites] = useState<TeamInvite[]>([]);
  const [showInviteForm, setShowInviteForm] = useState(false);
  const [inviteEmail, setInviteEmail] = useState('');
  const [sendingInvite, setSendingInvite] = useState(false);

  const load = () => {
    Promise.all([api.getTeams(), api.getUsers(), api.getWatermarks()]).then(([t, u, wm]) => {
      setTeams(t);
      setUsers(u);
      setWatermarks(wm as Watermark[]);
    }).catch(() => toast.error('Failed to load data'));
  };

  useEffect(() => { load(); }, []);

  const createTeam = async () => {
    if (!teamName.trim()) { toast.error('Name is required'); return; }
    setCreating(true);
    try {
      await api.createTeam({ name: teamName.trim() });
      setShowForm(false);
      setTeamName('');
      load();
      toast.success('Team created');
    } catch (err: any) {
      toast.error(err.message);
    } finally {
      setCreating(false);
    }
  };

  const deleteTeam = async (id: string) => {
    if (!confirm('Delete this team? Members will be unassigned.')) return;
    try {
      await api.deleteTeam(id);
      load();
      toast.success('Team deleted');
    } catch (err: any) {
      toast.error(err.message);
    }
  };

  const openMembers = (team: Team) => {
    setEditTeam(team);
    setSelectedUserIds(team.members.map(m => m.id));
    setInvites([]);
    setShowInviteForm(false);
    setInviteEmail('');
    api.getTeamInvites(team.id).then(setInvites).catch(() => {});
  };

  const saveMembers = async () => {
    if (!editTeam) return;
    setSavingMembers(true);
    try {
      await api.setTeamMembers(editTeam.id, selectedUserIds);
      setEditTeam(null);
      load();
      toast.success('Members updated');
    } catch (err: any) {
      toast.error(err.message);
    } finally {
      setSavingMembers(false);
    }
  };

  const toggleUser = (userId: string) => {
    setSelectedUserIds(prev =>
      prev.includes(userId) ? prev.filter(id => id !== userId) : [...prev, userId]
    );
  };

  const sendInvite = async () => {
    if (!editTeam || !inviteEmail) { toast.error('Email address is required'); return; }
    setSendingInvite(true);
    try {
      const result = await api.createTeamInvite(editTeam.id, inviteEmail);
      if (result.emailError) {
        toast.success('Invitation created, but email delivery failed: ' + result.emailError);
      } else {
        toast.success('Invitation sent');
      }
      setShowInviteForm(false);
      setInviteEmail('');
      api.getTeamInvites(editTeam.id).then(setInvites).catch(() => {});
    } catch (err: any) {
      toast.error(err.message);
    } finally {
      setSendingInvite(false);
    }
  };

  const deleteInvite = async (inviteId: string) => {
    if (!editTeam || !confirm('Delete this invitation?')) return;
    try {
      await api.deleteTeamInvite(editTeam.id, inviteId);
      setInvites(prev => prev.filter(i => i.id !== inviteId));
      toast.success('Invitation deleted');
    } catch (err: any) {
      toast.error(err.message);
    }
  };

  const generateToken = async (teamId: string) => {
    try {
      const res = await api.generateTeamToken(teamId);
      setTeams(prev => prev.map(t => t.id === teamId ? { ...t, apiToken: res.apiToken } : t));
      toast.success('API token generated');
    } catch (err: any) {
      toast.error(err.message);
    }
  };

  const copyToken = (token: string) => {
    navigator.clipboard.writeText(token);
    toast.success('Copied to clipboard');
  };

  const openBggSettings = async (team: Team) => {
    setBggTeam(team);
    setEpisodeNewsBearerToken('');
    setNewsCreatorBearerToken('');
    setEpisodeCreatorBearerToken('');
    try {
      const s = await api.getAdminTeamBggSettings(team.id);
      setBggSettings(s);
      setNewsCreatorUrl(s.newsCreatorUrl || '');
      setNewsCreatorWatermarkId(s.newsCreatorWatermarkId || '');
      setEpisodeCreatorUrl(s.episodeCreatorUrl || '');
      setEpisodeCreatorWatermarkId(s.episodeCreatorWatermarkId || '');
      setEpisodeOverlayNewsId(s.episodeOverlayNewsId || '');
      setEpisodeOverlayNewsOffsetX(s.episodeOverlayNewsOffsetX ?? 0);
      setEpisodeOverlayNewsOffsetY(s.episodeOverlayNewsOffsetY ?? 0);
      setEpisodeOverlayReviewId(s.episodeOverlayReviewId || '');
      setEpisodeOverlayReviewOffsetX(s.episodeOverlayReviewOffsetX ?? 0);
      setEpisodeOverlayReviewOffsetY(s.episodeOverlayReviewOffsetY ?? 0);
      setEpisodeOverlaySpecialId(s.episodeOverlaySpecialId || '');
      setEpisodeOverlaySpecialOffsetX(s.episodeOverlaySpecialOffsetX ?? 0);
      setEpisodeOverlaySpecialOffsetY(s.episodeOverlaySpecialOffsetY ?? 0);
    } catch {
      setBggSettings({});
      setNewsCreatorUrl('');
      setNewsCreatorWatermarkId('');
      setEpisodeCreatorUrl('');
      setEpisodeCreatorWatermarkId('');
      setEpisodeOverlayNewsId('');
      setEpisodeOverlayNewsOffsetX(0);
      setEpisodeOverlayNewsOffsetY(0);
      setEpisodeOverlayReviewId('');
      setEpisodeOverlayReviewOffsetX(0);
      setEpisodeOverlayReviewOffsetY(0);
      setEpisodeOverlaySpecialId('');
      setEpisodeOverlaySpecialOffsetX(0);
      setEpisodeOverlaySpecialOffsetY(0);
    }
  };

  const saveBggSettings = async () => {
    if (!bggTeam) return;
    setSavingBgg(true);
    try {
      const data: any = {
        bggWatermarkId: bggSettings.bggWatermarkId || null,
        bggCoverOffsetX: bggSettings.bggCoverOffsetX ?? 0,
        bggCoverOffsetY: bggSettings.bggCoverOffsetY ?? 0,
        episodeNewsUrl: bggSettings.episodeNewsUrl || '',
        bggHandleLookupEnabled: bggSettings.bggHandleLookupEnabled ?? true,
      };
      if (episodeNewsBearerToken) {
        data.episodeNewsBearerToken = episodeNewsBearerToken;
      }
      data.newsCreatorUrl = newsCreatorUrl || '';
      data.newsCreatorWatermarkId = newsCreatorWatermarkId || null;
      if (newsCreatorBearerToken) {
        data.newsCreatorBearerToken = newsCreatorBearerToken;
      }
      data.episodeCreatorUrl = episodeCreatorUrl || '';
      data.episodeCreatorWatermarkId = episodeCreatorWatermarkId || null;
      data.episodeOverlayNewsId = episodeOverlayNewsId || '';
      data.episodeOverlayNewsOffsetX = episodeOverlayNewsOffsetX;
      data.episodeOverlayNewsOffsetY = episodeOverlayNewsOffsetY;
      data.episodeOverlayReviewId = episodeOverlayReviewId || '';
      data.episodeOverlayReviewOffsetX = episodeOverlayReviewOffsetX;
      data.episodeOverlayReviewOffsetY = episodeOverlayReviewOffsetY;
      data.episodeOverlaySpecialId = episodeOverlaySpecialId || '';
      data.episodeOverlaySpecialOffsetX = episodeOverlaySpecialOffsetX;
      data.episodeOverlaySpecialOffsetY = episodeOverlaySpecialOffsetY;
      if (episodeCreatorBearerToken) {
        data.episodeCreatorBearerToken = episodeCreatorBearerToken;
      }
      await api.updateAdminTeamBggSettings(bggTeam.id, data);
      toast.success('Team settings saved');
      setBggTeam(null);
    } catch (err: any) {
      toast.error(err.message || 'Failed to save settings');
    } finally {
      setSavingBgg(false);
    }
  };

  const openPlugins = async (team: Team) => {
    setPluginsTeam(team);
    try {
      const res = await api.getTeamPlugins(team.id);
      setEnabledPlugins(res.enabledPlugins ?? []);
    } catch {
      setEnabledPlugins([]);
    }
  };

  const togglePlugin = (pluginId: string) => {
    setEnabledPlugins(prev =>
      prev.includes(pluginId) ? prev.filter(p => p !== pluginId) : [...prev, pluginId]
    );
  };

  const savePlugins = async () => {
    if (!pluginsTeam) return;
    setSavingPlugins(true);
    try {
      await api.updateTeamPlugins(pluginsTeam.id, enabledPlugins);
      toast.success('Plugins updated');
      setPluginsTeam(null);
    } catch (err: any) {
      toast.error(err.message || 'Failed to save plugins');
    } finally {
      setSavingPlugins(false);
    }
  };

  return (
    <div className="page">
      <div className="page-header">
        <h1>Teams</h1>
        <button className="btn btn-primary" onClick={() => setShowForm(true)}>
          <Plus size={16} /> New Team
        </button>
      </div>

      {teams.length === 0 ? (
        <div className="card" style={{ textAlign: 'center', padding: 40, color: 'var(--text-muted)' }}>
          No teams yet. Create a team to share calendars and API tokens between users.
        </div>
      ) : (
        <div style={{ display: 'flex', flexDirection: 'column', gap: 16 }}>
          {teams.map(team => (
            <div key={team.id} className="card">
              <div style={{ display: 'flex', justifyContent: 'space-between', alignItems: 'flex-start', marginBottom: 16 }}>
                <div>
                  <h3 style={{ margin: 0 }}>{team.name}</h3>
                  <span style={{ fontSize: 13, color: 'var(--text-muted)' }}>
                    Created {format(new Date(team.createdAt), 'MMM d, yyyy')}
                  </span>
                </div>
                <div style={{ display: 'flex', gap: 8 }}>
                  <button className="btn btn-secondary btn-sm" onClick={() => openMembers(team)}>
                    Manage Members ({team.members.length})
                  </button>
                  <button className="btn btn-secondary btn-sm" onClick={() => openBggSettings(team)}>
                    <Settings size={14} /> Team Settings
                  </button>
                  <button className="btn btn-secondary btn-sm" onClick={() => openPlugins(team)}>
                    <Puzzle size={14} /> Plugins
                  </button>
                  <button className="btn btn-ghost btn-sm" onClick={() => deleteTeam(team.id)}>
                    <Trash2 size={14} color="var(--danger)" />
                  </button>
                </div>
              </div>

              <div style={{ marginBottom: 12 }}>
                <div style={{ fontSize: 13, color: 'var(--text-secondary)', marginBottom: 6 }}>Members</div>
                {team.members.length === 0 ? (
                  <span style={{ fontSize: 13, color: 'var(--text-muted)' }}>No members assigned</span>
                ) : (
                  <div style={{ display: 'flex', flexWrap: 'wrap', gap: 6 }}>
                    {team.members.map(m => (
                      <span key={m.id} className="badge badge-scheduled">{m.name}</span>
                    ))}
                  </div>
                )}
              </div>

              <hr style={{ border: 'none', borderTop: '1px solid var(--border)', margin: '12px 0' }} />

              <div style={{ display: 'flex', alignItems: 'center', gap: 10 }}>
                <Key size={14} color="var(--text-muted)" />
                <span style={{ fontSize: 13, color: 'var(--text-secondary)' }}>Team API Token:</span>
                {team.apiToken ? (
                  <>
                    <code style={{ fontSize: 12, flex: 1, overflow: 'hidden', textOverflow: 'ellipsis', whiteSpace: 'nowrap' }}>
                      {team.apiToken}
                    </code>
                    <button className="btn btn-ghost btn-sm" onClick={() => copyToken(team.apiToken!)}>
                      <Copy size={14} />
                    </button>
                  </>
                ) : (
                  <span style={{ fontSize: 13, color: 'var(--text-muted)', flex: 1 }}>Not generated</span>
                )}
                <button className="btn btn-secondary btn-sm" onClick={() => generateToken(team.id)}>
                  <RefreshCw size={14} /> {team.apiToken ? 'Regenerate' : 'Generate'}
                </button>
              </div>
            </div>
          ))}
        </div>
      )}

      {/* Create Team Modal */}
      {showForm && (
        <div className="modal-overlay" onClick={() => setShowForm(false)}>
          <div className="modal" onClick={e => e.stopPropagation()}>
            <div style={{ display: 'flex', justifyContent: 'space-between', alignItems: 'center', marginBottom: 20 }}>
              <h2>New Team</h2>
              <button className="btn btn-ghost btn-sm" onClick={() => setShowForm(false)}>
                <X size={18} />
              </button>
            </div>

            <div className="form-group">
              <label>Team Name</label>
              <input
                className="input"
                placeholder="e.g. Marketing"
                value={teamName}
                onChange={e => setTeamName(e.target.value)}
                onKeyDown={e => e.key === 'Enter' && createTeam()}
              />
            </div>

            <div className="modal-actions">
              <button className="btn btn-secondary" onClick={() => setShowForm(false)}>Cancel</button>
              <button className="btn btn-primary" onClick={createTeam} disabled={creating}>
                {creating ? 'Creating...' : 'Create Team'}
              </button>
            </div>
          </div>
        </div>
      )}

      {/* Team Settings Modal */}
      {bggTeam && (
        <div className="modal-overlay" onClick={() => setBggTeam(null)}>
          <div className="modal" onClick={e => e.stopPropagation()}>
            <div style={{ display: 'flex', justifyContent: 'space-between', alignItems: 'center', marginBottom: 20 }}>
              <h2>Team Settings: {bggTeam.name}</h2>
              <button className="btn btn-ghost btn-sm" onClick={() => setBggTeam(null)}>
                <X size={18} />
              </button>
            </div>

            <h3 style={{ fontSize: 14, marginBottom: 8, marginTop: 0 }}>BoardGameGeek Integration</h3>
            <p style={{ color: 'var(--text-secondary)', fontSize: 13, marginBottom: 16, marginTop: 0 }}>
              When importing a game from BGG, the cover is scaled to fit within a 1080x1080 square. The optional overlay is applied on top at full frame.
            </p>

            <div className="form-group" style={{ marginBottom: 20 }}>
              <label>BGG Overlay</label>
              <select
                className="select"
                value={bggSettings.bggWatermarkId || ''}
                onChange={e => setBggSettings(s => ({ ...s, bggWatermarkId: e.target.value }))}
              >
                <option value="">None (no overlay)</option>
                {watermarks.map(wm => (
                  <option key={wm.id} value={wm.id}>{wm.name}</option>
                ))}
              </select>
              {watermarks.length === 0 && (
                <span style={{ fontSize: 12, color: 'var(--text-muted)', display: 'block', marginTop: 4 }}>
                  No watermarks uploaded yet. Add watermarks on the Watermarks page first.
                </span>
              )}
            </div>

            <div style={{ marginBottom: 20 }}>
              <label style={{ display: 'block', marginBottom: 6, fontWeight: 500, fontSize: 14 }}>Cover position offset</label>
              <p style={{ color: 'var(--text-secondary)', fontSize: 12, marginBottom: 10, marginTop: 0 }}>
                Shift the scaled cover within the canvas. Use a negative X offset to push the cover away from a left sidebar overlay, for example.
              </p>
              <div style={{ display: 'flex', gap: 16 }}>
                <div className="form-group" style={{ flex: 1, marginBottom: 0 }}>
                  <label style={{ fontSize: 13 }}>Horizontal offset (px)</label>
                  <input
                    type="number"
                    className="input"
                    value={bggSettings.bggCoverOffsetX ?? 0}
                    onChange={e => setBggSettings(s => ({ ...s, bggCoverOffsetX: parseInt(e.target.value, 10) || 0 }))}
                    placeholder="0"
                  />
                </div>
                <div className="form-group" style={{ flex: 1, marginBottom: 0 }}>
                  <label style={{ fontSize: 13 }}>Vertical offset (px)</label>
                  <input
                    type="number"
                    className="input"
                    value={bggSettings.bggCoverOffsetY ?? 0}
                    onChange={e => setBggSettings(s => ({ ...s, bggCoverOffsetY: parseInt(e.target.value, 10) || 0 }))}
                    placeholder="0"
                  />
                </div>
              </div>
            </div>

            <hr style={{ border: 'none', borderTop: '1px solid var(--border)', margin: '24px 0' }} />

            <div className="form-group" style={{ marginBottom: 20 }}>
              <label style={{ display: 'flex', alignItems: 'center', gap: 10, cursor: 'pointer' }}>
                <input
                  type="checkbox"
                  checked={bggSettings.bggHandleLookupEnabled ?? true}
                  onChange={e => setBggSettings(s => ({ ...s, bggHandleLookupEnabled: e.target.checked }))}
                />
                <span>Enable AI handle lookup for BGG imports</span>
              </label>
              <p style={{ fontSize: 12, color: 'var(--text-muted)', margin: '4px 0 0 22px' }}>
                When enabled and an OpenRouter API key is configured, publisher and designer handles are
                automatically resolved during BGG imports. Requires a catalog entry or AI lookup per
                platform (Instagram, Bluesky, etc.).
              </p>
            </div>

            <hr style={{ border: 'none', borderTop: '1px solid var(--border)', margin: '24px 0' }} />

            <h3 style={{ fontSize: 14, marginBottom: 8, marginTop: 0 }}>Episode News Integration</h3>
            <p style={{ color: 'var(--text-secondary)', fontSize: 13, marginBottom: 16, marginTop: 0 }}>
              When configured, an "Add News" option appears in the post editor. Submitting a post with news enabled sends episode details to this URL.
            </p>

            <div className="form-group" style={{ marginBottom: 16 }}>
              <label>News Endpoint URL</label>
              <input
                type="url"
                className="input"
                value={bggSettings.episodeNewsUrl || ''}
                onChange={e => setBggSettings(s => ({ ...s, episodeNewsUrl: e.target.value }))}
                placeholder="https://example.com/api/episode-news"
              />
            </div>
            <div className="form-group" style={{ marginBottom: 20 }}>
              <label>Bearer Token</label>
              <input
                type="password"
                className="input"
                value={episodeNewsBearerToken}
                onChange={e => setEpisodeNewsBearerToken(e.target.value)}
                placeholder={bggSettings.hasEpisodeNewsBearerToken ? '••••••••  (unchanged — enter new value to replace)' : 'Enter bearer token'}
              />
              {bggSettings.hasEpisodeNewsBearerToken && !episodeNewsBearerToken && (
                <span style={{ fontSize: 12, color: 'var(--text-muted)', display: 'block', marginTop: 4 }}>
                  A bearer token is already configured. Enter a new value to replace it.
                </span>
              )}
            </div>

            <hr style={{ border: 'none', borderTop: '1px solid var(--border)', margin: '24px 0' }} />

            <h3 style={{ fontSize: 14, marginBottom: 8, marginTop: 0 }}>News Creator Integration</h3>
            <p style={{ color: 'var(--text-secondary)', fontSize: 13, marginBottom: 16, marginTop: 0 }}>
              Configure the n8n webhook URL and bearer token for the News Creator plugin.
            </p>

            <div className="form-group" style={{ marginBottom: 16 }}>
              <label>News Creator URL</label>
              <input
                type="url"
                className="input"
                value={newsCreatorUrl}
                onChange={e => setNewsCreatorUrl(e.target.value)}
                placeholder="https://n8n.example.com/webhook/..."
              />
            </div>
            <div className="form-group" style={{ marginBottom: 16 }}>
              <label>News Watermark</label>
              <select
                className="select"
                value={newsCreatorWatermarkId}
                onChange={e => setNewsCreatorWatermarkId(e.target.value)}
              >
                <option value="">None (no overlay)</option>
                {watermarks.map(wm => (
                  <option key={wm.id} value={wm.id}>{wm.name}</option>
                ))}
              </select>
              <span style={{ fontSize: 12, color: 'var(--text-muted)', display: 'block', marginTop: 4 }}>
                Watermark to overlay on cropped news images.
              </span>
            </div>
            <div className="form-group" style={{ marginBottom: 20 }}>
              <label>Bearer Token</label>
              <input
                type="password"
                className="input"
                value={newsCreatorBearerToken}
                onChange={e => setNewsCreatorBearerToken(e.target.value)}
                placeholder={bggSettings.hasNewsCreatorBearerToken ? '••••••••  (unchanged — enter new value to replace)' : 'Enter bearer token'}
              />
              {bggSettings.hasNewsCreatorBearerToken && !newsCreatorBearerToken && (
                <span style={{ fontSize: 12, color: 'var(--text-muted)', display: 'block', marginTop: 4 }}>
                  A bearer token is already configured. Enter a new value to replace it.
                </span>
              )}
            </div>

            <hr style={{ border: 'none', borderTop: '1px solid var(--border)', margin: '24px 0' }} />

            <h3 style={{ fontSize: 14, marginBottom: 8, marginTop: 0 }}>Episode Creator Integration</h3>
            <p style={{ color: 'var(--text-secondary)', fontSize: 13, marginBottom: 16, marginTop: 0 }}>
              Configure the webhook URL and bearer token for the Episode Creator plugin.
            </p>

            <div className="form-group" style={{ marginBottom: 16 }}>
              <label>Episode Creator URL</label>
              <input
                type="url"
                className="input"
                value={episodeCreatorUrl}
                onChange={e => setEpisodeCreatorUrl(e.target.value)}
                placeholder="https://n8n.example.com/webhook/..."
              />
            </div>
            <div className="form-group" style={{ marginBottom: 16 }}>
              <label>Default Episode Watermark</label>
              <select
                className="select"
                value={episodeCreatorWatermarkId}
                onChange={e => setEpisodeCreatorWatermarkId(e.target.value)}
              >
                <option value="">None (no overlay)</option>
                {watermarks.map(wm => (
                  <option key={wm.id} value={wm.id}>{wm.name}</option>
                ))}
              </select>
              <span style={{ fontSize: 12, color: 'var(--text-muted)', display: 'block', marginTop: 4 }}>
                Fallback watermark when no type-specific overlay is configured.
              </span>
            </div>

            <div style={{ padding: 12, background: 'var(--bg-secondary, #1e293b)', borderRadius: 8, border: '1px solid var(--border)', display: 'flex', flexDirection: 'column', gap: 12, marginBottom: 16 }}>
              <span style={{ fontSize: 13, fontWeight: 500 }}>Per-Type Overlays</span>
              <span style={{ fontSize: 12, color: 'var(--text-muted)', marginTop: -8 }}>
                Optionally set a different overlay image for each episode type. Falls back to the default above if not set.
              </span>
              {[
                { label: 'News Overlay', value: episodeOverlayNewsId, setter: setEpisodeOverlayNewsId, offsetX: episodeOverlayNewsOffsetX, setOffsetX: setEpisodeOverlayNewsOffsetX, offsetY: episodeOverlayNewsOffsetY, setOffsetY: setEpisodeOverlayNewsOffsetY },
                { label: 'Review Overlay', value: episodeOverlayReviewId, setter: setEpisodeOverlayReviewId, offsetX: episodeOverlayReviewOffsetX, setOffsetX: setEpisodeOverlayReviewOffsetX, offsetY: episodeOverlayReviewOffsetY, setOffsetY: setEpisodeOverlayReviewOffsetY },
                { label: 'Special Overlay', value: episodeOverlaySpecialId, setter: setEpisodeOverlaySpecialId, offsetX: episodeOverlaySpecialOffsetX, setOffsetX: setEpisodeOverlaySpecialOffsetX, offsetY: episodeOverlaySpecialOffsetY, setOffsetY: setEpisodeOverlaySpecialOffsetY },
              ].map(item => (
                <div key={item.label} style={{ marginBottom: 0 }}>
                  <div className="form-group" style={{ marginBottom: 8 }}>
                    <label style={{ fontSize: 13 }}>{item.label}</label>
                    <select
                      className="select"
                      value={item.value}
                      onChange={e => item.setter(e.target.value)}
                    >
                      <option value="">Use default</option>
                      {watermarks.map(wm => (
                        <option key={wm.id} value={wm.id}>{wm.name}</option>
                      ))}
                    </select>
                  </div>
                  {item.value && (
                    <div style={{ display: 'flex', gap: 12, marginBottom: 4 }}>
                      <div className="form-group" style={{ flex: 1, marginBottom: 0 }}>
                        <label style={{ fontSize: 12 }}>Offset X (px)</label>
                        <input
                          type="number"
                          className="input"
                          value={item.offsetX}
                          onChange={e => item.setOffsetX(parseInt(e.target.value, 10) || 0)}
                          placeholder="0"
                        />
                      </div>
                      <div className="form-group" style={{ flex: 1, marginBottom: 0 }}>
                        <label style={{ fontSize: 12 }}>Offset Y (px)</label>
                        <input
                          type="number"
                          className="input"
                          value={item.offsetY}
                          onChange={e => item.setOffsetY(parseInt(e.target.value, 10) || 0)}
                          placeholder="0"
                        />
                      </div>
                    </div>
                  )}
                </div>
              ))}
            </div>
            <div className="form-group" style={{ marginBottom: 20 }}>
              <label>Bearer Token</label>
              <input
                type="password"
                className="input"
                value={episodeCreatorBearerToken}
                onChange={e => setEpisodeCreatorBearerToken(e.target.value)}
                placeholder={bggSettings.hasEpisodeCreatorBearerToken ? '••••••••  (unchanged — enter new value to replace)' : 'Enter bearer token'}
              />
              {bggSettings.hasEpisodeCreatorBearerToken && !episodeCreatorBearerToken && (
                <span style={{ fontSize: 12, color: 'var(--text-muted)', display: 'block', marginTop: 4 }}>
                  A bearer token is already configured. Enter a new value to replace it.
                </span>
              )}
            </div>

            <div className="modal-actions">
              <button className="btn btn-secondary" onClick={() => setBggTeam(null)}>Cancel</button>
              <button className="btn btn-primary" onClick={saveBggSettings} disabled={savingBgg}>
                {savingBgg ? 'Saving...' : 'Save Settings'}
              </button>
            </div>
          </div>
        </div>
      )}

      {/* Plugins Modal */}
      {pluginsTeam && (
        <div className="modal-overlay" onClick={() => setPluginsTeam(null)}>
          <div className="modal" onClick={e => e.stopPropagation()}>
            <div style={{ display: 'flex', justifyContent: 'space-between', alignItems: 'center', marginBottom: 20 }}>
              <h2>Plugins: {pluginsTeam.name}</h2>
              <button className="btn btn-ghost btn-sm" onClick={() => setPluginsTeam(null)}>
                <X size={18} />
              </button>
            </div>

            <p style={{ color: 'var(--text-secondary)', fontSize: 13, marginBottom: 20, marginTop: 0 }}>
              Enable or disable optional plugins for this team. Disabled plugins are hidden from all team members.
            </p>

            <div style={{ display: 'flex', flexDirection: 'column', gap: 12 }}>
              {AVAILABLE_PLUGINS.map(plugin => (
                <label
                  key={plugin.id}
                  style={{
                    display: 'flex',
                    alignItems: 'flex-start',
                    gap: 12,
                    padding: '12px 14px',
                    borderRadius: 'var(--radius-sm)',
                    background: enabledPlugins.includes(plugin.id) ? 'var(--accent-muted)' : 'var(--bg-primary)',
                    border: '1px solid var(--border)',
                    cursor: 'pointer',
                  }}
                >
                  <input
                    type="checkbox"
                    checked={enabledPlugins.includes(plugin.id)}
                    onChange={() => togglePlugin(plugin.id)}
                    style={{ marginTop: 2, flexShrink: 0 }}
                  />
                  <div>
                    <div style={{ fontWeight: 500, fontSize: 14 }}>{plugin.label}</div>
                    <div style={{ fontSize: 12, color: 'var(--text-muted)', marginTop: 2 }}>{plugin.description}</div>
                  </div>
                </label>
              ))}
            </div>

            <div className="modal-actions">
              <button className="btn btn-secondary" onClick={() => setPluginsTeam(null)}>Cancel</button>
              <button className="btn btn-primary" onClick={savePlugins} disabled={savingPlugins}>
                {savingPlugins ? 'Saving...' : 'Save Plugins'}
              </button>
            </div>
          </div>
        </div>
      )}

      {/* Manage Members Modal */}
      {editTeam && (
        <div className="modal-overlay" onClick={() => setEditTeam(null)}>
          <div className="modal" onClick={e => e.stopPropagation()} style={{ maxWidth: 560 }}>
            <div style={{ display: 'flex', justifyContent: 'space-between', alignItems: 'center', marginBottom: 20 }}>
              <h2>Members: {editTeam.name}</h2>
              <button className="btn btn-ghost btn-sm" onClick={() => setEditTeam(null)}>
                <X size={18} />
              </button>
            </div>

            <p style={{ color: 'var(--text-secondary)', fontSize: 14, marginBottom: 16 }}>
              Select users to add to this team. Team members share a calendar and API token.
            </p>

            <div style={{ display: 'flex', flexDirection: 'column', gap: 8, maxHeight: 300, overflow: 'auto' }}>
              {users.map(u => (
                <label
                  key={u.id}
                  style={{
                    display: 'flex',
                    alignItems: 'center',
                    gap: 10,
                    padding: '8px 12px',
                    borderRadius: 'var(--radius-sm)',
                    background: selectedUserIds.includes(u.id) ? 'var(--accent-muted)' : 'var(--bg-primary)',
                    border: '1px solid var(--border)',
                    cursor: 'pointer',
                  }}
                >
                  <input
                    type="checkbox"
                    checked={selectedUserIds.includes(u.id)}
                    onChange={() => toggleUser(u.id)}
                  />
                  <div>
                    <div style={{ fontWeight: 500 }}>{u.name}</div>
                    <div style={{ fontSize: 12, color: 'var(--text-muted)' }}>{u.email}</div>
                  </div>
                </label>
              ))}
            </div>

            <div className="modal-actions">
              <button className="btn btn-secondary" onClick={() => setEditTeam(null)}>Cancel</button>
              <button className="btn btn-primary" onClick={saveMembers} disabled={savingMembers}>
                {savingMembers ? 'Saving...' : 'Save Members'}
              </button>
            </div>

            <hr style={{ border: 'none', borderTop: '1px solid var(--border)', margin: '24px 0' }} />

            <div style={{ display: 'flex', justifyContent: 'space-between', alignItems: 'center', marginBottom: 12 }}>
              <h3 style={{ margin: 0, fontSize: 15 }}>Invite by Email</h3>
              {!showInviteForm && (
                <button className="btn btn-secondary btn-sm" onClick={() => setShowInviteForm(true)}>
                  <Mail size={14} /> Invite
                </button>
              )}
            </div>

            <p style={{ color: 'var(--text-secondary)', fontSize: 13, margin: '0 0 12px' }}>
              Send an invitation email to someone who doesn't have an account yet.
            </p>

            {showInviteForm && (
              <div style={{ display: 'flex', gap: 8, marginBottom: 16 }}>
                <input
                  className="input"
                  type="email"
                  placeholder="newuser@example.com"
                  value={inviteEmail}
                  onChange={e => setInviteEmail(e.target.value)}
                  onKeyDown={e => e.key === 'Enter' && sendInvite()}
                  style={{ flex: 1 }}
                />
                <button className="btn btn-primary btn-sm" onClick={sendInvite} disabled={sendingInvite}>
                  {sendingInvite ? 'Sending...' : 'Send'}
                </button>
                <button className="btn btn-secondary btn-sm" onClick={() => { setShowInviteForm(false); setInviteEmail(''); }}>
                  Cancel
                </button>
              </div>
            )}

            {invites.length > 0 && (
              <div style={{ display: 'flex', flexDirection: 'column', gap: 8 }}>
                {invites.map(invite => (
                  <div
                    key={invite.id}
                    style={{
                      display: 'flex',
                      alignItems: 'center',
                      justifyContent: 'space-between',
                      padding: '8px 12px',
                      borderRadius: 'var(--radius-sm)',
                      border: '1px solid var(--border)',
                      background: 'var(--bg-primary)',
                    }}
                  >
                    <div>
                      <div style={{ fontWeight: 500, fontSize: 14 }}>{invite.email}</div>
                      <div style={{ fontSize: 12, color: 'var(--text-muted)' }}>
                        Expires {format(new Date(invite.expiresAt), 'MMM d, yyyy')}
                      </div>
                    </div>
                    <div style={{ display: 'flex', alignItems: 'center', gap: 8 }}>
                      {invite.status === 'pending' && new Date(invite.expiresAt) > new Date() ? (
                        <span className="badge badge-scheduled"><Clock size={12} style={{ marginRight: 4 }} />Pending</span>
                      ) : invite.status === 'accepted' ? (
                        <span className="badge badge-published">Accepted</span>
                      ) : (
                        <span className="badge badge-failed">Expired</span>
                      )}
                      <button className="btn btn-ghost btn-sm" onClick={() => deleteInvite(invite.id)}>
                        <Trash2 size={14} color="var(--danger)" />
                      </button>
                    </div>
                  </div>
                ))}
              </div>
            )}
          </div>
        </div>
      )}
    </div>
  );
}
