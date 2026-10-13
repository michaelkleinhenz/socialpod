import { useCallback, useEffect, useRef, useState } from 'react';
import { api } from '../../services/api';
import { useAuth } from '../../contexts/AuthContext';
import type { BackupConfig, BackupEntry, BackupFrequency, BackupRestoreResult } from '../../types';
import { Archive, Download, Loader, Play, RotateCcw, Save, Trash2, Upload } from 'lucide-react';
import toast from 'react-hot-toast';

const FREQUENCIES: { value: BackupFrequency; label: string }[] = [
  { value: 'off', label: 'Off (manual only)' },
  { value: 'hourly', label: 'Every hour' },
  { value: '6h', label: 'Every 6 hours' },
  { value: '12h', label: 'Every 12 hours' },
  { value: 'daily', label: 'Daily' },
  { value: 'weekly', label: 'Weekly' },
];

const KIND_LABELS: Record<string, string> = {
  scheduled: 'Scheduled',
  manual: 'Manual',
  'pre-restore': 'Before restore',
};

const RESTORE_WARNING =
  'This replaces ALL current data (users, teams, accounts, settings, posts, drafts, images) with the backup. ' +
  'A safety backup of the current state is stored first.';

const hint = { fontSize: 12, color: 'var(--text-muted)' } as const;

function formatBytes(bytes: number) {
  if (bytes < 1024) return `${bytes} B`;
  if (bytes < 1024 * 1024) return `${(bytes / 1024).toFixed(1)} KB`;
  if (bytes < 1024 * 1024 * 1024) return `${(bytes / (1024 * 1024)).toFixed(1)} MB`;
  return `${(bytes / (1024 * 1024 * 1024)).toFixed(2)} GB`;
}

function errorMessage(err: unknown, fallback: string) {
  return err instanceof Error && err.message ? err.message : fallback;
}

function formatDate(iso?: string) {
  return iso ? new Date(iso).toLocaleString() : 'never';
}

/** Full-system backup settings, the stored backups and restore, on the admin Settings page. */
export function BackupsCard() {
  const { logout } = useAuth();
  const [config, setConfig] = useState<BackupConfig | null>(null);
  const [backups, setBackups] = useState<BackupEntry[]>([]);
  const [loadError, setLoadError] = useState('');

  const [frequency, setFrequency] = useState<BackupFrequency>('off');
  const [retention, setRetention] = useState('7');
  const [passphrase, setPassphrase] = useState('');
  const [passphraseConfirm, setPassphraseConfirm] = useState('');
  const [saving, setSaving] = useState(false);

  const [busy, setBusy] = useState(false);
  const [restoreTarget, setRestoreTarget] = useState<string | null>(null);
  const [restorePassphrase, setRestorePassphrase] = useState('');

  const [uploadFile, setUploadFile] = useState<File | null>(null);
  const [uploadPassphrase, setUploadPassphrase] = useState('');
  const [uploadConfirm, setUploadConfirm] = useState(false);
  const fileInput = useRef<HTMLInputElement>(null);

  const [restoreDone, setRestoreDone] = useState<BackupRestoreResult | null>(null);

  const refresh = useCallback(async () => {
    try {
      const cfg = await api.getBackupConfig();
      setConfig(cfg);
      if (cfg.storageConfigured) setBackups(await api.listBackups());
      setLoadError('');
      return cfg;
    } catch (err) {
      setLoadError(errorMessage(err, 'Could not load the backup settings'));
      return null;
    }
  }, []);

  useEffect(() => {
    refresh().then(cfg => {
      if (cfg) {
        setFrequency(cfg.frequency);
        setRetention(String(cfg.retention));
      }
    });
  }, [refresh]);

  // While a backup runs, poll so the status and the list update by themselves.
  useEffect(() => {
    if (!config?.running) return;
    const id = window.setInterval(refresh, 2000);
    return () => window.clearInterval(id);
  }, [config?.running, refresh]);

  const saveConfig = async () => {
    const patch: { frequency: BackupFrequency; retention: number; passphrase?: string } = {
      frequency,
      retention: parseInt(retention, 10) || 0,
    };
    if (passphrase || passphraseConfirm) {
      if (passphrase !== passphraseConfirm) {
        toast.error('The passphrases do not match');
        return;
      }
      patch.passphrase = passphrase;
    }
    setSaving(true);
    try {
      setConfig(await api.updateBackupConfig(patch));
      setPassphrase('');
      setPassphraseConfirm('');
      toast.success('Backup settings saved');
    } catch (err) {
      toast.error(errorMessage(err, 'Saving failed'));
    } finally {
      setSaving(false);
    }
  };

  const runNow = async () => {
    try {
      await api.runBackup();
      setConfig(c => (c ? { ...c, running: true } : c));
      toast.success('Backup started');
    } catch (err) {
      toast.error(errorMessage(err, 'Backup failed to start'));
    }
  };

  const download = async (name: string) => {
    const id = toast.loading('Downloading backup...');
    try {
      await api.downloadBackup(name);
      toast.dismiss(id);
    } catch (err) {
      toast.error(errorMessage(err, 'Download failed'), { id });
    }
  };

  const remove = async (name: string) => {
    if (!window.confirm(`Delete ${name} permanently? This cannot be undone.`)) return;
    try {
      await api.deleteBackup(name);
      await refresh();
    } catch (err) {
      toast.error(errorMessage(err, 'Delete failed'));
    }
  };

  const restoreStored = async (name: string) => {
    setBusy(true);
    try {
      setRestoreDone(await api.restoreBackup(name, restorePassphrase || undefined));
      setRestoreTarget(null);
    } catch (err) {
      toast.error(errorMessage(err, 'Restore failed'));
    } finally {
      setBusy(false);
      setRestorePassphrase('');
    }
  };

  const restoreUpload = async () => {
    if (!uploadFile) return;
    setBusy(true);
    try {
      setRestoreDone(await api.restoreBackupUpload(uploadFile, uploadPassphrase));
      setUploadFile(null);
      setUploadPassphrase('');
      setUploadConfirm(false);
      if (fileInput.current) fileInput.current.value = '';
    } catch (err) {
      toast.error(errorMessage(err, 'Restore failed'));
    } finally {
      setBusy(false);
    }
  };

  const title = (
    <h3 style={{ display: 'flex', alignItems: 'center', gap: 8 }}>
      <Archive size={16} /> Backups
    </h3>
  );

  if (restoreDone) {
    return (
      <div className="card settings-card">
        {title}
        <p style={{ fontSize: 14, marginTop: 0 }}><strong>Restore complete.</strong></p>
        <p style={{ fontSize: 13, color: 'var(--text-muted)' }}>
          All data has been replaced with the backup. Users and passwords are now those from the backup, so please log in again.
        </p>
        {restoreDone.safetyBackup && (
          <p style={{ fontSize: 13, color: 'var(--text-muted)' }}>
            The previous state was saved as <code>{restoreDone.safetyBackup}</code>.
          </p>
        )}
        <button className="btn btn-primary" onClick={logout}>Log in again</button>
      </div>
    );
  }

  if (!config) {
    return (
      <div className="card settings-card">
        {title}
        {loadError ? <p style={{ fontSize: 13, color: 'var(--danger)' }}>{loadError}</p> : <Loader size={16} className="admin-spin" />}
      </div>
    );
  }

  const canBackup = config.storageConfigured && config.passphraseSet;
  const inProgress = busy || config.running;

  return (
    <div className="card settings-card">
      {title}
      <p style={{ fontSize: 13, color: 'var(--text-muted)', marginTop: 0 }}>
        A backup holds everything: all users (with password hashes and API tokens), teams, connected social accounts,
        settings and secrets, posts, drafts, queues, agents and every uploaded image, video and font. It is stored as an
        AES-256 encrypted ZIP under <code>backups/</code> in the S3 bucket. Server environment settings (JWT secret,
        MongoDB and S3 credentials) are not part of it.
      </p>
      {!config.storageConfigured && (
        <p style={{ fontSize: 13, color: 'var(--danger)' }}>
          No S3 bucket is configured. Set <code>AWS_S3_BUCKET_NAME</code>, <code>AWS_ACCESS_KEY_ID</code> and{' '}
          <code>AWS_SECRET_ACCESS_KEY</code> (plus <code>AWS_ENDPOINT_URL</code> and <code>AWS_DEFAULT_REGION</code> for
          non-AWS storage) on the server to store backups. Restoring an uploaded file works without it.
        </p>
      )}

      <div className="settings-grid">
        <div style={{ display: 'flex', gap: 16, flexWrap: 'wrap' }}>
          <div className="form-group" style={{ minWidth: 200 }}>
            <label>Schedule</label>
            <select className="input" value={frequency} onChange={e => setFrequency(e.target.value as BackupFrequency)}>
              {FREQUENCIES.map(f => <option key={f.value} value={f.value}>{f.label}</option>)}
            </select>
          </div>
          <div className="form-group" style={{ maxWidth: 160 }}>
            <label>Backups to keep</label>
            <input className="input" type="number" min={1} max={365} value={retention} onChange={e => setRetention(e.target.value)} />
          </div>
        </div>

        <div className="form-group">
          <label>
            Encryption passphrase {config.passphraseSet
              ? <span style={{ color: 'var(--success, #22c55e)', fontWeight: 400 }}>(set)</span>
              : <span style={{ color: 'var(--danger)', fontWeight: 400 }}>(not set)</span>}
          </label>
          <div style={{ display: 'flex', gap: 8, flexWrap: 'wrap' }}>
            <input
              className="input"
              type="password"
              autoComplete="new-password"
              placeholder={config.passphraseSet ? 'New passphrase (empty = keep)' : `At least ${config.minPassphraseLength} characters`}
              value={passphrase}
              onChange={e => setPassphrase(e.target.value)}
              style={{ flex: 1, minWidth: 200 }}
            />
            <input
              className="input"
              type="password"
              autoComplete="new-password"
              placeholder="Repeat passphrase"
              value={passphraseConfirm}
              onChange={e => setPassphraseConfirm(e.target.value)}
              style={{ flex: 1, minWidth: 200 }}
            />
          </div>
          <span style={hint}>
            Keep it somewhere safe: without it a backup cannot be restored. Changing it only affects new backups; older
            ones still need the old passphrase. Downloaded backups open with 7-Zip, WinZip or Keka using it.
          </span>
        </div>
      </div>

      <div style={{ marginTop: 16 }}>
        <button className="btn btn-primary" onClick={saveConfig} disabled={saving}>
          <Save size={16} /> {saving ? 'Saving...' : 'Save Backup Settings'}
        </button>
      </div>

      <h4 style={{ margin: '28px 0 8px', fontSize: 14 }}>Stored backups</h4>
      <div style={{ display: 'flex', gap: 12, alignItems: 'center', flexWrap: 'wrap', fontSize: 13 }}>
        <div style={{ flex: 1, minWidth: 220 }}>
          <div>Last successful backup: <strong>{formatDate(config.lastSuccessAt)}</strong></div>
          {config.lastError && (
            <div style={{ color: 'var(--danger)', marginTop: 4 }}>
              Last attempt failed ({formatDate(config.lastAttemptAt)}): {config.lastError}
            </div>
          )}
        </div>
        <button className="btn btn-secondary" onClick={runNow} disabled={!canBackup || inProgress}>
          {config.running
            ? <><Loader size={14} className="admin-spin" /> Backup running...</>
            : <><Play size={14} /> Back up now</>}
        </button>
      </div>

      {config.storageConfigured && (
        backups.length === 0 ? (
          <p style={{ ...hint, fontSize: 13 }}>No backups stored yet.</p>
        ) : (
          <div style={{ display: 'flex', flexDirection: 'column', gap: 8, marginTop: 12 }}>
            {backups.map(b => (
              <div key={b.name} style={{ border: '1px solid var(--border)', borderRadius: 8, padding: '8px 12px' }}>
                <div style={{ display: 'flex', alignItems: 'center', gap: 8, flexWrap: 'wrap' }}>
                  <div style={{ flex: 1, minWidth: 180, fontSize: 13 }}>
                    <div><strong>{new Date(b.createdAt).toLocaleString()}</strong></div>
                    <div style={hint}>{KIND_LABELS[b.kind] ?? b.kind} · {formatBytes(b.size)}</div>
                  </div>
                  <button className="btn btn-secondary" style={{ padding: '6px 10px' }} onClick={() => download(b.name)} title="Download">
                    <Download size={14} />
                  </button>
                  <button
                    className="btn btn-secondary"
                    style={{ padding: '6px 10px' }}
                    onClick={() => { setRestoreTarget(restoreTarget === b.name ? null : b.name); setRestorePassphrase(''); }}
                    disabled={inProgress}
                    title="Restore"
                  >
                    <RotateCcw size={14} />
                  </button>
                  <button className="btn btn-secondary" style={{ padding: '6px 10px' }} onClick={() => remove(b.name)} disabled={inProgress} title="Delete">
                    <Trash2 size={14} />
                  </button>
                </div>
                {restoreTarget === b.name && (
                  <div style={{ marginTop: 10, display: 'flex', flexDirection: 'column', gap: 8 }}>
                    <span style={{ fontSize: 13, color: 'var(--danger)' }}>{RESTORE_WARNING}</span>
                    <div style={{ display: 'flex', gap: 8, flexWrap: 'wrap' }}>
                      <input
                        className="input"
                        type="password"
                        autoComplete="off"
                        placeholder="Passphrase (empty = the current one)"
                        value={restorePassphrase}
                        onChange={e => setRestorePassphrase(e.target.value)}
                        style={{ flex: 1, minWidth: 200 }}
                      />
                      <button className="btn btn-danger" onClick={() => restoreStored(b.name)} disabled={busy}>
                        {busy ? <><Loader size={14} className="admin-spin" /> Restoring...</> : 'Replace all data'}
                      </button>
                      <button className="btn btn-secondary" onClick={() => setRestoreTarget(null)} disabled={busy}>Cancel</button>
                    </div>
                  </div>
                )}
              </div>
            ))}
          </div>
        )
      )}

      <h4 style={{ margin: '28px 0 8px', fontSize: 14, display: 'flex', alignItems: 'center', gap: 6 }}>
        <Upload size={14} /> Restore from file
      </h4>
      <p style={{ ...hint, fontSize: 13, marginTop: 0 }}>
        Upload a backup ZIP, for example one downloaded from this or another SocialPod server, to restore it.
      </p>
      <div style={{ display: 'flex', gap: 8, flexWrap: 'wrap', alignItems: 'center' }}>
        <input
          ref={fileInput}
          type="file"
          accept=".zip,application/zip"
          onChange={e => { setUploadFile(e.target.files?.[0] ?? null); setUploadConfirm(false); }}
          style={{ fontSize: 13 }}
        />
        <input
          className="input"
          type="password"
          autoComplete="off"
          placeholder="Passphrase of this backup (empty = the current one)"
          value={uploadPassphrase}
          onChange={e => setUploadPassphrase(e.target.value)}
          style={{ flex: 1, minWidth: 220 }}
        />
      </div>
      {uploadConfirm && <p style={{ fontSize: 13, color: 'var(--danger)' }}>{RESTORE_WARNING}</p>}
      <div style={{ display: 'flex', gap: 8, marginTop: 12 }}>
        {!uploadConfirm ? (
          <button className="btn btn-secondary" onClick={() => setUploadConfirm(true)} disabled={!uploadFile || inProgress}>
            <Upload size={14} /> Restore uploaded backup
          </button>
        ) : (
          <>
            <button className="btn btn-danger" onClick={restoreUpload} disabled={busy}>
              {busy ? <><Loader size={14} className="admin-spin" /> Restoring...</> : 'Replace all data'}
            </button>
            <button className="btn btn-secondary" onClick={() => setUploadConfirm(false)} disabled={busy}>Cancel</button>
          </>
        )}
      </div>
    </div>
  );
}
