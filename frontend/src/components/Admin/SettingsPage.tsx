import { useState, useEffect } from 'react';
import { api } from '../../services/api';
import type { AppSettings, UploadSweepReport } from '../../types';
import { Save, Copy, Check, Smartphone, HardDrive, Search, Trash2, Loader } from 'lucide-react';
import toast from 'react-hot-toast';
import './Admin.css';

export function SettingsPage() {
  const [settings, setSettings] = useState<AppSettings>({
    appUrl: '',
    instagramAppId: '',
    webhookVerifyToken: '',
    adobeExpressClientId: '',
    allowSelfRegistration: true,
    imprintHtml: '',
    privacyPolicyHtml: '',
    cookieBannerEnabled: false,
    cookieBannerText: '',
    openRouterModel: '',
    openRouterVisionModel: '',
    aiLanguage: '',
    promptGameSummary: '',
    promptGameAbstract: '',
    promptHashtags: '',
    promptHandleLookup: '',
    promptSocialPost: '',
    promptDashboardInsights: '',
  });
  const [igSecret, setIgSecret] = useState('');
  const [linkedInClientSecret, setLinkedInClientSecret] = useState('');
  const [youtubeClientSecret, setYoutubeClientSecret] = useState('');
  const [openRouterKey, setOpenRouterKey] = useState('');
  const [bggApiToken, setBggApiToken] = useState('');
  const [mailgunApiKey, setMailgunApiKey] = useState('');
  const [saving, setSaving] = useState(false);
  const [linkCopied, setLinkCopied] = useState(false);

  // Storage maintenance
  const [sweepMinAgeHours, setSweepMinAgeHours] = useState('24');
  const [sweepReport, setSweepReport] = useState<UploadSweepReport | null>(null);
  const [scanning, setScanning] = useState(false);
  const [sweeping, setSweeping] = useState(false);

  const mobileCreateUrl = `${(settings.appUrl || window.location.origin).replace(/\/+$/, '')}/m/create`;

  const copyMobileCreateLink = async () => {
    try {
      await navigator.clipboard.writeText(mobileCreateUrl);
      setLinkCopied(true);
      setTimeout(() => setLinkCopied(false), 2000);
    } catch {
      toast.error('Could not copy link');
    }
  };

  useEffect(() => {
    api.getSettings().then(setSettings).catch(() => {});
  }, []);

  const minAgeHours = () => {
    const parsed = parseInt(sweepMinAgeHours, 10);
    return Number.isFinite(parsed) && parsed >= 0 ? parsed : 24;
  };

  const formatBytes = (bytes: number) => {
    if (bytes < 1024) return `${bytes} B`;
    if (bytes < 1024 * 1024) return `${(bytes / 1024).toFixed(1)} KB`;
    if (bytes < 1024 * 1024 * 1024) return `${(bytes / (1024 * 1024)).toFixed(1)} MB`;
    return `${(bytes / (1024 * 1024 * 1024)).toFixed(2)} GB`;
  };

  const scanStorage = async () => {
    setScanning(true);
    try {
      setSweepReport(await api.scanOrphanedUploads(minAgeHours()));
    } catch (err: any) {
      toast.error(err.message || 'Scan failed');
    } finally {
      setScanning(false);
    }
  };

  const sweepStorage = async () => {
    if (!sweepReport || sweepReport.orphanedFiles === 0) return;
    const what = `${sweepReport.orphanedFiles} file${sweepReport.orphanedFiles === 1 ? '' : 's'} (${formatBytes(sweepReport.orphanedBytes)})`;
    if (!window.confirm(`Permanently delete ${what}? This cannot be undone.`)) return;
    setSweeping(true);
    try {
      const report = await api.sweepOrphanedUploads(minAgeHours());
      setSweepReport(report);
      toast.success(`Deleted ${report.deletedFiles} file${report.deletedFiles === 1 ? '' : 's'}, freed ${formatBytes(report.deletedBytes)}`);
    } catch (err: any) {
      toast.error(err.message || 'Sweep failed');
    } finally {
      setSweeping(false);
    }
  };

  const save = async () => {
    setSaving(true);
    try {
      const data: any = {
        appUrl: settings.appUrl,
        instagramAppId: settings.instagramAppId,
        allowSelfRegistration: settings.allowSelfRegistration,
        webhookVerifyToken: settings.webhookVerifyToken,
        adobeExpressClientId: settings.adobeExpressClientId,
        imprintHtml: settings.imprintHtml,
        privacyPolicyHtml: settings.privacyPolicyHtml,
        cookieBannerEnabled: settings.cookieBannerEnabled,
        cookieBannerText: settings.cookieBannerText,
        linkedInClientId: settings.linkedInClientId || '',
      };
      if (igSecret) data.instagramAppSecret = igSecret;
      if (linkedInClientSecret) data.linkedInClientSecret = linkedInClientSecret;
      if (youtubeClientSecret) data.youtubeClientSecret = youtubeClientSecret;
      data.youtubeClientId = settings.youtubeClientId || '';
      if (openRouterKey) data.openRouterApiKey = openRouterKey;
      if (bggApiToken) data.bggApiToken = bggApiToken;
      if (mailgunApiKey) data.mailgunApiKey = mailgunApiKey;
      data.mailgunBaseUrl = settings.mailgunBaseUrl || '';
      data.mailgunDomain = settings.mailgunDomain || '';
      data.mailgunFromEmail = settings.mailgunFromEmail || '';
      data.openRouterModel = settings.openRouterModel;
      data.openRouterVisionModel = settings.openRouterVisionModel;
      data.aiLanguage = settings.aiLanguage;
      data.promptGameSummary = settings.promptGameSummary || '';
      data.promptGameAbstract = settings.promptGameAbstract || '';
      data.promptHashtags = settings.promptHashtags || '';
      data.promptHandleLookup = settings.promptHandleLookup || '';
      data.promptSocialPost = settings.promptSocialPost || '';
      data.promptDashboardInsights = settings.promptDashboardInsights || '';
      const updated = await api.updateSettings(data);
      setSettings(updated);
      toast.success('Settings saved');
    } catch (err: any) {
      toast.error(err.message);
    } finally {
      setSaving(false);
    }
  };

  return (
    <div className="page">
      <div className="page-header">
        <h1>Settings</h1>
      </div>

      <div className="card settings-card">
        <h3>General</h3>

        <div className="settings-grid">
          <div className="form-group">
            <label>Application URL</label>
            <input
              className="input"
              placeholder="https://your-domain.com"
              value={settings.appUrl}
              onChange={e => setSettings(s => ({ ...s, appUrl: e.target.value }))}
            />
            <span style={{ fontSize: 12, color: 'var(--text-muted)' }}>
              Public URL where this app is reachable (used for Instagram OAuth callback)
            </span>
          </div>

          <div className="form-group">
            <label style={{ display: 'flex', alignItems: 'center', gap: 10, cursor: 'pointer' }}>
              <input
                type="checkbox"
                checked={settings.allowSelfRegistration}
                onChange={e => setSettings(s => ({ ...s, allowSelfRegistration: e.target.checked }))}
              />
              Allow self-registration
            </label>
            <span style={{ fontSize: 12, color: 'var(--text-muted)' }}>
              When disabled, only admins can create new user accounts
            </span>
          </div>
        </div>

        <hr style={{ border: 'none', borderTop: '1px solid var(--border)', margin: '28px 0' }} />

        <h3>Mobile Quick Post</h3>

        <div className="settings-grid">
          <div className="form-group">
            <label style={{ display: 'flex', alignItems: 'center', gap: 6 }}>
              <Smartphone size={14} /> Phone create link
            </label>
            <div style={{ display: 'flex', gap: 8, alignItems: 'center', flexWrap: 'wrap' }}>
              <input className="input" readOnly value={mobileCreateUrl} style={{ flex: 1, minWidth: 220 }} onFocus={e => e.currentTarget.select()} />
              <button type="button" className="btn btn-secondary" onClick={copyMobileCreateLink}>
                {linkCopied ? <Check size={14} /> : <Copy size={14} />}
                {linkCopied ? 'Copied' : 'Copy'}
              </button>
            </div>
            <span style={{ fontSize: 12, color: 'var(--text-muted)' }}>
              A phone-friendly page (no sidebar) for quickly creating a post, story or reel. Open it on your phone and use "Add to Home Screen" to pin it as a shortcut. Uses the Application URL above when set, otherwise the current address.
            </span>
          </div>
        </div>

        <hr style={{ border: 'none', borderTop: '1px solid var(--border)', margin: '28px 0' }} />

        <h3>Instagram Standalone App</h3>

        <div className="settings-grid">
          <div className="form-group">
            <label>Instagram App ID</label>
            <input
              className="input"
              placeholder="Your Instagram App ID"
              value={settings.instagramAppId}
              onChange={e => setSettings(s => ({ ...s, instagramAppId: e.target.value }))}
            />
          </div>

          <div className="form-group">
            <label>Instagram App Secret</label>
            <input
              className="input"
              type="password"
              placeholder="Enter to update (hidden for security)"
              value={igSecret}
              onChange={e => setIgSecret(e.target.value)}
            />
          </div>

          <div className="form-group">
            <label>Webhook Verify Token</label>
            <input
              className="input"
              placeholder="Token for Instagram webhook verification"
              value={settings.webhookVerifyToken}
              onChange={e => setSettings(s => ({ ...s, webhookVerifyToken: e.target.value }))}
            />
            <span style={{ fontSize: 12, color: 'var(--text-muted)' }}>
              Enter any secret string here, then use the same value when configuring the webhook callback URL in the Meta App Dashboard. Webhook URL: <code>{settings.appUrl}/api/webhooks/instagram</code>
            </span>
          </div>
        </div>

        <hr style={{ border: 'none', borderTop: '1px solid var(--border)', margin: '28px 0' }} />

        <h3>LinkedIn App</h3>

        <div className="settings-grid">
          <div className="form-group">
            <label>LinkedIn Client ID</label>
            <input
              className="input"
              placeholder="Your LinkedIn App Client ID"
              value={settings.linkedInClientId || ''}
              onChange={e => setSettings(s => ({ ...s, linkedInClientId: e.target.value }))}
            />
          </div>

          <div className="form-group">
            <label>LinkedIn Client Secret</label>
            <input
              className="input"
              type="password"
              placeholder={settings.hasLinkedInClientSecret ? 'Secret configured (enter to update)' : 'Enter to set'}
              value={linkedInClientSecret}
              onChange={e => setLinkedInClientSecret(e.target.value)}
            />
            <span style={{ fontSize: 12, color: 'var(--text-muted)' }}>
              Create an app at the LinkedIn Developer Portal. Set the redirect URL to <code>{settings.appUrl}/api/auth/linkedin/callback</code>. Required scopes: <code>openid profile w_member_social</code>.
            </span>
          </div>
        </div>

        <hr style={{ border: 'none', borderTop: '1px solid var(--border)', margin: '28px 0' }} />

        <h3>YouTube App</h3>

        <div className="settings-grid">
          <div className="form-group">
            <label>YouTube Client ID</label>
            <input
              className="input"
              placeholder="Your Google Cloud OAuth Client ID"
              value={settings.youtubeClientId || ''}
              onChange={e => setSettings(s => ({ ...s, youtubeClientId: e.target.value }))}
            />
          </div>

          <div className="form-group">
            <label>YouTube Client Secret</label>
            <input
              className="input"
              type="password"
              placeholder={settings.hasYouTubeClientSecret ? 'Secret configured (enter to update)' : 'Enter to set'}
              value={youtubeClientSecret}
              onChange={e => setYoutubeClientSecret(e.target.value)}
            />
            <span style={{ fontSize: 12, color: 'var(--text-muted)' }}>
              Create a project in the Google Cloud Console, enable the YouTube Data API v3, and create OAuth 2.0 credentials. Set the redirect URL to <code>{settings.appUrl}/api/auth/youtube/callback</code>. Required scopes: <code>youtube.upload youtube.readonly</code>.
            </span>
          </div>
        </div>

        <hr style={{ border: 'none', borderTop: '1px solid var(--border)', margin: '28px 0' }} />

        <h3>Adobe Express</h3>

        <div className="settings-grid">
          <div className="form-group">
            <label>Adobe Express Client ID</label>
            <input
              className="input"
              placeholder="Your Adobe Express Embed SDK Client ID"
              value={settings.adobeExpressClientId}
              onChange={e => setSettings(s => ({ ...s, adobeExpressClientId: e.target.value }))}
            />
            <span style={{ fontSize: 12, color: 'var(--text-muted)' }}>
              Get a Client ID from the <a href="https://developer.adobe.com/express/embed-sdk/" target="_blank" rel="noopener noreferrer" style={{ color: 'var(--accent)' }}>Adobe Developer Console</a>. Enables in-app image creation when composing posts.
            </span>
          </div>
        </div>

        <hr style={{ border: 'none', borderTop: '1px solid var(--border)', margin: '28px 0' }} />

        <h3>BoardGameGeek (BGG) Import</h3>

        <div className="settings-grid">
          <div className="form-group">
            <label>BGG API Token</label>
            <input
              className="input"
              type="password"
              placeholder={settings.hasBggApiToken ? 'Token configured (enter to update)' : 'Enter your BGG API token (optional)'}
              value={bggApiToken}
              onChange={e => setBggApiToken(e.target.value)}
            />
            <span style={{ fontSize: 12, color: 'var(--text-muted)' }}>
              Optional. Register your app at{' '}
              <a href="https://boardgamegeek.com/applications" target="_blank" rel="noopener noreferrer" style={{ color: 'var(--accent)' }}>boardgamegeek.com/applications</a>.
            </span>
          </div>
        </div>

        <hr style={{ border: 'none', borderTop: '1px solid var(--border)', margin: '28px 0' }} />

        <h3>AI Text Generation (OpenRouter)</h3>

        <div className="settings-grid">
          <div className="form-group">
            <label>OpenRouter API Key</label>
            <input
              className="input"
              type="password"
              placeholder={settings.hasOpenRouterKey ? 'Key configured (enter to update)' : 'Enter your OpenRouter API key'}
              value={openRouterKey}
              onChange={e => setOpenRouterKey(e.target.value)}
            />
            <span style={{ fontSize: 12, color: 'var(--text-muted)' }}>
              Get an API key from <a href="https://openrouter.ai/keys" target="_blank" rel="noopener noreferrer" style={{ color: 'var(--accent)' }}>openrouter.ai/keys</a>. Enables AI text generation in the post editor.
            </span>
          </div>

          <div className="form-group">
            <label>Text Model</label>
            <input
              className="input"
              placeholder="openai/gpt-4o-mini"
              value={settings.openRouterModel}
              onChange={e => setSettings(s => ({ ...s, openRouterModel: e.target.value }))}
            />
            <span style={{ fontSize: 12, color: 'var(--text-muted)' }}>
              Model for text generation (drafts, captions, summaries). See <a href="https://openrouter.ai/models" target="_blank" rel="noopener noreferrer" style={{ color: 'var(--accent)' }}>available models</a>.
            </span>
          </div>

          <div className="form-group">
            <label>Vision Model</label>
            <input
              className="input"
              placeholder="anthropic/claude-haiku-4-5"
              value={settings.openRouterVisionModel}
              onChange={e => setSettings(s => ({ ...s, openRouterVisionModel: e.target.value }))}
            />
            <span style={{ fontSize: 12, color: 'var(--text-muted)' }}>
              Model for image analysis (picking article images, convention photo captions). Must support vision.
            </span>
          </div>

          <div className="form-group">
            <label>AI Output Language</label>
            <select
              className="input"
              value={settings.aiLanguage}
              onChange={e => setSettings(s => ({ ...s, aiLanguage: e.target.value }))}
            >
              <option value="">English (default)</option>
              <option value="German">German</option>
              <option value="French">French</option>
              <option value="Spanish">Spanish</option>
              <option value="Italian">Italian</option>
              <option value="Dutch">Dutch</option>
              <option value="Portuguese">Portuguese</option>
              <option value="Brazilian Portuguese">Brazilian Portuguese</option>
              <option value="Japanese">Japanese</option>
              <option value="Korean">Korean</option>
              <option value="Chinese">Chinese</option>
              <option value="Arabic">Arabic</option>
              <option value="Polish">Polish</option>
              <option value="Swedish">Swedish</option>
              <option value="Norwegian">Norwegian</option>
              <option value="Danish">Danish</option>
              <option value="Finnish">Finnish</option>
            </select>
            <span style={{ fontSize: 12, color: 'var(--text-muted)' }}>
              Language for all AI-generated text (captions, post copy, dashboard insights).
            </span>
          </div>

        </div>

        <hr style={{ border: 'none', borderTop: '1px solid var(--border)', margin: '28px 0' }} />

        <h3>AI Prompt Templates</h3>
        <p style={{ fontSize: 12, color: 'var(--text-muted)', margin: '-8px 0 16px' }}>
          Customize the system prompts used for AI-generated content. Leave blank to use the built-in defaults.
        </p>

        <div className="settings-grid">
          <div className="form-group">
            <label>BGG Game Abstract (Shownotes)</label>
            <textarea
              className="textarea"
              rows={3}
              placeholder="You are a board game expert writing quick abstracts for podcast shownotes..."
              value={settings.promptGameAbstract || ''}
              onChange={e => setSettings(s => ({ ...s, promptGameAbstract: e.target.value }))}
              style={{ fontFamily: 'monospace', fontSize: 13 }}
            />
            <span style={{ fontSize: 12, color: 'var(--text-muted)' }}>
              Generates a 1-2 line abstract prepended to the shownotes when importing from BGG.
            </span>
          </div>

          <div className="form-group">
            <label>BGG Game Summary (Social Post)</label>
            <textarea
              className="textarea"
              rows={3}
              placeholder="You are a board game expert. Write exactly one sentence..."
              value={settings.promptGameSummary || ''}
              onChange={e => setSettings(s => ({ ...s, promptGameSummary: e.target.value }))}
              style={{ fontFamily: 'monospace', fontSize: 13 }}
            />
            <span style={{ fontSize: 12, color: 'var(--text-muted)' }}>
              Generates a one-sentence summary included in the suggested social media post text.
            </span>
          </div>

          <div className="form-group">
            <label>Social Media Post</label>
            <textarea
              className="textarea"
              rows={3}
              placeholder="You are a social media copywriter..."
              value={settings.promptSocialPost || ''}
              onChange={e => setSettings(s => ({ ...s, promptSocialPost: e.target.value }))}
              style={{ fontFamily: 'monospace', fontSize: 13 }}
            />
            <span style={{ fontSize: 12, color: 'var(--text-muted)' }}>
              Used when generating social media post text with the "Generate with AI" button.
            </span>
          </div>

          <div className="form-group">
            <label>Hashtag Generation</label>
            <textarea
              className="textarea"
              rows={3}
              placeholder="You are a social media expert for board game content..."
              value={settings.promptHashtags || ''}
              onChange={e => setSettings(s => ({ ...s, promptHashtags: e.target.value }))}
              style={{ fontFamily: 'monospace', fontSize: 13 }}
            />
            <span style={{ fontSize: 12, color: 'var(--text-muted)' }}>
              Generates hashtag suggestions when importing a game from BGG.
            </span>
          </div>

          <div className="form-group">
            <label>Handle Lookup</label>
            <textarea
              className="textarea"
              rows={3}
              placeholder="You are a board game industry expert..."
              value={settings.promptHandleLookup || ''}
              onChange={e => setSettings(s => ({ ...s, promptHandleLookup: e.target.value }))}
              style={{ fontFamily: 'monospace', fontSize: 13 }}
            />
            <span style={{ fontSize: 12, color: 'var(--text-muted)' }}>
              Looks up social media handles for publishers, designers, and artists.
            </span>
          </div>

          <div className="form-group">
            <label>Dashboard Insights</label>
            <textarea
              className="textarea"
              rows={3}
              placeholder="You are a social media strategy analyst..."
              value={settings.promptDashboardInsights || ''}
              onChange={e => setSettings(s => ({ ...s, promptDashboardInsights: e.target.value }))}
              style={{ fontFamily: 'monospace', fontSize: 13 }}
            />
            <span style={{ fontSize: 12, color: 'var(--text-muted)' }}>
              Analyzes posting data and generates recommendations on the admin dashboard.
            </span>
          </div>
        </div>

        <hr style={{ border: 'none', borderTop: '1px solid var(--border)', margin: '28px 0' }} />

        <h3>Mailgun (Email)</h3>

        <div className="settings-grid">
          <div className="form-group">
            <label>Mailgun API Base URL</label>
            <input
              className="input"
              placeholder="https://api.mailgun.net"
              value={settings.mailgunBaseUrl || ''}
              onChange={e => setSettings(s => ({ ...s, mailgunBaseUrl: e.target.value }))}
            />
            <span style={{ fontSize: 12, color: 'var(--text-muted)' }}>
              Defaults to <code>https://api.mailgun.net</code>. Use <code>https://api.eu.mailgun.net</code> for EU regions.
            </span>
          </div>

          <div className="form-group">
            <label>Mailgun Domain</label>
            <input
              className="input"
              placeholder="mg.your-domain.com"
              value={settings.mailgunDomain || ''}
              onChange={e => setSettings(s => ({ ...s, mailgunDomain: e.target.value }))}
            />
            <span style={{ fontSize: 12, color: 'var(--text-muted)' }}>
              Your verified Mailgun sending domain.
            </span>
          </div>

          <div className="form-group">
            <label>Mailgun API Key</label>
            <input
              className="input"
              type="password"
              placeholder={settings.hasMailgunApiKey ? 'Key configured (enter to update)' : 'Enter your Mailgun API key'}
              value={mailgunApiKey}
              onChange={e => setMailgunApiKey(e.target.value)}
            />
          </div>

          <div className="form-group">
            <label>From Email Address</label>
            <input
              className="input"
              placeholder="noreply@mg.your-domain.com"
              value={settings.mailgunFromEmail || ''}
              onChange={e => setSettings(s => ({ ...s, mailgunFromEmail: e.target.value }))}
            />
            <span style={{ fontSize: 12, color: 'var(--text-muted)' }}>
              Sender address for outgoing emails (team invitations). Defaults to <code>noreply@{'{'}domain{'}'}</code>
            </span>
          </div>
        </div>

        <hr style={{ border: 'none', borderTop: '1px solid var(--border)', margin: '28px 0' }} />

        <h3>Legal</h3>

        <div className="settings-grid">
          <div className="form-group">
            <label style={{ display: 'flex', alignItems: 'center', gap: 10, cursor: 'pointer' }}>
              <input
                type="checkbox"
                checked={settings.cookieBannerEnabled}
                onChange={e => setSettings(s => ({ ...s, cookieBannerEnabled: e.target.checked }))}
              />
              Show cookie consent banner on login page
            </label>
          </div>

          <div className="form-group">
            <label>Cookie Banner Text</label>
            <input
              className="input"
              placeholder="We use cookies to improve your experience."
              value={settings.cookieBannerText}
              onChange={e => setSettings(s => ({ ...s, cookieBannerText: e.target.value }))}
            />
            <span style={{ fontSize: 12, color: 'var(--text-muted)' }}>
              Displayed in the cookie consent banner. Leave blank to use the default text.
            </span>
          </div>

          <div className="form-group">
            <label>Imprint / Legal Notice (HTML)</label>
            <textarea
              className="textarea"
              rows={8}
              placeholder={'<p>Company Name<br>Street Address<br>City, Country</p>\n<p>Email: contact@example.com</p>'}
              value={settings.imprintHtml}
              onChange={e => setSettings(s => ({ ...s, imprintHtml: e.target.value }))}
              style={{ fontFamily: 'monospace', fontSize: 13 }}
            />
            <span style={{ fontSize: 12, color: 'var(--text-muted)' }}>
              HTML content for the imprint / legal notice. A link to view it will appear on the login page when this is non-empty.
            </span>
          </div>

          <div className="form-group">
            <label>Privacy Policy (HTML)</label>
            <textarea
              className="textarea"
              rows={8}
              placeholder={'<h2>Privacy Policy</h2>\n<p>Your privacy policy content here...</p>'}
              value={settings.privacyPolicyHtml}
              onChange={e => setSettings(s => ({ ...s, privacyPolicyHtml: e.target.value }))}
              style={{ fontFamily: 'monospace', fontSize: 13 }}
            />
            <span style={{ fontSize: 12, color: 'var(--text-muted)' }}>
              HTML content for the privacy policy. When non-empty, a link appears on the login page and a public page is available at <code>/privacy</code>.
            </span>
          </div>
        </div>

        <div style={{ marginTop: 28 }}>
          <button className="btn btn-primary" onClick={save} disabled={saving}>
            <Save size={16} /> {saving ? 'Saving...' : 'Save Settings'}
          </button>
        </div>
      </div>

      <div className="card settings-card">
        <h3 style={{ display: 'flex', alignItems: 'center', gap: 8 }}>
          <HardDrive size={16} /> Storage
        </h3>
        <p style={{ fontSize: 13, color: 'var(--text-muted)', marginTop: 0 }}>
          Uploaded images are freed automatically when the post, draft, watermark or queue item
          holding them is deleted. This sweep clears what is left over: images from entries deleted
          before that existed, images replaced while editing, and imports abandoned before anything
          referenced them. An image any entry still uses is never touched.
        </p>

        <div style={{ display: 'flex', gap: 12, alignItems: 'flex-end', flexWrap: 'wrap', marginTop: 16 }}>
          <div className="form-group" style={{ maxWidth: 220 }}>
            <label>Keep uploads newer than</label>
            <div style={{ display: 'flex', alignItems: 'center', gap: 8 }}>
              <input
                className="input"
                type="number"
                min={0}
                value={sweepMinAgeHours}
                onChange={e => setSweepMinAgeHours(e.target.value)}
                style={{ width: 90 }}
              />
              <span style={{ fontSize: 13, color: 'var(--text-muted)' }}>hours</span>
            </div>
            <span style={{ fontSize: 12, color: 'var(--text-muted)' }}>
              Protects images an editor that is still open has uploaded but not saved yet.
            </span>
          </div>

          <button className="btn btn-secondary" onClick={scanStorage} disabled={scanning || sweeping} style={{ marginBottom: 20 }}>
            {scanning ? <><Loader size={14} className="admin-spin" /> Scanning...</> : <><Search size={14} /> Scan</>}
          </button>

          {sweepReport && sweepReport.orphanedFiles > 0 && (
            <button className="btn btn-danger" onClick={sweepStorage} disabled={scanning || sweeping} style={{ marginBottom: 20 }}>
              {sweeping
                ? <><Loader size={14} className="admin-spin" /> Deleting...</>
                : <><Trash2 size={14} /> Delete {sweepReport.orphanedFiles} orphaned file{sweepReport.orphanedFiles === 1 ? '' : 's'} ({formatBytes(sweepReport.orphanedBytes)})</>}
            </button>
          )}
        </div>

        {sweepReport && (
          <div style={{ marginTop: 8, fontSize: 13, display: 'flex', flexDirection: 'column', gap: 6 }}>
            <div>
              <strong>{sweepReport.totalFiles}</strong> uploads stored ({formatBytes(sweepReport.totalBytes)}) —{' '}
              <strong>{sweepReport.orphanedFiles}</strong> orphaned ({formatBytes(sweepReport.orphanedBytes)})
              {sweepReport.skippedTooRecent > 0 && (
                <span style={{ color: 'var(--text-muted)' }}>
                  {' '}· {sweepReport.skippedTooRecent} unreferenced but too recent to sweep
                </span>
              )}
            </div>
            {!sweepReport.dryRun && (
              <div style={{ color: 'var(--success, #22c55e)' }}>
                Deleted {sweepReport.deletedFiles} file{sweepReport.deletedFiles === 1 ? '' : 's'}, freed {formatBytes(sweepReport.deletedBytes)}.
              </div>
            )}
            {sweepReport.dryRun && sweepReport.orphanedFiles === 0 && (
              <div style={{ color: 'var(--text-muted)' }}>Nothing to clean up.</div>
            )}
            {sweepReport.sample.length > 0 && (
              <details>
                <summary style={{ cursor: 'pointer', color: 'var(--text-muted)' }}>
                  Sample of {sweepReport.sample.length} orphaned file{sweepReport.sample.length === 1 ? '' : 's'} (newest first)
                </summary>
                <ul style={{ margin: '8px 0 0', paddingLeft: 18, color: 'var(--text-muted)', fontSize: 12, maxHeight: 220, overflowY: 'auto' }}>
                  {sweepReport.sample.map(f => (
                    <li key={f.filename}>
                      {f.filename} — {formatBytes(f.size)}, {new Date(f.createdAt).toLocaleDateString()}
                      {!f.inDatabase && ' (file only, no database record)'}
                      {!f.onDisk && ' (database record only, file missing)'}
                    </li>
                  ))}
                </ul>
              </details>
            )}
            {sweepReport.errors && sweepReport.errors.length > 0 && (
              <ul style={{ margin: 0, paddingLeft: 18, color: 'var(--danger)', fontSize: 12 }}>
                {sweepReport.errors.map((e, i) => <li key={i}>{e}</li>)}
              </ul>
            )}
          </div>
        )}
      </div>
    </div>
  );
}
