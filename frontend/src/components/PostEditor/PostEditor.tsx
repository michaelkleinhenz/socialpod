import { useState, useRef, useEffect, useCallback, useMemo } from 'react';
import { format, parseISO } from 'date-fns';
import { api } from '../../services/api';
import type { Post, Platform, PostType, Suffix, SocialAccount, MentionEntry, TeamSettings } from '../../types';
import { X, Image, Send, Trash2, Clock, Tag, Wand2, MessageSquare, Sparkles, BadgeCheck, Film, Pencil, Newspaper } from 'lucide-react';
import { PlatformIcon } from '../Common/PlatformIcon';
import toast from 'react-hot-toast';
import FilerobotImageEditor, { TABS } from 'react-filerobot-image-editor';
import { MentionTextarea } from './MentionTextarea';
import './PostEditor.css';

// Module-level cache so the SDK is initialized once per page load and login
// state persists across PostEditor open/close cycles.
let _ccEditor: any = null;
let _ccClientId = '';

interface Props {
  post?: Post | null;
  postType?: PostType;
  defaultDate?: Date | null;
  onSave: (data: any, files?: File[]) => void;
  onDelete?: () => void;
  onClose: () => void;
}

const BLUESKY_LIMIT = 300;
const INSTAGRAM_LIMIT = 2200;
const TWITTER_LIMIT = 280;
const MASTODON_LIMIT = 500;
const LINKEDIN_LIMIT = 3000;

function renderWithHashtags(text: string) {
  if (!text) return null;
  const parts = text.split(/(#[\w\u00C0-\u024F]+)/g);
  return parts.map((part, i) =>
    part.startsWith('#')
      ? <span key={i} className="preview-hashtag">{part}</span>
      : <span key={i}>{part}</span>
  );
}

interface PreviewProps {
  content: string;
  contentOverrides?: Record<string, string>;
  platforms: Platform[];
  imageUrls: string[];
  scheduledAt: string;
  apiUrl: string;
  blueskyAccount?: SocialAccount | null;
  instagramAccount?: SocialAccount | null;
  twitterAccount?: SocialAccount | null;
  mastodonAccount?: SocialAccount | null;
  linkedinAccount?: SocialAccount | null;
  bluskySuffix?: string;
  instagramSuffix?: string;
  twitterSuffix?: string;
  mastodonSuffix?: string;
  linkedinSuffix?: string;
}

function PostPreview({ content, contentOverrides, platforms, imageUrls, scheduledAt, apiUrl, blueskyAccount, instagramAccount, twitterAccount, mastodonAccount, linkedinAccount, bluskySuffix, instagramSuffix, twitterSuffix, mastodonSuffix, linkedinSuffix }: PreviewProps) {
  const time = scheduledAt
    ? format(parseISO(new Date(scheduledAt).toISOString()), 'MMM d, yyyy · HH:mm')
    : '';

  const bskyDisplayName = blueskyAccount?.displayName || blueskyAccount?.accountName || 'Your Name';
  const bskyHandle = blueskyAccount?.accountName
    ? (blueskyAccount.accountName.includes('.') ? blueskyAccount.accountName : `${blueskyAccount.accountName}.bsky.social`)
    : 'yourhandle.bsky.social';
  const igHandle = instagramAccount?.accountName || 'yourhandle';

  const bskyBase = (contentOverrides?.['bluesky'] ?? '') || content;
  const igBase = (contentOverrides?.['instagram'] ?? '') || content;
  const twBase = (contentOverrides?.['twitter'] ?? '') || content;
  const mstBase = (contentOverrides?.['mastodon'] ?? '') || content;
  const liBase = (contentOverrides?.['linkedin'] ?? '') || content;
  const bskyContent = bluskySuffix ? bskyBase + '\n' + bluskySuffix : bskyBase;
  const igContent = instagramSuffix ? igBase + '\n' + instagramSuffix : igBase;
  const twContent = twitterSuffix ? twBase + '\n' + twitterSuffix : twBase;
  const mstContent = mastodonSuffix ? mstBase + '\n' + mastodonSuffix : mstBase;
  const liContent = linkedinSuffix ? liBase + '\n' + linkedinSuffix : liBase;

  if (platforms.length === 0) {
    return (
      <div className="preview-empty">
        <p>Select a platform to see a preview</p>
      </div>
    );
  }

  const bskyAvatarUrl = blueskyAccount?.avatarUrl;
  const igAvatarUrl = instagramAccount?.avatarUrl;
  const twDisplayName = twitterAccount?.displayName || twitterAccount?.accountName || 'Your Name';
  const twHandle = twitterAccount?.accountName || 'yourhandle';
  const twAvatarUrl = twitterAccount?.avatarUrl;
  const mstDisplayName = mastodonAccount?.displayName || mastodonAccount?.accountName || 'Your Name';
  const mstHandle = mastodonAccount?.accountName || 'yourhandle';
  const mstInstance = mastodonAccount?.mastodonInstance || 'mastodon.social';
  const mstAvatarUrl = mastodonAccount?.avatarUrl;
  const liDisplayName = linkedinAccount?.displayName || linkedinAccount?.accountName || 'Your Name';
  const liAvatarUrl = linkedinAccount?.avatarUrl;

  return (
    <div className="preview-cards">
      {platforms.includes('bluesky') && (
        <div className="preview-card preview-bluesky">
          <div className="preview-card-header">
            {bskyAvatarUrl
              ? <img src={bskyAvatarUrl} alt="" className="preview-avatar" />
              : <div className="preview-avatar preview-avatar-placeholder" />}
            <div className="preview-user-info">
              <span className="preview-display-name">{bskyDisplayName}</span>
              <span className="preview-handle">@{bskyHandle}</span>
            </div>
            <div className="preview-platform-badge bluesky">Bluesky</div>
          </div>
          <div className="preview-content">
            {content
              ? <p className="preview-text">{renderWithHashtags(bskyContent)}</p>
              : <p className="preview-placeholder">Start typing to see a preview…</p>
            }
          </div>
          {imageUrls.length > 0 && (
            <div className={`preview-images preview-images-${Math.min(imageUrls.length, 4)}`}>
              {imageUrls.slice(0, 4).map((url, i) => {
                const src = url.startsWith('/') ? apiUrl + url : url;
                return /\.(mp4|mov)$/i.test(url)
                  ? <video key={i} src={src} muted className="preview-image" />
                  : <img key={i} src={src} alt="" className="preview-image" />;
              })}
            </div>
          )}
          <div className="preview-footer">
            <span className="preview-time">{time}</span>
          </div>
        </div>
      )}

      {platforms.includes('instagram') && (
        <div className="preview-card preview-instagram">
          <div className="preview-card-header">
            {igAvatarUrl
              ? <img src={igAvatarUrl} alt="" className="preview-avatar" />
              : <div className="preview-avatar preview-avatar-placeholder" />}
            <div className="preview-user-info">
              <span className="preview-display-name">
                {igHandle}
                <BadgeCheck size={14} className="preview-ig-verified" />
              </span>
            </div>
            <div className="preview-platform-badge instagram">Instagram</div>
          </div>
          {imageUrls.length > 0 ? (
            <div className="preview-ig-image">
              <img src={imageUrls[0].startsWith('/') ? apiUrl + imageUrls[0] : imageUrls[0]} alt="" />
            </div>
          ) : (
            <div className="preview-ig-image-placeholder">
              <Image size={32} />
              <span>No image selected</span>
            </div>
          )}
          <div className="preview-content preview-ig-caption">
            {content
              ? <p className="preview-text"><strong className="preview-display-name">{igHandle}</strong>{' '}{renderWithHashtags(igContent)}</p>
              : <p className="preview-placeholder">Start typing to see a preview…</p>
            }
          </div>
        </div>
      )}

      {platforms.includes('twitter') && (
        <div className="preview-card preview-twitter">
          <div className="preview-card-header">
            {twAvatarUrl
              ? <img src={twAvatarUrl} alt="" className="preview-avatar" />
              : <div className="preview-avatar preview-avatar-placeholder" />}
            <div className="preview-user-info">
              <span className="preview-display-name">{twDisplayName}</span>
              <span className="preview-handle">@{twHandle}</span>
            </div>
            <div className="preview-platform-badge twitter">X / Twitter</div>
          </div>
          <div className="preview-content">
            {content
              ? <p className="preview-text">{renderWithHashtags(twContent)}</p>
              : <p className="preview-placeholder">Start typing to see a preview…</p>
            }
          </div>
          {imageUrls.length > 0 && (
            <div className={`preview-images preview-images-${Math.min(imageUrls.length, 4)}`}>
              {imageUrls.slice(0, 4).map((url, i) => {
                const src = url.startsWith('/') ? apiUrl + url : url;
                return /\.(mp4|mov)$/i.test(url)
                  ? <video key={i} src={src} muted className="preview-image" />
                  : <img key={i} src={src} alt="" className="preview-image" />;
              })}
            </div>
          )}
          <div className="preview-footer">
            <span className="preview-time">{time}</span>
          </div>
        </div>
      )}

      {platforms.includes('mastodon') && (
        <div className="preview-card preview-mastodon">
          <div className="preview-card-header">
            {mstAvatarUrl
              ? <img src={mstAvatarUrl} alt="" className="preview-avatar" />
              : <div className="preview-avatar preview-avatar-placeholder" />}
            <div className="preview-user-info">
              <span className="preview-display-name">{mstDisplayName}</span>
              <span className="preview-handle">@{mstHandle}@{mstInstance}</span>
            </div>
            <div className="preview-platform-badge mastodon">Mastodon</div>
          </div>
          <div className="preview-content">
            {content
              ? <p className="preview-text">{renderWithHashtags(mstContent)}</p>
              : <p className="preview-placeholder">Start typing to see a preview…</p>
            }
          </div>
          {imageUrls.length > 0 && (
            <div className={`preview-images preview-images-${Math.min(imageUrls.length, 4)}`}>
              {imageUrls.slice(0, 4).map((url, i) => {
                const src = url.startsWith('/') ? apiUrl + url : url;
                return /\.(mp4|mov)$/i.test(url)
                  ? <video key={i} src={src} muted className="preview-image" />
                  : <img key={i} src={src} alt="" className="preview-image" />;
              })}
            </div>
          )}
          <div className="preview-footer">
            <span className="preview-time">{time}</span>
          </div>
        </div>
      )}

      {platforms.includes('linkedin') && (
        <div className="preview-card preview-linkedin">
          <div className="preview-card-header">
            {liAvatarUrl
              ? <img src={liAvatarUrl} alt="" className="preview-avatar" />
              : <div className="preview-avatar preview-avatar-placeholder" />}
            <div className="preview-user-info">
              <span className="preview-display-name">{liDisplayName}</span>
            </div>
            <div className="preview-platform-badge linkedin">LinkedIn</div>
          </div>
          <div className="preview-content">
            {content
              ? <p className="preview-text">{renderWithHashtags(liContent)}</p>
              : <p className="preview-placeholder">Start typing to see a preview…</p>
            }
          </div>
          {imageUrls.length > 0 && (
            <div className={`preview-images preview-images-${Math.min(imageUrls.length, 4)}`}>
              {imageUrls.slice(0, 4).map((url, i) => {
                const src = url.startsWith('/') ? apiUrl + url : url;
                return /\.(mp4|mov)$/i.test(url)
                  ? <video key={i} src={src} muted className="preview-image" />
                  : <img key={i} src={src} alt="" className="preview-image" />;
              })}
            </div>
          )}
          <div className="preview-footer">
            <span className="preview-time">{time}</span>
          </div>
        </div>
      )}
    </div>
  );
}

interface StoryPreviewProps {
  imageUrl: string;
  apiUrl: string;
  instagramAccount?: SocialAccount | null;
  isVideo?: boolean;
}

function StoryPreview({ imageUrl, apiUrl, instagramAccount, isVideo }: StoryPreviewProps) {
  const igHandle = instagramAccount?.accountName || 'yourhandle';
  const igAvatarUrl = instagramAccount?.avatarUrl;
  const src = imageUrl.startsWith('/') ? apiUrl + imageUrl : imageUrl;

  return (
    <div className="preview-cards">
      <div className="preview-story">
        <div className="preview-story-frame">
          {isVideo ? (
            <video src={src} className="preview-story-media" muted autoPlay loop playsInline />
          ) : imageUrl ? (
            <img src={src} alt="" className="preview-story-media" />
          ) : (
            <div className="preview-story-empty">
              <Image size={32} />
              <span>Add an image or video</span>
            </div>
          )}
          <div className="preview-story-header">
            {igAvatarUrl
              ? <img src={igAvatarUrl} alt="" className="preview-story-avatar" />
              : <div className="preview-story-avatar preview-avatar-placeholder" />}
            <span className="preview-story-handle">{igHandle}</span>
          </div>
        </div>
        <div className="preview-platform-badge instagram" style={{ marginTop: 8, alignSelf: 'center' }}>Instagram Story</div>
      </div>
    </div>
  );
}

interface ReelPreviewProps {
  videoUrl: string;
  apiUrl: string;
  instagramAccount?: SocialAccount | null;
  content: string;
}

function ReelPreview({ videoUrl, apiUrl, instagramAccount, content }: ReelPreviewProps) {
  const igHandle = instagramAccount?.accountName || 'yourhandle';
  const igAvatarUrl = instagramAccount?.avatarUrl;
  const src = videoUrl.startsWith('/') ? apiUrl + videoUrl : videoUrl;

  return (
    <div className="preview-cards">
      <div className="preview-story">
        <div className="preview-story-frame">
          {videoUrl ? (
            <video src={src} className="preview-story-media" muted autoPlay loop playsInline />
          ) : (
            <div className="preview-story-empty">
              <Film size={32} />
              <span>Add a video</span>
            </div>
          )}
          <div className="preview-story-header">
            {igAvatarUrl
              ? <img src={igAvatarUrl} alt="" className="preview-story-avatar" />
              : <div className="preview-story-avatar preview-avatar-placeholder" />}
            <span className="preview-story-handle">{igHandle}</span>
          </div>
          {content && (
            <div className="preview-reel-caption">{content}</div>
          )}
        </div>
        <div className="preview-platform-badge instagram" style={{ marginTop: 8, alignSelf: 'center' }}>Instagram Reel</div>
      </div>
    </div>
  );
}

export function PostEditor({ post, postType: propPostType, defaultDate, onSave, onDelete, onClose }: Props) {
  const defaultTime = defaultDate
    ? format(defaultDate, "yyyy-MM-dd'T'HH:mm")
    : format(new Date(Date.now() + 3600000), "yyyy-MM-dd'T'HH:mm");

  const isStory = (post?.postType || propPostType || 'post') === 'story';
  const isReel = (post?.postType || propPostType || 'post') === 'reel';

  const [content, setContent] = useState(post?.content || '');
  const [contentOverrides, setContentOverrides] = useState<Record<string, string>>(post?.contentOverrides || {});
  const [customizePerPlatform, setCustomizePerPlatform] = useState(
    post?.contentOverrides != null && Object.keys(post.contentOverrides).length > 0
  );
  const [firstComment, setFirstComment] = useState(post?.firstComment || '');
  const [platforms, setPlatforms] = useState<Platform[]>(post?.platforms || (isStory || isReel ? ['instagram'] : ['bluesky']));
  const [scheduledAt, setScheduledAt] = useState(
    post ? format(new Date(post.scheduledAt), "yyyy-MM-dd'T'HH:mm") : defaultTime
  );
  // Unified image list: existing server URLs and local File objects not yet uploaded.
  type ImageItem = { kind: 'url'; url: string } | { kind: 'file'; file: File };
  const [images, setImages] = useState<ImageItem[]>(
    (post?.imageUrls || []).map(url => ({ kind: 'url' as const, url }))
  );
  // Cache for object URLs so we don't create a new one on every render.
  const objUrlCache = useRef<Map<File, string>>(new Map());
  const [tags] = useState<string[]>(post?.tags || []);
  const [status, setStatus] = useState(post?.status || 'scheduled');
  const [suffixes, setSuffixes] = useState<Suffix[]>([]);
  const [suffixIds, setSuffixIds] = useState<Record<string, string>>(post?.suffixIds || {});
  const [mentions, setMentions] = useState<MentionEntry[]>([]);
  const [bggUrl, setBggUrl] = useState('');
  const [bggImportedUrl, setBggImportedUrl] = useState('');
  const [fetchingBgg, setFetchingBgg] = useState(false);
  const [bggError, setBggError] = useState('');
  const [bggEnabled, setBggEnabled] = useState(false);
  const [newsEnabled, setNewsEnabled] = useState(false);
  const [addNews, setAddNews] = useState(post?.episodeNews?.enabled || false);
  const [newsEpisodeNumber, setNewsEpisodeNumber] = useState(post?.episodeNews?.episodeNumber || '');
  const [newsTitle, setNewsTitle] = useState(post?.episodeNews?.title || '');
  const [newsAdditionalText, setNewsAdditionalText] = useState(post?.episodeNews?.additionalText || '');
  const [saving, setSaving] = useState(false);
  const [accounts, setAccounts] = useState<SocialAccount[]>([]);
  const [accountsLoaded, setAccountsLoaded] = useState(false);
  const [adobeClientId, setAdobeClientId] = useState('');
  const [adobeLoading, setAdobeLoading] = useState(false);
  const [adobeActive, setAdobeActive] = useState(false);
  const [aiEnabled, setAiEnabled] = useState(false);
  const [generating, setGenerating] = useState<boolean | Platform>(false);
  const [editingImageIdx, setEditingImageIdx] = useState<number | null>(null);
  const [watermarkGallery, setWatermarkGallery] = useState<{ url: string; previewUrl: string }[]>([]);
  const fileRef = useRef<HTMLInputElement>(null);
  const textareaRef = useRef<HTMLTextAreaElement>(null);

  useEffect(() => {
    textareaRef.current?.focus();
    api.getPublicSettings().then(s => {
      if (s.adobeExpressClientId) setAdobeClientId(s.adobeExpressClientId);
      if (s.openRouterEnabled) setAiEnabled(true);
      setBggEnabled(true);
    }).catch(() => {});
    api.getTeamSettings().then((s: TeamSettings) => {
      if (s.episodeNewsUrl && s.hasEpisodeNewsBearerToken) {
        setNewsEnabled(true);
      }
    }).catch(() => {});
    api.getSuffixes().then(setSuffixes).catch(() => {});
    api.getMentions().then(setMentions).catch(() => {});
    api.getActiveAccounts().then(accs => { setAccounts(accs); setAccountsLoaded(true); }).catch(() => { setAccountsLoaded(true); });
    api.getWatermarks().then((wms: any[]) => {
      const base = import.meta.env.VITE_API_URL || '';
      setWatermarkGallery(wms.map(w => {
        const src = w.url.startsWith('/') ? base + w.url : w.url;
        return { url: src, previewUrl: src };
      }));
    }).catch(() => {});
    return () => {
      document.body.classList.remove('adobe-express-open');
      document.getElementById('adobe-zindex-fix')?.remove();
      objUrlCache.current.forEach(url => URL.revokeObjectURL(url));
    };
  }, []);

  useEffect(() => {
    if (!accountsLoaded) return;
    const hasBsky = accounts.some(a => a.platform === 'bluesky');
    const hasIg = accounts.some(a => a.platform === 'instagram');
    const hasTw = accounts.some(a => a.platform === 'twitter');
    const hasMst = accounts.some(a => a.platform === 'mastodon');
    const hasThreads = accounts.some(a => a.platform === 'threads');
    const hasLinkedin = accounts.some(a => a.platform === 'linkedin');
    const hasYoutube = accounts.some(a => a.platform === 'youtube');
    setPlatforms(prev => prev.filter(p =>
      (p === 'bluesky' && hasBsky) ||
      (p === 'instagram' && hasIg) ||
      (p === 'twitter' && hasTw) ||
      (p === 'mastodon' && hasMst) ||
      (p === 'threads' && hasThreads) ||
      (p === 'linkedin' && hasLinkedin) ||
      (p === 'youtube' && hasYoutube)
    ));
  }, [accountsLoaded]); // eslint-disable-line react-hooks/exhaustive-deps

  const apiUrl = import.meta.env.VITE_API_URL || '';

  const launchAdobeExpress = useCallback(async () => {
    if (!adobeClientId) return;
    setAdobeLoading(true);

    // Hide the PostEditor overlay and sidebar BEFORE any async work so
    // the Adobe auth/editor UI is never blocked by our overlay.
    setAdobeActive(true);
    document.body.classList.add('adobe-express-open');

    // Inject CSS so the Adobe SDK's body-level container (appended outside
    // #root) always stacks above the sidebar and any other positioned element,
    // regardless of which class/id the SDK uses internally.
    if (!document.getElementById('adobe-zindex-fix')) {
      const s = document.createElement('style');
      s.id = 'adobe-zindex-fix';
      s.textContent = 'body > div:not(#root) { z-index: 9999 !important; }';
      document.head.appendChild(s);
    }

    const closeAdobe = () => {
      setAdobeActive(false);
      document.body.classList.remove('adobe-express-open');
      document.getElementById('adobe-zindex-fix')?.remove();
    };

    try {
      // Re-initialize only if the client ID changed or SDK was never loaded.
      if (!_ccEditor || _ccClientId !== adobeClientId) {
        if (!(window as any).CCEverywhere) {
          await import('https://cc-embed.adobe.com/sdk/v4/CCEverywhere.js' as any);
        }
        const { editor } = await (window as any).CCEverywhere.initialize(
          { clientId: adobeClientId, appName: 'SocialPod' },
          { loginMode: 'delayed' },
        );
        _ccEditor = editor;
        _ccClientId = adobeClientId;
      }

      _ccEditor.create(
        {
          canvasSize: (isStory || isReel)
            ? { width: 1080, height: 1920, unit: 'px' }
            : { width: 1080, height: 1080, unit: 'px' },
        },
        {
          callbacks: {
            onPublish: async (...args: any[]) => {
              // Find the object that contains .asset (could be first or second arg).
              const params = args.find((a: any) => a?.asset) || args[args.length - 1];
              const assetData = params?.asset?.[0]?.data;
              if (!assetData) {
                console.warn('Adobe Express onPublish args:', args);
                closeAdobe();
                toast.error('No image data received');
                return;
              }
              try {
                let file: File;
                if (assetData.startsWith('data:')) {
                  // Base64 data URI — convert to File directly (no network request).
                  const res = await fetch(assetData);
                  const blob = await res.blob();
                  file = new File([blob], 'design.png', { type: blob.type || 'image/png' });
                } else {
                  // External URL (S3) — proxy download through backend.
                  const uploaded = await api.uploadFromURL(assetData);
                  if (isStory || isReel) {
                    setImages([{ kind: 'url' as const, url: uploaded.url }]);
                  } else {
                    setImages(prev => [...prev, { kind: 'url' as const, url: uploaded.url }]);
                  }
                  closeAdobe();
                  toast.success('Design added');
                  return;
                }
                if (isStory || isReel) {
                  setImages([{ kind: 'file' as const, file }]);
                } else {
                  setImages(prev => [...prev, { kind: 'file' as const, file }]);
                }
                closeAdobe();
                toast.success('Design added');
              } catch (err: any) {
                console.error('Adobe Express design save failed:', err);
                closeAdobe();
                toast.error(err.message || 'Failed to save design');
              }
            },
            onCancel: closeAdobe,
            onError: (err: any) => {
              console.error('Adobe Express onError:', err);
              closeAdobe();
              toast.error('Adobe Express error: ' + (err?.message || err?.toString() || 'Unknown error'));
            },
          },
        },
        [
          {
            id: 'save-to-post',
            label: 'Add to Post',
            action: { target: 'publish' },
            style: { uiType: 'button' },
          },
        ],
      );
    } catch {
      closeAdobe();
      toast.error('Failed to open Adobe Express');
    } finally {
      setAdobeLoading(false);
    }
  }, [adobeClientId, platforms, isStory]);

  const generateText = useCallback(async (platform?: Platform) => {
    const prompt = platform ? (contentOverrides[platform] || '').trim() : content.trim();
    if (!prompt) { toast.error('Enter a prompt first'); return; }
    setGenerating(platform || true);
    try {
      const { text } = await api.generateText(prompt, platform ? [platform] : platforms);
      if (platform) {
        setContentOverrides(prev => ({ ...prev, [platform]: text }));
      } else {
        setContent(text);
      }
    } catch (err: any) {
      toast.error(err.message || 'Text generation failed');
    } finally {
      setGenerating(false);
    }
  }, [content, contentOverrides, platforms]);

  const handleMentionInsert = useCallback(
    (mention: MentionEntry, queryStart: number, queryLength: number) => {
      if (platforms.length === 1) {
        // Single platform: insert the handle directly into base content
        const handle = mention.handles[platforms[0]] || `@${mention.name}`;
        const normalized = handle.startsWith('@') ? handle : `@${handle}`;
        setContent(prev => prev.slice(0, queryStart) + normalized + ' ' + prev.slice(queryStart + queryLength));
        return;
      }

      // Multiple platforms: enable per-platform mode and expand each with the correct handle
      setCustomizePerPlatform(true);
      const newOverrides: Record<string, string> = {};
      for (const p of platforms) {
        const baseText = contentOverrides[p] ?? content;
        const handle = mention.handles[p] || `@${mention.name}`;
        const normalized = handle.startsWith('@') ? handle : `@${handle}`;
        newOverrides[p] = baseText.slice(0, queryStart) + normalized + ' ' + baseText.slice(queryStart + queryLength);
      }
      setContentOverrides(newOverrides);
      // Keep base content in sync with first platform's handle so char count stays consistent
      const firstHandle = mention.handles[platforms[0]] || `@${mention.name}`;
      const firstNormalized = firstHandle.startsWith('@') ? firstHandle : `@${firstHandle}`;
      setContent(prev => prev.slice(0, queryStart) + firstNormalized + ' ' + prev.slice(queryStart + queryLength));
    },
    [platforms, contentOverrides, content],
  );

  const togglePlatform = (p: Platform) => {
    setPlatforms(prev =>
      prev.includes(p) ? prev.filter(x => x !== p) : [...prev, p]
    );
  };

  const suffixLen = (platform: Platform) => {
    const id = suffixIds[platform];
    if (!id) return 0;
    const s = suffixes.find(x => x.id === id);
    return s ? s.content.length + 1 : 0; // +1 for newline separator
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
  const overLimit = charCount > charLimit || (customizePerPlatform && platforms.some(p => {
    const val = contentOverrides[p];
    return val != null && val.length > effectiveLimit(p);
  }));
  const charClass = charCount > charLimit ? 'danger' : charCount > charLimit * 0.9 ? 'warning' : '';

  const handleImageUpload = (files: FileList | null) => {
    if (!files) return;
    const items = Array.from(files).map(file => ({ kind: 'file' as const, file }));
    if (isStory || isReel) {
      // Stories and Reels only support a single media file — replace any existing.
      setImages(items.slice(0, 1));
    } else {
      setImages(prev => [...prev, ...items]);
    }
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

  // Detect whether an ImageItem is a video (works for both file and URL items).
  const isVideoItem = (item: ImageItem): boolean => {
    if (item.kind === 'file') return item.file.type.startsWith('video/');
    return /\.(mp4|mov)$/i.test(item.url);
  };

  // Returns a displayable URL for any ImageItem without leaking object URLs.
  const previewUrl = useMemo(() => (item: ImageItem) => {
    if (item.kind === 'url') return item.url.startsWith('/') ? apiUrl + item.url : item.url;
    if (!objUrlCache.current.has(item.file)) {
      objUrlCache.current.set(item.file, URL.createObjectURL(item.file));
    }
    return objUrlCache.current.get(item.file)!;
  }, [apiUrl]);

  // Image URLs for the preview panel (server URLs + object URLs for local files).
  const previewImageUrls = useMemo(() =>
    images.map(item => previewUrl(item)),
  [images, previewUrl]);

  const blueskyAccount = accounts.find(a => a.platform === 'bluesky') ?? null;
  const instagramAccount = accounts.find(a => a.platform === 'instagram') ?? null;
  const twitterAccount = accounts.find(a => a.platform === 'twitter') ?? null;
  const mastodonAccount = accounts.find(a => a.platform === 'mastodon') ?? null;
  const threadsAccount = accounts.find(a => a.platform === 'threads') ?? null;
  const linkedinAccount = accounts.find(a => a.platform === 'linkedin') ?? null;
  const youtubeAccount = accounts.find(a => a.platform === 'youtube') ?? null;

  const handleSubmit = async () => {
    if (isStory) {
      if (images.length === 0) { toast.error('A story requires an image or video'); return; }
    } else if (isReel) {
      if (images.length === 0) { toast.error('A reel requires a video'); return; }
    } else if (customizePerPlatform && platforms.length > 1) {
      const missing = platforms.filter(p => !(contentOverrides[p] || '').trim());
      if (missing.length > 0) { toast.error(`Content is required for ${missing.join(', ')}`); return; }
      const over = platforms.find(p => (contentOverrides[p] || '').length > effectiveLimit(p));
      if (over) { toast.error(`Content for ${over} exceeds ${effectiveLimit(over)} character limit`); return; }
    } else {
      if (!content.trim()) { toast.error('Content is required'); return; }
      if (content.length > charLimit) { toast.error(`Content exceeds ${charLimit} character limit`); return; }
    }
    if (platforms.length === 0) { toast.error('Select at least one platform'); return; }

    const imageUrls = images.filter(i => i.kind === 'url').map(i => (i as { kind: 'url'; url: string }).url);
    const imageFiles = images.filter(i => i.kind === 'file').map(i => (i as { kind: 'file'; file: File }).file);

    const postType = isStory ? 'story' : isReel ? 'reel' : 'post';

    const accountIds: Record<string, string> = {};
    if (platforms.includes('instagram') && instagramAccount) accountIds['instagram'] = instagramAccount.id;
    if (platforms.includes('bluesky') && blueskyAccount) accountIds['bluesky'] = blueskyAccount.id;
    if (platforms.includes('twitter') && twitterAccount) accountIds['twitter'] = twitterAccount.id;
    if (platforms.includes('mastodon') && mastodonAccount) accountIds['mastodon'] = mastodonAccount.id;
    if (platforms.includes('threads') && threadsAccount) accountIds['threads'] = threadsAccount.id;
    if (platforms.includes('linkedin') && linkedinAccount) accountIds['linkedin'] = linkedinAccount.id;
    if (platforms.includes('youtube') && youtubeAccount) accountIds['youtube'] = youtubeAccount.id;

    setSaving(true);
    try {
      const postData: any = {
        content: content || '',
        postType,
        firstComment: (isStory || isReel) ? undefined : (firstComment.trim() || undefined),
        platforms,
        scheduledAt: new Date(scheduledAt).toISOString(),
        imageUrls,
        tags,
        status,
        suffixIds: (isStory || isReel) ? {} : suffixIds,
        contentOverrides: (isStory || isReel || !customizePerPlatform) ? {} : contentOverrides,
        accountIds,
      };

      if (addNews && newsEnabled && !isStory && !isReel) {
        postData.episodeNews = {
          episodeNumber: newsEpisodeNumber,
          title: newsTitle,
          additionalText: newsAdditionalText,
          bggLink: bggImportedUrl || '',
        };
      }

      await onSave(postData, imageFiles.length > 0 ? imageFiles : undefined);
    } finally {
      setSaving(false);
    }
  };

  const isDirty = content !== (post?.content || '')
    || firstComment !== (post?.firstComment || '')
    || scheduledAt !== (post ? format(new Date(post.scheduledAt), "yyyy-MM-dd'T'HH:mm") : defaultTime)
    || status !== (post?.status || 'scheduled')
    || JSON.stringify(platforms) !== JSON.stringify(post?.platforms || ['bluesky'])
    || images.length !== (post?.imageUrls || []).length
    || JSON.stringify(suffixIds) !== JSON.stringify(post?.suffixIds || {})
    || JSON.stringify(contentOverrides) !== JSON.stringify(post?.contentOverrides || {});

  const fetchBGGData = async () => {
    if (!bggUrl.trim()) return;
    setFetchingBgg(true);
    setBggError('');
    try {
      const data = await api.fetchBGGGame(bggUrl.trim());
      setBggImportedUrl(bggUrl.trim());
      if (data.imageBase64) {
        const byteStr = atob(data.imageBase64);
        const arr = new Uint8Array(byteStr.length);
        for (let i = 0; i < byteStr.length; i++) arr[i] = byteStr.charCodeAt(i);
        const blob = new Blob([arr], { type: 'image/jpeg' });
        const file = new File([blob], data.imageFilename || 'bgg-cover.jpg', { type: 'image/jpeg' });
        setImages(prev => [{ kind: 'file', file }, ...prev]);
      }
      if (customizePerPlatform && platforms.length > 1) {
        // Route full text to the platform with the highest character limit,
        // then use AI to produce shorter versions for the remaining platforms.
        const sorted = [...platforms].sort((a, b) => effectiveLimit(b) - effectiveLimit(a));
        const primary = sorted[0];
        const rest = sorted.slice(1);
        const overrides: Record<string, string> = { [primary]: data.suggestedContent };
        if (aiEnabled) {
          for (const p of rest) {
            try {
              const { text } = await api.generateText(data.suggestedContent, [p]);
              overrides[p] = text;
            } catch {
              overrides[p] = data.suggestedContent;
            }
          }
          toast.success('BGG data imported and text adapted per platform');
        } else {
          for (const p of rest) {
            overrides[p] = data.suggestedContent;
          }
          toast.success('BGG data imported');
        }
        setContentOverrides(overrides);
        setContent(data.suggestedContent);
      } else {
        setContent(data.suggestedContent);
        toast.success('BGG data imported');
      }
    } catch (e: any) {
      setBggError(e.message || 'Failed to fetch BGG data');
    } finally {
      setFetchingBgg(false);
    }
  };

  const handleClose = () => {
    if (isDirty && !window.confirm('You have unsaved changes. Discard them?')) return;
    onClose();
  };

  return (
    <div className="modal-overlay" style={adobeActive ? { display: 'none' } : undefined}>
      <div className="modal post-editor-modal" onClick={e => e.stopPropagation()}>
        <div className="editor-header">
          <h2>{post ? (isStory ? 'Edit Story' : isReel ? 'Edit Reel' : 'Edit Post') : (isStory ? 'New Story' : isReel ? 'New Reel' : 'New Post')}</h2>
          <button className="btn btn-ghost btn-sm" onClick={handleClose}>
            <X size={18} />
          </button>
        </div>

        <div className="editor-layout">
          <div className="editor-body">
            {!isStory && !isReel && (
              <>
                {/* Platform selector */}
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
                  {youtubeAccount && isReel && (
                    <div
                      className={`platform-option youtube ${platforms.includes('youtube') ? 'selected' : ''}`}
                      onClick={() => togglePlatform('youtube')}
                      title={`Will post to ${youtubeAccount.displayName || youtubeAccount.accountName}`}
                    >
                      <PlatformIcon platform="youtube" size={18} />
                    </div>
                  )}
                </div>

                {/* BGG Import */}
                {bggEnabled && (
                <div className="form-group">
                  <label>BoardGameGeek Import</label>
                  <div style={{ display: 'flex', gap: 8 }}>
                    <input
                      type="url"
                      className="input"
                      value={bggUrl}
                      onChange={e => { setBggUrl(e.target.value); setBggError(''); }}
                      onKeyDown={e => e.key === 'Enter' && fetchBGGData()}
                      placeholder="https://boardgamegeek.com/boardgame/822/carcassonne"
                      style={{ flex: 1 }}
                    />
                    <button
                      className="btn btn-secondary btn-sm"
                      onClick={fetchBGGData}
                      disabled={fetchingBgg || !bggUrl.trim()}
                      style={{ whiteSpace: 'nowrap' }}
                    >
                      {fetchingBgg ? 'Fetching...' : 'Import'}
                    </button>
                  </div>
                  {bggError && (
                    <span style={{ marginTop: 4, fontSize: '0.8rem', color: 'var(--danger)', display: 'block' }}>{bggError}</span>
                  )}
                </div>
                )}

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
                            setCustomizePerPlatform(checked);
                            if (checked) {
                              setContentOverrides(prev => {
                                const merged: Record<string, string> = { ...prev };
                                platforms.forEach(p => {
                                  if (!merged[p]) merged[p] = content;
                                });
                                return merged;
                              });
                            } else {
                              setContentOverrides({});
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
                        const platformLimit = effectiveLimit(platform);
                        const platformCount = overrideVal.length;
                        const platformOver = platformCount > platformLimit;
                        const platformClass = platformOver ? 'danger' : platformCount > platformLimit * 0.9 ? 'warning' : '';
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
                              placeholder={`Text for ${platform}…`}
                              rows={4}
                            />
                            <div style={{ display: 'flex', alignItems: 'center', justifyContent: 'space-between' }}>
                              {aiEnabled && (
                                <button className="btn btn-ghost btn-sm" onClick={() => generateText(platform)} disabled={!!generating || !overrideVal.trim()}>
                                  <Sparkles size={14} /> {generating === platform ? 'Generating...' : 'Generate'}
                                </button>
                              )}
                              <div className={`char-counter ${platformClass}`} style={{ marginLeft: 'auto' }}>{platformCount} / {platformLimit}</div>
                            </div>
                          </div>
                        );
                      })}
                    </>
                  ) : (
                    <>
                      <MentionTextarea
                        textareaRef={textareaRef}
                        className="textarea post-textarea"
                        value={content}
                        onChange={setContent}
                        onMentionInsert={handleMentionInsert}
                        mentions={mentions}
                        placeholder="What's on your mind? Use #hashtags for tags..."
                        rows={5}
                      />
                      <div style={{ display: 'flex', alignItems: 'center', justifyContent: 'space-between' }}>
                        {aiEnabled && (
                          <button className="btn btn-ghost btn-sm" onClick={() => generateText()} disabled={!!generating || !content.trim()}>
                            <Sparkles size={14} /> {generating === true ? 'Generating...' : 'Generate'}
                          </button>
                        )}
                        <div className={`char-counter ${charClass}`} style={{ marginLeft: 'auto' }}>
                          {charCount} / {charLimit}
                        </div>
                      </div>
                    </>
                  )}
                </div>

                {/* Suffix selectors */}
                {suffixes.length > 0 && platforms.length > 0 && (() => {
                  const suffixPlatformLabels: Record<string, string> = {
                    bluesky: 'Bluesky',
                    instagram: 'Instagram',
                    twitter: 'X/Twitter',
                    mastodon: 'Mastodon',
                    threads: 'Threads',
                    linkedin: 'LinkedIn',
                  };
                  return (
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
                  );
                })()}

                {/* First Comment */}
                <div className="form-group">
                  <label style={{ whiteSpace: 'nowrap' }}><MessageSquare size={14} /> First Comment <span className="first-comment-label-hint">(optional)</span></label>
                  <textarea
                    className="textarea first-comment-textarea"
                    value={firstComment}
                    onChange={e => setFirstComment(e.target.value)}
                    placeholder="Add a first comment to your post…"
                    rows={3}
                  />
                </div>

                {/* Episode News */}
                {newsEnabled && (
                  <div className="form-group">
                    <label style={{ display: 'flex', alignItems: 'center', gap: 8, cursor: 'pointer', margin: 0, marginBottom: 8 }}>
                      <input
                        type="checkbox"
                        checked={addNews}
                        onChange={e => setAddNews(e.target.checked)}
                      />
                      <Newspaper size={14} /> Add News
                    </label>
                    {addNews && (
                      <div style={{ display: 'flex', flexDirection: 'column', gap: 10, padding: '12px', background: 'var(--bg-secondary, #1e293b)', borderRadius: 8, border: '1px solid var(--border)' }}>
                        <div className="form-group" style={{ marginBottom: 0 }}>
                          <label style={{ fontSize: 13 }}>Episode Number</label>
                          <input
                            type="text"
                            className="input"
                            value={newsEpisodeNumber}
                            onChange={e => setNewsEpisodeNumber(e.target.value)}
                            placeholder="e.g. 42"
                          />
                        </div>
                        <div className="form-group" style={{ marginBottom: 0 }}>
                          <label style={{ fontSize: 13 }}>Title</label>
                          <input
                            type="text"
                            className="input"
                            value={newsTitle}
                            onChange={e => setNewsTitle(e.target.value)}
                            placeholder="Episode title"
                          />
                        </div>
                        <div className="form-group" style={{ marginBottom: 0 }}>
                          <label style={{ fontSize: 13 }}>Additional Text <span style={{ color: 'var(--text-muted)', fontWeight: 400 }}>(optional)</span></label>
                          <textarea
                            className="textarea"
                            value={newsAdditionalText}
                            onChange={e => setNewsAdditionalText(e.target.value)}
                            placeholder="Any extra notes for this episode news…"
                            rows={2}
                          />
                        </div>
                      </div>
                    )}
                  </div>
                )}
              </>
            )}

            {isStory && (
              <div className="story-hint">
                <Film size={16} />
                <span>Upload an image or video for your Instagram Story (9:16 recommended)</span>
              </div>
            )}

            {isReel && (
              <>
                <div className="story-hint">
                  <Film size={16} />
                  <span>Upload a video for your Instagram Reel (9:16 recommended, MP4)</span>
                </div>
                <div className="form-group">
                  <MentionTextarea
                    textareaRef={textareaRef}
                    className="textarea post-textarea"
                    value={content}
                    onChange={setContent}
                    onMentionInsert={handleMentionInsert}
                    mentions={mentions}
                    platform="instagram"
                    placeholder="Add a caption to your Reel… (optional)"
                    rows={4}
                  />
                  <div style={{ display: 'flex', alignItems: 'center', justifyContent: 'space-between' }}>
                    {aiEnabled && (
                      <button className="btn btn-ghost btn-sm" onClick={() => generateText()} disabled={!!generating || !content.trim()}>
                        <Sparkles size={14} /> {generating === true ? 'Generating...' : 'Generate'}
                      </button>
                    )}
                    <div className={`char-counter ${charClass}`} style={{ marginLeft: 'auto' }}>
                      {charCount} / {INSTAGRAM_LIMIT}
                    </div>
                  </div>
                </div>
              </>
            )}

            {/* Images */}
            <div className="editor-images">
              {images.length > 0 && (
                <div className="image-preview-grid">
                  {images.map((item, i) => {
                    const isVid = isVideoItem(item);
                    return (
                      <div key={i} className="image-preview">
                        {isVid
                          ? <video src={previewUrl(item)} muted className="image-preview-video" />
                          : <img src={previewUrl(item)} alt="" />}
                        {!isVid && (
                          <button className="edit-btn" onClick={() => setEditingImageIdx(i)} title="Edit image">
                            <Pencil size={12} />
                          </button>
                        )}
                        <button className="remove-btn" onClick={() => removeImage(i)}>x</button>
                      </div>
                    );
                  })}
                </div>
              )}
              <input
                ref={fileRef}
                type="file"
                accept={isReel ? 'video/mp4,video/quicktime' : isStory ? 'image/*,video/mp4,video/quicktime' : 'image/*'}
                multiple={!isStory && !isReel}
                style={{ display: 'none' }}
                onChange={e => handleImageUpload(e.target.files)}
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
              <select className="select" value={status} onChange={e => setStatus(e.target.value as any)}>
                <option value="scheduled">Scheduled</option>
                <option value="draft">Draft</option>
              </select>
            </div>
          </div>

          {/* Live preview */}
          <div className="editor-preview">
            <div className="editor-preview-label">Preview</div>
            {isReel ? (
              <ReelPreview
                videoUrl={previewImageUrls[0] || ''}
                apiUrl={apiUrl}
                instagramAccount={instagramAccount}
                content={content}
              />
            ) : isStory ? (
              <StoryPreview
                imageUrl={previewImageUrls[0] || ''}
                apiUrl={apiUrl}
                instagramAccount={instagramAccount}
                isVideo={images[0] ? isVideoItem(images[0]) : false}
              />
            ) : (
              <PostPreview
                content={content}
                contentOverrides={customizePerPlatform ? contentOverrides : undefined}
                platforms={platforms}
                imageUrls={previewImageUrls}
                scheduledAt={scheduledAt}
                apiUrl={apiUrl}
                blueskyAccount={blueskyAccount}
                instagramAccount={instagramAccount}
                twitterAccount={twitterAccount}
                mastodonAccount={mastodonAccount}
                linkedinAccount={linkedinAccount}
                bluskySuffix={suffixes.find(s => s.id === suffixIds['bluesky'])?.content}
                instagramSuffix={suffixes.find(s => s.id === suffixIds['instagram'])?.content}
                twitterSuffix={suffixes.find(s => s.id === suffixIds['twitter'])?.content}
                mastodonSuffix={suffixes.find(s => s.id === suffixIds['mastodon'])?.content}
                linkedinSuffix={suffixes.find(s => s.id === suffixIds['linkedin'])?.content}
              />
            )}
          </div>
        </div>

        <div className="editor-footer">
          <div className="footer-left">
            <button className="btn btn-ghost btn-sm" onClick={() => fileRef.current?.click()}>
              {(isStory || isReel) ? <><Film size={16} /> Upload {isReel ? 'Video' : 'Media'}</> : <><Image size={16} /> Upload</>}
            </button>
            {adobeClientId && (
              <button className="btn btn-ghost btn-sm" onClick={launchAdobeExpress} disabled={adobeLoading}>
                <Wand2 size={16} /> {adobeLoading ? 'Opening...' : (isStory ? 'Create Story Design' : isReel ? 'Create Reel Design' : 'Create Design')}
              </button>
            )}
            {onDelete && (
              <button className="btn btn-danger btn-sm" onClick={() => { if (window.confirm('Are you sure you want to delete this post?')) onDelete(); }}>
                <Trash2 size={16} /> Delete
              </button>
            )}
          </div>
          <div className="footer-right">
            <button className="btn btn-secondary" onClick={handleClose}>Cancel</button>
            <button className="btn btn-primary" onClick={handleSubmit} disabled={saving || overLimit || (addNews && newsEnabled && (!newsEpisodeNumber.trim() || !newsTitle.trim()))}>
              <Send size={16} /> {saving ? 'Saving...' : post ? 'Update' : 'Schedule'}
            </button>
          </div>
        </div>
      </div>

      {editingImageIdx !== null && (
        <div className="image-editor-overlay">
          <FilerobotImageEditor
            source={previewUrl(images[editingImageIdx])}
            tabsIds={[TABS.ADJUST, TABS.ANNOTATE, TABS.WATERMARK, TABS.FILTERS, TABS.FINETUNE, TABS.RESIZE]}
            defaultTabId={TABS.ANNOTATE}
            savingPixelRatio={2}
            previewPixelRatio={2}
            defaultSavedImageType="png"
            theme={{
              palette: {
                'bg-secondary': '#1e293b',
                'bg-primary': '#0f172a',
                'bg-primary-active': '#334155',
                'bg-primary-hover': '#263348',
                'bg-primary-light': '#1e293b',
                'bg-primary-stateless': '#334155',
                'bg-primary-0-5-opacity': 'rgba(30, 41, 59, 0.5)',
                'bg-stateless': '#1e293b',
                'bg-hover': '#263348',
                'bg-active': '#334155',
                'bg-grey': '#334155',
                'bg-tooltip': '#475569',
                'txt-primary': '#f1f5f9',
                'txt-secondary': '#94a3b8',
                'txt-secondary-invert': '#f1f5f9',
                'txt-placeholder': '#64748b',
                'accent-primary': '#818cf8',
                'accent-primary-hover': '#a5b4fc',
                'accent-primary-active': '#6366f1',
                'accent-stateless': '#818cf8',
                'accent-stateless_0_4_opacity': 'rgba(129, 140, 248, 0.4)',
                'accent_0_5_opacity': 'rgba(129, 140, 248, 0.05)',
                'accent_1_2_opacity': 'rgba(129, 140, 248, 0.12)',
                'accent_1_8_opacity': 'rgba(129, 140, 248, 0.18)',
                'accent_2_8_opacity': 'rgba(129, 140, 248, 0.28)',
                'accent_4_0_opacity': 'rgba(129, 140, 248, 0.4)',
                'accent-primary-disabled': '#334155',
                'accent-secondary-disabled': '#1e293b',
                'icon-primary': '#cbd5e1',
                'icons-primary-opacity-0-6': 'rgba(203, 213, 225, 0.6)',
                'icons-secondary': '#94a3b8',
                'icons-secondary-hover': '#cbd5e1',
                'icons-primary-hover': '#f1f5f9',
                'icons-invert': '#f1f5f9',
                'icons-placeholder': '#475569',
                'icons-muted': '#64748b',
                'borders-primary': '#334155',
                'borders-primary-hover': '#64748b',
                'borders-secondary': '#1e293b',
                'borders-strong': '#475569',
                'borders-invert': '#1e293b',
                'borders-item': '#334155',
                'borders-button': '#64748b',
                'borders-disabled': 'rgba(99, 102, 241, 0.3)',
                'border-primary-stateless': '#334155',
                'border-hover-bottom': 'rgba(129, 140, 248, 0.18)',
                'border-active-bottom': '#818cf8',
                'btn-primary-text': '#ffffff',
                'btn-primary-text-0-6': 'rgba(255, 255, 255, 0.6)',
                'btn-primary-text-0-4': 'rgba(255, 255, 255, 0.4)',
                'btn-secondary-text': '#f1f5f9',
                'btn-disabled-text': '#64748b',
                'link-primary': '#94a3b8',
                'link-hover': '#f1f5f9',
                'link-active': '#f1f5f9',
                'link-stateless': '#94a3b8',
                'link-pressed': '#818cf8',
                'link-muted': '#64748b',
                'active-secondary': '#1e293b',
                'active-secondary-hover': 'rgba(99, 102, 241, 0.15)',
                'error': '#ef4444',
                'error-hover': '#dc2626',
                'error-active': '#b91c1c',
                'success': '#22c55e',
                'success-hover': '#16a34a',
                'warning': '#f59e0b',
                'warning-hover': '#d97706',
                'info': '#3b82f6',
                'tag': '#94a3b8',
                'bg-base-light': '#1e293b',
                'bg-base-medium': '#334155',
                'borders-base-light': '#334155',
                'borders-base-medium': '#475569',
                'extra-0-3-overlay': 'rgba(15, 23, 42, 0.3)',
                'extra-0-5-overlay': 'rgba(15, 23, 42, 0.5)',
                'extra-0-7-overlay': 'rgba(15, 23, 42, 0.7)',
                'extra-0-9-overlay': 'rgba(15, 23, 42, 0.9)',
                'white-0-7-8-overlay': 'rgba(15, 23, 42, 0.78)',
                'gradient-right': 'linear-gradient(270deg, #0f172a 1.56%, rgba(15,23,42,0.89) 52.4%, rgba(15,23,42,0.53) 76.04%, rgba(15,23,42,0) 100%)',
                'gradient-right-active': 'linear-gradient(270deg, #1e293b 1.56%, #1e293b 52.4%, rgba(30,41,59,0.53) 76.04%, rgba(30,41,59,0) 100%)',
                'gradient-right-hover': 'linear-gradient(270deg, #263348 1.56%, #263348 52.4%, rgba(38,51,72,0.53) 76.04%, rgba(38,51,72,0) 100%)',
              },
              typography: {
                fontFamily: 'Inter, -apple-system, BlinkMacSystemFont, sans-serif',
              },
            }}
            Text={{
              fonts: [
                'Arial',
                'Helvetica',
                'Times New Roman',
                'Georgia',
                'Verdana',
                'Courier New',
                'Trebuchet MS',
                'Impact',
                'Comic Sans MS',
                { label: 'Rockwell', value: 'Rockwell, "Rockwell Nova", "Roboto Slab", "DejaVu Serif", "Sitka Small", serif' },
              ],
            }}
            Watermark={{
              gallery: watermarkGallery,
            }}
            Crop={{
              presetsItems: [
                { titleKey: 'Square (1:1)', ratio: 1 },
                { titleKey: 'Story (9:16)', ratio: 9 / 16 },
                { titleKey: 'Landscape (16:9)', ratio: 16 / 9 },
                { titleKey: 'Portrait (4:5)', ratio: 4 / 5 },
              ],
            }}
            onBeforeSave={() => false}
            onSave={(savedImageData: any) => {
              const canvas = savedImageData.imageCanvas as HTMLCanvasElement | undefined;
              const base64 = savedImageData.imageBase64 as string | undefined;
              const src = base64 || canvas?.toDataURL('image/png');
              if (!src) {
                toast.error('Failed to get edited image');
                setEditingImageIdx(null);
                return;
              }
              fetch(src)
                .then(res => res.blob())
                .then(blob => {
                  const ext = savedImageData.extension || 'png';
                  const file = new File([blob], `edited.${ext}`, { type: savedImageData.mimeType || 'image/png' });
                  const idx = editingImageIdx!;
                  const oldItem = images[idx];
                  if (oldItem.kind === 'file') {
                    const cached = objUrlCache.current.get(oldItem.file);
                    if (cached) { URL.revokeObjectURL(cached); objUrlCache.current.delete(oldItem.file); }
                  }
                  setImages(prev => prev.map((item, i) =>
                    i === idx ? { kind: 'file' as const, file } : item
                  ));
                  setEditingImageIdx(null);
                  toast.success('Image updated');
                })
                .catch(() => {
                  toast.error('Failed to process edited image');
                  setEditingImageIdx(null);
                });
            }}
            onClose={() => setEditingImageIdx(null)}
          />
        </div>
      )}
    </div>
  );
}
