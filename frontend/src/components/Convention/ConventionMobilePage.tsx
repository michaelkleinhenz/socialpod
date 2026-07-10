import { useEffect } from 'react';
import { useParams, useNavigate } from 'react-router-dom';
import { useAuth } from '../../contexts/AuthContext';
import { QueueDetailPage } from './QueueDetailPage';
import './Convention.css';

/**
 * Standalone, phone-optimised single-page view for one convention queue.
 * Rendered outside the app Layout (no sidebar) so nothing but this
 * convention is shown — the URL is meant to be bookmarked on a phone.
 * Reuses the full QueueDetailPage functionality via its `mobile` flag.
 */
export function ConventionMobilePage() {
  const { user, loading } = useAuth();
  const { id } = useParams<{ id: string }>();
  const navigate = useNavigate();

  useEffect(() => {
    if (!loading && !user) {
      navigate('/login?next=' + encodeURIComponent(`/m/convention/${id}`));
    }
  }, [loading, user, id, navigate]);

  if (loading || !user) {
    return (
      <div className="conv-mobile-page">
        <div className="loading-screen"><div className="spinner" /></div>
      </div>
    );
  }

  return <QueueDetailPage mobile />;
}
