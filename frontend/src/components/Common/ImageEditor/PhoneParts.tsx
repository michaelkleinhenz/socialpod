import { useRef, useState, type PointerEvent as ReactPointerEvent, type ReactNode } from 'react';
import { ArrowDown, ArrowUp, Check, ChevronLeft, Copy, Layers as LayersIcon, Trash2, type LucideIcon } from 'lucide-react';

// The editor's phone layout (768px and below), modelled on the mobile
// editors of Canva and Adobe Express: a bottom bar that holds the add tools,
// or the selected object's tools once one is tapped; one control at a time
// in a tray laid over the bottom of the picture, so the picture never
// resizes; a small toolbar on the selected object; and bottom sheets for
// browsing the templates, the media library and the objects.

export interface BarItem {
  id: string;
  label: string;
  icon: LucideIcon;
  onClick: () => void;
  active?: boolean;
  disabled?: boolean;
}

// With back set, the bar belongs to a selected object, and back deselects it.
export function PhoneBar({ items, back }: { items: BarItem[]; back?: () => void }) {
  return (
    <nav className="ie-mbar" aria-label="Tools">
      {back && (
        <button className="ie-mbar-back" onClick={back} aria-label="Deselect">
          <ChevronLeft size={20} /><span>Done</span>
        </button>
      )}
      {items.map(it => (
        <button key={it.id} className={it.active ? 'active' : ''} onClick={it.onClick} disabled={it.disabled} aria-pressed={it.active}>
          <it.icon size={19} /><span>{it.label}</span>
        </button>
      ))}
    </nav>
  );
}

export interface TraySub {
  id: string;
  label: string;
}

// One control, or a few small ones picked by the chips above it (subs), with
// a button to put it away.
export function PhoneTray({ subs, sub, onSub, lead, children, onDone }: {
  subs?: TraySub[];
  sub?: string | null;
  onSub?: (id: string) => void;
  lead?: ReactNode;
  children: ReactNode;
  onDone: () => void;
}) {
  return (
    <div className="ie-tray">
      <div className="ie-tray-main">
        {(lead || (subs && subs.length > 1)) && (
          <div className="ie-tray-subs">
            {lead}
            {subs && subs.length > 1 && subs.map(s => (
              <button key={s.id} className={`ie-tray-sub ${sub === s.id ? 'active' : ''}`} onClick={() => onSub?.(s.id)} aria-pressed={sub === s.id}>
                {s.label}
              </button>
            ))}
          </div>
        )}
        {children}
      </div>
      <button className="ie-tray-done" onClick={onDone} aria-label="Done"><Check size={20} /></button>
    </div>
  );
}

// Starts at half the screen; dragging its handle up (or tapping it) gives
// it the full height, dragging it down shrinks it again or closes it.
export function BottomSheet({ title, onClose, children }: { title: string; onClose: () => void; children: ReactNode }) {
  const [full, setFull] = useState(false);
  const [drag, setDrag] = useState(0);
  const start = useRef<number | null>(null);
  const onDown = (e: ReactPointerEvent<HTMLDivElement>) => {
    e.currentTarget.setPointerCapture(e.pointerId);
    start.current = e.clientY;
  };
  const onMove = (e: ReactPointerEvent<HTMLDivElement>) => {
    if (start.current === null) return;
    const dy = e.clientY - start.current;
    setDrag(Math.max(0, dy));
    if (dy < -40) setFull(true);
  };
  const onUp = (e: ReactPointerEvent<HTMLDivElement>) => {
    if (start.current === null) return;
    const dy = e.clientY - start.current;
    start.current = null;
    setDrag(0);
    if (dy > 80) { if (full) setFull(false); else onClose(); }
    else if (Math.abs(dy) < 5) setFull(f => !f);
  };
  return (
    <>
      <div className="ie-scrim" onClick={onClose} />
      <div
        className={`ie-sheet ${full ? 'full' : ''} ${drag ? 'dragging' : ''}`}
        style={drag ? { transform: `translateY(${drag}px)` } : undefined}
        role="dialog"
        aria-label={title}
      >
        <div className="ie-sheet-grab" onPointerDown={onDown} onPointerMove={onMove} onPointerUp={onUp} onPointerCancel={onUp}>
          <span />
        </div>
        <div className="ie-sheet-head">{title}</div>
        <div className="ie-sheet-body">{children}</div>
      </div>
    </>
  );
}

// The selected object's quick actions, placed next to it on the picture.
export function MiniToolbar({ left, top, canDuplicate, canForward, canBackward, onDuplicate, onForward, onBackward, onLayers, onDelete }: {
  left: number;
  top: number;
  canDuplicate: boolean;
  canForward: boolean;
  canBackward: boolean;
  onDuplicate: () => void;
  onForward: () => void;
  onBackward: () => void;
  onLayers: () => void;
  onDelete: () => void;
}) {
  return (
    <div className="ie-mini" style={{ left, top }} role="toolbar" aria-label="Object">
      <button onClick={onDuplicate} disabled={!canDuplicate} aria-label="Duplicate" title="Duplicate"><Copy size={18} /></button>
      <button onClick={onForward} disabled={!canForward} aria-label="Bring forward" title="Bring forward"><ArrowUp size={18} /></button>
      <button onClick={onBackward} disabled={!canBackward} aria-label="Send backward" title="Send backward"><ArrowDown size={18} /></button>
      <button onClick={onLayers} aria-label="Objects" title="Objects"><LayersIcon size={18} /></button>
      <button onClick={onDelete} className="danger" aria-label="Delete" title="Delete"><Trash2 size={18} /></button>
    </div>
  );
}
