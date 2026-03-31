import { useState, useEffect, useCallback } from 'react';
import { api } from '../../services/api';
import { Image, RefreshCw, Heart, MessageCircle, ExternalLink } from 'lucide-react';
import { formatDistanceToNow, parseISO } from 'date-fns';
import toast from 'react-hot-toast';
import './Inbox.css';

interface Account {
  id: string;
  platform: string;
  accountName: string;
  displayName: string;
}

interface FeedItem {
  id: string;
  caption: string;
  mediaType: string;
  mediaUrl: string;
  thumbnailUrl: string;
  permalink: string;
  timestamp: string;
  likeCount: number;
  commentsCount: number;
}

export function FeedPage() {
  const [accounts, setAccounts] = useState<Account[]>([]);
  const [selectedAccount, setSelectedAccount] = useState('');
  const [feed, setFeed] = useState<FeedItem[]>([]);
  const [loading, setLoading] = useState(false);
  const [accountsLoading, setAccountsLoading] = useState(true);

  useEffect(() => {
    api.getActiveAccounts()
      .then(all => {
        const ig = all.filter((a: Account) => a.platform === 'instagram');
        setAccounts(ig);
        if (ig.length > 0) setSelectedAccount(ig[0].id);
      })
      .catch(() => toast.error('Failed to load accounts'))
      .finally(() => setAccountsLoading(false));
  }, []);

  const loadFeed = useCallback(() => {
    if (!selectedAccount) return;
    setLoading(true);
    api.getInstagramFeed(selectedAccount)
      .then(setFeed)
      .catch(e => toast.error(e.message || 'Failed to load feed'))
      .finally(() => setLoading(false));
  }, [selectedAccount]);

  useEffect(() => {
    if (selectedAccount) loadFeed();
  }, [selectedAccount, loadFeed]);

  if (accountsLoading) {
    return <div className="page"><div className="loading-screen"><div className="spinner" /></div></div>;
  }

  return (
    <div className="page inbox-page">
      <div className="inbox-header">
        <h1>Instagram Feed</h1>
        <button className="btn btn-secondary" onClick={loadFeed} disabled={loading} style={{ gap: 6 }}>
          <RefreshCw size={14} />
          Refresh
        </button>
      </div>

      {accounts.length === 0 ? (
        <div className="inbox-empty">
          <Image size={48} />
          <p>No Instagram accounts connected</p>
          <p style={{ fontSize: 13 }}>Connect an Instagram account in Admin &gt; Accounts.</p>
        </div>
      ) : (
        <>
          {accounts.length > 1 && (
            <div className="inbox-account-select">
              <label>Account:</label>
              <select
                value={selectedAccount}
                onChange={e => setSelectedAccount(e.target.value)}
              >
                {accounts.map(a => (
                  <option key={a.id} value={a.id}>
                    @{a.accountName}
                  </option>
                ))}
              </select>
            </div>
          )}

          {loading ? (
            <div className="loading-screen" style={{ height: 200 }}>
              <div className="spinner" />
            </div>
          ) : feed.length === 0 ? (
            <div className="inbox-empty">
              <Image size={48} />
              <p>No posts found</p>
            </div>
          ) : (
            <div className="feed-grid">
              {feed.map(item => (
                <FeedCard key={item.id} item={item} />
              ))}
            </div>
          )}
        </>
      )}
    </div>
  );
}

function FeedCard({ item }: { item: FeedItem }) {
  const imageURL = item.thumbnailUrl || item.mediaUrl;
  const timeAgo = item.timestamp
    ? formatDistanceToNow(parseISO(item.timestamp), { addSuffix: true })
    : '';

  return (
    <div className="feed-card">
      {imageURL ? (
        <img
          src={imageURL}
          alt={item.caption?.slice(0, 80) || 'Instagram post'}
          className="feed-card-image"
          loading="lazy"
        />
      ) : (
        <div className="feed-card-placeholder">
          <Image size={32} />
        </div>
      )}

      <div className="feed-card-body">
        <div className="feed-card-type-badge">{item.mediaType}</div>

        {item.caption && (
          <div className="feed-card-caption">{item.caption}</div>
        )}

        <div className="feed-card-stats">
          <span className="feed-card-stat">
            <Heart size={12} />
            {item.likeCount ?? 0}
          </span>
          <span className="feed-card-stat">
            <MessageCircle size={12} />
            {item.commentsCount ?? 0}
          </span>
          {item.permalink && (
            <a
              href={item.permalink}
              target="_blank"
              rel="noopener noreferrer"
              className="feed-card-stat"
              style={{ marginLeft: 'auto', color: 'var(--text-muted)' }}
              onClick={e => e.stopPropagation()}
            >
              <ExternalLink size={12} />
            </a>
          )}
        </div>

        <div className="feed-card-time">{timeAgo}</div>
      </div>
    </div>
  );
}
