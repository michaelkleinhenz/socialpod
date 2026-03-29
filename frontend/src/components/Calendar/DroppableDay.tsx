import { useDroppable } from '@dnd-kit/core';
import type { ReactNode } from 'react';

interface Props {
  id: string;
  children: ReactNode;
}

export function DroppableDay({ id, children }: Props) {
  const { setNodeRef, isOver } = useDroppable({ id });

  return (
    <div
      ref={setNodeRef}
      className={`droppable-day ${isOver ? 'drag-over' : ''}`}
    >
      {children}
    </div>
  );
}
