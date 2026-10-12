import { Film, GalleryHorizontal, Image, CircleDashed, Sparkles, AlertTriangle } from 'lucide-react';
import type { SlotFormat, WeeklyPlanSlot } from '../../types';
import { FORMAT_LABELS, type WeekMix } from '../../services/instagramGrowth';

export function FormatIcon({ format, size = 12 }: { format: SlotFormat; size?: number }) {
  switch (format) {
    case 'reel': return <Film size={size} />;
    case 'carousel': return <GalleryHorizontal size={size} />;
    case 'image': return <Image size={size} />;
    case 'story': return <CircleDashed size={size} />;
    default: return <Sparkles size={size} />;
  }
}

/** An open slot of the weekly plan: a dashed placeholder that creates the post. */
export function PlanSlotGhost({ slot, compact, onCreate }: { slot: WeeklyPlanSlot; compact?: boolean; onCreate: () => void }) {
  const title = `Open slot: ${FORMAT_LABELS[slot.format]}${slot.time ? ` at ${slot.time}` : ''}${slot.label ? ` — ${slot.label}` : ''}. Click to create it.`;
  return (
    <button
      type="button"
      className={`plan-slot${compact ? ' compact' : ''}`}
      title={title}
      onClick={e => { e.stopPropagation(); onCreate(); }}
      onDoubleClick={e => e.stopPropagation()}
    >
      <FormatIcon format={slot.format} size={11} />
      {slot.time && <span className="plan-slot-time">{slot.time}</span>}
      {!compact && <span className="plan-slot-label">{slot.label || FORMAT_LABELS[slot.format]}</span>}
    </button>
  );
}

function MixPart({ label, value, target, over }: { label: string; value: number; target: number; over?: boolean }) {
  const short = target > 0 && value < target;
  return (
    <span className={`mix-part${over ? ' over' : short ? ' short' : target > 0 ? ' met' : ''}`}>
      {value}/{target} {label}
    </span>
  );
}

/** One week's Instagram format mix against the weekly plan. */
export function WeekMixBar({ label, mix }: { label: string; mix: WeekMix }) {
  return (
    <div className={`week-mix${mix.tooManyImages ? ' warn' : ''}`}>
      <span className="week-mix-label">{label}</span>
      <MixPart label="reels" value={mix.reels} target={mix.targetReels} />
      <span className="mix-sep">·</span>
      <MixPart label="carousels" value={mix.carousels} target={mix.targetCarousels} />
      <span className="mix-sep">·</span>
      <span className={`mix-part${mix.tooManyImages ? ' over' : ''}`}>
        {mix.images}/{mix.maxImages} single images
      </span>
      <span className="mix-sep">·</span>
      <MixPart label="feed posts" value={mix.feedPosts} target={mix.targetFeedPosts} />
      {mix.stories > 0 && <><span className="mix-sep">·</span><span className="mix-part">{mix.stories} stories</span></>}
      {mix.tooManyImages && (
        <span className="week-mix-warning" title="The plan keeps single images to big news — they rarely reach non-followers.">
          <AlertTriangle size={13} /> Too many single images
        </span>
      )}
    </div>
  );
}
