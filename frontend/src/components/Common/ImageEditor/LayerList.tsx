import {
  DndContext, KeyboardSensor, PointerSensor, closestCenter, useSensor, useSensors, type DragEndEvent, type Modifier,
} from '@dnd-kit/core';
import {
  SortableContext, sortableKeyboardCoordinates, useSortable, verticalListSortingStrategy,
} from '@dnd-kit/sortable';
import { CSS } from '@dnd-kit/utilities';
import { Brush, Circle, Film, GripVertical, Image as ImageIcon, Square, Stamp, Triangle, Type } from 'lucide-react';
import { cssFontFamily, layerName, type EditorLayer } from './editorModel';

// Rows only move up and down.
const verticalOnly: Modifier = ({ transform }) => ({ ...transform, x: 0 });

interface Props {
  // In drawing order: the first is drawn first, so it lies at the bottom.
  layers: EditorLayer[];
  selectedId: string | null;
  // The base, listed at the bottom; it cannot be moved. A photo, or the
  // colour of a blank canvas.
  background: { src: string } | { color: string };
  onSelect: (layer: EditorLayer) => void;
  onSelectBackground: () => void;
  // The new drawing order.
  onReorder: (layers: EditorLayer[]) => void;
}

// The editor's objects, top to bottom, as Adobe Express lists them: the order
// of the list is the order they are stacked in, and dragging a row restacks.
export function LayerList({ layers, selectedId, background, onSelect, onSelectBackground, onReorder }: Props) {
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
        <p className="ie-hint">Texts, images, shapes and paint you add are listed here. The top one lies on top.</p>
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
      <button className="ie-layer ie-layer-fixed" onClick={onSelectBackground} title="Always at the bottom">
        <span className="ie-layer-thumb">
          {'src' in background ? <img src={background.src} alt="" /> : <span className="ie-layer-color" style={{ background: background.color }} />}
        </span>
        <span className="ie-layer-name">Background</span>
      </button>
    </div>
  );
}

function LayerRow({ layer, selected, onSelect }: { layer: EditorLayer; selected: boolean; onSelect: () => void }) {
  const { attributes, listeners, setNodeRef, transform, transition, isDragging } = useSortable({ id: layer.id });
  const name = layerName(layer);
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
      <span className="ie-layer-thumb">{layerThumb(layer)}</span>
      <span className="ie-layer-name">
        {layerIcon(layer)}
        <span>{name}</span>
      </span>
      <GripVertical size={14} className="ie-layer-grip" />
    </button>
  );
}

function layerIcon(layer: EditorLayer) {
  switch (layer.kind) {
    case 'text': return <Type size={12} />;
    case 'shape': return layer.shape === 'ellipse' ? <Circle size={12} /> : layer.shape === 'triangle' ? <Triangle size={12} /> : <Square size={12} />;
    case 'paint': return <Brush size={12} />;
    default: return layer.clip ? <Film size={12} /> : layer.origin === 'watermark' ? <Stamp size={12} /> : <ImageIcon size={12} />;
  }
}

function layerThumb(layer: EditorLayer) {
  switch (layer.kind) {
    case 'text':
      return <span className="ie-layer-text" style={{ fontFamily: cssFontFamily(layer.fontFamily), color: layer.fill, fontWeight: layer.bold ? 700 : 400 }}>Aa</span>;
    case 'shape':
      return (
        <span
          className="ie-layer-shape"
          style={{
            background: layer.fill || 'transparent',
            border: layer.strokeWidth > 0 ? `2px solid ${layer.stroke}` : undefined,
            borderRadius: layer.shape === 'ellipse' ? '50%' : `${layer.cornerRadius ?? 0}%`,
            clipPath: layer.shape === 'triangle' ? 'polygon(50% 0, 100% 100%, 0 100%)' : undefined,
            opacity: layer.opacity,
          }}
        />
      );
    case 'paint': {
      const st = layer.strokes[layer.strokes.length - 1];
      return <span className="ie-layer-paint" style={{ background: `radial-gradient(circle, ${st?.color ?? '#000000'} 0%, transparent 70%)`, opacity: Math.max(0.4, st?.opacity ?? 1) }} />;
    }
    default:
      // A clip's thumbnail is its first frame, the video itself.
      if (layer.clip) return <video src={layer.img instanceof HTMLVideoElement ? layer.img.src : ''} muted preload="metadata" />;
      return <img src={layer.img instanceof HTMLImageElement ? layer.img.src : ''} alt="" />;
  }
}
