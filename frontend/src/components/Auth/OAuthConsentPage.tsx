import { useEffect, useMemo, useState } from 'react';
import { Navigate, useLocation, useNavigate } from 'react-router-dom';
import { Plug, ShieldCheck, AlertTriangle } from 'lucide-react';
import { useAuth } from '../../contexts/AuthContext';
import { api } from '../../services/api';
import type { OAuthAuthorization } from '../../types';
import './Auth.css';

const SCOPE_LABELS: Record<string, string> = {
  mcp: 'Use your SocialPod tools: posts, drafts, footers, mentions, watermarks and accounts',
  openid: 'Confirm who you are',
  profile: 'See your name',
  email: 'See your email address',
  offline_access: 'Stay connected until you disconnect it',
};

// The authorization endpoint of SocialPod's OAuth server. MCP clients such as
// Claude.ai connectors or ChatGPT send the user here; after signing in, the
// user approves or denies the client and is sent back to it. The page only
// relays the request's query string — the backend validates it and builds the
// redirect, and never redirects to a URI the client did not register.
export function OAuthConsentPage() {
  const { user, loading, logout } = useAuth();
  const location = useLocation();
  const navigate = useNavigate();
  const [info, setInfo] = useState<OAuthAuthorization | null>(null);
  const [error, setError] = useState<string | null>(null);
  const [submitting, setSubmitting] = useState(false);

  const query = location.search.replace(/^\?/, '');
  const params = useMemo(() => Object.fromEntries(new URLSearchParams(query)), [query]);
  const loginPath = `/login?next=${encodeURIComponent(location.pathname + location.search)}`;

  useEffect(() => {
    if (!user) return;
    api.getOAuthAuthorization(query)
      .then(res => {
        if (res.redirectUrl) {
          window.location.replace(res.redirectUrl);
          return;
        }
        setInfo(res);
      })
      .catch(err => setError(err.message));
  }, [user, query]);

  if (loading) return <div className="loading-screen"><div className="spinner" /></div>;
  if (!user) return <Navigate to={loginPath} replace />;

  const decide = async (approve: boolean) => {
    setSubmitting(true);
    try {
      const res = await api.decideOAuthAuthorization(params, approve);
      window.location.replace(res.redirectUrl);
    } catch (err) {
      setError(err instanceof Error ? err.message : 'Request failed');
      setSubmitting(false);
    }
  };

  const switchAccount = () => {
    logout();
    navigate(loginPath, { replace: true });
  };

  const clientName = info?.client?.name || 'An unnamed app';

  return (
    <div className="auth-page">
      <div className="auth-card">
        <div className="auth-header">
          <div className="logo-icon large">
            <Plug size={28} />
          </div>
          <h1>Connect an app</h1>
          {info && <p><strong>{clientName}</strong> wants to access your SocialPod account</p>}
        </div>

        {error && (
          <div className="oauth-error">
            <AlertTriangle size={16} />
            <span>{error}</span>
          </div>
        )}

        {!info && !error && <div className="spinner" style={{ margin: '0 auto' }} />}

        {info && (
          <>
            <ul className="oauth-scopes">
              {(info.scopes || []).map(s => (
                <li key={s}><ShieldCheck size={14} /> {SCOPE_LABELS[s] || s}</li>
              ))}
            </ul>

            <p className="oauth-note">
              The app acts as you, with the same access as your account on the Calendar, News and
              Episodes pages. Admin and team-admin functions are never available to connected apps.
              You can disconnect it at any time on your Profile page.
            </p>

            <p className="oauth-note">
              After approving you will be sent to <strong>{info.redirectHost}</strong>. Only approve
              if you started this connection there yourself.
            </p>

            <p className="oauth-note">
              Signed in as <strong>{info.user?.email}</strong>.{' '}
              <button className="link-btn" style={{ display: 'inline' }} onClick={switchAccount}>
                Use another account
              </button>
            </p>

            <div className="oauth-actions">
              <button className="btn btn-secondary" onClick={() => decide(false)} disabled={submitting}>
                Deny
              </button>
              <button className="btn btn-primary" onClick={() => decide(true)} disabled={submitting}>
                {submitting ? 'Connecting...' : 'Approve'}
              </button>
            </div>
          </>
        )}
      </div>
    </div>
  );
}
