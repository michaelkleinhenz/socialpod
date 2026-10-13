import {
  DndContext, KeyboardSensor, PointerSensor, closestCenter, useSensor, useSensors, type DragEndEvent, type Modifier,
} from '@dnd-kit/core';
import {
  SortableContext, sortableKeyboardCoordinates, useSortable, verticalListSortingStrategy,
} from '@dnd-kit/sortable';
import { CSS } from '@dnd-kit/utilities';
import { GripVertical, Image as ImageIcon, Stamp, Type } from 'lucide-react';
import type { EditorLayer } from './editorModel';

// Rows only move up and down.
const verticalOnly: Modifier = ({ transform }) => ({ ...transform, x: 0 });

interface Props {
  // In drawing order: the first is drawn first, so it lies at the bottom.
  layers: EditorLayer[];
  selectedId: string | null;
  // The base picture, listed at the bottom; it cannot be moved.
  backgroundSrc: string;
  onSelect: (layer: EditorLayer) => void;
  onSelectBackground: () => void;
  // The new drawing order.
  onReorder: (layers: EditorLayer[]) => void;
}

// The editor's objects, top to bottom, as Adobe Express lists them: the order
// of the list is the order they are stacked in, and dragging a row restacks.
export function LayerList({ layers, selectedId, backgroundSrc, onSelect, onSelectBackground, onReorder }: Props) {
  const sensors = useSensors(
    // A few pixels of travel before a drag starts, so a tap still selects.
    useSensor(PointerSensor, { activationConstraint: { distance: 4 } }),
    useSensor(KeyboardSensor, { coordinateGetter: sortableKeyboardCoordinates }),
  );
  const topFirst = [...layers].reverse();

  const onDragEnd = ({ active, over }: DragEndEvent) => {
    if (!over || active.id === over.id) return;
    const from = topFirst.findIndex(l => l.id === active.id);
    const to = topFirst.findIndex(l => l.id === over.id);
    if (from < 0 || to < 0) return;
    const next = [...topFirst];
    const [moved] = next.splice(from, 1);
    next.splice(to, 0, moved);
    onReorder(next.reverse());
  };

  return (
    <div className="ie-layers-list">
      {layers.length === 0 && (
        <p className="ie-hint">Texts, watermarks and images you add are listed here. The top one lies on top.</p>
      )}
      <DndContext
        sensors={sensors}
        collisionDetection={closestCenter}
        modifiers={[verticalOnly]}
        onDragEnd={onDragEnd}
      >
        <SortableContext items={topFirst.map(l => l.id)} strategy={verticalListSortingStrategy}>
          <div className="ie-layers-sortable">
            {topFirst.map(l => (
              <LayerRow key={l.id} layer={l} selected={l.id === selectedId} onSelect={() => onSelect(l)} />
            ))}
          </div>
        </SortableContext>
      </DndContext>
      <button className="ie-layer ie-layer-fixed" onClick={onSelectBackground} title="The picture itself, always at the bottom">
        <span className="ie-layer-thumb"><img src={backgroundSrc} alt="" /></span>
        <span className="ie-layer-name">Background</span>
      </button>
    </div>
  );
}

function LayerRow({ layer, selected, onSelect }: { layer: EditorLayer; selected: boolean; onSelect: () => void }) {
  const { attributes, listeners, setNodeRef, transform, transition, isDragging } = useSortable({ id: layer.id });
  const name = layer.kind === 'text'
    ? (layer.text.trim().split('\n')[0] || 'Empty text')
    : (layer.origin === 'watermark' ? 'Watermark' : 'Image');
  return (
    <button
      ref={setNodeRef}
      className={`ie-layer ${selected ? 'active' : ''} ${isDragging ? 'dragging' : ''}`}
      style={{ transform: CSS.Translate.toString(transform), transition }}
      onClick={onSelect}
      title={`${name}: drag to restack`}
      {...attributes}
      {...listeners}
    >
      <span className="ie-layer-thumb">
        {layer.kind === 'text' ? (
          <span className="ie-layer-text" style={{ fontFamily: layer.fontFamily, color: layer.fill, fontWeight: layer.bold ? 700 : 400 }}>Aa</span>
        ) : (
          <img src={layer.img.src} alt="" />
        )}
      </span>
      <span className="ie-layer-name">
        {layer.kind === 'text' ? <Type size={12} /> : layer.origin === 'watermark' ? <Stamp size={12} /> : <ImageIcon size={12} />}
        <span>{name}</span>
      </span>
      <GripVertical size={14} className="ie-layer-grip" />
    </button>
  );
}
