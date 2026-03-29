import { useState } from 'react';
import { useAuth } from '../../contexts/AuthContext';
import { api } from '../../services/api';
import { Key, Copy, RefreshCw, UsersRound } from 'lucide-react';
import toast from 'react-hot-toast';

  const generateToken = async () => {
    setGenerating(true);
    try {
      const res = await api.generateApiToken();
      setApiToken(res.apiToken);
      toast.success('API token generated');
    } catch (err: any) {
      toast.error(err.message);
    } finally {
      setGenerating(false);
    }
  };

  const copyToken = () => {
    if (apiToken) {
      navigator.clipboard.writeText(apiToken);
      toast.success('Copied to clipboard');
    }
  };

  return (
    <div className="page">
      <div className="page-header">
        <h1>Profile</h1>
      </div>

      <div className="card" style={{ maxWidth: 600 }}>
        <div className="form-group" style={{ marginBottom: 20 }}>
          <label>Name</label>
          <div style={{ fontSize: 16, color: 'var(--text-primary)' }}>{user?.name}</div>
        </div>

        <div className="form-group" style={{ marginBottom: 20 }}>
          <label>Email</label>
          <div style={{ fontSize: 16, color: 'var(--text-primary)' }}>{user?.email}</div>
        </div>

        <div className="form-group" style={{ marginBottom: 20 }}>
          <label>Role</label>
          <div>
            <span className={`badge ${user?.isAdmin ? 'badge-published' : 'badge-scheduled'}`}>
              {user?.isAdmin ? 'Admin' : 'User'}
            </span>
          </div>
        </div>

        {user?.teamName && (
          <div className="form-group" style={{ marginBottom: 20 }}>
            <label style={{ display: 'flex', alignItems: 'center', gap: 6 }}>
              <UsersRound size={14} /> Team
            </label>
            <div style={{ fontSize: 16, color: 'var(--text-primary)' }}>{user.teamName}</div>
            <span style={{ fontSize: 12, color: 'var(--text-muted)' }}>
              Shared calendar and API token with your team
            </span>
          </div>
        )}

        <hr style={{ border: 'none', borderTop: '1px solid var(--border)', margin: '24px 0' }} />

        <h3 style={{ marginBottom: 12, display: 'flex', alignItems: 'center', gap: 8 }}>
          <Key size={18} /> API Access
        </h3>
        <p style={{ color: 'var(--text-secondary)', fontSize: 14, marginBottom: 16 }}>
          Use an API token to schedule posts from external tools and scripts via the REST API.
        </p>

        {apiToken && (
          <div style={{
            background: 'var(--bg-primary)',
            border: '1px solid var(--border)',
            borderRadius: 'var(--radius-sm)',
            padding: '12px 16px',
            marginBottom: 16,
            display: 'flex',
            alignItems: 'center',
            gap: 10,
            fontFamily: 'monospace',
            fontSize: 13,
            wordBreak: 'break-all',
          }}>
            <span style={{ flex: 1 }}>{apiToken}</span>
            <button className="btn btn-ghost btn-sm" onClick={copyToken}>
              <Copy size={14} />
            </button>
          </div>
        )}

        <button className="btn btn-primary" onClick={generateToken} disabled={generating}>
          <RefreshCw size={16} />
          {generating ? 'Generating...' : apiToken ? 'Regenerate Token' : 'Generate API Token'}
        </button>
      </div>
    </div>
  );
}
