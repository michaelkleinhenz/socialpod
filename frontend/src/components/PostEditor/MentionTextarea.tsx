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

  const filteredMentions = mentionState
    ? mentions
        .filter(m => {
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
      const tokenLen = 1 + query.length; // '@' + query chars

      if (platform) {
        const handle = mention.handles[platform] || `@${mention.name}`;
        const normalized = handle.startsWith('@') ? handle : `@${handle}`;
        onChange(value.slice(0, start) + normalized + ' ' + value.slice(start + tokenLen));
      } else {
        onMentionInsert?.(mention, start, tokenLen);
      }

      setMentionState(null);
    },
    [mentionState, platform, value, onChange, onMentionInsert],
  );

  const handleChange = (e: React.ChangeEvent<HTMLTextAreaElement>) => {
    const newVal = e.target.value;
    const cursor = e.target.selectionStart ?? newVal.length;
    const state = findMentionQuery(newVal, cursor);
    setMentionState(state);
    if (state) setSelectedIdx(0);
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
    }
  };

  const handleBlur = () => {
    // Delay so mousedown on dropdown option fires first
    setTimeout(() => setMentionState(null), 150);
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
      {mentionState && filteredMentions.length > 0 && (
        <div className="mention-dropdown">
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
