import { useState, useEffect } from 'react';
import { api } from '../../services/api';
import type { Team, User, Watermark, TeamSettings } from '../../types';
import { Trash2, Plus, X, Key, Copy, RefreshCw, Settings } from 'lucide-react';
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
  const [savingBgg, setSavingBgg] = useState(false);

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
    try {
      const s = await api.getAdminTeamBggSettings(team.id);
      setBggSettings(s);
    } catch {
      setBggSettings({});
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
      };
      if (episodeNewsBearerToken) {
        data.episodeNewsBearerToken = episodeNewsBearerToken;
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

            <div className="modal-actions">
              <button className="btn btn-secondary" onClick={() => setBggTeam(null)}>Cancel</button>
              <button className="btn btn-primary" onClick={saveBggSettings} disabled={savingBgg}>
                {savingBgg ? 'Saving...' : 'Save Settings'}
              </button>
            </div>
          </div>
        </div>
      )}

      {/* Manage Members Modal */}
      {editTeam && (
        <div className="modal-overlay" onClick={() => setEditTeam(null)}>
          <div className="modal" onClick={e => e.stopPropagation()}>
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
          </div>
        </div>
      )}
    </div>
  );
}
