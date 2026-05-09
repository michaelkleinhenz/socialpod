import { useState, useEffect, useRef } from 'react';
import { useNavigate, useSearchParams } from 'react-router-dom';
import { useAuth } from '../../contexts/AuthContext';
import { api } from '../../services/api';
import toast from 'react-hot-toast';
import { Calendar, Tag, MessageSquare, Send, X, ImageIcon, Video } from 'lucide-react';
import './ShareTargetPage.css';

const SHARE_CACHE = 'share-target-v1';
const SHARE_META_KEY = '/share-cache/meta';
const SHARE_FILE_PREFIX = '/share-cache/file/';

interface SharedFileInfo {
  key: string;
  name: string;
  type: string;
  size: number;
}

interface SharedMeta {
  title: string;
  text: string;
  url: string;
  files: SharedFileInfo[];
  timestamp: number;
}

function defaultScheduledAt(): string {
  const d = new Date(Date.now() + 3600_000);
  d.setSeconds(0, 0);
  return d.toISOString().slice(0, 16);
}

export function ShareTargetPage() {
  const { user, loading: authLoading } = useAuth();
  const navigate = useNavigate();
  const [searchParams] = useSearchParams();

  const [loadingMedia, setLoadingMedia] = useState(true);
  const [sharedFiles, setSharedFiles] = useState<File[]>([]);
  const [previewUrls, setPreviewUrls] = useState<string[]>([]);

  // Form state
  const [caption, setCaption] = useState('');
  const [platforms, setPlatforms] = useState<string[]>([]);
  const [scheduledAt, setScheduledAt] = useState(defaultScheduledAt);
  const [postType, setPostType] = useState<'post' | 'story' | 'reel'>('post');
  const [firstComment, setFirstComment] = useState('');
  const [tags, setTags] = useState('');
  const [submitting, setSubmitting] = useState(false);

  const objectUrlsRef = useRef<string[]>([]);

  // Redirect to login if not authenticated (preserve share state in cache)
  useEffect(() => {
    if (!authLoading && !user) {
      navigate('/login?next=/share-target' + (searchParams.has('shared') ? '%3Fshared%3D1' : ''));
    }
  }, [authLoading, user, navigate, searchParams]);

  // Load available accounts to pre-select platforms
  useEffect(() => {
    if (!user) return;
    api.getActiveAccounts()
      .then(accounts => {
        const available = [...new Set<string>(accounts.map((a: any) => a.platform))];
        setPlatforms(available);
      })
      .catch(() => {});
  }, [user]);

  // Load shared files from service worker cache
  useEffect(() => {
    if (authLoading || !user) return;

    async function load() {
      if (!searchParams.has('shared')) {
        setLoadingMedia(false);
        return;
      }
      try {
        if (!('caches' in window)) {
          setLoadingMedia(false);
          return;
        }
        const cache = await caches.open(SHARE_CACHE);
        const metaRes = await cache.match(SHARE_META_KEY);
        if (!metaRes) {
          setLoadingMedia(false);
          return;
        }

        const meta: SharedMeta = await metaRes.json();
        if (meta.text || meta.title) setCaption(meta.text || meta.title);

        const files: File[] = [];
        const urls: string[] = [];

        for (const info of meta.files) {
          const res = await cache.match(info.key.startsWith(SHARE_FILE_PREFIX)
            ? new Request(info.key)
            : new Request(`${SHARE_FILE_PREFIX}${info.name}`));
          if (!res) continue;
          const blob = await res.blob();
          const file = new File([blob], info.name, { type: info.type || blob.type });
          files.push(file);
          const objectUrl = URL.createObjectURL(blob);
          urls.push(objectUrl);
          objectUrlsRef.current.push(objectUrl);
        }

        setSharedFiles(files);
        setPreviewUrls(urls);
      } catch (err) {
        console.error('ShareTarget: failed to load shared data', err);
        toast.error('Could not load shared media');
      } finally {
        setLoadingMedia(false);
      }
    }

    load();

    return () => {
      objectUrlsRef.current.forEach(u => URL.revokeObjectURL(u));
    };
  }, [authLoading, user, searchParams]);

  const togglePlatform = (platform: string) => {
    setPlatforms(prev =>
      prev.includes(platform) ? prev.filter(p => p !== platform) : [...prev, platform]
    );
  };

  const removeFile = (index: number) => {
    URL.revokeObjectURL(previewUrls[index]);
    setSharedFiles(prev => prev.filter((_, i) => i !== index));
    setPreviewUrls(prev => prev.filter((_, i) => i !== index));
  };

  const handleSubmit = async () => {
    if (platforms.length === 0) {
      toast.error('Select at least one platform');
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

      await api.createPost(postData, sharedFiles.length > 0 ? sharedFiles : undefined);

      // Clear share cache after successful submission
      if ('caches' in window) {
        const cache = await caches.open(SHARE_CACHE);
        const keys = await cache.keys();
        await Promise.all(keys.map(k => cache.delete(k)));
      }

      toast.success('Post scheduled!');
      setTimeout(() => navigate('/'), 1200);
    } catch (err: any) {
      toast.error(err.message || 'Failed to schedule post');
    } finally {
      setSubmitting(false);
    }
  };

  if (authLoading || loadingMedia) {
    return (
      <div className="st-loading">
        <div className="spinner" />
        <p>Loading…</p>
      </div>
    );
  }

  const hasVideo = sharedFiles.some(f => f.type.startsWith('video/'));

  return (
    <div className="st-page">
      {/* Header */}
      <header className="st-header">
        <h1 className="st-title">Schedule Post</h1>
        <button className="st-close" onClick={() => navigate('/')} aria-label="Close">
          <X size={20} />
        </button>
      </header>

      <div className="st-body">
        {/* Media preview */}
        {previewUrls.length > 0 && (
          <section className="st-section">
            <div className="st-media-grid">
              {previewUrls.map((src, i) => (
                <div key={i} className="st-media-item">
                  {sharedFiles[i]?.type.startsWith('video/') ? (
                    <video src={src} className="st-media-asset" controls playsInline />
                  ) : (
                    <img src={src} alt="" className="st-media-asset" />
                  )}
                  <button
                    className="st-media-remove"
                    onClick={() => removeFile(i)}
                    aria-label="Remove"
                  >
                    <X size={14} />
                  </button>
                </div>
              ))}
            </div>
            <p className="st-media-hint">
              {sharedFiles.length} file{sharedFiles.length !== 1 ? 's' : ''} ready
              {hasVideo && <span> · <Video size={12} className="st-inline-icon" /> video</span>}
            </p>
          </section>
        )}

        {previewUrls.length === 0 && searchParams.has('shared') && (
          <section className="st-section st-no-media">
            <ImageIcon size={40} className="st-no-media-icon" />
            <p>No media attached</p>
          </section>
        )}

        {/* Caption */}
        <section className="st-section">
          <label className="st-label">Caption</label>
          <textarea
            className="st-textarea"
            value={caption}
            onChange={e => setCaption(e.target.value)}
            placeholder="Write your caption…"
            rows={4}
          />
          <div className="st-char-count">{caption.length} chars</div>
        </section>

        {/* Platforms */}
        <section className="st-section">
          <label className="st-label">Platforms</label>
          <div className="st-platform-row">
            <button
              type="button"
              className={`st-platform-btn st-bluesky ${platforms.includes('bluesky') ? 'active' : ''}`}
              onClick={() => togglePlatform('bluesky')}
            >
              <span className="st-platform-dot" />
              Bluesky
            </button>
            <button
              type="button"
              className={`st-platform-btn st-instagram ${platforms.includes('instagram') ? 'active' : ''}`}
              onClick={() => togglePlatform('instagram')}
            >
              <span className="st-platform-dot" />
              Instagram
            </button>
          </div>
        </section>

        {/* Post type (Instagram only) */}
        {platforms.includes('instagram') && (
          <section className="st-section">
            <label className="st-label">Post Type</label>
            <div className="st-segmented">
              {(['post', 'story', 'reel'] as const).map(t => (
                <button
                  key={t}
                  type="button"
                  className={`st-seg-btn ${postType === t ? 'active' : ''}`}
                  onClick={() => setPostType(t)}
                >
                  {t.charAt(0).toUpperCase() + t.slice(1)}
                </button>
              ))}
            </div>
          </section>
        )}

        {/* Schedule date/time */}
        <section className="st-section">
          <label className="st-label">
            <Calendar size={14} className="st-label-icon" />
            Schedule
          </label>
          <input
            type="datetime-local"
            className="st-input"
            value={scheduledAt}
            onChange={e => setScheduledAt(e.target.value)}
          />
        </section>

        {/* Tags */}
        <section className="st-section">
          <label className="st-label">
            <Tag size={14} className="st-label-icon" />
            Tags
          </label>
          <input
            type="text"
            className="st-input"
            value={tags}
            onChange={e => setTags(e.target.value)}
            placeholder="nature, photography, travel"
          />
          <p className="st-hint">Comma-separated</p>
        </section>

        {/* First comment (Instagram) */}
        {platforms.includes('instagram') && (
          <section className="st-section">
            <label className="st-label">
              <MessageSquare size={14} className="st-label-icon" />
              First Comment
            </label>
            <input
              type="text"
              className="st-input"
              value={firstComment}
              onChange={e => setFirstComment(e.target.value)}
              placeholder="Optional first comment…"
            />
          </section>
        )}
      </div>

      {/* Sticky footer with submit */}
      <footer className="st-footer">
        <button
          type="button"
          className="st-submit-btn"
          onClick={handleSubmit}
          disabled={submitting || platforms.length === 0}
        >
          <Send size={18} />
          {submitting ? 'Scheduling…' : 'Schedule Post'}
        </button>
      </footer>
    </div>
  );
}
