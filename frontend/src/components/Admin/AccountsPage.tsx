import { useState, useEffect } from 'react';
import { api } from '../../services/api';
import type { SocialAccount, Team } from '../../types';
import { Plus, Trash2, ToggleLeft, ToggleRight, ExternalLink } from 'lucide-react';
import { PlatformIcon } from '../Common/PlatformIcon';
import toast from 'react-hot-toast';
import './Admin.css';

export function AccountsPage() {
  const [accounts, setAccounts] = useState<SocialAccount[]>([]);
  const [teams, setTeams] = useState<Team[]>([]);
  const [showBlueskyForm, setShowBlueskyForm] = useState(false);
  const [handle, setHandle] = useState('');
  const [appPassword, setAppPassword] = useState('');
  const [pdsHost, setPdsHost] = useState('');
  const [pdsHostConfirmed, setPdsHostConfirmed] = useState(false);
  const [newAccountTeamId, setNewAccountTeamId] = useState('');
  const [adding, setAdding] = useState(false);
  const [showTwitterForm, setShowTwitterForm] = useState(false);
  const [twConsumerKey, setTwConsumerKey] = useState('');
  const [twConsumerSecret, setTwConsumerSecret] = useState('');
  const [twAccessToken, setTwAccessToken] = useState('');
  const [twAccessTokenSecret, setTwAccessTokenSecret] = useState('');
  const [twTeamId, setTwTeamId] = useState('');
  const [addingTwitter, setAddingTwitter] = useState(false);
  const [showMastodonForm, setShowMastodonForm] = useState(false);
  const [mastodonInstance, setMastodonInstance] = useState('');
  const [mastodonAccessToken, setMastodonAccessToken] = useState('');
  const [mastodonTeamId, setMastodonTeamId] = useState('');
  const [addingMastodon, setAddingMastodon] = useState(false);
  const [showThreadsForm, setShowThreadsForm] = useState(false);
  const [threadsAccessToken, setThreadsAccessToken] = useState('');
  const [threadsTeamId, setThreadsTeamId] = useState('');
  const [addingThreads, setAddingThreads] = useState(false);
  const [showLinkedInForm, setShowLinkedInForm] = useState(false);
  const [linkedInAccessToken, setLinkedInAccessToken] = useState('');
  const [linkedInTeamId, setLinkedInTeamId] = useState('');
  const [addingLinkedIn, setAddingLinkedIn] = useState(false);

  useEffect(() => {
    loadAccounts();
    api.getTeams().then(setTeams).catch(() => {});

    const params = new URLSearchParams(window.location.search);
    if (params.get('mastodon') === 'connected') {
      toast.success('Mastodon account connected');
      window.history.replaceState({}, '', window.location.pathname);
    } else if (params.get('instagram') === 'connected') {
      toast.success('Instagram account connected');
      window.history.replaceState({}, '', window.location.pathname);
    } else if (params.get('error')) {
      toast.error('Connection failed: ' + params.get('error'));
      window.history.replaceState({}, '', window.location.pathname);
    }
  }, []);

  const loadAccounts = async () => {
    try {
      setAccounts(await api.getAccounts());
    } catch {
      toast.error('Failed to load accounts');
    }
  };

  const addBluesky = async () => {
    if (!handle || !appPassword) { toast.error('Handle and app password required'); return; }
    if (pdsHost && !pdsHostConfirmed) { toast.error('Please confirm the custom PDS host before adding the account'); return; }
    setAdding(true);
    try {
      await api.addBlueskyAccount({
        handle,
        appPassword,
        pdsHost: pdsHost || undefined,
        teamId: newAccountTeamId || undefined,
      });
      toast.success('Bluesky account added');
      setShowBlueskyForm(false);
      setHandle(''); setAppPassword(''); setPdsHost(''); setPdsHostConfirmed(false); setNewAccountTeamId('');
      loadAccounts();
    } catch (err: any) {
      toast.error(err.message);
    } finally {
      setAdding(false);
    }
  };

  const assignTeam = async (accountId: string, teamId: string) => {
    try {
      const updated = await api.assignAccountTeam(accountId, teamId || null);
      setAccounts(prev => prev.map(a => a.id === accountId ? updated : a));
      toast.success(teamId ? 'Team assigned' : 'Team unassigned');
    } catch {
      toast.error('Failed to assign team');
    }
  };

  const addTwitter = async () => {
    if (!twConsumerKey || !twConsumerSecret || !twAccessToken || !twAccessTokenSecret) {
      toast.error('All Twitter API credentials are required');
      return;
    }
    setAddingTwitter(true);
    try {
      await api.addTwitterAccount({
        consumerKey: twConsumerKey,
        consumerSecret: twConsumerSecret,
        accessToken: twAccessToken,
        accessTokenSecret: twAccessTokenSecret,
        teamId: twTeamId || undefined,
      });
      toast.success('X/Twitter account added');
      setShowTwitterForm(false);
      setTwConsumerKey(''); setTwConsumerSecret(''); setTwAccessToken(''); setTwAccessTokenSecret(''); setTwTeamId('');
      loadAccounts();
    } catch (err: any) {
      toast.error(err.message);
    } finally {
      setAddingTwitter(false);
    }
  };

  const addMastodon = async () => {
    if (!mastodonInstance || !mastodonAccessToken) {
      toast.error('Instance and access token are required');
      return;
    }
    setAddingMastodon(true);
    try {
      await api.addMastodonAccount({
        instance: mastodonInstance,
        accessToken: mastodonAccessToken,
        teamId: mastodonTeamId || undefined,
      });
      toast.success('Mastodon account added');
      setShowMastodonForm(false);
      setMastodonInstance(''); setMastodonAccessToken(''); setMastodonTeamId('');
      loadAccounts();
    } catch (err: any) {
      toast.error(err.message);
    } finally {
      setAddingMastodon(false);
    }
  };

  const addThreads = async () => {
    if (!threadsAccessToken) { toast.error('Access token is required'); return; }
    setAddingThreads(true);
    try {
      await api.addThreadsAccount({ accessToken: threadsAccessToken, teamId: threadsTeamId || undefined });
      toast.success('Threads account added');
      setShowThreadsForm(false);
      setThreadsAccessToken(''); setThreadsTeamId('');
      loadAccounts();
    } catch (err: any) {
      toast.error(err.message);
    } finally {
      setAddingThreads(false);
    }
  };

  const addLinkedIn = async () => {
    if (!linkedInAccessToken) { toast.error('Access token is required'); return; }
    setAddingLinkedIn(true);
    try {
      await api.addLinkedInAccount({ accessToken: linkedInAccessToken, teamId: linkedInTeamId || undefined });
      toast.success('LinkedIn account added');
      setShowLinkedInForm(false);
      setLinkedInAccessToken(''); setLinkedInTeamId('');
      loadAccounts();
    } catch (err: any) {
      toast.error(err.message);
    } finally {
      setAddingLinkedIn(false);
    }
  };

  const connectMastodonOAuth = async () => {
    if (!mastodonInstance) { toast.error('Please enter your instance URL first'); return; }
    setAddingMastodon(true);
    try {
      const { url } = await api.getMastodonAuthUrl(mastodonInstance);
      window.location.href = url;
    } catch (err: any) {
      toast.error(err.message);
      setAddingMastodon(false);
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
          <button className="btn btn-secondary" onClick={() => setShowTwitterForm(true)} style={{ borderColor: 'var(--twitter)', color: 'var(--twitter)' }}>
            <Plus size={16} /> Add X/Twitter
          </button>
          <button className="btn btn-secondary" onClick={() => setShowMastodonForm(true)} style={{ borderColor: 'var(--mastodon)', color: 'var(--mastodon)' }}>
            <Plus size={16} /> Add Mastodon
          </button>
          <button className="btn btn-secondary" onClick={connectInstagram} style={{ borderColor: 'var(--instagram)', color: 'var(--instagram)' }}>
            <ExternalLink size={16} /> Connect Instagram
          </button>
          <button className="btn btn-secondary" onClick={() => setShowThreadsForm(true)} style={{ borderColor: 'var(--threads)', color: 'var(--threads)' }}>
            <Plus size={16} /> Add Threads
          </button>
          <button className="btn btn-secondary" onClick={() => setShowLinkedInForm(true)} style={{ borderColor: 'var(--linkedin)', color: 'var(--linkedin)' }}>
            <Plus size={16} /> Add LinkedIn
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
          {accounts.map(account => {
            const teamName = account.teamId
              ? teams.find(t => t.id === account.teamId)?.name
              : null;
            return (
              <div key={account.id} className={`account-card ${!account.isActive ? 'inactive' : ''}`}>
                <div className="account-header">
                  <PlatformIcon platform={account.platform} size={12} />
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
                {!account.isActive && <span className="badge badge-draft" style={{ marginBottom: 6 }}>Disabled</span>}
                <div style={{ marginTop: 8 }}>
                  <select
                    className="input"
                    style={{ fontSize: 12, padding: '4px 8px', height: 'auto' }}
                    value={account.teamId || ''}
                    onChange={e => assignTeam(account.id, e.target.value)}
                  >
                    <option value="">— Unassigned —</option>
                    {teams.map(t => (
                      <option key={t.id} value={t.id}>{t.name}</option>
                    ))}
                  </select>
                  {teamName && (
                    <div style={{ fontSize: 11, color: 'var(--text-muted)', marginTop: 4 }}>
                      Team: {teamName}
                    </div>
                  )}
                </div>
              </div>
            );
          })}
        </div>
      )}

      {showTwitterForm && (
        <div className="modal-overlay" onClick={() => setShowTwitterForm(false)}>
          <div className="modal" onClick={e => e.stopPropagation()}>
            <h2>Add X/Twitter Account</h2>
            <div style={{ background: 'var(--warning-bg, #fff8e1)', border: '1px solid var(--warning-border, #f9a825)', borderRadius: 6, padding: '10px 14px', fontSize: 13, color: 'var(--warning-text, #7a5c00)', marginBottom: 8 }}>
              Your X Developer App must have <strong>Read and Write</strong> permissions. After changing app permissions in the X Developer Portal, you must regenerate your Access Token and Secret for the new permissions to take effect.
            </div>
            <div style={{ display: 'flex', flexDirection: 'column', gap: 16 }}>
              <div className="form-group">
                <label>API Key (Consumer Key)</label>
                <input className="input" type="password" placeholder="API Key" value={twConsumerKey} onChange={e => setTwConsumerKey(e.target.value)} />
              </div>
              <div className="form-group">
                <label>API Key Secret (Consumer Secret)</label>
                <input className="input" type="password" placeholder="API Key Secret" value={twConsumerSecret} onChange={e => setTwConsumerSecret(e.target.value)} />
              </div>
              <div className="form-group">
                <label>Access Token</label>
                <input className="input" type="password" placeholder="Access Token" value={twAccessToken} onChange={e => setTwAccessToken(e.target.value)} />
              </div>
              <div className="form-group">
                <label>Access Token Secret</label>
                <input className="input" type="password" placeholder="Access Token Secret" value={twAccessTokenSecret} onChange={e => setTwAccessTokenSecret(e.target.value)} />
                <span style={{ fontSize: 12, color: 'var(--text-muted)' }}>
                  Get these from the X Developer Portal under your app's Keys and Tokens tab.
                </span>
              </div>
              <div className="form-group">
                <label>Assign to Team</label>
                <select className="input" value={twTeamId} onChange={e => setTwTeamId(e.target.value)}>
                  <option value="">— Unassigned —</option>
                  {teams.map(t => (
                    <option key={t.id} value={t.id}>{t.name}</option>
                  ))}
                </select>
              </div>
            </div>
            <div className="modal-actions">
              <button className="btn btn-secondary" onClick={() => setShowTwitterForm(false)}>Cancel</button>
              <button className="btn btn-primary" onClick={addTwitter} disabled={addingTwitter}>
                {addingTwitter ? 'Adding...' : 'Add Account'}
              </button>
            </div>
          </div>
        </div>
      )}

      {showMastodonForm && (
        <div className="modal-overlay" onClick={() => setShowMastodonForm(false)}>
          <div className="modal" onClick={e => e.stopPropagation()}>
            <h2>Add Mastodon Account</h2>
            <div style={{ display: 'flex', flexDirection: 'column', gap: 16 }}>
              <div className="form-group">
                <label>Instance URL</label>
                <input className="input" placeholder="mastodon.social" value={mastodonInstance} onChange={e => setMastodonInstance(e.target.value)} />
                <span style={{ fontSize: 12, color: 'var(--text-muted)' }}>
                  Your Mastodon server, e.g. mastodon.social or fosstodon.org
                </span>
              </div>
              {teams.length > 0 && (
                <div className="form-group">
                  <label>Assign to Team</label>
                  <select className="input" value={mastodonTeamId} onChange={e => setMastodonTeamId(e.target.value)}>
                    <option value="">— Unassigned —</option>
                    {teams.map(t => (
                      <option key={t.id} value={t.id}>{t.name}</option>
                    ))}
                  </select>
                </div>
              )}
              <button className="btn btn-primary" onClick={connectMastodonOAuth} disabled={addingMastodon} style={{ borderColor: 'var(--mastodon)', backgroundColor: 'var(--mastodon)', color: '#fff' }}>
                {addingMastodon ? 'Redirecting…' : 'Connect via OAuth'}
              </button>
              <details style={{ marginTop: 8 }}>
                <summary style={{ fontSize: 13, cursor: 'pointer', color: 'var(--text-muted)' }}>
                  Or enter an access token manually
                </summary>
                <div style={{ display: 'flex', flexDirection: 'column', gap: 12, marginTop: 12 }}>
                  <div className="form-group">
                    <label>Access Token</label>
                    <input className="input" type="password" placeholder="Access token" value={mastodonAccessToken} onChange={e => setMastodonAccessToken(e.target.value)} />
                    <span style={{ fontSize: 12, color: 'var(--text-muted)' }}>
                      Generate at your instance under Settings &gt; Development &gt; New application. Required scopes: <code>read write</code>.
                    </span>
                  </div>
                  <button className="btn btn-secondary" onClick={addMastodon} disabled={addingMastodon}>
                    {addingMastodon ? 'Adding…' : 'Add with Token'}
                  </button>
                </div>
              </details>
            </div>
            <div className="modal-actions">
              <button className="btn btn-secondary" onClick={() => setShowMastodonForm(false)}>Cancel</button>
            </div>
          </div>
        </div>
      )}

      {showThreadsForm && (
        <div className="modal-overlay" onClick={() => setShowThreadsForm(false)}>
          <div className="modal" onClick={e => e.stopPropagation()}>
            <h2>Add Threads Account</h2>
            <div style={{ display: 'flex', flexDirection: 'column', gap: 16 }}>
              <div className="form-group">
                <label>Access Token</label>
                <input className="input" type="password" placeholder="Long-lived user access token" value={threadsAccessToken} onChange={e => setThreadsAccessToken(e.target.value)} />
                <span style={{ fontSize: 12, color: 'var(--text-muted)' }}>
                  Obtain a long-lived token via the Meta for Developers Threads API (threads.net).
                </span>
              </div>
              <div className="form-group">
                <label>Assign to Team</label>
                <select className="input" value={threadsTeamId} onChange={e => setThreadsTeamId(e.target.value)}>
                  <option value="">— Unassigned —</option>
                  {teams.map(t => (
                    <option key={t.id} value={t.id}>{t.name}</option>
                  ))}
                </select>
              </div>
            </div>
            <div className="modal-actions">
              <button className="btn btn-secondary" onClick={() => setShowThreadsForm(false)}>Cancel</button>
              <button className="btn btn-primary" onClick={addThreads} disabled={addingThreads}>
                {addingThreads ? 'Adding...' : 'Add Account'}
              </button>
            </div>
          </div>
        </div>
      )}

      {showLinkedInForm && (
        <div className="modal-overlay" onClick={() => setShowLinkedInForm(false)}>
          <div className="modal" onClick={e => e.stopPropagation()}>
            <h2>Add LinkedIn Account</h2>
            <div style={{ display: 'flex', flexDirection: 'column', gap: 16 }}>
              <div className="form-group">
                <label>Access Token</label>
                <input className="input" type="password" placeholder="OAuth 2.0 access token" value={linkedInAccessToken} onChange={e => setLinkedInAccessToken(e.target.value)} />
                <span style={{ fontSize: 12, color: 'var(--text-muted)' }}>
                  Obtain via the LinkedIn Developer Portal. Requires the w_member_social scope.
                </span>
              </div>
              <div className="form-group">
                <label>Assign to Team</label>
                <select className="input" value={linkedInTeamId} onChange={e => setLinkedInTeamId(e.target.value)}>
                  <option value="">— Unassigned —</option>
                  {teams.map(t => (
                    <option key={t.id} value={t.id}>{t.name}</option>
                  ))}
                </select>
              </div>
            </div>
            <div className="modal-actions">
              <button className="btn btn-secondary" onClick={() => setShowLinkedInForm(false)}>Cancel</button>
              <button className="btn btn-primary" onClick={addLinkedIn} disabled={addingLinkedIn}>
                {addingLinkedIn ? 'Adding...' : 'Add Account'}
              </button>
            </div>
          </div>
        </div>
      )}

      {showBlueskyForm && (
        <div className="modal-overlay" onClick={() => setShowBlueskyForm(false)}>
          <div className="modal" onClick={e => e.stopPropagation()}>
            <h2>Add Bluesky Account</h2>

            <div style={{ display: 'flex', flexDirection: 'column', gap: 16 }}>
              <div className="form-group">
                <label>Handle</label>
                <input className="input" placeholder="user.bsky.social" value={handle} onChange={e => setHandle(e.target.value.replace(/^@+/, ''))} />
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
                <input className="input" placeholder="https://bsky.social" value={pdsHost} onChange={e => { setPdsHost(e.target.value); setPdsHostConfirmed(false); }} />
                {pdsHost && (
                  <div style={{ marginTop: 8, background: 'var(--warning-bg, #fff8e1)', border: '1px solid var(--warning-border, #f9a825)', borderRadius: 6, padding: '10px 14px', fontSize: 13, color: 'var(--warning-text, #7a5c00)' }}>
                    <label style={{ display: 'flex', alignItems: 'flex-start', gap: 8, cursor: 'pointer', fontWeight: 'normal' }}>
                      <input type="checkbox" checked={pdsHostConfirmed} onChange={e => setPdsHostConfirmed(e.target.checked)} style={{ marginTop: 2, flexShrink: 0 }} />
                      <span>Normal Bluesky accounts (bsky.social) do not need a custom PDS host. Only check this if you are connecting a self-hosted PDS account.</span>
                    </label>
                  </div>
                )}
              </div>

              <div className="form-group">
                <label>Assign to Team</label>
                <select className="input" value={newAccountTeamId} onChange={e => setNewAccountTeamId(e.target.value)}>
                  <option value="">— Unassigned —</option>
                  {teams.map(t => (
                    <option key={t.id} value={t.id}>{t.name}</option>
                  ))}
                </select>
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
