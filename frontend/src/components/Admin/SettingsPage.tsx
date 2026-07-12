import { useState, useEffect } from 'react';
import { api } from '../../services/api';
import type { AppSettings } from '../../types';
import { Save, Copy, Check, Smartphone } from 'lucide-react';
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
    cookieBannerEnabled: false,
    cookieBannerText: '',
    openRouterModel: '',
    aiLanguage: '',
  });
  const [igSecret, setIgSecret] = useState('');
  const [linkedInClientSecret, setLinkedInClientSecret] = useState('');
  const [youtubeClientSecret, setYoutubeClientSecret] = useState('');
  const [openRouterKey, setOpenRouterKey] = useState('');
  const [bggApiToken, setBggApiToken] = useState('');
  const [mailgunApiKey, setMailgunApiKey] = useState('');
  const [saving, setSaving] = useState(false);
  const [linkCopied, setLinkCopied] = useState(false);

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
      data.aiLanguage = settings.aiLanguage;
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
            <label>Model</label>
            <input
              className="input"
              placeholder="openai/gpt-4o-mini"
              value={settings.openRouterModel}
              onChange={e => setSettings(s => ({ ...s, openRouterModel: e.target.value }))}
            />
            <span style={{ fontSize: 12, color: 'var(--text-muted)' }}>
              OpenRouter model identifier (e.g. <code>openai/gpt-4o-mini</code>, <code>anthropic/claude-sonnet-4</code>). See <a href="https://openrouter.ai/models" target="_blank" rel="noopener noreferrer" style={{ color: 'var(--accent)' }}>available models</a>.
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
        </div>

        <div style={{ marginTop: 28 }}>
          <button className="btn btn-primary" onClick={save} disabled={saving}>
            <Save size={16} /> {saving ? 'Saving...' : 'Save Settings'}
          </button>
        </div>
      </div>
    </div>
  );
}
