import { useEffect, useState, type CSSProperties } from 'react';
import { CheckCircle2, AlertTriangle, Circle, X, Users, AtSign, Wand2, Loader, RefreshCw, Radar, ExternalLink, Megaphone } from 'lucide-react';
import { format, parseISO } from 'date-fns';
import toast from 'react-hot-toast';
import { api } from '../../services/api';
import { Modal } from '../Common/Modal';
import { checkCaption, IG_USERNAME_RE, normalizeHandle, percent } from '../../services/instagramGrowth';
import type { NewsDraft, Post, RadarContent } from '../../types';

/** A list of Instagram usernames edited as chips. */
export function HandleChips({ values, onChange, max, placeholder, onRemove }: {
  values: string[];
  onChange: (next: string[]) => void;
  max: number;
  placeholder: string;
  /** Called instead of onChange's removal when the parent tracks removals itself. */
  onRemove?: (handle: string) => void;
}) {
  const [draft, setDraft] = useState('');
  const add = () => {
    const names = draft.split(/[\s,]+/).map(normalizeHandle).filter(Boolean);
    if (names.length === 0) return;
    const next = [...values];
    for (const n of names) {
      if (!IG_USERNAME_RE.test(n)) { toast.error(`"${n}" is not a valid Instagram username`); continue; }
      if (next.some(v => v.toLowerCase() === n.toLowerCase())) continue;
      if (next.length >= max) { toast.error(`At most ${max}`); break; }
      next.push(n);
    }
    onChange(next);
    setDraft('');
  };
  return (
    <div className="handle-chips">
      {values.map(v => (
        <span key={v} className="handle-chip">
          @{v}
          <button type="button" onClick={() => onRemove ? onRemove(v) : onChange(values.filter(x => x !== v))} title={`Remove @${v}`}>
            <X size={11} />
          </button>
        </span>
      ))}
      {values.length < max && (
        <input
          className="handle-chip-input"
          value={draft}
          onChange={e => setDraft(e.target.value)}
          onKeyDown={e => {
            if (e.key === 'Enter' || e.key === ',' || e.key === ' ') { e.preventDefault(); add(); }
            if (e.key === 'Backspace' && !draft && values.length > 0) {
              const last = values[values.length - 1];
              if (onRemove) onRemove(last); else onChange(values.slice(0, -1));
            }
          }}
          onBlur={add}
          placeholder={values.length === 0 ? placeholder : ''}
        />
      )}
    </div>
  );
}

/** Collaborators and image tags for an Instagram post or reel. */
export function InstagramPeople({ collaborators, onCollaborators, tags, onTags, onRemoveTag }: {
  collaborators: string[];
  onCollaborators: (v: string[]) => void;
  tags: string[];
  onTags: (v: string[]) => void;
  onRemoveTag: (handle: string) => void;
}) {
  return (
    <div className="ig-people">
      <div className="form-group">
        <label title="Invited accounts get a collab request; once accepted the post shows on their profile too.">
          <Users size={14} /> Instagram collaborators <span className="first-comment-label-hint">(max 3)</span>
        </label>
        <HandleChips values={collaborators} onChange={onCollaborators} max={3} placeholder="Add a username, e.g. pegasusspiele" />
      </div>
      <div className="form-group">
        <label title="Tagged accounts are notified. Accounts @mentioned in the Instagram text are added automatically.">
          <AtSign size={14} /> Image tags
        </label>
        <HandleChips values={tags} onChange={onTags} onRemove={onRemoveTag} max={20} placeholder="Mention a publisher with @ in the text, or add one here" />
      </div>
    </div>
  );
}

/** Rewrites the Instagram text with the team's "Rewrite for Instagram" prompt. */
export function RewriteInstagramButton({ rewriting, disabled, onRewrite, style }: {
  rewriting: boolean;
  disabled?: boolean;
  onRewrite: () => void;
  style?: CSSProperties;
}) {
  return (
    <button type="button" className="btn btn-ghost btn-sm" style={style} onClick={onRewrite} disabled={rewriting || disabled} title="Rewrites the Instagram text in the team's tone and caption rules">
      {rewriting ? <><Loader size={14} className="post-editor-spin" /> Rewriting…</> : <><Wand2 size={14} /> Rewrite for Instagram</>}
    </button>
  );
}

/** Live caption checks plus the "Rewrite for Instagram" button. */
export function CaptionChecker({ text, blueskyText, taggedCount, aiEnabled, rewriting, onRewrite }: {
  text: string;
  blueskyText?: string;
  taggedCount: number;
  aiEnabled: boolean;
  rewriting: boolean;
  onRewrite: () => void;
}) {
  const checks = checkCaption(text, { blueskyText, taggedCount });
  const passed = checks.filter(c => c.level === 'ok').length;
  return (
    <div className="caption-checker">
      <div className="caption-checker-header">
        <span>Instagram caption check · {passed}/{checks.length}</span>
        {aiEnabled && <RewriteInstagramButton rewriting={rewriting} disabled={!text.trim()} onRewrite={onRewrite} />}
      </div>
      <ul>
        {checks.map(c => (
          <li key={c.id} className={`caption-check ${c.level}`} title={c.hint}>
            {c.level === 'ok' ? <CheckCircle2 size={13} /> : c.level === 'warn' ? <AlertTriangle size={13} /> : <Circle size={13} />}
            <span>{c.label}</span>
            {c.level !== 'ok' && <span className="caption-check-hint">{c.hint}</span>}
          </li>
        ))}
      </ul>
    </div>
  );
}

/** The Instagram numbers of a published post, with a refresh. */
export function InsightsStrip({ post, onUpdated }: { post: Post; onUpdated: (p: Post) => void }) {
  const result = post.results?.find(r => r.platform === 'instagram' && r.success);
  const [refreshing, setRefreshing] = useState(false);
  if (!result) return null;
  const ins = result.insights;
  const refresh = async () => {
    setRefreshing(true);
    try {
      onUpdated(await api.refreshPostInsights(post.id));
      toast.success('Insights updated');
    } catch (err) {
      toast.error(err instanceof Error ? err.message : 'Failed to read Insights');
    } finally {
      setRefreshing(false);
    }
  };
  return (
    <div className="insights-strip">
      <div className="insights-strip-header">
        <span>Instagram Insights{ins?.fetchedAt ? ` · ${format(parseISO(ins.fetchedAt), 'MMM d, HH:mm')}` : ''}</span>
        <span style={{ display: 'flex', gap: 4 }}>
          {ins?.permalink && (
            <a className="btn btn-ghost btn-sm" href={ins.permalink} target="_blank" rel="noreferrer"><ExternalLink size={13} /></a>
          )}
          <button type="button" className="btn btn-ghost btn-sm" onClick={refresh} disabled={refreshing} title="Read the numbers from Instagram now">
            <RefreshCw size={13} className={refreshing ? 'post-editor-spin' : ''} />
          </button>
        </span>
      </div>
      {result.warning && <div className="insights-warning"><AlertTriangle size={13} /> {result.warning}</div>}
      {ins && (ins.reach > 0 || !ins.error) ? (
        <div className="insights-grid">
          <div><strong>{ins.reach}</strong><span>reach</span></div>
          <div><strong>{ins.views}</strong><span>views</span></div>
          <div><strong>{ins.shares}</strong><span>sends · {percent(ins.reach ? ins.shares / ins.reach : 0)}</span></div>
          <div><strong>{ins.saved}</strong><span>saves · {percent(ins.reach ? ins.saved / ins.reach : 0)}</span></div>
          <div><strong>{ins.likes}</strong><span>likes</span></div>
          <div><strong>{ins.comments}</strong><span>comments</span></div>
          <div><strong>{ins.follows}</strong><span>follows</span></div>
          <div><strong>{ins.profileVisits}</strong><span>profile visits</span></div>
        </div>
      ) : (
        <p className="insights-empty">
          {ins?.error ? `Instagram did not return Insights: ${ins.error}` : 'No numbers yet — they are read hourly after publishing.'}
        </p>
      )}
    </div>
  );
}

const releaseLabel: Record<string, string> = { ja: 'Ja', nein: 'Nein', offen: 'Offen' };

/** Picks news from the Radar queue and builds the carousel. */
export function RadarPicker({ onClose, onApply }: { onClose: () => void; onApply: (content: RadarContent) => void }) {
  const apiUrl = import.meta.env.VITE_API_URL || '';
  const [queue, setQueue] = useState<NewsDraft[] | null>(null);
  const [picked, setPicked] = useState<string[]>([]);
  const [building, setBuilding] = useState(false);

  useEffect(() => {
    api.getRadarQueue()
      .then(q => { setQueue(q); setPicked(q.slice(0, 5).map(d => d.id)); })
      .catch(err => { toast.error(err.message || 'Failed to load the Radar queue'); setQueue([]); });
  }, []);

  const toggle = (id: string) => setPicked(prev => {
    if (prev.includes(id)) return prev.filter(x => x !== id);
    if (prev.length >= 8) { toast.error('A Radar holds at most 8 news items'); return prev; }
    return [...prev, id];
  });

  const build = async () => {
    setBuilding(true);
    try {
      onApply(await api.generateRadar(picked));
    } catch (err) {
      toast.error(err instanceof Error ? err.message : 'Failed to build the Radar');
    } finally {
      setBuilding(false);
    }
  };

  return (
    <Modal onClose={onClose} dirty={false} style={{ maxWidth: 640 }}>
      <div style={{ display: 'flex', justifyContent: 'space-between', alignItems: 'center', marginBottom: 12 }}>
        <h2 style={{ display: 'flex', alignItems: 'center', gap: 8 }}><Radar size={18} /> Radar Content</h2>
        <button className="btn btn-ghost btn-sm" onClick={onClose}><X size={18} /></button>
      </div>
      <p style={{ color: 'var(--text-secondary)', fontSize: 13, marginBottom: 12 }}>
        Pick the news for this week's carousel, in slide order. The slides, caption and tags replace this post's text and images.
        The news leave the Radar queue once the post is saved.
      </p>
      {queue === null ? (
        <div className="panel-loading"><div className="spinner" /></div>
      ) : queue.length === 0 ? (
        <div className="empty-state" style={{ padding: 24 }}>
          <Radar size={36} />
          <p>The Radar queue is empty.</p>
          <p style={{ color: 'var(--text-muted)', fontSize: 13 }}>Tick “Radar Post?” on a news item to queue it.</p>
        </div>
      ) : (
        <div className="radar-queue">
          {queue.map(d => {
            const order = picked.indexOf(d.id);
            const img = d.imageUrls?.[0];
            return (
              <label key={d.id} className={`radar-queue-item${order >= 0 ? ' picked' : ''}`}>
                <input type="checkbox" checked={order >= 0} onChange={() => toggle(d.id)} />
                <span className="radar-order">{order >= 0 ? order + 1 : ''}</span>
                {img ? <img src={img.startsWith('/') ? apiUrl + img : img} alt="" /> : <span className="radar-noimg" />}
                <span className="radar-item-text">
                  <strong>{d.newsTagline || d.content?.split('\n')[0] || '(No tagline)'}</strong>
                  <span>
                    {d.germanRelease && <span className={`radar-badge ${d.germanRelease}`}>{releaseLabel[d.germanRelease]}</span>}
                    {format(parseISO(d.createdAt), 'MMM d')}
                  </span>
                </span>
              </label>
            );
          })}
        </div>
      )}
      <div style={{ display: 'flex', justifyContent: 'flex-end', gap: 8, marginTop: 16 }}>
        <button className="btn btn-secondary" onClick={onClose}>Cancel</button>
        <button className="btn btn-primary" onClick={build} disabled={building || picked.length === 0}>
          {building ? <><Loader size={16} className="post-editor-spin" /> Building…</> : <><Megaphone size={16} /> Build carousel ({picked.length + 2} slides)</>}
        </button>
      </div>
    </Modal>
  );
}
