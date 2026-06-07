import { useState, useEffect } from 'react';
import { useSearchParams } from 'react-router-dom';
import { api } from '../../services/api';
import type { SocialAccount, User, PublicSettings, TeamSettings } from '../../types';
import {
  Plus, Trash2, ToggleLeft, ToggleRight, ExternalLink, X, Users, Share2, Settings,
} from 'lucide-react';
import { PlatformIcon } from '../Common/PlatformIcon';
import { format } from 'date-fns';
import toast from 'react-hot-toast';
import { useAuth } from '../../contexts/AuthContext';
import '../Admin/Admin.css';

type Tab = 'accounts' | 'members' | 'settings';

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
  const [pdsHostConfirmed, setPdsHostConfirmed] = useState(false);
  const [addingAccount, setAddingAccount] = useState(false);
  const [showTwitterForm, setShowTwitterForm] = useState(false);
  const [twConsumerKey, setTwConsumerKey] = useState('');
  const [twConsumerSecret, setTwConsumerSecret] = useState('');
  const [twAccessToken, setTwAccessToken] = useState('');
  const [twAccessTokenSecret, setTwAccessTokenSecret] = useState('');
  const [addingTwitter, setAddingTwitter] = useState(false);
  const [showMastodonForm, setShowMastodonForm] = useState(false);
  const [mastodonInstance, setMastodonInstance] = useState('');
  const [mastodonAccessToken, setMastodonAccessToken] = useState('');
  const [addingMastodon, setAddingMastodon] = useState(false);
  const [showThreadsForm, setShowThreadsForm] = useState(false);
  const [threadsAccessToken, setThreadsAccessToken] = useState('');
  const [addingThreads, setAddingThreads] = useState(false);
  const [connectingLinkedIn, setConnectingLinkedIn] = useState(false);
  const [connectingYouTube, setConnectingYouTube] = useState(false);
  const [youtubeConfigured, setYoutubeConfigured] = useState(false);

  // Members state
  const [members, setMembers] = useState<User[]>([]);
  const [showMemberForm, setShowMemberForm] = useState(false);
  const [memberEmail, setMemberEmail] = useState('');
  const [addingMember, setAddingMember] = useState(false);

  // Team settings state
  const [teamSettings, setTeamSettings] = useState<TeamSettings>({});
  const [savingSettings, setSavingSettings] = useState(false);

  // News Creator settings state
  const [newsCreatorUrl, setNewsCreatorUrl] = useState('');
  const [newsCreatorBearerToken, setNewsCreatorBearerToken] = useState('');

  // Team self-creation state (when team admin has no team yet)
  const [newTeamName, setNewTeamName] = useState('');
  const [creatingTeam, setCreatingTeam] = useState(false);


  useEffect(() => {
    if (!user?.teamId) return;
    loadAccounts();
    loadMembers();
    api.getTeamSettings().then(s => {
      setTeamSettings(s);
      if (s.newsCreatorUrl) setNewsCreatorUrl(s.newsCreatorUrl);
    }).catch(() => {});
    if (searchParams.get('instagram') === 'connected') {
      toast.success('Instagram account connected');
    }
    if (searchParams.get('mastodon') === 'connected') {
      toast.success('Mastodon account connected');
    }
    if (searchParams.get('linkedin') === 'connected') {
      toast.success('LinkedIn account connected');
    }
    if (searchParams.get('youtube') === 'connected') {
      toast.success('YouTube account connected');
    }
    api.getPublicSettings().then((s: PublicSettings) => {
      setYoutubeConfigured(s.youtubeConfigured);
    }).catch(() => {});
    if (searchParams.get('error')) {
      toast.error('Connection failed: ' + searchParams.get('error'));
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
    if (pdsHost && !pdsHostConfirmed) { toast.error('Please confirm the custom PDS host before adding the account'); return; }
    setAddingAccount(true);
    try {
      await api.addTeamBlueskyAccount({ handle, appPassword, pdsHost: pdsHost || undefined });
      toast.success('Bluesky account added');
      setShowBlueskyForm(false);
      setHandle(''); setAppPassword(''); setPdsHost(''); setPdsHostConfirmed(false);
      loadAccounts();
    } catch (err: any) {
      toast.error(err.message);
    } finally {
      setAddingAccount(false);
    }
  };

  const addTwitter = async () => {
    if (!twConsumerKey || !twConsumerSecret || !twAccessToken || !twAccessTokenSecret) {
      toast.error('All Twitter API credentials are required');
      return;
    }
    setAddingTwitter(true);
    try {
      await api.addTeamTwitterAccount({
        consumerKey: twConsumerKey,
        consumerSecret: twConsumerSecret,
        accessToken: twAccessToken,
        accessTokenSecret: twAccessTokenSecret,
      });
      toast.success('X/Twitter account added');
      setShowTwitterForm(false);
      setTwConsumerKey(''); setTwConsumerSecret(''); setTwAccessToken(''); setTwAccessTokenSecret('');
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
      await api.addTeamMastodonAccount({ instance: mastodonInstance, accessToken: mastodonAccessToken });
      toast.success('Mastodon account added');
      setShowMastodonForm(false);
      setMastodonInstance(''); setMastodonAccessToken('');
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
      await api.addTeamThreadsAccount({ accessToken: threadsAccessToken });
      toast.success('Threads account added');
      setShowThreadsForm(false);
      setThreadsAccessToken('');
      loadAccounts();
    } catch (err: any) {
      toast.error(err.message);
    } finally {
      setAddingThreads(false);
    }
  };

  const connectLinkedIn = async () => {
    setConnectingLinkedIn(true);
    try {
      const { url } = await api.getTeamLinkedInAuthUrl();
      window.location.href = url;
    } catch (err: any) {
      toast.error(err.message);
      setConnectingLinkedIn(false);
    }
  };

  const connectMastodonOAuth = async () => {
    if (!mastodonInstance) { toast.error('Please enter your instance URL first'); return; }
    setAddingMastodon(true);
    try {
      const { url } = await api.getTeamMastodonAuthUrl(mastodonInstance);
      window.location.href = url;
    } catch (err: any) {
      toast.error(err.message);
      setAddingMastodon(false);
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

  const connectYouTube = async () => {
    setConnectingYouTube(true);
    try {
      const { url } = await api.getTeamYouTubeAuthUrl();
      window.location.href = url;
    } catch (err: any) {
      toast.error(err.message);
      setConnectingYouTube(false);
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

  const saveTeamSettings = async () => {
    setSavingSettings(true);
    try {
      const payload: any = { bggHandleLookupEnabled: teamSettings.bggHandleLookupEnabled ?? true };
      if (newsCreatorUrl !== undefined) payload.newsCreatorUrl = newsCreatorUrl;
      if (newsCreatorBearerToken) payload.newsCreatorBearerToken = newsCreatorBearerToken;
      await api.updateTeamSettings(payload);
      if (newsCreatorBearerToken) {
        setNewsCreatorBearerToken('');
        setTeamSettings(s => ({ ...s, hasNewsCreatorBearerToken: true }));
      }
      toast.success('Settings saved');
    } catch (err: any) {
      toast.error(err.message || 'Failed to save settings');
    } finally {
      setSavingSettings(false);
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
      <div style={{ display: 'flex', gap: 8, marginBottom: 24, borderBottom: '1px solid var(--border)', paddingBottom: 0, overflowX: 'auto', WebkitOverflowScrolling: 'touch' as any }}>
        {(['accounts', 'members', 'settings'] as Tab[]).map(t => (
          <button
            key={t}
            onClick={() => setTab(t)}
            style={{
              padding: '8px 12px',
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
              whiteSpace: 'nowrap',
              flexShrink: 0,
            }}
          >
            {t === 'accounts' ? <Share2 size={15} /> : t === 'members' ? <Users size={15} /> : <Settings size={15} />}
            {t === 'accounts' ? 'Social Accounts' : t === 'members' ? 'Members' : 'Settings'}
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
                onClick={() => setShowTwitterForm(true)}
                style={{ borderColor: 'var(--twitter)', color: 'var(--twitter)' }}
              >
                <Plus size={16} /> Add X/Twitter
              </button>
              <button
                className="btn btn-secondary"
                onClick={() => setShowMastodonForm(true)}
                style={{ borderColor: 'var(--mastodon)', color: 'var(--mastodon)' }}
              >
                <Plus size={16} /> Add Mastodon
              </button>
              <button
                className="btn btn-secondary"
                onClick={connectInstagram}
                style={{ borderColor: 'var(--instagram)', color: 'var(--instagram)' }}
              >
                <ExternalLink size={16} /> Connect Instagram
              </button>
              <button
                className="btn btn-secondary"
                onClick={() => setShowThreadsForm(true)}
                style={{ borderColor: 'var(--threads)', color: 'var(--threads)' }}
              >
                <Plus size={16} /> Add Threads
              </button>
              <button
                className="btn btn-secondary"
                onClick={connectLinkedIn}
                disabled={connectingLinkedIn}
                style={{ borderColor: 'var(--linkedin)', color: 'var(--linkedin)' }}
              >
                <ExternalLink size={16} /> Connect LinkedIn
              </button>
              {youtubeConfigured && (
                <button
                  className="btn btn-secondary"
                  onClick={connectYouTube}
                  disabled={connectingYouTube}
                  style={{ borderColor: 'var(--youtube)', color: 'var(--youtube)' }}
                >
                  <ExternalLink size={16} /> Connect YouTube
                </button>
              )}
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

          {showTwitterForm && (
            <div className="modal-overlay" onClick={() => setShowTwitterForm(false)}>
              <div className="modal" onClick={e => e.stopPropagation()}>
                <div style={{ display: 'flex', justifyContent: 'space-between', alignItems: 'center', marginBottom: 20 }}>
                  <h2>Add X/Twitter Account</h2>
                  <button className="btn btn-ghost btn-sm" onClick={() => setShowTwitterForm(false)}>
                    <X size={18} />
                  </button>
                </div>
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
                <div style={{ display: 'flex', justifyContent: 'space-between', alignItems: 'center', marginBottom: 20 }}>
                  <h2>Add Mastodon Account</h2>
                  <button className="btn btn-ghost btn-sm" onClick={() => setShowMastodonForm(false)}>
                    <X size={18} />
                  </button>
                </div>
                <div style={{ display: 'flex', flexDirection: 'column', gap: 16 }}>
                  <div className="form-group">
                    <label>Instance URL</label>
                    <input className="input" placeholder="mastodon.social" value={mastodonInstance} onChange={e => setMastodonInstance(e.target.value)} />
                    <span style={{ fontSize: 12, color: 'var(--text-muted)' }}>
                      Your Mastodon server, e.g. mastodon.social or fosstodon.org
                    </span>
                  </div>
                  <button className="btn btn-primary" onClick={connectMastodonOAuth} disabled={addingMastodon} style={{ borderColor: 'var(--mastodon)', backgroundColor: 'var(--mastodon)', color: '#fff' }}>
                    {addingMastodon ? 'Redirecting…' : 'Connect via OAuth'}
                  </button>
                  <details style={{ marginTop: 4 }}>
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
                <div style={{ display: 'flex', justifyContent: 'space-between', alignItems: 'center', marginBottom: 20 }}>
                  <h2>Add Threads Account</h2>
                  <button className="btn btn-ghost btn-sm" onClick={() => setShowThreadsForm(false)}>
                    <X size={18} />
                  </button>
                </div>
                <div style={{ display: 'flex', flexDirection: 'column', gap: 16 }}>
                  <div className="form-group">
                    <label>Access Token</label>
                    <input className="input" type="password" placeholder="Long-lived user access token" value={threadsAccessToken} onChange={e => setThreadsAccessToken(e.target.value)} />
                    <span style={{ fontSize: 12, color: 'var(--text-muted)' }}>
                      Obtain a long-lived token via the Meta for Developers Threads API (threads.net).
                    </span>
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

      {/* Settings Tab */}
      {tab === 'settings' && (
        <div style={{ maxWidth: 540 }}>
          <div className="card" style={{ padding: 24 }}>
            <h3 style={{ margin: '0 0 4px', fontSize: 15 }}>BGG Import</h3>
            <p style={{ color: 'var(--text-secondary)', fontSize: 13, margin: '0 0 16px' }}>
              Controls whether publisher and designer handles are automatically looked up
              when importing games from BoardGameGeek. Requires an OpenRouter API key
              configured by the admin.
            </p>

            <label style={{ display: 'flex', alignItems: 'center', gap: 10, cursor: 'pointer', marginBottom: 6 }}>
              <input
                type="checkbox"
                checked={teamSettings.bggHandleLookupEnabled ?? true}
                onChange={e => setTeamSettings(s => ({ ...s, bggHandleLookupEnabled: e.target.checked }))}
              />
              <span style={{ fontSize: 14 }}>Enable AI handle lookup for BGG imports</span>
            </label>
            <p style={{ fontSize: 12, color: 'var(--text-muted)', margin: '0 0 20px 22px' }}>
              When enabled, publisher and designer names are resolved to @handles
              (Instagram, Bluesky, etc.) and inserted into the post content automatically.
            </p>

            {teamSettings.enabledPlugins?.includes('news_creator') && (
              <>
                <div style={{ borderTop: '1px solid var(--border)', margin: '20px 0' }} />
                <h3 style={{ margin: '0 0 4px', fontSize: 15 }}>News Creator</h3>
                <p style={{ color: 'var(--text-secondary)', fontSize: 13, margin: '0 0 16px' }}>
                  Configure the n8n webhook URL and bearer token for the News Creator plugin.
                </p>

                <div className="form-group">
                  <label>N8N Webhook URL</label>
                  <input
                    className="input"
                    type="text"
                    placeholder="https://n8n.example.com/webhook/..."
                    value={newsCreatorUrl}
                    onChange={e => setNewsCreatorUrl(e.target.value)}
                  />
                </div>

                <div className="form-group">
                  <label>N8N Bearer Token</label>
                  <input
                    className="input"
                    type="password"
                    placeholder={teamSettings.hasNewsCreatorBearerToken ? '(token is set — enter new value to replace)' : 'Enter bearer token'}
                    value={newsCreatorBearerToken}
                    onChange={e => setNewsCreatorBearerToken(e.target.value)}
                  />
                  {teamSettings.hasNewsCreatorBearerToken && !newsCreatorBearerToken && (
                    <span style={{ fontSize: 12, color: 'var(--text-muted)' }}>
                      A bearer token is already configured. Leave blank to keep the current token.
                    </span>
                  )}
                </div>
              </>
            )}

            <button className="btn btn-primary" onClick={saveTeamSettings} disabled={savingSettings}>
              {savingSettings ? 'Saving…' : 'Save Settings'}
            </button>
          </div>
        </div>
      )}

    </div>
  );
}
