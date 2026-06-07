import { useState, useEffect, useRef, useMemo } from 'react';
import { format } from 'date-fns';
import { api } from '../../services/api';
import type { Platform, Suffix, SocialAccount, MentionEntry, TeamSettings } from '../../types';
import { Newspaper, Image, Send, Pencil, Clock, Tag, MessageSquare } from 'lucide-react';
import { PlatformIcon } from '../Common/PlatformIcon';
import { MentionTextarea } from '../PostEditor/MentionTextarea';
import toast from 'react-hot-toast';
import '../PostEditor/PostEditor.css';

const BLUESKY_LIMIT = 300;
const INSTAGRAM_LIMIT = 2200;
const TWITTER_LIMIT = 280;
const MASTODON_LIMIT = 500;
const LINKEDIN_LIMIT = 3000;

export function NewsPage() {
  // Plugin availability
  const [pluginReady, setPluginReady] = useState<boolean | null>(null);

  // News fields
  const [episodeNumber, setEpisodeNumber] = useState('');
  const [newsTagline, setNewsTagline] = useState('');
  const [articleUrl, setArticleUrl] = useState('');
  const [shownotes, setShownotes] = useState('');

  // Image handling
  type ImageItem = { kind: 'url'; url: string } | { kind: 'file'; file: File };
  const [images, setImages] = useState<ImageItem[]>([]);
  const objUrlCache = useRef<Map<File, string>>(new Map());
  const fileRef = useRef<HTMLInputElement>(null);

  // Social posting toggle
  const [addSocialPost, setAddSocialPost] = useState(false);

  // Social posting fields
  const [content, setContent] = useState('');
  const [contentOverrides, setContentOverrides] = useState<Record<string, string>>({});
  const [customizePerPlatform, setCustomizePerPlatform] = useState(false);
  const [firstComment, setFirstComment] = useState('');
  const [platforms, setPlatforms] = useState<Platform[]>([]);
  const [scheduledAt, setScheduledAt] = useState(
    format(new Date(Date.now() + 3600000), "yyyy-MM-dd'T'HH:mm")
  );
  const [status, setStatus] = useState<'scheduled' | 'draft'>('scheduled');
  const [suffixes, setSuffixes] = useState<Suffix[]>([]);
  const [suffixIds, setSuffixIds] = useState<Record<string, string>>({});
  const [mentions, setMentions] = useState<MentionEntry[]>([]);
  const [accounts, setAccounts] = useState<SocialAccount[]>([]);
  const [accountsLoaded, setAccountsLoaded] = useState(false);

  const [submitting, setSubmitting] = useState(false);

  const apiUrl = import.meta.env.VITE_API_URL || '';

  // Check plugin availability on mount
  useEffect(() => {
    api.getTeamSettings()
      .then((s: TeamSettings) => {
        if (s.enabledPlugins?.includes('news_creator') && s.newsCreatorUrl && s.hasNewsCreatorBearerToken) {
          setPluginReady(true);
        } else {
          setPluginReady(false);
        }
      })
      .catch(() => setPluginReady(false));

    return () => {
      objUrlCache.current.forEach(url => URL.revokeObjectURL(url));
    };
  }, []);

  // Load social posting data when toggle is enabled
  useEffect(() => {
    if (!addSocialPost || accountsLoaded) return;
    api.getActiveAccounts().then(accs => { setAccounts(accs); setAccountsLoaded(true); }).catch(() => setAccountsLoaded(true));
    api.getSuffixes().then(setSuffixes).catch(() => {});
    api.getMentions().then(setMentions).catch(() => {});
  }, [addSocialPost, accountsLoaded]);

  // Filter platforms when accounts load
  useEffect(() => {
    if (!accountsLoaded) return;
    setPlatforms(prev => {
      if (prev.length === 0) {
        // Auto-select the first available platform
        const first = accounts[0];
        return first ? [first.platform] : [];
      }
      return prev.filter(p => accounts.some(a => a.platform === p));
    });
  }, [accountsLoaded]); // eslint-disable-line react-hooks/exhaustive-deps

  // Collapse per-platform mode when platforms drop to 1 or fewer
  useEffect(() => {
    if (customizePerPlatform && platforms.length <= 1) {
      if (platforms.length === 1) {
        const override = contentOverrides[platforms[0]];
        if (override) setContent(override);
      }
      setContentOverrides({});
      setCustomizePerPlatform(false);
    }
  }, [platforms.length]); // eslint-disable-line react-hooks/exhaustive-deps

  const previewUrl = useMemo(() => (item: ImageItem) => {
    if (item.kind === 'url') return item.url.startsWith('/') ? apiUrl + item.url : item.url;
    if (!objUrlCache.current.has(item.file)) {
      objUrlCache.current.set(item.file, URL.createObjectURL(item.file));
    }
    return objUrlCache.current.get(item.file)!;
  }, [apiUrl]);

  // Account lookups
  const blueskyAccount = accounts.find(a => a.platform === 'bluesky') ?? null;
  const instagramAccount = accounts.find(a => a.platform === 'instagram') ?? null;
  const twitterAccount = accounts.find(a => a.platform === 'twitter') ?? null;
  const mastodonAccount = accounts.find(a => a.platform === 'mastodon') ?? null;
  const threadsAccount = accounts.find(a => a.platform === 'threads') ?? null;
  const linkedinAccount = accounts.find(a => a.platform === 'linkedin') ?? null;

  const togglePlatform = (p: Platform) => {
    setPlatforms(prev =>
      prev.includes(p) ? prev.filter(x => x !== p) : [...prev, p]
    );
  };

  const suffixLen = (platform: Platform) => {
    const id = suffixIds[platform];
    if (!id) return 0;
    const s = suffixes.find(x => x.id === id);
    return s ? s.content.length + 1 : 0;
  };

  const platformLimit = (platform: Platform) => {
    if (platform === 'bluesky') return BLUESKY_LIMIT;
    if (platform === 'twitter') return TWITTER_LIMIT;
    if (platform === 'mastodon') return MASTODON_LIMIT;
    if (platform === 'linkedin') return LINKEDIN_LIMIT;
    return INSTAGRAM_LIMIT;
  };

  const effectiveLimit = (platform: Platform) =>
    platformLimit(platform) - suffixLen(platform);

  const charLimit = platforms.length === 0 ? effectiveLimit('instagram') : Math.min(
    ...(platforms.includes('bluesky') ? [effectiveLimit('bluesky')] : []),
    ...(platforms.includes('instagram') ? [effectiveLimit('instagram')] : []),
    ...(platforms.includes('twitter') ? [effectiveLimit('twitter')] : []),
    ...(platforms.includes('mastodon') ? [effectiveLimit('mastodon')] : []),
    ...(platforms.includes('linkedin') ? [effectiveLimit('linkedin')] : []),
  );
  const charCount = content.length;
  const overLimit = (customizePerPlatform && platforms.length > 1)
    ? platforms.some(p => {
        const val = contentOverrides[p];
        return val != null && val.length > effectiveLimit(p);
      })
    : charCount > charLimit;
  const charClass = charCount > charLimit ? 'danger' : charCount > charLimit * 0.9 ? 'warning' : '';

  const handleImageUpload = (files: FileList | null) => {
    if (!files) return;
    const items = Array.from(files).map(file => ({ kind: 'file' as const, file }));
    setImages(prev => [...prev, ...items]);
  };

  const removeImage = (idx: number) => {
    setImages(prev => {
      const item = prev[idx];
      if (item.kind === 'file') {
        const cached = objUrlCache.current.get(item.file);
        if (cached) { URL.revokeObjectURL(cached); objUrlCache.current.delete(item.file); }
      }
      return prev.filter((_, i) => i !== idx);
    });
  };

  const resetForm = () => {
    setEpisodeNumber('');
    setNewsTagline('');
    setArticleUrl('');
    setShownotes('');
    setImages([]);
    setAddSocialPost(false);
    setContent('');
    setContentOverrides({});
    setCustomizePerPlatform(false);
    setFirstComment('');
    setPlatforms([]);
    setScheduledAt(format(new Date(Date.now() + 3600000), "yyyy-MM-dd'T'HH:mm"));
    setStatus('scheduled');
    setSuffixIds({});
  };

  const handleSubmit = async () => {
    // Validate news fields
    if (!episodeNumber.trim()) { toast.error('Episode number is required'); return; }
    if (!newsTagline.trim()) { toast.error('News tagline is required'); return; }
    if (!articleUrl.trim()) { toast.error('Article URL is required'); return; }

    // Validate social post fields if enabled
    if (addSocialPost) {
      if (platforms.length === 0) { toast.error('Select at least one platform'); return; }
      if (customizePerPlatform && platforms.length > 1) {
        const missing = platforms.filter(p => !(contentOverrides[p] || '').trim());
        if (missing.length > 0) { toast.error(`Content is required for ${missing.join(', ')}`); return; }
        const over = platforms.find(p => (contentOverrides[p] || '').length > effectiveLimit(p));
        if (over) { toast.error(`Content for ${over} exceeds ${effectiveLimit(over)} character limit`); return; }
      } else {
        if (!content.trim()) { toast.error('Post content is required'); return; }
        if (content.length > charLimit) { toast.error(`Content exceeds ${charLimit} character limit`); return; }
      }
    }

    const imageFiles = images.filter(i => i.kind === 'file').map(i => (i as { kind: 'file'; file: File }).file);
    const imageUrls = images.filter(i => i.kind === 'url').map(i => (i as { kind: 'url'; url: string }).url);

    const data: any = {
      episodeNumber: episodeNumber.trim(),
      newsTagline: newsTagline.trim(),
      articleUrl: articleUrl.trim(),
      shownotes: shownotes.trim() || undefined,
      imageUrls,
    };

    data.addSocialPosting = addSocialPost;

    if (addSocialPost) {
      const accountIds: Record<string, string> = {};
      if (platforms.includes('bluesky') && blueskyAccount) accountIds['bluesky'] = blueskyAccount.id;
      if (platforms.includes('instagram') && instagramAccount) accountIds['instagram'] = instagramAccount.id;
      if (platforms.includes('twitter') && twitterAccount) accountIds['twitter'] = twitterAccount.id;
      if (platforms.includes('mastodon') && mastodonAccount) accountIds['mastodon'] = mastodonAccount.id;
      if (platforms.includes('threads') && threadsAccount) accountIds['threads'] = threadsAccount.id;
      if (platforms.includes('linkedin') && linkedinAccount) accountIds['linkedin'] = linkedinAccount.id;

      data.content = content || '';
      data.firstComment = firstComment.trim() || undefined;
      data.platforms = platforms;
      data.scheduledAt = new Date(scheduledAt).toISOString();
      data.status = status;
      data.suffixIds = suffixIds;
      data.contentOverrides = customizePerPlatform ? contentOverrides : {};
      data.accountIds = accountIds;
      data.postType = 'post';
    }

    setSubmitting(true);
    try {
      await api.submitNews(data, imageFiles.length > 0 ? imageFiles : undefined);
      toast.success('News submitted successfully');
      resetForm();
    } catch (err: any) {
      toast.error(err.message || 'Failed to submit news');
    } finally {
      setSubmitting(false);
    }
  };

  if (pluginReady === null) {
    return (
      <div className="page">
        <div className="loading-screen"><div className="spinner" /></div>
      </div>
    );
  }

  if (!pluginReady) {
    return (
      <div className="page">
        <div className="page-header">
          <h1><Newspaper size={24} /> News Creator</h1>
        </div>
        <div className="empty-state">
          <Newspaper size={48} />
          <p>The News Creator plugin is not enabled for your team.</p>
          <p style={{ color: 'var(--text-muted)', fontSize: 14 }}>
            Ask your team admin to enable the News Creator plugin and configure the webhook settings.
          </p>
        </div>
      </div>
    );
  }

  const suffixPlatformLabels: Record<string, string> = {
    bluesky: 'Bluesky',
    instagram: 'Instagram',
    twitter: 'X/Twitter',
    mastodon: 'Mastodon',
    threads: 'Threads',
    linkedin: 'LinkedIn',
  };

  return (
    <div className="page">
      <div className="page-header">
        <h1><Newspaper size={24} /> News Creator</h1>
      </div>

      <div style={{ maxWidth: 720 }}>
        <div className="card" style={{ padding: 24 }}>
          {/* Episode Number */}
          <div className="form-group">
            <label>Episode Number <span style={{ color: 'var(--danger)' }}>*</span></label>
            <input
              type="text"
              className="input"
              value={episodeNumber}
              onChange={e => setEpisodeNumber(e.target.value)}
              placeholder="e.g. 42"
            />
          </div>

          {/* News Tagline */}
          <div className="form-group">
            <label>News Tagline <span style={{ color: 'var(--danger)' }}>*</span></label>
            <input
              type="text"
              className="input"
              value={newsTagline}
              onChange={e => setNewsTagline(e.target.value)}
              placeholder="Short tagline for the episode"
            />
          </div>

          {/* Article URL */}
          <div className="form-group">
            <label>Article URL <span style={{ color: 'var(--danger)' }}>*</span></label>
            <input
              type="url"
              className="input"
              value={articleUrl}
              onChange={e => setArticleUrl(e.target.value)}
              placeholder="https://example.com/article"
            />
          </div>

          {/* Shownotes */}
          <div className="form-group">
            <label>Shownotes <span style={{ color: 'var(--text-muted)', fontWeight: 400 }}>(optional)</span></label>
            <textarea
              className="textarea"
              value={shownotes}
              onChange={e => setShownotes(e.target.value)}
              placeholder="Additional show notes..."
              rows={4}
            />
          </div>

          {/* Image upload */}
          <div className="form-group">
            <label><Image size={14} /> Images</label>
            <div className="editor-images">
              {images.length > 0 && (
                <div className="image-preview-grid">
                  {images.map((item, i) => (
                    <div key={i} className="image-preview">
                      <img src={previewUrl(item)} alt="" />
                      <button className="remove-btn" onClick={() => removeImage(i)}>x</button>
                    </div>
                  ))}
                </div>
              )}
              <input
                ref={fileRef}
                type="file"
                accept="image/*"
                multiple
                style={{ display: 'none' }}
                onChange={e => handleImageUpload(e.target.files)}
              />
              <button className="btn btn-ghost btn-sm" onClick={() => fileRef.current?.click()} style={{ marginTop: 8 }}>
                <Image size={16} /> Upload Images
              </button>
            </div>
          </div>

          {/* Social Posting Toggle */}
          <div className="form-group" style={{ borderTop: '1px solid var(--border)', paddingTop: 20, marginTop: 12 }}>
            <label style={{ display: 'flex', alignItems: 'center', gap: 8, cursor: 'pointer', margin: 0 }}>
              <input
                type="checkbox"
                checked={addSocialPost}
                onChange={e => setAddSocialPost(e.target.checked)}
              />
              Add as Social Media Posting
            </label>
          </div>

          {/* Social Posting Fields */}
          {addSocialPost && (
            <div style={{ display: 'flex', flexDirection: 'column', gap: 0, padding: '16px', background: 'var(--bg-secondary, #1e293b)', borderRadius: 8, border: '1px solid var(--border)', marginTop: 8 }}>
              {/* Platform selector */}
              <div className="form-group">
                <label>Platforms</label>
                <div className="platform-selector">
                  <div
                    className={`platform-option bluesky ${platforms.includes('bluesky') ? 'selected' : ''} ${!blueskyAccount ? 'disabled' : ''}`}
                    onClick={() => blueskyAccount && togglePlatform('bluesky')}
                    title={!blueskyAccount ? 'No Bluesky account configured' : `Will post as @${blueskyAccount.accountName}`}
                  >
                    <PlatformIcon platform="bluesky" size={18} />
                  </div>
                  <div
                    className={`platform-option instagram ${platforms.includes('instagram') ? 'selected' : ''} ${!instagramAccount ? 'disabled' : ''}`}
                    onClick={() => instagramAccount && togglePlatform('instagram')}
                    title={!instagramAccount ? 'No Instagram account configured' : `Will post as @${instagramAccount.accountName}`}
                  >
                    <PlatformIcon platform="instagram" size={18} />
                  </div>
                  <div
                    className={`platform-option twitter ${platforms.includes('twitter') ? 'selected' : ''} ${!twitterAccount ? 'disabled' : ''}`}
                    onClick={() => twitterAccount && togglePlatform('twitter')}
                    title={!twitterAccount ? 'No X/Twitter account configured' : `Will post as @${twitterAccount.accountName}`}
                  >
                    <PlatformIcon platform="twitter" size={18} />
                  </div>
                  <div
                    className={`platform-option mastodon ${platforms.includes('mastodon') ? 'selected' : ''} ${!mastodonAccount ? 'disabled' : ''}`}
                    onClick={() => mastodonAccount && togglePlatform('mastodon')}
                    title={!mastodonAccount ? 'No Mastodon account configured' : `Will post as @${mastodonAccount.accountName}`}
                  >
                    <PlatformIcon platform="mastodon" size={18} />
                  </div>
                  <div
                    className={`platform-option threads ${platforms.includes('threads') ? 'selected' : ''} ${!threadsAccount ? 'disabled' : ''}`}
                    onClick={() => threadsAccount && togglePlatform('threads')}
                    title={!threadsAccount ? 'No Threads account configured' : `Will post as @${threadsAccount.accountName}`}
                  >
                    <PlatformIcon platform="threads" size={18} />
                  </div>
                  <div
                    className={`platform-option linkedin ${platforms.includes('linkedin') ? 'selected' : ''} ${!linkedinAccount ? 'disabled' : ''}`}
                    onClick={() => linkedinAccount && togglePlatform('linkedin')}
                    title={!linkedinAccount ? 'No LinkedIn account configured' : `Will post as ${linkedinAccount.displayName || linkedinAccount.accountName}`}
                  >
                    <PlatformIcon platform="linkedin" size={18} />
                  </div>
                </div>
              </div>

              {/* Content */}
              <div className="form-group">
                <div style={{ display: 'flex', alignItems: 'center', justifyContent: 'space-between', marginBottom: 4 }}>
                  <label style={{ margin: 0 }}>Content</label>
                  {platforms.length > 1 && (
                    <label className="customize-per-platform-toggle" style={{ display: 'flex', alignItems: 'center', gap: 6, fontSize: '0.8rem', cursor: 'pointer', margin: 0 }}>
                      <input
                        type="checkbox"
                        checked={customizePerPlatform}
                        onChange={e => {
                          const checked = e.target.checked;
                          if (checked) {
                            const baseText = content;
                            setCustomizePerPlatform(true);
                            setContent('');
                            const newOverrides: Record<string, string> = {};
                            for (const p of platforms) {
                              newOverrides[p] = baseText;
                            }
                            setContentOverrides(newOverrides);
                          } else {
                            let longest = '';
                            for (const p of platforms) {
                              const val = contentOverrides[p] || '';
                              if (val.length > longest.length) longest = val;
                            }
                            setContent(longest);
                            setContentOverrides({});
                            setCustomizePerPlatform(false);
                          }
                        }}
                      />
                      Customize per platform
                    </label>
                  )}
                </div>

                {customizePerPlatform && platforms.length > 1 ? (
                  <>
                    {platforms.map(platform => {
                      const overrideVal = contentOverrides[platform] ?? '';
                      const limit = effectiveLimit(platform);
                      const count = overrideVal.length;
                      const cls = count > limit ? 'danger' : count > limit * 0.9 ? 'warning' : '';
                      return (
                        <div key={platform} style={{ marginBottom: 8 }}>
                          <div style={{ display: 'flex', alignItems: 'center', gap: 6, marginBottom: 4 }}>
                            <PlatformIcon platform={platform} size={12} />
                            <span style={{ fontSize: '0.8rem', textTransform: 'capitalize' }}>{platform}</span>
                          </div>
                          <MentionTextarea
                            className="textarea post-textarea"
                            value={overrideVal}
                            onChange={val => setContentOverrides(prev => ({ ...prev, [platform]: val }))}
                            mentions={mentions}
                            platform={platform}
                            placeholder={`Text for ${platform}...`}
                            rows={4}
                          />
                          <div className={`char-counter ${cls}`} style={{ textAlign: 'right' }}>{count} / {limit}</div>
                        </div>
                      );
                    })}
                  </>
                ) : (
                  <>
                    <MentionTextarea
                      className="textarea post-textarea"
                      value={content}
                      onChange={setContent}
                      mentions={mentions}
                      placeholder="What's on your mind? Use #hashtags for tags..."
                      rows={5}
                    />
                    <div className={`char-counter ${charClass}`} style={{ textAlign: 'right' }}>
                      {charCount} / {charLimit}
                    </div>
                  </>
                )}
              </div>

              {/* Suffix selectors */}
              {suffixes.length > 0 && platforms.length > 0 && (
                <div className="suffix-selectors">
                  {platforms.map(platform => (
                    <div key={platform} className="suffix-selector-row">
                      <label className="suffix-label">
                        <PlatformIcon platform={platform} size={12} /> {suffixPlatformLabels[platform] ?? platform} suffix
                      </label>
                      <select
                        className="select suffix-select"
                        value={suffixIds[platform] || ''}
                        onChange={e => setSuffixIds(prev => {
                          const next = { ...prev };
                          if (e.target.value) next[platform] = e.target.value;
                          else delete next[platform];
                          return next;
                        })}
                      >
                        <option value="">None</option>
                        {suffixes.map(s => (
                          <option key={s.id} value={s.id}>{s.name}</option>
                        ))}
                      </select>
                    </div>
                  ))}
                </div>
              )}

              {/* First Comment */}
              <div className="form-group">
                <label style={{ whiteSpace: 'nowrap' }}><MessageSquare size={14} /> First Comment <span style={{ color: 'var(--text-muted)', fontWeight: 400 }}>(optional)</span></label>
                <textarea
                  className="textarea first-comment-textarea"
                  value={firstComment}
                  onChange={e => setFirstComment(e.target.value)}
                  placeholder="Add a first comment to your post..."
                  rows={3}
                />
              </div>

              {/* Schedule */}
              <div className="form-group">
                <label><Clock size={14} /> Schedule</label>
                <input
                  type="datetime-local"
                  className="input"
                  value={scheduledAt}
                  onChange={e => setScheduledAt(e.target.value)}
                />
              </div>

              {/* Status */}
              <div className="form-group">
                <label><Tag size={14} /> Status</label>
                <select className="select" value={status} onChange={e => setStatus(e.target.value as 'scheduled' | 'draft')}>
                  <option value="scheduled">Scheduled</option>
                  <option value="draft">Draft</option>
                </select>
              </div>
            </div>
          )}

          {/* Submit */}
          <div style={{ marginTop: 24, display: 'flex', justifyContent: 'flex-end' }}>
            <button
              className="btn btn-primary"
              onClick={handleSubmit}
              disabled={submitting || overLimit}
            >
              <Send size={16} /> {submitting ? 'Submitting...' : 'Submit News'}
            </button>
          </div>
        </div>
      </div>
    </div>
  );
}
