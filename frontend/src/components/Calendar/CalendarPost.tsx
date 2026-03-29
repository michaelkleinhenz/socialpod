import type { Post } from '../../types';
import { format, parseISO } from 'date-fns';
import { Image } from 'lucide-react';

interface Props {
  post: Post;
  onClick: () => void;
  isDragging?: boolean;
}

export function CalendarPost({ post, onClick, isDragging }: Props) {
  const time = format(parseISO(post.scheduledAt), 'HH:mm');
  const preview = post.content.length > 60 ? post.content.slice(0, 60) + '...' : post.content;

  return (
    <div
      className={`calendar-post ${post.status} ${isDragging ? 'dragging' : ''}`}
      onClick={onClick}
    >
      <div className="post-time">{time}</div>
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
