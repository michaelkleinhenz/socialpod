import type { CSSProperties, ReactNode } from 'react';
import {
  DndContext, KeyboardSensor, MouseSensor, TouchSensor, closestCenter, useSensor, useSensors, type DragEndEvent,
} from '@dnd-kit/core';
import {
  SortableContext, rectSortingStrategy, sortableKeyboardCoordinates, useSortable,
} from '@dnd-kit/sortable';
import { CSS } from '@dnd-kit/utilities';

// Tiles that are put in order by dragging them: a carousel's images. A mouse
// drags after moving a few pixels, a finger after resting a moment (so a
// swipe still scrolls the page or a strip), and the keyboard picks a focused
// tile up with Space and moves it with the arrow keys. Buttons inside a tile
// keep working: a click never starts a drag.

export function SortableTiles({
  ids,
  onMove,
  className,
  children,
}: {
  ids: string[];
  /** The tile at index from was dropped at index to. */
  onMove: (from: number, to: number) => void;
  className?: string;
  children: ReactNode;
}) {
  const sensors = useSensors(
    useSensor(MouseSensor, { activationConstraint: { distance: 4 } }),
    useSensor(TouchSensor, { activationConstraint: { delay: 250, tolerance: 8 } }),
    useSensor(KeyboardSensor, { coordinateGetter: sortableKeyboardCoordinates }),
  );

  const onDragEnd = ({ active, over }: DragEndEvent) => {
    if (!over || active.id === over.id) return;
    const from = ids.indexOf(String(active.id));
    const to = ids.indexOf(String(over.id));
    if (from >= 0 && to >= 0) onMove(from, to);
  };

  return (
    <DndContext sensors={sensors} collisionDetection={closestCenter} onDragEnd={onDragEnd}>
      <SortableContext items={ids} strategy={rectSortingStrategy}>
        <div className={className}>{children}</div>
      </SortableContext>
    </DndContext>
  );
}

export function SortableTile({
  id,
  className,
  style,
  title,
  children,
}: {
  id: string;
  className?: string;
  style?: CSSProperties;
  title?: string;
  children: ReactNode;
}) {
  const { attributes, listeners, setNodeRef, transform, transition, isDragging } = useSortable({ id });
  return (
    <div
      ref={setNodeRef}
      className={`${className ?? ''}${isDragging ? ' sortable-dragging' : ''}`}
      style={{
        ...style,
        transform: CSS.Transform.toString(transform),
        transition,
        zIndex: isDragging ? 2 : undefined,
        cursor: 'grab',
      }}
      title={title}
      {...attributes}
      {...listeners}
    >
      {children}
    </div>
  );
}
