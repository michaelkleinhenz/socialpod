import { useState, useEffect } from 'react';
import { api } from '../../services/api';
import type { Team, User } from '../../types';
import { Trash2, Plus, X, Key, Copy, RefreshCw } from 'lucide-react';
import { format } from 'date-fns';
import toast from 'react-hot-toast';
import './Admin.css';

export function TeamsPage() {
  const [teams, setTeams] = useState<Team[]>([]);
  const [users, setUsers] = useState<User[]>([]);
  const [showForm, setShowForm] = useState(false);
  const [teamName, setTeamName] = useState('');
  const [creating, setCreating] = useState(false);
  const [editTeam, setEditTeam] = useState<Team | null>(null);
  const [selectedUserIds, setSelectedUserIds] = useState<string[]>([]);
  const [savingMembers, setSavingMembers] = useState(false);

  const load = () => {
    Promise.all([api.getTeams(), api.getUsers()]).then(([t, u]) => {
      setTeams(t);
      setUsers(u);
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
