import { useState, useEffect } from 'react';
import { api } from '../../services/api';
import type { AppSettings } from '../../types';
import { Save } from 'lucide-react';
import toast from 'react-hot-toast';
import './Admin.css';

export function SettingsPage() {
  const [settings, setSettings] = useState<AppSettings>({
    appUrl: '',
    instagramAppId: '',
    defaultPostTime: '09:00',
    autoPublish: false,
  });
  const [igSecret, setIgSecret] = useState('');
  const [saving, setSaving] = useState(false);

  useEffect(() => {
    api.getSettings().then(setSettings).catch(() => {});
  }, []);

  const save = async () => {
    setSaving(true);
    try {
      const data: any = {
        appUrl: settings.appUrl,
        instagramAppId: settings.instagramAppId,
        defaultPostTime: settings.defaultPostTime,
        autoPublish: settings.autoPublish,
      };
      if (igSecret) data.instagramAppSecret = igSecret;
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
            <label>Default Post Time</label>
            <input
              type="time"
              className="input"
              value={settings.defaultPostTime}
              onChange={e => setSettings(s => ({ ...s, defaultPostTime: e.target.value }))}
            />
          </div>

          <div className="form-group">
            <label style={{ display: 'flex', alignItems: 'center', gap: 10, cursor: 'pointer' }}>
              <input
                type="checkbox"
                checked={settings.autoPublish}
                onChange={e => setSettings(s => ({ ...s, autoPublish: e.target.checked }))}
              />
              Auto-publish scheduled posts
            </label>
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
