import { useState, useEffect } from 'react';
import { api } from '../../services/api';
import type { Post } from '../../types';
import { format, parseISO } from 'date-fns';
import { CheckCircle, XCircle, Clock, AlertTriangle, Filter, ExternalLink } from 'lucide-react';
import toast from 'react-hot-toast';
import './Log.css';

type LogFilter = 'all' | 'published' | 'failed';

export function LogPage() {
  const [posts, setPosts] = useState<Post[]>([]);
  const [loading, setLoading] = useState(true);
  const [filter, setFilter] = useState<LogFilter>('all');

  useEffect(() => {
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
            <LogEntry key={post.id} post={post} />
          ))}
        </div>
      )}
    </div>
  );
}

function PlatformIcon({ platform, size = 14 }: { platform: string; size?: number }) {
  if (platform === 'bluesky') {
    return (
      <svg width={size} height={size} viewBox="0 0 600 530" fill="var(--bluesky)" style={{ flexShrink: 0 }}>
        <path d="M135.72 44.03C202.216 93.951 273.74 195.86 300 249.834c26.262-53.974 97.784-155.883 164.28-205.804C520.211-1.618 590.2-24.926 590.2 51.664c0 15.318-8.78 128.688-13.923 147.024-17.89 63.78-83.096 80.05-141.37 70.202 101.97 16.066 127.874 69.15 71.846 122.229C398.266 489.881 337.2 392.22 300 327.206c-37.2 65.014-97.674 161.932-206.753 63.913-56.028-53.079-30.124-106.163 71.846-122.23-58.274 9.849-123.48-6.421-141.37-70.201C18.58 180.352 9.8 67.065 9.8 51.747c0-76.59 69.989-53.282 125.92-7.717z" />
      </svg>
    );
  }
  if (platform === 'instagram') {
    return (
      <svg width={size} height={size} viewBox="0 0 24 24" fill="none" stroke="var(--instagram)" strokeWidth="2" strokeLinecap="round" strokeLinejoin="round" style={{ flexShrink: 0 }}>
        <rect x="2" y="2" width="20" height="20" rx="5" ry="5" />
        <circle cx="12" cy="12" r="5" />
        <circle cx="17.5" cy="6.5" r="1.5" fill="var(--instagram)" stroke="none" />
      </svg>
    );
  }
  return <span className={`platform-dot ${platform}`} />;
}

function getPostUrl(platform: string, postId: string): string | null {
  if (platform === 'bluesky' && postId.startsWith('at://')) {
    // at://did:plc:xxx/app.bsky.feed.post/rkey -> https://bsky.app/profile/did:plc:xxx/post/rkey
    const parts = postId.replace('at://', '').split('/');
    if (parts.length >= 3) {
      return `https://bsky.app/profile/${parts[0]}/post/${parts[2]}`;
    }
  }
  return null;
}

function LogEntry({ post }: { post: Post }) {
  const preview = post.content.length > 120 ? post.content.slice(0, 120) + '...' : post.content;
  const isSuccess = post.status === 'published';
  const isFailed = post.status === 'failed';

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
      </div>
    </div>
  );
}
