import { useState, useEffect } from 'react';
import { api } from '../../services/api';
import type { User } from '../../types';
import { useAuth } from '../../contexts/AuthContext';
import { Trash2, Shield, Plus, X, Star, KeyRound, Copy } from 'lucide-react';
import { format } from 'date-fns';
import toast from 'react-hot-toast';
import './Admin.css';

export function UsersPage() {
  const { user: currentUser } = useAuth();
  const [users, setUsers] = useState<User[]>([]);
  const [showForm, setShowForm] = useState(false);
  const [name, setName] = useState('');
  const [email, setEmail] = useState('');
  const [password, setPassword] = useState('');
  const [isAdmin, setIsAdmin] = useState(false);
  const [creating, setCreating] = useState(false);
  const [resetPassword, setResetPassword] = useState<{ userId: string; password: string } | null>(null);

  useEffect(() => {
    api.getUsers().then(setUsers).catch(() => toast.error('Failed to load users'));
  }, []);

  const createUser = async () => {
    if (!name || !email || !password) {
      toast.error('All fields are required');
      return;
    }
    if (password.length < 8) {
      toast.error('Password must be at least 8 characters');
      return;
    }
    setCreating(true);
    try {
      const user = await api.createUser({ name, email, password, isAdmin });
      setUsers(prev => [...prev, user]);
      setShowForm(false);
      setName('');
      setEmail('');
      setPassword('');
      setIsAdmin(false);
      toast.success('User created');
    } catch (err: any) {
      toast.error(err.message);
    } finally {
      setCreating(false);
    }
  };

  const deleteUser = async (id: string) => {
    if (!confirm('Delete this user and all their posts?')) return;
    try {
      await api.deleteUser(id);
      setUsers(prev => prev.filter(u => u.id !== id));
      toast.success('User deleted');
    } catch (err: any) {
      toast.error(err.message);
    }
  };

  const toggleTeamAdmin = async (user: User) => {
    try {
      const updated = await api.updateUserRole(user.id, { isTeamAdmin: !user.isTeamAdmin });
      setUsers(prev => prev.map(u => u.id === user.id ? updated : u));
      toast.success(updated.isTeamAdmin ? 'Team admin enabled' : 'Team admin disabled');
    } catch (err: any) {
      toast.error(err.message);
    }
  };

  const handleResetPassword = async (user: User) => {
    if (!confirm(`Reset password for ${user.name}? A new random password will be generated and shown once.`)) return;
    try {
      const res = await api.adminResetUserPassword(user.id);
      setResetPassword({ userId: user.id, password: res.password });
    } catch (err: any) {
      toast.error(err.message);
    }
  };

  const copyResetPassword = () => {
    if (resetPassword) {
      navigator.clipboard.writeText(resetPassword.password);
      toast.success('Copied to clipboard');
    }
  };

  return (
    <div className="page">
      <div className="page-header">
        <h1>Users</h1>
        <button className="btn btn-primary" onClick={() => setShowForm(true)}>
          <Plus size={16} /> Add User
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
                <th>Team Admin</th>
                <th>Joined</th>
                <th></th>
              </tr>
            </thead>
            <tbody>
              {users.map(user => (
                <tr key={user.id}>
                  <td style={{ fontWeight: 500 }}>{user.name}</td>
                  <td style={{ color: 'var(--text-secondary)' }}>{user.email}</td>
                  <td>
                    {user.isAdmin ? (
                      <span className="badge badge-published"><Shield size={10} /> Admin</span>
                    ) : (
                      <span className="badge badge-scheduled">User</span>
                    )}
                  </td>
                  <td>
                    {!user.isAdmin && (
                      <button
                        className="btn btn-ghost btn-sm"
                        onClick={() => toggleTeamAdmin(user)}
                        title={user.isTeamAdmin ? 'Revoke team admin' : 'Grant team admin'}
                      >
                        <Star
                          size={14}
                          color={user.isTeamAdmin ? 'var(--warning)' : 'var(--text-muted)'}
                          fill={user.isTeamAdmin ? 'var(--warning)' : 'none'}
                        />
                      </button>
                    )}
                  </td>
                  <td style={{ color: 'var(--text-muted)', fontSize: 13 }}>
                    {format(new Date(user.createdAt), 'MMM d, yyyy')}
                  </td>
                  <td>
                    <div style={{ display: 'flex', gap: 4 }}>
                      {user.id !== currentUser?.id && (
                        <button
                          className="btn btn-ghost btn-sm"
                          onClick={() => handleResetPassword(user)}
                          title="Reset password"
                        >
                          <KeyRound size={14} color="var(--text-muted)" />
                        </button>
                      )}
                      {user.id !== currentUser?.id && (
                        <button className="btn btn-ghost btn-sm" onClick={() => deleteUser(user.id)}>
                          <Trash2 size={14} color="var(--danger)" />
                        </button>
                      )}
                    </div>
                  </td>
                </tr>
              ))}
            </tbody>
          </table>
        </div>
      </div>

      {resetPassword && (
        <div className="modal-overlay" onClick={() => setResetPassword(null)}>
          <div className="modal" onClick={e => e.stopPropagation()}>
            <div style={{ display: 'flex', justifyContent: 'space-between', alignItems: 'center', marginBottom: 20 }}>
              <h2>Password Reset</h2>
              <button className="btn btn-ghost btn-sm" onClick={() => setResetPassword(null)}>
                <X size={18} />
              </button>
            </div>

            <p style={{ color: 'var(--text-secondary)', fontSize: 14, marginBottom: 16 }}>
              The password has been reset. Copy it now and send it to the user — it will not be shown again.
            </p>

            <div style={{
              background: 'var(--bg-primary)',
              border: '1px solid var(--border)',
              borderRadius: 'var(--radius-sm)',
              padding: '12px 16px',
              marginBottom: 20,
              display: 'flex',
              alignItems: 'center',
              gap: 10,
              fontFamily: 'monospace',
              fontSize: 15,
            }}>
              <span style={{ flex: 1 }}>{resetPassword.password}</span>
              <button className="btn btn-ghost btn-sm" onClick={copyResetPassword} title="Copy">
                <Copy size={14} />
              </button>
            </div>

            <div className="modal-actions">
              <button className="btn btn-primary" onClick={() => setResetPassword(null)}>Done</button>
            </div>
          </div>
        </div>
      )}

      {showForm && (
        <div className="modal-overlay" onClick={() => setShowForm(false)}>
          <div className="modal" onClick={e => e.stopPropagation()}>
            <div style={{ display: 'flex', justifyContent: 'space-between', alignItems: 'center', marginBottom: 20 }}>
              <h2>Add User</h2>
              <button className="btn btn-ghost btn-sm" onClick={() => setShowForm(false)}>
                <X size={18} />
              </button>
            </div>

            <div style={{ display: 'flex', flexDirection: 'column', gap: 16 }}>
              <div className="form-group">
                <label>Name</label>
                <input className="input" placeholder="Full name" value={name} onChange={e => setName(e.target.value)} />
              </div>

              <div className="form-group">
                <label>Email</label>
                <input className="input" type="email" placeholder="user@example.com" value={email} onChange={e => setEmail(e.target.value)} />
              </div>

              <div className="form-group">
                <label>Password</label>
                <input className="input" type="password" placeholder="Min 8 characters" value={password} onChange={e => setPassword(e.target.value)} />
              </div>

              <div className="form-group">
                <label style={{ display: 'flex', alignItems: 'center', gap: 10, cursor: 'pointer' }}>
                  <input type="checkbox" checked={isAdmin} onChange={e => setIsAdmin(e.target.checked)} />
                  Admin privileges
                </label>
              </div>
            </div>

            <div className="modal-actions">
              <button className="btn btn-secondary" onClick={() => setShowForm(false)}>Cancel</button>
              <button className="btn btn-primary" onClick={createUser} disabled={creating}>
                {creating ? 'Creating...' : 'Create User'}
              </button>
            </div>
          </div>
        </div>
      )}
    </div>
  );
}
