import { useState, useEffect, type FormEvent } from 'react';
import { Link, useSearchParams, useNavigate } from 'react-router-dom';
import { useAuth } from '../../contexts/AuthContext';
import { api } from '../../services/api';
import type { PublicSettings } from '../../types';
import { Zap, X, Cookie, FileText, Shield } from 'lucide-react';
import toast from 'react-hot-toast';
import './Auth.css';

const COOKIE_CONSENT_KEY = 'cookie_consent';

export function LoginPage() {
  const { login } = useAuth();
  const [searchParams] = useSearchParams();
  const navigate = useNavigate();
  const [email, setEmail] = useState('');
  const [password, setPassword] = useState('');
  const [loading, setLoading] = useState(false);
  const [publicSettings, setPublicSettings] = useState<PublicSettings | null>(null);
  const [showBanner, setShowBanner] = useState(false);
  const [imprintOpen, setImprintOpen] = useState(false);

  useEffect(() => {
    api.getPublicSettings().then(s => {
      setPublicSettings(s);
      if (s.cookieBannerEnabled && !localStorage.getItem(COOKIE_CONSENT_KEY)) {
        setShowBanner(true);
      }
    }).catch(() => {});
  }, []);

  const acceptCookies = () => {
    localStorage.setItem(COOKIE_CONSENT_KEY, 'accepted');
    setShowBanner(false);
  };

  const declineCookies = () => {
    localStorage.setItem(COOKIE_CONSENT_KEY, 'declined');
    setShowBanner(false);
  };

  const handleSubmit = async (e: FormEvent) => {
    e.preventDefault();
    setLoading(true);
    try {
      await login(email, password);
      toast.success('Welcome back!');
      const next = searchParams.get('next');
      if (next) navigate(next);
    } catch (err: any) {
      toast.error(err.message);
    } finally {
      setLoading(false);
    }
  };

  const hasImprint = !!publicSettings?.imprintHtml;
  const hasPrivacyPolicy = !!publicSettings?.privacyPolicyHtml;

  return (
    <div className="auth-page">
      <div className="auth-card">
        <div className="auth-header">
          <div className="logo-icon large">
            <Zap size={28} />
          </div>
          <h1>SocialPod</h1>
          <p>Sign in to manage your social media</p>
        </div>

        <form onSubmit={handleSubmit} className="auth-form">
          <div className="form-group">
            <label>Email</label>
            <input
              type="email"
              className="input"
              value={email}
              onChange={e => setEmail(e.target.value)}
              placeholder="you@example.com"
              required
            />
          </div>

          <div className="form-group">
            <label>Password</label>
            <input
              type="password"
              className="input"
              value={password}
              onChange={e => setPassword(e.target.value)}
              placeholder="Enter your password"
              required
            />
          </div>

          <button type="submit" className="btn btn-primary auth-btn" disabled={loading}>
            {loading ? 'Signing in...' : 'Sign In'}
          </button>
        </form>

        <p className="auth-footer">
          Don't have an account? <Link to="/register">Create one</Link>
        </p>

        {(hasImprint || hasPrivacyPolicy) && (
          <p className="auth-legal-links">
            {hasImprint && (
              <button className="link-btn" onClick={() => setImprintOpen(true)}>
                <FileText size={12} /> Imprint
              </button>
            )}
            {hasPrivacyPolicy && (
              <Link to="/privacy" className="link-btn">
                <Shield size={12} /> Privacy Policy
              </Link>
            )}
          </p>
        )}
      </div>

      {/* Cookie banner */}
      {showBanner && (
        <div className="cookie-banner">
          <Cookie size={18} className="cookie-icon" />
          <p className="cookie-text">
            {publicSettings?.cookieBannerText || 'This site uses cookies to improve your experience.'}
          </p>
          <div className="cookie-actions">
            <button className="btn btn-primary btn-sm" onClick={acceptCookies}>Accept</button>
            <button className="btn btn-secondary btn-sm" onClick={declineCookies}>Decline</button>
          </div>
        </div>
      )}

      {/* Imprint modal */}
      {imprintOpen && (
        <div className="modal-overlay" onClick={() => setImprintOpen(false)}>
          <div className="modal imprint-modal" onClick={e => e.stopPropagation()}>
            <div className="imprint-header">
              <h2>Imprint</h2>
              <button className="btn btn-ghost btn-sm" onClick={() => setImprintOpen(false)}>
                <X size={18} />
              </button>
            </div>
            <div
              className="imprint-body"
              dangerouslySetInnerHTML={{ __html: publicSettings?.imprintHtml || '' }}
            />
          </div>
        </div>
      )}
    </div>
  );
}
