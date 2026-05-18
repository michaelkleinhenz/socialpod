import { useState, useEffect, useCallback } from 'react';
import { api } from '../../services/api';
import type { Post } from '../../types';
import { format, parseISO } from 'date-fns';
import { CheckCircle, XCircle, Clock, AlertTriangle, Filter, ExternalLink, RotateCcw } from 'lucide-react';
import toast from 'react-hot-toast';
import './Log.css';

type LogFilter = 'all' | 'published' | 'failed';

export function LogPage() {
  const [posts, setPosts] = useState<Post[]>([]);
  const [loading, setLoading] = useState(true);
  const [filter, setFilter] = useState<LogFilter>('all');

  const loadPosts = useCallback(() => {
    api.getPosts({})
      .then(data => {
        const withActivity = data.filter(
          (p: Post) => p.status === 'published' || p.status === 'failed' || (p.results && p.results.length > 0)
        );
        withActivity.sort((a: Post, b: Post) =>
          new Date(b.updatedAt).getTime() - new Date(a.updatedAt).getTime()
        );
        setPosts(withActivity);
      })
      .catch(() => toast.error('Failed to load log'))
      .finally(() => setLoading(false));
  }, []);

  useEffect(() => { loadPosts(); }, [loadPosts]);

  const filtered = filter === 'all'
    ? posts
    : posts.filter(p => p.status === filter);

  const counts = {
    all: posts.length,
    published: posts.filter(p => p.status === 'published').length,
    failed: posts.filter(p => p.status === 'failed').length,
  };

  if (loading) return <div className="page"><div className="loading-screen"><div className="spinner" /></div></div>;

  return (
    <div className="page">
      <div className="page-header">
        <h1>Posting Log</h1>
        <div className="log-filters">
          <Filter size={14} color="var(--text-muted)" />
          <button
            className={`log-filter-btn ${filter === 'all' ? 'active' : ''}`}
            onClick={() => setFilter('all')}
          >
            All <span className="filter-count">{counts.all}</span>
          </button>
          <button
            className={`log-filter-btn success ${filter === 'published' ? 'active' : ''}`}
            onClick={() => setFilter('published')}
          >
            Published <span className="filter-count">{counts.published}</span>
          </button>
          <button
            className={`log-filter-btn error ${filter === 'failed' ? 'active' : ''}`}
            onClick={() => setFilter('failed')}
          >
            Failed <span className="filter-count">{counts.failed}</span>
          </button>
        </div>
      </div>

      {filtered.length === 0 ? (
        <div className="empty-state">
          <Clock size={48} color="var(--text-muted)" />
          <p style={{ marginTop: 16 }}>No posting activity yet</p>
          <p style={{ color: 'var(--text-muted)', fontSize: 14 }}>
            Posts will appear here after they are published or fail.
          </p>
        </div>
      ) : (
        <div className="log-list">
          {filtered.map(post => (
            <LogEntry key={post.id} post={post} onRetry={loadPosts} />
          ))}
        </div>
      )}
    </div>
  );
}

import { PlatformIcon } from '../Common/PlatformIcon';

function getPostUrl(platform: string, postId: string): string | null {
  if (platform === 'bluesky' && postId.startsWith('at://')) {
    const parts = postId.replace('at://', '').split('/');
    if (parts.length >= 3) {
      return `https://bsky.app/profile/${parts[0]}/post/${parts[2]}`;
    }
  }
  if (platform === 'threads' && postId) {
    return `https://www.threads.net/post/${postId}`;
  }
  if (platform === 'linkedin' && postId) {
    return `https://www.linkedin.com/feed/update/${postId}`;
  }
  return null;
}

function LogEntry({ post, onRetry }: { post: Post; onRetry: () => void }) {
  const [retrying, setRetrying] = useState(false);
  const preview = post.content.length > 120 ? post.content.slice(0, 120) + '...' : post.content;
  const isSuccess = post.status === 'published';
  const isFailed = post.status === 'failed';

  const handleRetry = async () => {
    setRetrying(true);
    try {
      await api.retryPost(post.id);
      toast.success('Post queued for retry');
      onRetry();
    } catch (err: any) {
      toast.error(err.message || 'Retry failed');
    } finally {
      setRetrying(false);
    }
  };

  return (
    <div className={`log-entry ${post.status}`}>
      <div className="log-entry-icon">
        {isSuccess && <CheckCircle size={22} color="var(--success)" />}
        {isFailed && <XCircle size={22} color="var(--danger)" />}
        {!isSuccess && !isFailed && <AlertTriangle size={22} color="var(--warning)" />}
      </div>

      <div className="log-entry-body">
        <div className="log-entry-header">
          <div className="log-entry-status">
            <span className={`badge badge-${post.status}`}>{post.status}</span>
            <div className="log-entry-platforms">
              {post.platforms.map(p => (
                <span key={p} className={`badge badge-${p}`}>{p}</span>
              ))}
            </div>
          </div>
          <div className="log-entry-time">
            <span title="Scheduled for">
              {format(parseISO(post.scheduledAt), 'MMM d, yyyy HH:mm')}
            </span>
          </div>
        </div>

        <div className="log-entry-content">{preview}</div>

        {post.results && post.results.length > 0 && (
          <div className="log-entry-results">
            {post.results.map((r, i) => (
              <div key={i} className={`log-result ${r.success ? 'success' : 'error'}`}>
                <PlatformIcon platform={r.platform} />
                <span className="log-result-platform">{r.platform}</span>
                {r.success ? (
                  <>
                    <CheckCircle size={12} color="var(--success)" />
                    <span className="log-result-text">
                      Posted{r.postedAt ? ` at ${format(parseISO(r.postedAt), 'HH:mm:ss')}` : ''}
                    </span>
                    {r.postId && (() => {
                      const url = getPostUrl(r.platform, r.postId);
                      return url ? (
                        <a href={url} target="_blank" rel="noopener noreferrer" className="log-result-link">
                          View post <ExternalLink size={11} />
                        </a>
                      ) : (
                        <span className="log-result-id" title={r.postId}>
                          {r.postId.length > 40 ? r.postId.slice(0, 40) + '...' : r.postId}
                        </span>
                      );
                    })()}
                  </>
                ) : (
                  <>
                    <XCircle size={12} color="var(--danger)" />
                    <span className="log-result-text log-error-text">{r.error || 'Unknown error'}</span>
                  </>
                )}
              </div>
            ))}
          </div>
        )}

        {isFailed && (
          <div className="log-entry-actions">
            <button className="log-retry-btn" onClick={handleRetry} disabled={retrying}>
              <RotateCcw size={13} />
              {retrying ? 'Retrying...' : 'Retry'}
            </button>
          </div>
        )}
      </div>
    </div>
  );
}
