import { useState, useEffect } from 'react';
import { api } from '../../services/api';
import { useAuth } from '../../contexts/AuthContext';
import type { User, TeamInvite } from '../../types';
import { Trash2, Plus, Mail, Clock, X } from 'lucide-react';
import { format } from 'date-fns';
import toast from 'react-hot-toast';
import './Admin.css';

export function TeamInfoPage() {
  const { user } = useAuth();
  const [members, setMembers] = useState<User[]>([]);
  const [invites, setInvites] = useState<TeamInvite[]>([]);
  const [showAddForm, setShowAddForm] = useState(false);
  const [newEmail, setNewEmail] = useState('');
  const [addingMember, setAddingMember] = useState(false);
  const [showInviteForm, setShowInviteForm] = useState(false);
  const [inviteEmail, setInviteEmail] = useState('');
  const [sendingInvite, setSendingInvite] = useState(false);

  const load = () => {
    api.getTeamMembers().then(setMembers).catch(() => toast.error('Failed to load members'));
    api.getMyTeamInvites().then(setInvites).catch(() => {});
  };

  useEffect(() => { load(); }, []);

  const addMember = async () => {
    if (!newEmail.trim()) { toast.error('Email is required'); return; }
    setAddingMember(true);
    try {
      await api.addTeamMember({ email: newEmail.trim() });
      setShowAddForm(false);
      setNewEmail('');
      load();
      toast.success('Member added');
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
      load();
      toast.success('Member removed');
    } catch (err: any) {
      toast.error(err.message);
    }
  };

  const sendInvite = async () => {
    if (!inviteEmail.trim()) { toast.error('Email address is required'); return; }
    setSendingInvite(true);
    try {
      const result = await api.createMyTeamInvite(inviteEmail.trim());
      if (result.emailError) {
        toast.success('Invitation created, but email delivery failed: ' + result.emailError);
      } else {
        toast.success('Invitation sent');
      }
      setShowInviteForm(false);
      setInviteEmail('');
      api.getMyTeamInvites().then(setInvites).catch(() => {});
    } catch (err: any) {
      toast.error(err.message);
    } finally {
      setSendingInvite(false);
    }
  };

  const deleteInvite = async (inviteId: string) => {
    if (!confirm('Delete this invitation?')) return;
    try {
      await api.deleteMyTeamInvite(inviteId);
      setInvites(prev => prev.filter(i => i.id !== inviteId));
      toast.success('Invitation deleted');
    } catch (err: any) {
      toast.error(err.message);
    }
  };

  return (
    <div className="page">
      <div className="page-header">
        <h1>{user?.teamName || 'My Team'}</h1>
        <div style={{ display: 'flex', gap: 8 }}>
          <button className="btn btn-secondary" onClick={() => setShowAddForm(true)}>
            <Plus size={16} /> Add Member
          </button>
          <button className="btn btn-secondary" onClick={() => setShowInviteForm(true)}>
            <Mail size={16} /> Invite
          </button>
        </div>
      </div>

      <div className="card" style={{ marginBottom: 24 }}>
        <h3 style={{ margin: '0 0 16px' }}>Members</h3>
        {members.length === 0 ? (
          <p style={{ color: 'var(--text-muted)', fontSize: 14 }}>No members yet.</p>
        ) : (
          <div style={{ display: 'flex', flexDirection: 'column', gap: 8 }}>
            {members.map(m => (
              <div
                key={m.id}
                style={{
                  display: 'flex',
                  alignItems: 'center',
                  justifyContent: 'space-between',
                  padding: '10px 14px',
                  borderRadius: 'var(--radius-sm)',
                  border: '1px solid var(--border)',
                  background: 'var(--bg-primary)',
                }}
              >
                <div>
                  <div style={{ fontWeight: 500 }}>{m.name}</div>
                  <div style={{ fontSize: 12, color: 'var(--text-muted)' }}>{m.email}</div>
                </div>
                {m.id !== user?.id && (
                  <button className="btn btn-ghost btn-sm" onClick={() => removeMember(m.id)}>
                    <Trash2 size={14} color="var(--danger)" />
                  </button>
                )}
              </div>
            ))}
          </div>
        )}
      </div>

      {invites.length > 0 && (
        <div className="card">
          <h3 style={{ margin: '0 0 16px' }}>Pending Invitations</h3>
          <div style={{ display: 'flex', flexDirection: 'column', gap: 8 }}>
            {invites.map(invite => (
              <div
                key={invite.id}
                style={{
                  display: 'flex',
                  alignItems: 'center',
                  justifyContent: 'space-between',
                  padding: '10px 14px',
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
        </div>
      )}

      {showAddForm && (
        <div className="modal-overlay" onClick={() => setShowAddForm(false)}>
          <div className="modal" onClick={e => e.stopPropagation()}>
            <div style={{ display: 'flex', justifyContent: 'space-between', alignItems: 'center', marginBottom: 20 }}>
              <h2>Add Existing User</h2>
              <button className="btn btn-ghost btn-sm" onClick={() => setShowAddForm(false)}>
                <X size={18} />
              </button>
            </div>
            <p style={{ color: 'var(--text-secondary)', fontSize: 14, marginBottom: 16 }}>
              Add an existing user to your team by their email address.
            </p>
            <div className="form-group">
              <label>Email</label>
              <input
                className="input"
                type="email"
                placeholder="user@example.com"
                value={newEmail}
                onChange={e => setNewEmail(e.target.value)}
                onKeyDown={e => e.key === 'Enter' && addMember()}
              />
            </div>
            <div className="modal-actions">
              <button className="btn btn-secondary" onClick={() => setShowAddForm(false)}>Cancel</button>
              <button className="btn btn-primary" onClick={addMember} disabled={addingMember}>
                {addingMember ? 'Adding...' : 'Add Member'}
              </button>
            </div>
          </div>
        </div>
      )}

      {showInviteForm && (
        <div className="modal-overlay" onClick={() => setShowInviteForm(false)}>
          <div className="modal" onClick={e => e.stopPropagation()}>
            <div style={{ display: 'flex', justifyContent: 'space-between', alignItems: 'center', marginBottom: 20 }}>
              <h2>Invite by Email</h2>
              <button className="btn btn-ghost btn-sm" onClick={() => setShowInviteForm(false)}>
                <X size={18} />
              </button>
            </div>
            <p style={{ color: 'var(--text-secondary)', fontSize: 14, marginBottom: 16 }}>
              Send an invitation email to someone who doesn't have an account yet.
            </p>
            <div className="form-group">
              <label>Email</label>
              <input
                className="input"
                type="email"
                placeholder="newuser@example.com"
                value={inviteEmail}
                onChange={e => setInviteEmail(e.target.value)}
                onKeyDown={e => e.key === 'Enter' && sendInvite()}
              />
            </div>
            <div className="modal-actions">
              <button className="btn btn-secondary" onClick={() => setShowInviteForm(false)}>Cancel</button>
              <button className="btn btn-primary" onClick={sendInvite} disabled={sendingInvite}>
                {sendingInvite ? 'Sending...' : 'Send Invitation'}
              </button>
            </div>
          </div>
        </div>
      )}
    </div>
  );
}
