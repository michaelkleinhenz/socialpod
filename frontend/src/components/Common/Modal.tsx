import { useCallback, useRef, type CSSProperties, type ReactNode } from 'react';

/**
 * What the guard asks before throwing away a half-filled form. Kept generic so
 * every modal sounds the same.
 */
const DISCARD_PROMPT = 'Discard your changes? What you entered in this dialog will be lost.';

interface ModalProps {
  /** Closes the modal. Only called once the dismissal is allowed to go through. */
  onClose: () => void;
  /**
   * Overrides the automatic unsaved-input detection. Leave it unset and the
   * modal watches its own fields; set it when the modal holds state the DOM
   * does not show (a picked image, a reordered list).
   */
  dirty?: boolean;
  /** Replaces the discard wording, for a modal where "changes" reads wrong. */
  confirmMessage?: string;
  /**
   * Set to false for a modal that must be dismissed through its own buttons.
   * A backdrop click is then ignored entirely.
   */
  closeOnBackdrop?: boolean;
  /** Extra classes on the modal panel, e.g. "conv-modal". */
  className?: string;
  style?: CSSProperties;
  overlayStyle?: CSSProperties;
  children: ReactNode;
}

/**
 * Modal renders the standard overlay + panel and owns one piece of behaviour
 * the call sites kept getting wrong: a click on the backdrop used to close the
 * dialog immediately, taking every field the user had filled in with it.
 *
 * Dismissal now goes through requestClose, which asks first whenever the user
 * has entered anything. Dirtiness comes from native input/change events on the
 * panel's own subtree: those fire only for real typing and picking, never for
 * React's own state updates, so a modal whose fields are populated
 * asynchronously does not look edited. Clicking a modal's Cancel or Save
 * button is unaffected — those call their own handlers directly.
 */
export function Modal({
  onClose,
  dirty,
  confirmMessage,
  closeOnBackdrop = true,
  className,
  style,
  overlayStyle,
  children,
}: ModalProps) {
  const touched = useRef(false);
  // Where the gesture that produced a click started. A press inside the panel
  // that drifts onto the backdrop (selecting text, dragging a slider) reports
  // the overlay as the click target, and must not count as clicking outside.
  const pressedBackdrop = useRef(false);

  const markTouched = useCallback(() => {
    touched.current = true;
  }, []);

  const requestClose = useCallback(() => {
    const unsaved = dirty ?? touched.current;
    if (unsaved && !window.confirm(confirmMessage ?? DISCARD_PROMPT)) return;
    onClose();
  }, [dirty, confirmMessage, onClose]);

  return (
    <div
      className="modal-overlay"
      style={overlayStyle}
      onPointerDown={e => {
        pressedBackdrop.current = e.target === e.currentTarget;
      }}
      onClick={e => {
        if (!closeOnBackdrop) return;
        if (e.target !== e.currentTarget || !pressedBackdrop.current) return;
        requestClose();
      }}
    >
      <div
        className={className ? `modal ${className}` : 'modal'}
        style={style}
        onClick={e => e.stopPropagation()}
        onInput={markTouched}
        onChange={markTouched}
      >
        {children}
      </div>
    </div>
  );
}
