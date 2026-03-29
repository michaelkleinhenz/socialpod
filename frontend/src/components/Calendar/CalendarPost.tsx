import type { Post } from '../../types';
import { format, parseISO } from 'date-fns';
import { Image, CheckCircle, Clock, AlertTriangle, FileEdit } from 'lucide-react';

interface Props {
  post: Post;
  onClick: () => void;
  isDragging?: boolean;
}

const statusConfig = {
  published: { icon: CheckCircle, color: 'var(--success)', label: 'Published' },
  scheduled: { icon: Clock, color: 'var(--accent)', label: 'Scheduled' },
  failed:    { icon: AlertTriangle, color: 'var(--danger)', label: 'Failed' },
  draft:     { icon: FileEdit, color: 'var(--text-muted)', label: 'Draft' },
} as const;

export function CalendarPost({ post, onClick, isDragging }: Props) {
  const time = format(parseISO(post.scheduledAt), 'HH:mm');
  const preview = post.content.length > 60 ? post.content.slice(0, 60) + '...' : post.content;
  const cfg = statusConfig[post.status] || statusConfig.scheduled;
  const StatusIcon = cfg.icon;

  return (
    <div
      className={`calendar-post ${post.status} ${isDragging ? 'dragging' : ''}`}
      onClick={onClick}
    >
      <div className="post-top-row">
        <span className="post-time">{time}</span>
        <span className="post-status-icon" title={cfg.label}>
          <StatusIcon size={12} color={cfg.color} />
        </span>
      </div>
      <div className="post-preview">{preview}</div>
      <div className="post-meta">
        {post.platforms.map(p => (
          <span key={p} className={`platform-dot ${p}`} title={p} />
        ))}
        {post.imageUrls && post.imageUrls.length > 0 && (
          <Image size={10} style={{ color: 'var(--text-muted)' }} />
        )}
      </div>
    </div>
  );
}
