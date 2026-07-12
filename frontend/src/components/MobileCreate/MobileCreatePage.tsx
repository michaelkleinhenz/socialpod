import { useState, useEffect, useRef } from 'react';
import { useNavigate } from 'react-router-dom';
import { useAuth } from '../../contexts/AuthContext';
import { usePageManifest } from '../../hooks/usePageManifest';
import { api } from '../../services/api';
import type { Platform, PostType } from '../../types';
import toast from 'react-hot-toast';
import {
  Calendar, Tag, MessageSquare, Send, X, Plus, ArrowLeft,
  Image as ImageIcon, Clapperboard, Circle, Video, Zap,
} from 'lucide-react';
import './MobileCreate.css';

const PLATFORM_META: Record<Platform, { label: string; color: string }> = {
  bluesky: { label: 'Bluesky', color: 'var(--bluesky)' },
  instagram: { label: 'Instagram', color: 'var(--instagram)' },
  twitter: { label: 'X/Twitter', color: 'var(--twitter)' },
  mastodon: { label: 'Mastodon', color: 'var(--mastodon)' },
  threads: { label: 'Threads', color: 'var(--threads)' },
  linkedin: { label: 'LinkedIn', color: 'var(--linkedin)' },
  youtube: { label: 'YouTube', color: 'var(--youtube)' },
};

const TYPE_META: Record<PostType, { label: string; blurb: string; icon: typeof ImageIcon; accept: string }> = {
  post: { label: 'Post', blurb: 'Photo or carousel to the feed', icon: ImageIcon, accept: 'image/*,video/*' },
  story: { label: 'Story', blurb: '24-hour vertical story (Instagram)', icon: Circle, accept: 'image/*,video/*' },
  reel: { label: 'Reel', blurb: 'Short vertical video (Instagram)', icon: Clapperboard, accept: 'video/*' },
};

function defaultScheduledAt(): string {
  const d = new Date(Date.now() + 3600_000);
  d.setSeconds(0, 0);
  // Adjust for timezone so the datetime-local shows local wall-clock time.
  const tzOffset = d.getTimezoneOffset() * 60_000;
  return new Date(d.getTime() - tzOffset).toISOString().slice(0, 16);
}

/**
 * Standalone, phone-optimised page for quickly composing a new post.
 * Rendered outside the app Layout (no sidebar), mirroring the convention
 * mobile view — the URL is meant to be bookmarked / added to the home
 * screen so a post of a chosen type can be created on the go.
 */
export function MobileCreatePage() {
  const { user, loading: authLoading } = useAuth();
  const navigate = useNavigate();

  usePageManifest('/m/create');

  // Step 1 = choose type, Step 2 = compose. `postType` null while choosing.
  const [postType, setPostType] = useState<PostType | null>(null);

  const [availablePlatforms, setAvailablePlatforms] = useState<Platform[]>([]);
  const [platforms, setPlatforms] = useState<Platform[]>([]);
  const [caption, setCaption] = useState('');
  const [firstComment, setFirstComment] = useState('');
  const [tags, setTags] = useState('');
  const [scheduledAt, setScheduledAt] = useState(defaultScheduledAt);
  const [files, setFiles] = useState<File[]>([]);
  const [previewUrls, setPreviewUrls] = useState<string[]>([]);
  const [submitting, setSubmitting] = useState(false);

  const fileRef = useRef<HTMLInputElement>(null);
  const objectUrlsRef = useRef<string[]>([]);

  // Redirect to login if not authenticated, preserving the destination.
  useEffect(() => {
    if (!authLoading && !user) {
      navigate('/login?next=' + encodeURIComponent('/m/create'));
    }
  }, [authLoading, user, navigate]);

  // Load available platforms from the user's active accounts.
  useEffect(() => {
    if (!user) return;
    api.getActiveAccounts()
      .then(accounts => {
        const available = [...new Set<Platform>(accounts.map((a: any) => a.platform))];
        setAvailablePlatforms(available);
      })
      .catch(() => {});
  }, [user]);

  useEffect(() => () => {
    objectUrlsRef.current.forEach(u => URL.revokeObjectURL(u));
  }, []);

  const chooseType = (type: PostType) => {
    setPostType(type);
    // Stories and reels are Instagram-only; otherwise default to everything available.
    if (type === 'story' || type === 'reel') {
      setPlatforms(availablePlatforms.includes('instagram') ? ['instagram'] : []);
    } else {
      setPlatforms(availablePlatforms);
    }
  };

  const backToTypes = () => {
    setPostType(null);
  };

  const togglePlatform = (platform: Platform) => {
    setPlatforms(prev =>
      prev.includes(platform) ? prev.filter(p => p !== platform) : [...prev, platform]
    );
  };

  const addFiles = (list: FileList | null) => {
    if (!list || list.length === 0) return;
    const incoming = Array.from(list);
    // Stories and reels take a single media item.
    const next = postType === 'post' ? [...files, ...incoming] : incoming.slice(0, 1);
    // Revoke old previews when replacing.
    if (postType !== 'post') {
      previewUrls.forEach(u => URL.revokeObjectURL(u));
    }
    const urls = next.map(f => {
      const u = URL.createObjectURL(f);
      objectUrlsRef.current.push(u);
      return u;
    });
    setFiles(next);
    setPreviewUrls(urls);
  };

  const removeFile = (index: number) => {
    URL.revokeObjectURL(previewUrls[index]);
    setFiles(prev => prev.filter((_, i) => i !== index));
    setPreviewUrls(prev => prev.filter((_, i) => i !== index));
  };

  const handleSubmit = async () => {
    if (!postType) return;
    if (platforms.length === 0) {
      toast.error('Select at least one platform');
      return;
    }
    if ((postType === 'story' || postType === 'reel') && files.length === 0) {
      toast.error(`A ${postType} needs media`);
      return;
    }
    if (!scheduledAt) {
      toast.error('Choose a scheduled date and time');
      return;
    }

    setSubmitting(true);
    try {
      const tagList = tags.split(',').map(t => t.trim()).filter(Boolean);
      const postData: Record<string, any> = {
        content: caption,
        platforms,
        scheduledAt: new Date(scheduledAt).toISOString(),
        status: 'scheduled',
        postType,
      };
      if (firstComment) postData.firstComment = firstComment;
      if (tagList.length > 0) postData.tags = tagList;

      await api.createPost(postData, files.length > 0 ? files : undefined);

      toast.success(`${TYPE_META[postType].label} scheduled!`);
      // Reset back to the type chooser for the next quick post.
      previewUrls.forEach(u => URL.revokeObjectURL(u));
      setFiles([]);
      setPreviewUrls([]);
      setCaption('');
      setFirstComment('');
      setTags('');
      setScheduledAt(defaultScheduledAt());
      setPostType(null);
    } catch (err: any) {
      toast.error(err.message || 'Failed to schedule post');
    } finally {
      setSubmitting(false);
    }
  };

  if (authLoading || !user) {
    return (
      <div className="mc-loading">
        <div className="spinner" />
        <p>Loading…</p>
      </div>
    );
  }

  // ── Step 1: choose the post type ───────────────────────────────
  if (!postType) {
    return (
      <div className="mc-page">
        <header className="mc-header">
          <div className="mc-brand">
            <span className="mc-brand-icon"><Zap size={18} /></span>
            <span className="mc-brand-name">SocialPod</span>
          </div>
          <button className="mc-close" onClick={() => navigate('/')} aria-label="Close">
            <X size={20} />
          </button>
        </header>

        <div className="mc-body">
          <p className="mc-intro">What would you like to create?</p>
          <div className="mc-type-list">
            {(Object.keys(TYPE_META) as PostType[]).map(type => {
              const meta = TYPE_META[type];
              const Icon = meta.icon;
              return (
                <button key={type} className="mc-type-card" onClick={() => chooseType(type)}>
                  <span className="mc-type-icon"><Icon size={24} /></span>
                  <span className="mc-type-text">
                    <span className="mc-type-label">{meta.label}</span>
                    <span className="mc-type-blurb">{meta.blurb}</span>
                  </span>
                  <ArrowLeft size={18} className="mc-type-chevron" />
                </button>
              );
            })}
          </div>
        </div>
      </div>
    );
  }

  // ── Step 2: compose the chosen type ────────────────────────────
  const meta = TYPE_META[postType];
  const hasVideo = files.some(f => f.type.startsWith('video/'));
  // Stories and reels are Instagram-only, so only offer that platform.
  const platformOptions = (postType === 'story' || postType === 'reel')
    ? availablePlatforms.filter(p => p === 'instagram')
    : availablePlatforms;

  return (
    <div className="mc-page">
      <header className="mc-header">
        <button className="mc-close" onClick={backToTypes} aria-label="Back">
          <ArrowLeft size={20} />
        </button>
        <h1 className="mc-title">New {meta.label}</h1>
        <button className="mc-close" onClick={() => navigate('/')} aria-label="Close">
          <X size={20} />
        </button>
      </header>

      <div className="mc-body">
        {/* Media */}
        <section className="mc-section">
          <label className="mc-label">Media</label>
          <div className="mc-media-grid">
            {previewUrls.map((src, i) => (
              <div key={i} className="mc-media-item">
                {files[i]?.type.startsWith('video/') ? (
                  <video src={src} className="mc-media-asset" controls playsInline />
                ) : (
                  <img src={src} alt="" className="mc-media-asset" />
                )}
                <button className="mc-media-remove" onClick={() => removeFile(i)} aria-label="Remove">
                  <X size={14} />
                </button>
              </div>
            ))}
            {(postType === 'post' || files.length === 0) && (
              <button className="mc-media-add" onClick={() => fileRef.current?.click()}>
                <Plus size={22} />
                <span>Add</span>
              </button>
            )}
          </div>
          <input
            ref={fileRef}
            type="file"
            accept={meta.accept}
            multiple={postType === 'post'}
            hidden
            onChange={e => { addFiles(e.target.files); e.target.value = ''; }}
          />
          <p className="mc-hint">
            {postType === 'reel'
              ? 'A vertical video (9:16, MP4) works best for reels.'
              : postType === 'story'
                ? 'One image or video (9:16 recommended).'
                : files.length > 0
                  ? `${files.length} file${files.length !== 1 ? 's' : ''} attached`
                  : 'Optional for text platforms; required for Instagram.'}
            {hasVideo && <span> · <Video size={12} className="mc-inline-icon" /> video</span>}
          </p>
        </section>

        {/* Caption */}
        <section className="mc-section">
          <label className="mc-label">Caption</label>
          <textarea
            className="mc-textarea"
            value={caption}
            onChange={e => setCaption(e.target.value)}
            placeholder="Write your caption…"
            rows={4}
          />
          <div className="mc-char-count">{caption.length} chars</div>
        </section>

        {/* Platforms */}
        <section className="mc-section">
          <label className="mc-label">Platforms</label>
          {platformOptions.length === 0 ? (
            <p className="mc-hint">
              {postType === 'post'
                ? 'No active social accounts. Add one in the app first.'
                : 'No active Instagram account. Add one in the app first.'}
            </p>
          ) : (
            <div className="mc-platform-row">
              {platformOptions.map(p => {
                const pm = PLATFORM_META[p];
                const active = platforms.includes(p);
                return (
                  <button
                    key={p}
                    type="button"
                    className={`mc-platform-btn ${active ? 'active' : ''}`}
                    style={active ? { borderColor: pm.color, color: pm.color } : undefined}
                    onClick={() => togglePlatform(p)}
                  >
                    <span className="mc-platform-dot" style={{ background: pm.color }} />
                    {pm.label}
                  </button>
                );
              })}
            </div>
          )}
          {(postType === 'story' || postType === 'reel') && (
            <p className="mc-hint">Stories and reels publish to Instagram only.</p>
          )}
        </section>

        {/* Schedule */}
        <section className="mc-section">
          <label className="mc-label">
            <Calendar size={14} className="mc-label-icon" />
            Schedule
          </label>
          <input
            type="datetime-local"
            className="mc-input"
            value={scheduledAt}
            onChange={e => setScheduledAt(e.target.value)}
          />
        </section>

        {/* Tags */}
        <section className="mc-section">
          <label className="mc-label">
            <Tag size={14} className="mc-label-icon" />
            Tags
          </label>
          <input
            type="text"
            className="mc-input"
            value={tags}
            onChange={e => setTags(e.target.value)}
            placeholder="nature, photography, travel"
          />
          <p className="mc-hint">Comma-separated</p>
        </section>

        {/* First comment (Instagram) */}
        {platforms.includes('instagram') && (
          <section className="mc-section">
            <label className="mc-label">
              <MessageSquare size={14} className="mc-label-icon" />
              First Comment
            </label>
            <input
              type="text"
              className="mc-input"
              value={firstComment}
              onChange={e => setFirstComment(e.target.value)}
              placeholder="Optional first comment…"
            />
          </section>
        )}
      </div>

      <footer className="mc-footer">
        <button
          type="button"
          className="mc-submit-btn"
          onClick={handleSubmit}
          disabled={submitting || platforms.length === 0}
        >
          <Send size={18} />
          {submitting ? 'Scheduling…' : `Schedule ${meta.label}`}
        </button>
      </footer>
    </div>
  );
}
