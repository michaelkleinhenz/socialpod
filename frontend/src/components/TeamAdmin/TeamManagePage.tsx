import { useState, useEffect } from 'react';
import { useSearchParams } from 'react-router-dom';
import { api } from '../../services/api';
import type { SocialAccount, User } from '../../types';
import {
  Plus, Trash2, ToggleLeft, ToggleRight, ExternalLink, X, Users, Share2,
} from 'lucide-react';
import { PlatformIcon } from '../Common/PlatformIcon';
import { format } from 'date-fns';
import toast from 'react-hot-toast';
import { useAuth } from '../../contexts/AuthContext';
import '../Admin/Admin.css';

type Tab = 'accounts' | 'members';

export function TeamManagePage() {
  const [searchParams] = useSearchParams();
  const { user, refreshUser } = useAuth();
  const [tab, setTab] = useState<Tab>('accounts');

  // Accounts state
  const [accounts, setAccounts] = useState<SocialAccount[]>([]);
  const [showBlueskyForm, setShowBlueskyForm] = useState(false);
  const [handle, setHandle] = useState('');
  const [appPassword, setAppPassword] = useState('');
  const [pdsHost, setPdsHost] = useState('');
  const [addingAccount, setAddingAccount] = useState(false);

  // Members state
  const [members, setMembers] = useState<User[]>([]);
  const [showMemberForm, setShowMemberForm] = useState(false);
  const [memberEmail, setMemberEmail] = useState('');
  const [addingMember, setAddingMember] = useState(false);

  // Team self-creation state (when team admin has no team yet)
  const [newTeamName, setNewTeamName] = useState('');
  const [creatingTeam, setCreatingTeam] = useState(false);

  useEffect(() => {
    if (!user?.teamId) return;
    loadAccounts();
    loadMembers();
    if (searchParams.get('instagram') === 'connected') {
      toast.success('Instagram account connected');
    }
  }, [user?.teamId]);

  const loadAccounts = async () => {
    try {
      setAccounts(await api.getTeamAccounts());
    } catch {
      toast.error('Failed to load accounts');
    }
  };

  const loadMembers = async () => {
    try {
      setMembers(await api.getTeamMembers());
    } catch {
      toast.error('Failed to load members');
    }
  };

  const addBluesky = async () => {
    if (!handle || !appPassword) { toast.error('Handle and app password required'); return; }
    setAddingAccount(true);
    try {
      await api.addTeamBlueskyAccount({ handle, appPassword, pdsHost: pdsHost || undefined });
      toast.success('Bluesky account added');
      setShowBlueskyForm(false);
      setHandle(''); setAppPassword(''); setPdsHost('');
      loadAccounts();
    } catch (err: any) {
      toast.error(err.message);
    } finally {
      setAddingAccount(false);
    }
  };

  const connectInstagram = async () => {
    try {
      const { url } = await api.getTeamInstagramAuthUrl();
      window.location.href = url;
    } catch (err: any) {
      toast.error(err.message);
    }
  };

  const toggleAccount = async (id: string) => {
    try {
      const updated = await api.toggleTeamAccount(id);
      setAccounts(prev => prev.map(a => a.id === id ? updated : a));
    } catch {
      toast.error('Failed to toggle account');
    }
  };

  const deleteAccount = async (id: string) => {
    if (!confirm('Delete this account?')) return;
    try {
      await api.deleteTeamAccount(id);
      setAccounts(prev => prev.filter(a => a.id !== id));
      toast.success('Account deleted');
    } catch {
      toast.error('Failed to delete account');
    }
  };

  const addMember = async () => {
    if (!memberEmail) {
      toast.error('Email address is required');
      return;
    }
    setAddingMember(true);
    try {
      await api.addTeamMember({ email: memberEmail });
      toast.success('Member added');
      setShowMemberForm(false);
      setMemberEmail('');
      loadMembers();
    } catch (err: any) {
      toast.error(err.message);
    } finally {
      setAddingMember(false);
    }
  };

  const removeMember = async (id: string) => {
    if (!confirm('Remove this member from the team?')) return;
    try {
      await api.removeTeamMember(id);
      setMembers(prev => prev.filter(m => m.id !== id));
      toast.success('Member removed');
    } catch (err: any) {
      toast.error(err.message);
    }
  };

  const createTeam = async () => {
    if (!newTeamName.trim()) { toast.error('Team name is required'); return; }
    setCreatingTeam(true);
    try {
      await api.createOwnTeam({ name: newTeamName.trim() });
      toast.success('Team created');
      await refreshUser();
    } catch (err: any) {
      toast.error(err.message || 'Failed to create team');
    } finally {
      setCreatingTeam(false);
    }
  };

  if (!user?.teamId) {
    return (
      <div className="page">
        <div className="page-header">
          <h1>Team Management</h1>
        </div>
        <div style={{ maxWidth: 480 }}>
          <div className="card" style={{ padding: 24 }}>
            <h2 style={{ marginBottom: 8 }}>Create Your Team</h2>
            <p style={{ color: 'var(--text-secondary)', marginBottom: 20, fontSize: 14 }}>
              You don't have a team yet. Create one to start managing members and social accounts.
            </p>
            <div className="form-group">
              <label>Team Name</label>
              <input
                className="input"
                placeholder="My Team"
                value={newTeamName}
                onChange={e => setNewTeamName(e.target.value)}
                onKeyDown={e => e.key === 'Enter' && createTeam()}
              />
            </div>
            <button
              className="btn btn-primary"
              onClick={createTeam}
              disabled={creatingTeam}
              style={{ marginTop: 16 }}
            >
              {creatingTeam ? 'Creating...' : 'Create Team'}
            </button>
          </div>
        </div>
      </div>
    );
  }

  return (
    <div className="page">
      <div className="page-header">
        <h1>Team Management</h1>
      </div>

      {/* Tabs */}
      <div style={{ display: 'flex', gap: 8, marginBottom: 24, borderBottom: '1px solid var(--border)', paddingBottom: 0 }}>
        {(['accounts', 'members'] as Tab[]).map(t => (
          <button
            key={t}
            onClick={() => setTab(t)}
            style={{
              padding: '8px 20px',
              background: 'none',
              border: 'none',
              borderBottom: tab === t ? '2px solid var(--accent)' : '2px solid transparent',
              color: tab === t ? 'var(--accent)' : 'var(--text-secondary)',
              fontWeight: tab === t ? 600 : 400,
              cursor: 'pointer',
              fontSize: 14,
              display: 'flex',
              alignItems: 'center',
              gap: 6,
            }}
          >
            {t === 'accounts' ? <Share2 size={15} /> : <Users size={15} />}
            {t === 'accounts' ? 'Social Accounts' : 'Members'}
          </button>
        ))}
      </div>

      {/* Accounts Tab */}
      {tab === 'accounts' && (
        <>
          <div className="page-header" style={{ marginBottom: 16 }}>
            <div />
            <div style={{ display: 'flex', gap: 10 }}>
              <button className="btn btn-primary" onClick={() => setShowBlueskyForm(true)}>
                <Plus size={16} /> Add Bluesky
              </button>
              <button
                className="btn btn-secondary"
                onClick={connectInstagram}
                style={{ borderColor: 'var(--instagram)', color: 'var(--instagram)' }}
              >
                <ExternalLink size={16} /> Connect Instagram
              </button>
            </div>
          </div>

          {accounts.length === 0 ? (
            <div className="empty-state">
              <p>No social accounts connected to your team yet.</p>
              <p style={{ color: 'var(--text-muted)', fontSize: 14 }}>
                Add a Bluesky or Instagram account to start scheduling posts.
              </p>
            </div>
          ) : (
            <div className="accounts-grid">
              {accounts.map(account => (
                <div key={account.id} className={`account-card ${!account.isActive ? 'inactive' : ''}`}>
                  <div className="account-header">
                    <PlatformIcon platform={account.platform} size={12} />
                    <span className="account-platform">{account.platform}</span>
                    <div style={{ marginLeft: 'auto', display: 'flex', gap: 8 }}>
                      <button
                        className="btn btn-ghost btn-sm"
                        onClick={() => toggleAccount(account.id)}
                        title={account.isActive ? 'Disable' : 'Enable'}
                      >
                        {account.isActive
                          ? <ToggleRight size={18} color="var(--success)" />
                          : <ToggleLeft size={18} />}
                      </button>
                      <button className="btn btn-ghost btn-sm" onClick={() => deleteAccount(account.id)}>
                        <Trash2 size={16} color="var(--danger)" />
                      </button>
                    </div>
                  </div>
                  <div className="account-name">{account.displayName || account.accountName}</div>
                  <div className="account-handle">@{account.accountName}</div>
                  {!account.isActive && <span className="badge badge-draft">Disabled</span>}
                </div>
              ))}
            </div>
          )}

          {showBlueskyForm && (
            <div className="modal-overlay" onClick={() => setShowBlueskyForm(false)}>
              <div className="modal" onClick={e => e.stopPropagation()}>
                <div style={{ display: 'flex', justifyContent: 'space-between', alignItems: 'center', marginBottom: 20 }}>
                  <h2>Add Bluesky Account</h2>
                  <button className="btn btn-ghost btn-sm" onClick={() => setShowBlueskyForm(false)}>
                    <X size={18} />
                  </button>
                </div>
                <div style={{ display: 'flex', flexDirection: 'column', gap: 16 }}>
                  <div className="form-group">
                    <label>Handle</label>
                    <input className="input" placeholder="user.bsky.social" value={handle} onChange={e => setHandle(e.target.value)} />
                  </div>
                  <div className="form-group">
                    <label>App Password</label>
                    <input className="input" type="password" placeholder="xxxx-xxxx-xxxx-xxxx" value={appPassword} onChange={e => setAppPassword(e.target.value)} />
                    <span style={{ fontSize: 12, color: 'var(--text-muted)' }}>
                      Generate at Settings &gt; App Passwords on bsky.app
                    </span>
                  </div>
                  <div className="form-group">
                    <label>PDS Host (optional)</label>
                    <input className="input" placeholder="https://bsky.social" value={pdsHost} onChange={e => setPdsHost(e.target.value)} />
                  </div>
                </div>
                <div className="modal-actions">
                  <button className="btn btn-secondary" onClick={() => setShowBlueskyForm(false)}>Cancel</button>
                  <button className="btn btn-primary" onClick={addBluesky} disabled={addingAccount}>
                    {addingAccount ? 'Adding...' : 'Add Account'}
                  </button>
                </div>
              </div>
            </div>
          )}
        </>
      )}

      {/* Members Tab */}
      {tab === 'members' && (
        <>
          <div className="page-header" style={{ marginBottom: 16 }}>
            <div />
            <button className="btn btn-primary" onClick={() => setShowMemberForm(true)}>
              <Plus size={16} /> Add Member
            </button>
          </div>

          <div className="card">
            <div className="table-container">
              <table>
                <thead>
                  <tr>
                    <th>Name</th>
                    <th>Email</th>
                    <th>Role</th>
                    <th>Joined</th>
                    <th></th>
                  </tr>
                </thead>
                <tbody>
                  {members.length === 0 ? (
                    <tr>
                      <td colSpan={5} style={{ textAlign: 'center', color: 'var(--text-muted)', padding: 32 }}>
                        No members in your team yet.
                      </td>
                    </tr>
                  ) : members.map(member => (
                    <tr key={member.id}>
                      <td style={{ fontWeight: 500 }}>{member.name}</td>
                      <td style={{ color: 'var(--text-secondary)' }}>{member.email}</td>
                      <td>
                        {member.isAdmin ? (
                          <span className="badge badge-published">Admin</span>
                        ) : member.isTeamAdmin ? (
                          <span className="badge badge-scheduled">Team Admin</span>
                        ) : (
                          <span className="badge badge-draft">Member</span>
                        )}
                      </td>
                      <td style={{ color: 'var(--text-muted)', fontSize: 13 }}>
                        {format(new Date(member.createdAt), 'MMM d, yyyy')}
                      </td>
                      <td>
                        {!member.isAdmin && (
                          <button className="btn btn-ghost btn-sm" onClick={() => removeMember(member.id)}>
                            <Trash2 size={14} color="var(--danger)" />
                          </button>
                        )}
                      </td>
                    </tr>
                  ))}
                </tbody>
              </table>
            </div>
          </div>

          {showMemberForm && (
            <div className="modal-overlay" onClick={() => setShowMemberForm(false)}>
              <div className="modal" onClick={e => e.stopPropagation()}>
                <div style={{ display: 'flex', justifyContent: 'space-between', alignItems: 'center', marginBottom: 20 }}>
                  <h2>Add Team Member</h2>
                  <button className="btn btn-ghost btn-sm" onClick={() => setShowMemberForm(false)}>
                    <X size={18} />
                  </button>
                </div>
                <div style={{ display: 'flex', flexDirection: 'column', gap: 16 }}>
                  <p style={{ color: 'var(--text-secondary)', fontSize: 13, margin: 0 }}>
                    Enter the email address of an existing account. Team admins cannot create new accounts — contact an admin to create one first.
                  </p>
                  <div className="form-group">
                    <label>Email Address</label>
                    <input
                      className="input"
                      type="email"
                      placeholder="user@example.com"
                      value={memberEmail}
                      onChange={e => setMemberEmail(e.target.value)}
                      onKeyDown={e => e.key === 'Enter' && addMember()}
                    />
                  </div>
                </div>
                <div className="modal-actions">
                  <button className="btn btn-secondary" onClick={() => setShowMemberForm(false)}>Cancel</button>
                  <button className="btn btn-primary" onClick={addMember} disabled={addingMember}>
                    {addingMember ? 'Adding...' : 'Add Member'}
                  </button>
                </div>
              </div>
            </div>
          )}
        </>
      )}
    </div>
  );
}
