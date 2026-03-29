import { useState, useEffect } from 'react';
import { api } from '../../services/api';
import type { SocialAccount } from '../../types';
import { Plus, Trash2, ToggleLeft, ToggleRight, ExternalLink } from 'lucide-react';
import toast from 'react-hot-toast';
import './Admin.css';

export function AccountsPage() {
  const [accounts, setAccounts] = useState<SocialAccount[]>([]);
  const [showBlueskyForm, setShowBlueskyForm] = useState(false);
  const [handle, setHandle] = useState('');
  const [appPassword, setAppPassword] = useState('');
  const [pdsHost, setPdsHost] = useState('');
  const [adding, setAdding] = useState(false);

  useEffect(() => {
    loadAccounts();
  }, []);

  const loadAccounts = async () => {
    try {
      const data = await api.getAccounts();
      setAccounts(data);
    } catch {
      toast.error('Failed to load accounts');
    }
  };

  const addBluesky = async () => {
    if (!handle || !appPassword) { toast.error('Handle and app password required'); return; }
    setAdding(true);
    try {
      await api.addBlueskyAccount({ handle, appPassword, pdsHost: pdsHost || undefined });
      toast.success('Bluesky account added');
      setShowBlueskyForm(false);
      setHandle('');
      setAppPassword('');
      setPdsHost('');
      loadAccounts();
    } catch (err: any) {
      toast.error(err.message);
    } finally {
      setAdding(false);
    }
  };

  const connectInstagram = async () => {
    try {
      const { url } = await api.getInstagramAuthUrl();
      window.location.href = url;
    } catch (err: any) {
      toast.error(err.message);
    }
  };

  const toggleAccount = async (id: string) => {
    try {
      const updated = await api.toggleAccount(id);
      setAccounts(prev => prev.map(a => a.id === id ? updated : a));
    } catch {
      toast.error('Failed to toggle account');
    }
  };

  const deleteAccount = async (id: string) => {
    if (!confirm('Delete this account?')) return;
    try {
      await api.deleteAccount(id);
      setAccounts(prev => prev.filter(a => a.id !== id));
      toast.success('Account deleted');
    } catch {
      toast.error('Failed to delete');
    }
  };

  return (
    <div className="page">
      <div className="page-header">
        <h1>Social Accounts</h1>
        <div style={{ display: 'flex', gap: 10 }}>
          <button className="btn btn-primary" onClick={() => setShowBlueskyForm(true)}>
            <Plus size={16} /> Add Bluesky
          </button>
          <button className="btn btn-secondary" onClick={connectInstagram} style={{ borderColor: 'var(--instagram)', color: 'var(--instagram)' }}>
            <ExternalLink size={16} /> Connect Instagram
          </button>
        </div>
      </div>

      {accounts.length === 0 ? (
        <div className="empty-state">
          <p>No social accounts connected yet.</p>
          <p style={{ color: 'var(--text-muted)', fontSize: 14 }}>Add a Bluesky or Instagram account to start scheduling posts.</p>
        </div>
      ) : (
        <div className="accounts-grid">
          {accounts.map(account => (
            <div key={account.id} className={`account-card ${!account.isActive ? 'inactive' : ''}`}>
              <div className="account-header">
                <span className={`platform-dot ${account.platform}`} style={{ width: 12, height: 12 }} />
                <span className="account-platform">{account.platform}</span>
                <div style={{ marginLeft: 'auto', display: 'flex', gap: 8 }}>
                  <button className="btn btn-ghost btn-sm" onClick={() => toggleAccount(account.id)} title={account.isActive ? 'Disable' : 'Enable'}>
                    {account.isActive ? <ToggleRight size={18} color="var(--success)" /> : <ToggleLeft size={18} />}
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
            <h2>Add Bluesky Account</h2>

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
              <button className="btn btn-primary" onClick={addBluesky} disabled={adding}>
                {adding ? 'Adding...' : 'Add Account'}
              </button>
            </div>
          </div>
        </div>
      )}
    </div>
  );
}
