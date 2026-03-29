import { useDraggable } from '@dnd-kit/core';
import type { ReactNode } from 'react';

interface Props {
  id: string;
  children: ReactNode;
}

export function DraggablePost({ id, children }: Props) {
  const { attributes, listeners, setNodeRef, isDragging } = useDraggable({ id });

  return (
    <div
      ref={setNodeRef}
      {...listeners}
      {...attributes}
      style={{ opacity: isDragging ? 0.4 : 1, cursor: 'grab' }}
    >
      {children}
    </div>
  );
}
