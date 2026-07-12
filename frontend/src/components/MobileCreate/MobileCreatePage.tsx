import { useState, useEffect } from 'react';
import { useNavigate } from 'react-router-dom';
import { useAuth } from '../../contexts/AuthContext';
import { usePageManifest } from '../../hooks/usePageManifest';
import { api } from '../../services/api';
import type { PostType } from '../../types';
import toast from 'react-hot-toast';
import {
  X, ArrowLeft, Image as ImageIcon, Clapperboard, Circle, Zap,
} from 'lucide-react';
import { PostEditor } from '../PostEditor/PostEditor';
import './MobileCreate.css';

const TYPE_META: Record<PostType, { label: string; blurb: string; icon: typeof ImageIcon }> = {
  post: { label: 'Post', blurb: 'Photo or carousel to the feed', icon: ImageIcon },
  story: { label: 'Story', blurb: '24-hour vertical story (Instagram)', icon: Circle },
  reel: { label: 'Reel', blurb: 'Short vertical video (Instagram)', icon: Clapperboard },
};

/**
 * Standalone, phone-optimised page for quickly composing a new post.
 * Rendered outside the app Layout (no sidebar), mirroring the convention
 * mobile view — the URL is meant to be bookmarked / added to the home
 * screen so a post of a chosen type can be created on the go.
 *
 * Step 1 lets the user pick what kind of post to create; step 2 reuses the
 * full-featured PostEditor from the main app (which renders as a full-screen
 * modal on mobile, so no sidebar is involved).
 */
export function MobileCreatePage() {
  const { user, loading: authLoading } = useAuth();
  const navigate = useNavigate();

  usePageManifest('/m/create');

  // Step 1 = choose type, Step 2 = compose. `postType` null while choosing.
  const [postType, setPostType] = useState<PostType | null>(null);

  // Redirect to login if not authenticated, preserving the destination.
  useEffect(() => {
    if (!authLoading && !user) {
      navigate('/login?next=' + encodeURIComponent('/m/create'));
    }
  }, [authLoading, user, navigate]);

  const backToTypes = () => setPostType(null);

  const handleSave = async (data: any, files?: File[]) => {
    try {
      await api.createPost(data, files);
      toast.success('Post scheduled!');
      // Back to the type chooser for the next quick post.
      setPostType(null);
    } catch (err: any) {
      toast.error(err.message || 'Failed to schedule post');
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

  // ── Step 2: compose the chosen type with the shared PostEditor ──
  if (postType) {
    return (
      <PostEditor
        postType={postType}
        onSave={handleSave}
        onClose={backToTypes}
      />
    );
  }

  // ── Step 1: choose the post type ───────────────────────────────
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
              <button key={type} className="mc-type-card" onClick={() => setPostType(type)}>
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
