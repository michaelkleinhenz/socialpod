import { useEffect, useRef, useState } from 'react';
import { addWeeks, format } from 'date-fns';
import { Target, CalendarRange, Wand2, Radar, Plus, Trash2, Save, Upload, X, Bot } from 'lucide-react';
import toast from 'react-hot-toast';
import { api } from '../../services/api';
import type { GrowthSettings, InstagramGoal, SlotFormat, WeeklyPlanSlot } from '../../types';
import { FORMAT_LABELS } from '../../services/instagramGrowth';
import './Growth.css';

const WEEKDAYS = ['Monday', 'Tuesday', 'Wednesday', 'Thursday', 'Friday', 'Saturday', 'Sunday'];
const SLOT_FORMATS: SlotFormat[] = ['reel', 'carousel', 'image', 'story', 'any'];

// The growth plan's week: six feed posts, three of them reels.
const EXAMPLE_PLAN: Omit<WeeklyPlanSlot, 'id'>[] = [
  { weekday: 0, format: 'carousel', label: 'Radar der Woche' },
  { weekday: 1, format: 'reel', label: 'Meinung / News in 45 Sek.' },
  { weekday: 2, time: '06:00', format: 'reel', label: 'Episode: Review-Fazit' },
  { weekday: 3, format: 'any', label: 'Einzelbild oder Trial Reel' },
  { weekday: 4, format: 'reel', label: 'Hosts / Community / Tatort Spieltisch' },
  { weekday: 5, time: '06:00', format: 'carousel', label: 'Episode: Themen der News-Folge' },
];

const DEFAULT_REWRITE_PROMPT = `You rewrite social media posts into Instagram captions for a board game podcast.
Tone: knowledgeable, dry humour, a clear opinion. Never bland filler like "we are excited" — say what you actually think.
Rules:
1. The first line is a hook that stops the scroll (under 80 characters).
2. State one clear opinion.
3. Add a share prompt, e.g. "Send this to the person in your group who always explains the rules."
4. End the text with a question to the community.
5. Keep every @mention of the original.
6. Finish with 3 to 5 specific hashtags on their own line — never more.
7. Do not copy the original wording one to one; this is not a cross-post.
Reply with ONLY the caption, no quotes, no commentary.`;

function newSlotId() {
  return Math.random().toString(36).slice(2, 10);
}

function SlideImagePicker({ label, url, onChange }: { label: string; url?: string; onChange: (url: string) => void }) {
  const apiUrl = import.meta.env.VITE_API_URL || '';
  const fileRef = useRef<HTMLInputElement>(null);
  const [uploading, setUploading] = useState(false);
  const upload = async (file?: File) => {
    if (!file) return;
    setUploading(true);
    try {
      const res = await api.uploadImage(file);
      onChange(res.url);
    } catch (err) {
      toast.error(err instanceof Error ? err.message : 'Upload failed');
    } finally {
      setUploading(false);
    }
  };
  return (
    <div className="form-group">
      <label>{label}</label>
      <div className="slide-picker">
        <div className="slide-preview">
          {url ? <img src={url.startsWith('/') ? apiUrl + url : url} alt="" /> : 'drawn automatically'}
        </div>
        <div style={{ display: 'flex', flexDirection: 'column', gap: 6 }}>
          <button type="button" className="btn btn-secondary btn-sm" onClick={() => fileRef.current?.click()} disabled={uploading}>
            <Upload size={14} /> {uploading ? 'Uploading…' : url ? 'Replace' : 'Upload'}
          </button>
          {url && (
            <button type="button" className="btn btn-ghost btn-sm" onClick={() => onChange('')}><X size={14} /> Remove</button>
          )}
          <span style={{ fontSize: 11, color: 'var(--text-muted)' }}>1080 × 1350 (4:5) fits exactly;<br />other sizes are cropped to fill.</span>
        </div>
        <input ref={fileRef} type="file" accept="image/jpeg,image/png,image/webp" style={{ display: 'none' }}
          onChange={e => { upload(e.target.files?.[0]); e.target.value = ''; }} />
      </div>
    </div>
  );
}

// Extensions plus every MIME type TrueType/OpenType files are reported under,
// so pickers that filter by type (iOS among them) don't grey the fonts out.
const FONT_ACCEPT = '.ttf,.otf,font/ttf,font/otf,font/sfnt,application/x-font-ttf,application/x-font-truetype,application/x-font-otf,application/x-font-opentype,application/font-sfnt,application/vnd.ms-opentype';

// RadarFontPicker uploads the TrueType/OpenType font the Radar slides are
// drawn with and previews it with the Radar title.
function RadarFontPicker({ url, name, sample, onChange }: {
  url?: string; name?: string; sample: string; onChange: (font: { url: string; name: string } | null) => void;
}) {
  const apiUrl = import.meta.env.VITE_API_URL || '';
  const fileRef = useRef<HTMLInputElement>(null);
  const [uploading, setUploading] = useState(false);
  const [family, setFamily] = useState<string | null>(null);

  useEffect(() => {
    setFamily(null);
    if (!url) return;
    let cancelled = false;
    const fam = `radar-font-${url.replace(/[^a-z0-9]/gi, '')}`;
    const face = new FontFace(fam, `url(${url.startsWith('/') ? apiUrl + url : url})`);
    face.load().then(loaded => {
      if (cancelled) return;
      document.fonts.add(loaded);
      setFamily(fam);
    }).catch(() => { /* the preview falls back to the page font */ });
    return () => { cancelled = true; };
  }, [url, apiUrl]);

  const upload = async (file?: File) => {
    if (!file) return;
    setUploading(true);
    try {
      onChange(await api.uploadRadarFont(file));
    } catch (err) {
      toast.error(err instanceof Error ? err.message : 'Upload failed');
    } finally {
      setUploading(false);
    }
  };
  return (
    <div className="form-group">
      <label>Font</label>
      <div className="slide-picker">
        <div className="font-preview" style={family ? { fontFamily: `"${family}"` } : { color: 'var(--text-muted)', fontSize: 13, fontWeight: 400 }}>
          {url ? sample : 'built-in font (Go)'}
        </div>
        <div style={{ display: 'flex', flexDirection: 'column', gap: 6 }}>
          <button type="button" className="btn btn-secondary btn-sm" onClick={() => fileRef.current?.click()} disabled={uploading}>
            <Upload size={14} /> {uploading ? 'Uploading…' : url ? 'Replace' : 'Upload'}
          </button>
          {url && (
            <button type="button" className="btn btn-ghost btn-sm" onClick={() => onChange(null)}><X size={14} /> Remove</button>
          )}
        </div>
        <input ref={fileRef} type="file" accept={FONT_ACCEPT} style={{ display: 'none' }}
          onChange={e => { upload(e.target.files?.[0]); e.target.value = ''; }} />
      </div>
      <span style={{ fontSize: 11, color: 'var(--text-muted)' }}>
        {name ? `${name}: ` : ''}a .ttf or .otf file, used for all slide text. Pick the bold cut for a punchy title.
      </span>
    </div>
  );
}

export function GrowthSettingsPage() {
  const [settings, setSettings] = useState<GrowthSettings | null>(null);
  const [goalEnabled, setGoalEnabled] = useState(false);
  const [saving, setSaving] = useState(false);

  useEffect(() => {
    api.getGrowthSettings().then(s => {
      setSettings(s);
      setGoalEnabled(!!s.instagramGoal);
    }).catch(err => toast.error(err.message || 'Failed to load settings'));
  }, []);

  if (!settings) {
    return <div className="page"><div className="loading-screen"><div className="spinner" /></div></div>;
  }

  const today = format(new Date(), 'yyyy-MM-dd');
  const goal: InstagramGoal = settings.instagramGoal ?? {
    target: 1000,
    startDate: today,
    endDate: format(addWeeks(new Date(), 12), 'yyyy-MM-dd'),
    halfwayThreshold: 0.4,
  };
  const radar = settings.radar ?? {};

  const setGoal = (patch: Partial<InstagramGoal>) => setSettings(s => s && ({ ...s, instagramGoal: { ...goal, ...patch } }));
  const setRadar = (patch: Partial<typeof radar>) => setSettings(s => s && ({ ...s, radar: { ...radar, ...patch } }));
  const setSlot = (id: string, patch: Partial<WeeklyPlanSlot>) =>
    setSettings(s => s && ({ ...s, weeklyPlan: s.weeklyPlan.map(sl => sl.id === id ? { ...sl, ...patch } : sl) }));
  const removeSlot = (id: string) => setSettings(s => s && ({ ...s, weeklyPlan: s.weeklyPlan.filter(sl => sl.id !== id) }));
  const addSlot = () => setSettings(s => s && ({ ...s, weeklyPlan: [...s.weeklyPlan, { id: newSlotId(), weekday: 0, format: 'reel' }] }));

  const save = async () => {
    setSaving(true);
    try {
      const payload: GrowthSettings = {
        ...settings,
        instagramGoal: goalEnabled ? { ...goal, halfwayDate: goal.halfwayDate || undefined } : undefined,
        weeklyPlan: [...settings.weeklyPlan].sort((a, b) => a.weekday - b.weekday || (a.time || '').localeCompare(b.time || '')),
      };
      setSettings(await api.updateGrowthSettings(payload));
      toast.success('Growth settings saved');
    } catch (err) {
      toast.error(err instanceof Error ? err.message : 'Failed to save');
    } finally {
      setSaving(false);
    }
  };

  return (
    <div className="page growth-settings">
      <div className="page-header">
        <h1>Growth Settings</h1>
        <button className="btn btn-primary" onClick={save} disabled={saving}><Save size={16} /> {saving ? 'Saving…' : 'Save'}</button>
      </div>

      <div className="card">
        <h3><Target size={16} /> Follower goal</h3>
        <p className="hint">Tracked on the Growth page from the daily follower snapshots.</p>
        <label style={{ display: 'flex', alignItems: 'center', gap: 8, marginBottom: 12 }}>
          <input type="checkbox" checked={goalEnabled} onChange={e => setGoalEnabled(e.target.checked)} /> Track a follower goal
        </label>
        {goalEnabled && (
          <div className="form-row">
            <div className="form-group">
              <label>Target followers</label>
              <input className="input" type="number" min={1} value={goal.target} onChange={e => setGoal({ target: Number(e.target.value) })} />
            </div>
            <div className="form-group">
              <label>Start date</label>
              <input className="input" type="date" value={goal.startDate} onChange={e => setGoal({ startDate: e.target.value })} />
            </div>
            <div className="form-group">
              <label>End date</label>
              <input className="input" type="date" value={goal.endDate} onChange={e => setGoal({ endDate: e.target.value })} />
            </div>
            <div className="form-group">
              <label title="Leave empty to use the count recorded on the start date">Start followers (optional)</label>
              <input className="input" type="number" min={0} placeholder="from snapshot"
                value={goal.startFollowers ?? ''}
                onChange={e => setGoal({ startFollowers: e.target.value === '' ? undefined : Number(e.target.value) })} />
            </div>
            <div className="form-group">
              <label title="Defaults to halfway between start and end">Halfway review</label>
              <input className="input" type="date" value={goal.halfwayDate ?? ''} onChange={e => setGoal({ halfwayDate: e.target.value })} />
            </div>
            <div className="form-group">
              <label>Needed by then (% of the way)</label>
              <input className="input" type="number" min={1} max={99}
                value={Math.round((goal.halfwayThreshold ?? 0.4) * 100)}
                onChange={e => setGoal({ halfwayThreshold: Number(e.target.value) / 100 })} />
            </div>
          </div>
        )}
      </div>

      <div className="card">
        <h3><CalendarRange size={16} /> Weekly plan</h3>
        <p className="hint">Recurring Instagram slots. The calendar shows open slots as placeholders and counts each week's format mix against them.</p>
        {settings.weeklyPlan.length === 0 ? (
          <button className="btn btn-secondary btn-sm" style={{ marginBottom: 12 }}
            onClick={() => setSettings(s => s && ({ ...s, weeklyPlan: EXAMPLE_PLAN.map(sl => ({ ...sl, id: newSlotId() })) }))}>
            Start from the growth plan's week (6 feed posts, 3 reels)
          </button>
        ) : (
          <table className="slot-table">
            <thead><tr><th>Day</th><th>Time</th><th>Format</th><th>Label</th><th /></tr></thead>
            <tbody>
              {settings.weeklyPlan.map(slot => (
                <tr key={slot.id}>
                  <td>
                    <select className="select" value={slot.weekday} onChange={e => setSlot(slot.id, { weekday: Number(e.target.value) })}>
                      {WEEKDAYS.map((d, i) => <option key={d} value={i}>{d}</option>)}
                    </select>
                  </td>
                  <td><input className="input" type="time" value={slot.time ?? ''} onChange={e => setSlot(slot.id, { time: e.target.value || undefined })} /></td>
                  <td>
                    <select className="select" value={slot.format} onChange={e => setSlot(slot.id, { format: e.target.value as SlotFormat })}>
                      {SLOT_FORMATS.map(f => <option key={f} value={f}>{FORMAT_LABELS[f]}</option>)}
                    </select>
                  </td>
                  <td><input className="input" value={slot.label ?? ''} placeholder="e.g. Radar der Woche" onChange={e => setSlot(slot.id, { label: e.target.value })} /></td>
                  <td><button className="btn btn-ghost btn-sm" onClick={() => removeSlot(slot.id)} title="Remove slot"><Trash2 size={14} /></button></td>
                </tr>
              ))}
            </tbody>
          </table>
        )}
        <div style={{ display: 'flex', gap: 12, alignItems: 'flex-end', marginTop: 8, flexWrap: 'wrap' }}>
          <button className="btn btn-ghost btn-sm" onClick={addSlot}><Plus size={14} /> Add slot</button>
          <div className="form-group" style={{ marginBottom: 0, marginLeft: 'auto' }}>
            <label>Warn above this many single images per week</label>
            <input className="input" type="number" min={0} style={{ maxWidth: 120 }}
              value={settings.maxSingleImagesPerWeek ?? 2}
              onChange={e => setSettings(s => s && ({ ...s, maxSingleImagesPerWeek: Number(e.target.value) }))} />
          </div>
        </div>
      </div>

      <div className="card">
        <h3><Bot size={16} /> Content agents</h3>
        <p className="hint">
          Agents connected over MCP can find content, build images and fill the Drafts list on their own. Their drafts are marked with
          where they came from. This limit caps how many of them may wait for review at once; an agent cannot add more until people have
          scheduled or deleted some. 0 means no limit.
        </p>
        <div className="form-group" style={{ marginBottom: 0 }}>
          <label>Open agent drafts at most</label>
          <input className="input" type="number" min={0} max={500} style={{ maxWidth: 120 }}
            value={settings.maxAgentDrafts ?? 0}
            onChange={e => setSettings(s => s && ({ ...s, maxAgentDrafts: Math.max(0, Number(e.target.value) || 0) }))} />
        </div>
      </div>

      <div className="card">
        <h3><Wand2 size={16} /> Instagram caption tone</h3>
        <p className="hint">The instructions behind “Rewrite for Instagram” in the post editor. Leave empty for the default shown as placeholder.</p>
        <textarea className="textarea" rows={10} value={settings.instagramRewritePrompt ?? ''} placeholder={DEFAULT_REWRITE_PROMPT}
          onChange={e => setSettings(s => s && ({ ...s, instagramRewritePrompt: e.target.value }))} />
        {!settings.instagramRewritePrompt && (
          <button className="btn btn-ghost btn-sm" style={{ marginTop: 6 }}
            onClick={() => setSettings(s => s && ({ ...s, instagramRewritePrompt: DEFAULT_REWRITE_PROMPT }))}>
            Start from the default
          </button>
        )}
      </div>

      <div className="card">
        <h3><Radar size={16} /> Radar der Woche</h3>
        <p className="hint">The carousel built by “Radar Content” in the post editor: an opening slide, one slide per queued news item with its “Kommt's auf Deutsch?” badge, and a closing slide.</p>
        <div className="form-row">
          <div className="form-group">
            <label>Title</label>
            <input className="input" value={radar.title ?? ''} onChange={e => setRadar({ title: e.target.value })} />
          </div>
          <div className="form-group">
            <label>Badge question</label>
            <input className="input" value={radar.badgeQuestion ?? ''} onChange={e => setRadar({ badgeQuestion: e.target.value })} />
          </div>
          <div className="form-group">
            <label>Accent colour</label>
            <input className="input" type="color" value={radar.accentColor || '#6366f1'} onChange={e => setRadar({ accentColor: e.target.value })} style={{ height: 38, padding: 2 }} />
          </div>
        </div>
        <div className="form-row">
          <SlideImagePicker label="Opening slide" url={radar.coverImageUrl} onChange={url => setRadar({ coverImageUrl: url || undefined })} />
          <SlideImagePicker label="Closing slide" url={radar.closingImageUrl} onChange={url => setRadar({ closingImageUrl: url || undefined })} />
        </div>
        <RadarFontPicker url={radar.fontUrl} name={radar.fontName} sample={radar.title || 'Radar der Woche'}
          onChange={font => setRadar({ fontUrl: font?.url, fontName: font?.name })} />
        {!radar.closingImageUrl && (
          <div className="form-group">
            <label>Closing slide text (when there is no closing image)</label>
            <input className="input" value={radar.closingText ?? ''} placeholder="Welche Neuheit interessiert euch am meisten? Schreibt es in die Kommentare!"
              onChange={e => setRadar({ closingText: e.target.value })} />
          </div>
        )}
        <div className="form-group">
          <label>Caption intro</label>
          <textarea className="textarea" rows={2} value={radar.captionIntro ?? ''} placeholder="📡 Radar der Woche: die Ankündigungen, die ihr kennen solltet."
            onChange={e => setRadar({ captionIntro: e.target.value })} />
        </div>
        <div className="form-group">
          <label>Caption outro (question, share prompt, hashtags)</label>
          <textarea className="textarea" rows={3} value={radar.captionOutro ?? ''}
            placeholder={'Schickt das eurer Spielerunde! Und: Welche davon landet auf eurer Liste? 👇\n\n#brettspiele #brettspielneuheiten #boardgames'}
            onChange={e => setRadar({ captionOutro: e.target.value })} />
        </div>
      </div>
    </div>
  );
}
