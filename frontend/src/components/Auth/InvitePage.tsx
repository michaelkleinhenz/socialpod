import { useState, useEffect, type FormEvent } from 'react';
import { Link, useSearchParams } from 'react-router-dom';
import { useAuth } from '../../contexts/AuthContext';
import { api } from '../../services/api';
import { Zap } from 'lucide-react';
import toast from 'react-hot-toast';
import './Auth.css';

export function InvitePage() {
  const { login: authLogin } = useAuth();
  const [searchParams] = useSearchParams();
  const token = searchParams.get('token') || '';

  const [name, setName] = useState('');
  const [password, setPassword] = useState('');
  const [loading, setLoading] = useState(false);
  const [checking, setChecking] = useState(true);
  const [inviteEmail, setInviteEmail] = useState('');
  const [teamName, setTeamName] = useState('');
  const [error, setError] = useState('');

  useEffect(() => {
    if (!token) {
      setError('No invitation token provided');
      setChecking(false);
      return;
    }
    api.getInviteInfo(token)
      .then(info => {
        setInviteEmail(info.email);
        setTeamName(info.teamName);
      })
      .catch((err: any) => {
        setError(err.message || 'Invalid or expired invitation');
      })
      .finally(() => setChecking(false));
  }, [token]);

  if (checking) return <div className="loading-screen"><div className="spinner" /></div>;

  if (error) return (
    <div className="auth-page">
      <div className="auth-card">
        <div className="auth-header">
          <div className="logo-icon large">
            <Zap size={28} />
          </div>
          <h1>SocialPod</h1>
          <p>Invitation Error</p>
        </div>
        <p style={{ textAlign: 'center', color: 'var(--text-secondary)', fontSize: 14, marginBottom: 16 }}>
          {error}
        </p>
        <p className="auth-footer">
          Already have an account? <Link to="/login">Sign in</Link>
        </p>
      </div>
    </div>
  );

  const handleSubmit = async (e: FormEvent) => {
    e.preventDefault();
    if (password.length < 8) {
      toast.error('Password must be at least 8 characters');
      return;
    }
    setLoading(true);
    try {
      const result = await api.acceptInvite(token, name, password);
      api.setToken(result.token);
      await authLogin(inviteEmail, password);
      toast.success(`Welcome to ${teamName}!`);
    } catch (err: any) {
      toast.error(err.message);
    } finally {
      setLoading(false);
    }
  };

  return (
    <div className="auth-page">
      <div className="auth-card">
        <div className="auth-header">
          <div className="logo-icon large">
            <Zap size={28} />
          </div>
          <h1>SocialPod</h1>
          <p>You've been invited to join <strong>{teamName}</strong></p>
        </div>

        <form onSubmit={handleSubmit} className="auth-form">
          <div className="form-group">
            <label>Email</label>
            <input
              type="email"
              className="input"
              value={inviteEmail}
              disabled
              style={{ opacity: 0.7 }}
            />
          </div>

          <div className="form-group">
            <label>Name</label>
            <input
              type="text"
              className="input"
              value={name}
              onChange={e => setName(e.target.value)}
              placeholder="Your name"
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
              placeholder="Min 8 characters"
              required
              minLength={8}
            />
          </div>

          <button type="submit" className="btn btn-primary auth-btn" disabled={loading}>
            {loading ? 'Creating account...' : 'Join Team'}
          </button>
        </form>

        <p className="auth-footer">
          Already have an account? <Link to="/login">Sign in</Link>
        </p>
      </div>
    </div>
  );
}
