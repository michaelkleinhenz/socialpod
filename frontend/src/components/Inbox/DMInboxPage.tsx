import { useState, useEffect, useCallback } from 'react';
import { api } from '../../services/api';
import { Mail, RefreshCw, Reply, CheckCircle2 } from 'lucide-react';
import { formatDistanceToNow, parseISO } from 'date-fns';
import toast from 'react-hot-toast';
import './Inbox.css';

interface InboxMessage {
  id: string;
  platform: string;
  messageType: string;
  accountName: string;
  externalId: string;
  senderId: string;
  senderName: string;
  senderAvatar: string;
  text: string;
  isRead: boolean;
  isReplied: boolean;
  receivedAt: string;
}

export function DMInboxPage() {
  const [messages, setMessages] = useState<InboxMessage[]>([]);
  const [loading, setLoading] = useState(true);

  const load = useCallback(() => {
    setLoading(true);
    api.getDMInbox()
      .then(setMessages)
      .catch(() => toast.error('Failed to load DMs'))
      .finally(() => setLoading(false));
  }, []);

  useEffect(() => { load(); }, [load]);

  const unread = messages.filter(m => !m.isRead).length;

  if (loading) {
    return <div className="page"><div className="loading-screen"><div className="spinner" /></div></div>;
  }

  return (
    <div className="page inbox-page">
      <div className="inbox-header">
        <h1>
          Direct Messages
          {unread > 0 && <span className="inbox-badge-count">{unread}</span>}
        </h1>
        <button className="btn btn-secondary" onClick={load} style={{ gap: 6 }}>
          <RefreshCw size={14} />
          Refresh
        </button>
      </div>

      {messages.length === 0 ? (
        <div className="inbox-empty">
          <Mail size={48} />
          <p>No direct messages yet</p>
          <p style={{ fontSize: 13 }}>
            Instagram DMs sent to your connected account will appear here via webhook.
          </p>
        </div>
      ) : (
        <div className="inbox-list">
          {messages.map(msg => (
            <DMEntry
              key={msg.id}
              message={msg}
              onReplied={() => {
                setMessages(prev =>
                  prev.map(m => m.id === msg.id ? { ...m, isReplied: true, isRead: true } : m)
                );
              }}
              onRead={() => {
                setMessages(prev =>
                  prev.map(m => m.id === msg.id ? { ...m, isRead: true } : m)
                );
              }}
            />
          ))}
        </div>
      )}
    </div>
  );
}

function DMEntry({
  message,
  onReplied,
  onRead,
}: {
  message: InboxMessage;
  onReplied: () => void;
  onRead: () => void;
}) {
  const [showReply, setShowReply] = useState(false);
  const [replyText, setReplyText] = useState('');
  const [sending, setSending] = useState(false);

  const handleMarkRead = () => {
    if (!message.isRead) {
      api.markInboxRead(message.id).then(onRead).catch(() => {});
    }
  };

  const handleReply = async () => {
    if (!replyText.trim()) return;
    setSending(true);
    try {
      await api.replyToDM(message.id, replyText.trim());
      toast.success('Message sent');
      setReplyText('');
      setShowReply(false);
      onReplied();
    } catch (e: any) {
      toast.error(e.message || 'Failed to send message');
    } finally {
      setSending(false);
    }
  };

  const initials = message.senderName
    ? message.senderName.slice(0, 2).toUpperCase()
    : message.senderId
      ? message.senderId.slice(0, 2).toUpperCase()
      : '?';

  const displayName = message.senderName || message.senderId || 'Unknown user';

  const timeAgo = message.receivedAt
    ? formatDistanceToNow(parseISO(message.receivedAt), { addSuffix: true })
    : '';

  return (
    <div
      className={`inbox-entry ${!message.isRead ? 'unread' : ''} ${message.isReplied ? 'replied' : ''}`}
      onClick={handleMarkRead}
    >
      <div className="inbox-entry-top">
        <div className="inbox-avatar">{initials}</div>

        <div className="inbox-entry-meta">
          <div className="inbox-entry-sender">
            <span className="inbox-sender-name">{displayName}</span>
            <span className={`inbox-platform-badge ${message.platform}`}>
              {message.platform}
            </span>
            {message.accountName && (
              <span style={{ fontSize: 12, color: 'var(--text-muted)' }}>
                @{message.accountName}
              </span>
            )}
            <span className="inbox-time">{timeAgo}</span>
          </div>

          <div className="inbox-entry-text">{message.text || <em style={{ color: 'var(--text-muted)' }}>(no text)</em>}</div>

          <div className="inbox-entry-actions" onClick={e => e.stopPropagation()}>
            {message.isReplied ? (
              <span className="replied-badge">
                <CheckCircle2 size={13} />
                Replied
              </span>
            ) : (
              <button
                className="reply-toggle-btn"
                onClick={() => setShowReply(v => !v)}
              >
                <Reply size={13} />
                {showReply ? 'Cancel' : 'Reply'}
              </button>
            )}

            {showReply && !message.isReplied && (
              <div className="reply-form">
                <textarea
                  className="reply-textarea"
                  placeholder="Write your reply…"
                  value={replyText}
                  onChange={e => setReplyText(e.target.value)}
                  rows={2}
                />
                <button
                  className="reply-send-btn"
                  onClick={handleReply}
                  disabled={sending || !replyText.trim()}
                >
                  {sending ? 'Sending…' : 'Send'}
                </button>
              </div>
            )}
          </div>
        </div>
      </div>
    </div>
  );
}
