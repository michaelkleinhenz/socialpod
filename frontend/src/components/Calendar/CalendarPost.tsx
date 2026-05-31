import type { Post } from '../../types';
import { format, parseISO } from 'date-fns';
import { Image, CheckCircle, Clock, AlertTriangle, FileEdit, Film } from 'lucide-react';
import { PlatformIcon } from '../Common/PlatformIcon';

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
  const isStory = post.postType === 'story';
  const isReel = post.postType === 'reel';
  const time = format(parseISO(post.scheduledAt), 'HH:mm');
  const displayContent = post.content
    || (post.contentOverrides && Object.keys(post.contentOverrides).length > 0
      ? post.contentOverrides[post.platforms[0]] || Object.values(post.contentOverrides).find(v => v) || ''
      : '');
  const preview = isStory
    ? 'Story'
    : isReel
    ? (displayContent ? (displayContent.length > 60 ? displayContent.slice(0, 60) + '...' : displayContent) : 'Reel')
    : (displayContent.length > 60 ? displayContent.slice(0, 60) + '...' : displayContent);
  const cfg = statusConfig[post.status] || statusConfig.scheduled;
  const StatusIcon = cfg.icon;

  return (
    <div
      className={`calendar-post ${post.status} ${isStory ? 'story' : ''} ${isReel ? 'reel' : ''} ${isDragging ? 'dragging' : ''}`}
      onClick={onClick}
    >
      <div className="post-top-row">
        <span className="post-time">{time}</span>
        <span className="post-status-icon" title={cfg.label}>
          <StatusIcon size={12} color={cfg.color} />
        </span>
      </div>
      <div className="post-preview">
        {(isStory || isReel) && <Film size={11} style={{ marginRight: 4, verticalAlign: -1 }} />}
        {preview}
      </div>
      <div className="post-meta">
        {post.platforms.map(p => (
          <PlatformIcon key={p} platform={p} size={10} />
        ))}
        {post.imageUrls && post.imageUrls.length > 0 && (
          <Image size={10} style={{ color: 'var(--text-muted)' }} />
        )}
      </div>
    </div>
  );
}
