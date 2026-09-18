import { useState, useRef, useCallback } from 'react';
import type { MentionEntry, Platform } from '../../types';

interface MentionTextareaProps {
  value: string;
  onChange: (value: string) => void;
  onMentionInsert?: (mention: MentionEntry, queryStart: number, queryLength: number) => void;
  mentions: MentionEntry[];
  platform?: Platform;
  textareaRef?: React.RefObject<HTMLTextAreaElement | null>;
  className?: string;
  placeholder?: string;
  rows?: number;
  disabled?: boolean;
}

interface MentionState {
  query: string;
  start: number;
}

function findMentionQuery(text: string, cursor: number): MentionState | null {
  const before = text.slice(0, cursor);
  const match = before.match(/@(\w*)$/);
  if (!match) return null;
  return { query: match[1], start: cursor - match[0].length };
}

function getCaretCoordinates(element: HTMLTextAreaElement, position: number): { top: number; left: number; lineHeight: number } {
  const mirror = document.createElement('div');
  const computed = getComputedStyle(element);

  const stylesToCopy = [
    'fontFamily', 'fontSize', 'fontWeight', 'fontStyle', 'fontVariant',
    'letterSpacing', 'textTransform', 'wordSpacing', 'textIndent',
    'paddingTop', 'paddingRight', 'paddingBottom', 'paddingLeft',
    'borderTopWidth', 'borderRightWidth', 'borderBottomWidth', 'borderLeftWidth',
    'boxSizing', 'lineHeight', 'tabSize',
  ];

  mirror.style.position = 'absolute';
  mirror.style.top = '0';
  mirror.style.left = '-9999px';
  mirror.style.visibility = 'hidden';
  mirror.style.whiteSpace = 'pre-wrap';
  mirror.style.wordWrap = 'break-word';
  mirror.style.overflow = 'hidden';
  mirror.style.width = `${element.offsetWidth}px`;

  for (const prop of stylesToCopy) {
    mirror.style.setProperty(
      prop.replace(/[A-Z]/g, c => `-${c.toLowerCase()}`),
      computed.getPropertyValue(prop.replace(/[A-Z]/g, c => `-${c.toLowerCase()}`)),
    );
  }

  mirror.appendChild(document.createTextNode(element.value.substring(0, position)));

  const marker = document.createElement('span');
  marker.textContent = '​';
  mirror.appendChild(marker);

  document.body.appendChild(mirror);

  const coords = {
    top: marker.offsetTop + parseInt(computed.borderTopWidth) - element.scrollTop,
    left: marker.offsetLeft + parseInt(computed.borderLeftWidth) - element.scrollLeft,
    lineHeight: parseInt(computed.lineHeight) || Math.ceil(parseFloat(computed.fontSize) * 1.2),
  };

  document.body.removeChild(mirror);
  return coords;
}

export function MentionTextarea({
  value,
  onChange,
  onMentionInsert,
  mentions,
  platform,
  textareaRef: externalRef,
  className,
  placeholder,
  rows,
  disabled,
}: MentionTextareaProps) {
  const internalRef = useRef<HTMLTextAreaElement | null>(null);
  const ref = externalRef || internalRef;
  const containerRef = useRef<HTMLDivElement>(null);

  const [mentionState, setMentionState] = useState<MentionState | null>(null);
  const [selectedIdx, setSelectedIdx] = useState(0);
  const [dropdownPos, setDropdownPos] = useState<{ top: number; left: number } | null>(null);

  const filteredMentions = mentionState
    ? mentions
        .filter(m => {
          if (platform && !m.handles[platform]) return false;
          const q = mentionState.query.toLowerCase();
          if (!q) return true;
          return (
            m.name.toLowerCase().includes(q) ||
            Object.values(m.handles).some(h => h.toLowerCase().replace(/^@/, '').includes(q))
          );
        })
        .slice(0, 8)
    : [];

  const selectMention = useCallback(
    (mention: MentionEntry) => {
      if (!mentionState) return;
      const { query, start } = mentionState;
      const tokenLen = 1 + query.length;

      if (platform) {
        const handle = mention.handles[platform];
        if (!handle) return;
        const normalized = handle.startsWith('@') ? handle : `@${handle}`;
        onChange(value.slice(0, start) + normalized + ' ' + value.slice(start + tokenLen));
      } else {
        onMentionInsert?.(mention, start, tokenLen);
      }

      setMentionState(null);
      setDropdownPos(null);
    },
    [mentionState, platform, value, onChange, onMentionInsert],
  );

  const updateMentionState = (textarea: HTMLTextAreaElement, newValue: string) => {
    const cursor = textarea.selectionStart ?? newValue.length;
    const state = findMentionQuery(newValue, cursor);
    setMentionState(state);
    if (state) {
      setSelectedIdx(0);
      const coords = getCaretCoordinates(textarea, state.start);
      setDropdownPos({ top: coords.top + coords.lineHeight, left: coords.left });
    } else {
      setDropdownPos(null);
    }
  };

  const handleChange = (e: React.ChangeEvent<HTMLTextAreaElement>) => {
    const newVal = e.target.value;
    updateMentionState(e.target, newVal);
    onChange(newVal);
  };

  const handleKeyDown = (e: React.KeyboardEvent<HTMLTextAreaElement>) => {
    if (!mentionState || filteredMentions.length === 0) return;

    if (e.key === 'ArrowDown') {
      e.preventDefault();
      setSelectedIdx(i => Math.min(i + 1, filteredMentions.length - 1));
    } else if (e.key === 'ArrowUp') {
      e.preventDefault();
      setSelectedIdx(i => Math.max(i - 1, 0));
    } else if (e.key === 'Enter' || e.key === 'Tab') {
      if (filteredMentions[selectedIdx]) {
        e.preventDefault();
        selectMention(filteredMentions[selectedIdx]);
      }
    } else if (e.key === 'Escape') {
      setMentionState(null);
      setDropdownPos(null);
    }
  };

  const handleBlur = () => {
    setTimeout(() => {
      setMentionState(null);
      setDropdownPos(null);
    }, 150);
  };

  return (
    <div ref={containerRef} style={{ position: 'relative' }}>
      <textarea
        ref={ref}
        className={className}
        value={value}
        onChange={handleChange}
        onKeyDown={handleKeyDown}
        onBlur={handleBlur}
        placeholder={placeholder}
        rows={rows}
        disabled={disabled}
      />
      {mentionState && filteredMentions.length > 0 && dropdownPos && (
        <div
          className="mention-dropdown"
          style={{ top: `${dropdownPos.top + 4}px`, left: `${dropdownPos.left}px` }}
        >
          {filteredMentions.map((mention, i) => {
            const handle = platform ? mention.handles[platform] : null;
            const platformCount = Object.values(mention.handles).filter(Boolean).length;
            return (
              <div
                key={mention.id}
                className={`mention-option${i === selectedIdx ? ' selected' : ''}`}
                onMouseDown={e => {
                  e.preventDefault();
                  selectMention(mention);
                }}
              >
                <span className="mention-option-name">{mention.name}</span>
                <span className="mention-option-handle">
                  {handle
                    ? (handle.startsWith('@') ? handle : `@${handle}`)
                    : platformCount > 0
                    ? `${platformCount} platform${platformCount !== 1 ? 's' : ''}`
                    : 'no handles'}
                </span>
              </div>
            );
          })}
        </div>
      )}
    </div>
  );
}
