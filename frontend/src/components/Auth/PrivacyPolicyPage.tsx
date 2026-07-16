import { useState, useEffect } from 'react';
import { Link } from 'react-router-dom';
import { api } from '../../services/api';
import type { PublicSettings } from '../../types';
import { ArrowLeft, Shield } from 'lucide-react';
import './Auth.css';

export function PrivacyPolicyPage() {
  const [settings, setSettings] = useState<PublicSettings | null>(null);
  const [loading, setLoading] = useState(true);

  useEffect(() => {
    api.getPublicSettings().then(s => {
      setSettings(s);
      setLoading(false);
    }).catch(() => setLoading(false));
  }, []);

  if (loading) {
    return (
      <div className="auth-page">
        <div className="loading-screen"><div className="spinner" /></div>
      </div>
    );
  }

  return (
    <div className="auth-page">
      <div className="auth-card" style={{ maxWidth: 720 }}>
        <div className="imprint-header">
          <h2 style={{ display: 'flex', alignItems: 'center', gap: 8 }}>
            <Shield size={20} /> Privacy Policy
          </h2>
          <Link to="/login" className="btn btn-ghost btn-sm">
            <ArrowLeft size={16} /> Back
          </Link>
        </div>
        <div
          className="imprint-body"
          dangerouslySetInnerHTML={{ __html: settings?.privacyPolicyHtml || '<p>No privacy policy has been configured.</p>' }}
        />
      </div>
    </div>
  );
}
