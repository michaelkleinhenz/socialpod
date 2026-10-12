// Helpers for the Instagram growth features: caption checks, post formats and
// matching posts against the team's weekly content plan.
import type { GermanRelease, Post, RadarQueueEntry, SlotFormat, WeeklyPlanSlot } from '../types';

// @usernames, not the "@" inside an e-mail address. A trailing full stop
// belongs to the sentence.
const MENTION_RE = /(^|[^A-Za-z0-9._@])@([A-Za-z0-9._]{1,30})/g;

export function extractHandles(text: string): string[] {
  const out: string[] = [];
  const seen = new Set<string>();
  for (const m of text.matchAll(MENTION_RE)) {
    const handle = m[2].replace(/\.+$/, '');
    const key = handle.toLowerCase();
    if (!handle || seen.has(key)) continue;
    seen.add(key);
    out.push(handle);
  }
  return out;
}

export function normalizeHandle(raw: string): string {
  return raw.trim().replace(/^@+/, '');
}

export const IG_USERNAME_RE = /^[A-Za-z0-9._]{1,30}$/;

const HASHTAG_RE = /(^|\s)#[\p{L}\p{N}_]+/gu;

export function countHashtags(text: string): number {
  return [...text.matchAll(HASHTAG_RE)].length;
}

// Phrases that ask people to pass a post on, in German and English. The plan
// counts sends per reach as the strongest signal, so a caption should ask for
// them.
const SHARE_PROMPT_RE = /\b(schick|schickt|schicke|teil|teilt|teile|leite|leitet|markier|markiert|markiere|send|share|tag)\w*\b.{0,60}\b(freund|spielerunde|runde|person|jemand|wem|friend|someone|crew|group|partner)/iu;

export type CheckLevel = 'ok' | 'warn' | 'missing';

export interface CaptionCheck {
  id: string;
  label: string;
  level: CheckLevel;
  hint: string;
}

/** Text without the trailing hashtag block, for checks about the prose. */
function proseOf(text: string): string {
  const lines = text.trim().split('\n');
  while (lines.length > 0) {
    const last = lines[lines.length - 1].trim();
    if (last === '' || /^(#[\p{L}\p{N}_]+\s*)+$/u.test(last)) lines.pop();
    else break;
  }
  return lines.join('\n').trim();
}

/**
 * Checks an Instagram caption against the growth plan's caption rules: a hook
 * in the first line, a closing question, a share prompt, 3–5 hashtags, a
 * tagged publisher, and not a copy of the Bluesky text.
 */
export function checkCaption(text: string, opts: { blueskyText?: string; taggedCount: number }): CaptionCheck[] {
  const trimmed = text.trim();
  const prose = proseOf(trimmed);
  const firstLine = trimmed.split('\n')[0]?.trim() ?? '';
  const hashtags = countHashtags(trimmed);
  const checks: CaptionCheck[] = [];

  const hookOk = firstLine.length >= 10 && firstLine.length <= 90 && !firstLine.startsWith('#') && !firstLine.startsWith('@');
  checks.push({
    id: 'hook',
    label: 'Hook in the first line',
    level: hookOk ? 'ok' : firstLine ? 'warn' : 'missing',
    hint: hookOk
      ? 'Short opening line people see before "more".'
      : firstLine.length > 90
        ? 'The first line is long — Instagram cuts it off. Lead with a short, pointed line.'
        : 'Open with a line that stops the scroll, e.g. "Der neue Feld: kaufen oder warten?"',
  });

  const lastProseLine = prose.split('\n').filter(l => l.trim()).pop()?.trim() ?? '';
  const questionOk = /\?\s*\S{0,4}$/u.test(lastProseLine);
  checks.push({
    id: 'question',
    label: 'Ends with a question',
    level: questionOk ? 'ok' : 'missing',
    hint: questionOk ? 'Invites comments.' : 'Close with a question to the community before the hashtags.',
  });

  const shareOk = SHARE_PROMPT_RE.test(prose);
  checks.push({
    id: 'share',
    label: 'Share prompt',
    level: shareOk ? 'ok' : 'missing',
    hint: shareOk ? 'Asks people to pass it on.' : 'Ask for a send, e.g. "Schick das der Person, die immer die Regeln erklärt."',
  });

  checks.push({
    id: 'hashtags',
    label: `3–5 hashtags (${hashtags})`,
    level: hashtags >= 3 && hashtags <= 5 ? 'ok' : hashtags > 5 ? 'warn' : 'missing',
    hint: hashtags > 5
      ? 'Too many — keep 3 to 5 specific ones like #brettspiele #kennerspiel.'
      : hashtags < 3
        ? 'Add 3 to 5 specific hashtags, e.g. #brettspiele #brettspielneuheiten #spielessen.'
        : 'Specific, not a long block.',
  });

  checks.push({
    id: 'tagged',
    label: 'Publisher tagged',
    level: opts.taggedCount > 0 ? 'ok' : 'missing',
    hint: opts.taggedCount > 0
      ? 'Tagged accounts are notified and often share it in their story.'
      : 'Mention the publisher (and author) with @ so they are tagged.',
  });

  if (opts.blueskyText !== undefined) {
    const same = opts.blueskyText.trim() !== '' && opts.blueskyText.trim() === trimmed;
    checks.push({
      id: 'mirror',
      label: 'Not a copy of the Bluesky text',
      level: same ? 'warn' : 'ok',
      hint: same ? 'Identical to the Bluesky text — write the Instagram caption for Instagram.' : 'Written for Instagram.',
    });
  }
  return checks;
}

export type PostFormat = 'reel' | 'carousel' | 'image' | 'story';

function isVideoUrl(url: string) {
  return /\.(mp4|mov)$/i.test(url);
}

/** The format a post takes on Instagram. */
export function postFormat(post: Pick<Post, 'postType' | 'imageUrls'>): PostFormat {
  if (post.postType === 'reel') return 'reel';
  if (post.postType === 'story') return 'story';
  const n = post.imageUrls?.length ?? 0;
  if (n > 1) return 'carousel';
  if (n === 1 && isVideoUrl(post.imageUrls![0])) return 'reel';
  return 'image';
}

/** Posts that count towards the Instagram plan: on Instagram and not failed. */
export function countsForPlan(post: Post): boolean {
  return post.platforms.includes('instagram') && post.status !== 'failed';
}

export const FORMAT_LABELS: Record<SlotFormat | PostFormat, string> = {
  reel: 'Reel',
  carousel: 'Carousel',
  image: 'Single image',
  story: 'Story',
  any: 'Any feed post',
};

function slotAccepts(format: SlotFormat, f: PostFormat): boolean {
  if (format === 'any') return f !== 'story';
  return format === f;
}

function minutesOf(time?: string): number | null {
  if (!time) return null;
  const [h, m] = time.split(':').map(Number);
  return h * 60 + m;
}

/** Monday-based weekday (0 = Monday) of a date. */
export function weekdayOf(d: Date): number {
  return (d.getDay() + 6) % 7;
}

/**
 * The slots of one day the posts that day leave open. Each post fills at most
 * one slot; slots with an exact format are matched before "any", and among
 * candidates the post closest in time wins.
 */
export function openSlotsForDay(day: Date, slots: WeeklyPlanSlot[], posts: Post[]): WeeklyPlanSlot[] {
  const daySlots = slots
    .filter(s => s.weekday === weekdayOf(day))
    .sort((a, b) => (a.format === 'any' ? 1 : 0) - (b.format === 'any' ? 1 : 0));
  const remaining = posts.filter(countsForPlan);
  const open: WeeklyPlanSlot[] = [];
  for (const slot of daySlots) {
    const target = minutesOf(slot.time);
    let best = -1;
    let bestDist = Infinity;
    remaining.forEach((p, i) => {
      if (!slotAccepts(slot.format, postFormat(p))) return;
      const at = new Date(p.scheduledAt);
      const dist = target === null ? 0 : Math.abs(at.getHours() * 60 + at.getMinutes() - target);
      if (dist < bestDist) { best = i; bestDist = dist; }
    });
    if (best >= 0) remaining.splice(best, 1);
    else open.push(slot);
  }
  return open.sort((a, b) => (minutesOf(a.time) ?? 0) - (minutesOf(b.time) ?? 0));
}

export interface WeekMix {
  reels: number;
  carousels: number;
  images: number;
  stories: number;
  feedPosts: number;
  targetReels: number;
  targetCarousels: number;
  targetImages: number;
  targetFeedPosts: number;
  maxImages: number;
  tooManyImages: boolean;
}

/** Counts a week's Instagram posts by format against the weekly plan. */
export function weekMix(posts: Post[], slots: WeeklyPlanSlot[], maxImages: number): WeekMix {
  const mix: WeekMix = {
    reels: 0, carousels: 0, images: 0, stories: 0, feedPosts: 0,
    targetReels: slots.filter(s => s.format === 'reel').length,
    targetCarousels: slots.filter(s => s.format === 'carousel').length,
    targetImages: slots.filter(s => s.format === 'image').length,
    targetFeedPosts: slots.filter(s => s.format !== 'story').length,
    maxImages,
    tooManyImages: false,
  };
  for (const p of posts) {
    if (!countsForPlan(p)) continue;
    const f = postFormat(p);
    if (f === 'reel') mix.reels++;
    else if (f === 'carousel') mix.carousels++;
    else if (f === 'image') mix.images++;
    else mix.stories++;
    if (f !== 'story') mix.feedPosts++;
  }
  mix.tooManyImages = mix.images > maxImages;
  return mix;
}

export function percent(v: number, digits = 1): string {
  return `${(v * 100).toFixed(digits)}%`;
}

export const radarReleaseLabels: Record<string, string> = { ja: 'Ja', nein: 'Nein', offen: 'Offen' };

/** The badge value a click on the Radar list's badge moves to: Offen → Ja → Nein → Offen. */
export function nextGermanRelease(v?: GermanRelease): GermanRelease {
  if (v === 'offen') return 'ja';
  if (v === 'ja') return 'nein';
  return 'offen';
}

/** Asks before taking a news item off the Radar; says so when a post already carries it. */
export function confirmRadarRemove(entry: RadarQueueEntry): boolean {
  const title = entry.newsTagline || '(No tagline)';
  return window.confirm(entry.radarPost
    ? `Remove “${title}” from the Radar? The Radar post that already carries its slide keeps it.`
    : `Remove “${title}” from the Radar? The news itself stays.`);
}
