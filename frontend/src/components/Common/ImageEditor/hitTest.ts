import Konva from 'konva';

// Konva finds the shape under the pointer by drawing every shape in its own
// colour on a hidden canvas and reading back the pixel there. Safari 26 adds
// noise to whatever a page reads back from a canvas (Advanced Fingerprinting
// Protection, on by default on iOS 26), so the colour no longer names a
// shape: a press on an object found nothing, or a handle somewhere else, and
// objects could neither be dragged nor resized on an iPhone. This finds the
// shape by geometry instead, never reading a pixel.

// Makes the layer find shapes by their boxes. Each shape counts by its own
// box (a text's, an image's, the frame of an outline) turned and scaled with
// it and widened by its hit stroke, inside the clips of the groups it is in;
// circles and ellipses by their round outline.
export function geometricHitTest(layer: Konva.Layer | null) {
  if (!layer) return;
  layer.getIntersection = (pos: { x: number; y: number }) => shapeAt(layer, pos);
}

export function shapeAt(container: Konva.Container, pos: { x: number; y: number }): Konva.Shape | null {
  const children = container.getChildren();
  for (let i = children.length - 1; i >= 0; i--) {
    const node = children[i];
    if (!node.visible() || !node.listening()) continue;
    if (node instanceof Konva.Container) {
      if (!insideClip(node, pos)) continue;
      const hit = shapeAt(node, pos);
      if (hit) return hit;
    } else if (node instanceof Konva.Shape && hits(node, pos)) {
      return node;
    }
  }
  return null;
}

function insideClip(node: Konva.Container, pos: { x: number; y: number }): boolean {
  const w = node.clipWidth();
  const h = node.clipHeight();
  if (!w || !h) return true;
  const p = node.getAbsoluteTransform().copy().invert().point(pos);
  const x = node.clipX() ?? 0;
  const y = node.clipY() ?? 0;
  return p.x >= x && p.x <= x + w && p.y >= y && p.y <= y + h;
}

function hits(shape: Konva.Shape, pos: { x: number; y: number }): boolean {
  // A transformer's back covers the whole box but takes no hits unless told
  // to; its anchors and turn handle are ordinary shapes.
  const parent = shape.getParent();
  if (shape.name() === 'back' && parent instanceof Konva.Transformer && !parent.shouldOverdrawWholeArea()) return false;
  const p = shape.getAbsoluteTransform().copy().invert().point(pos);
  const r = shape.getSelfRect();
  let pad = 0;
  if (shape.hasStroke() && shape.strokeHitEnabled()) {
    const hw = shape.hitStrokeWidth();
    pad = (hw === 'auto' ? shape.strokeWidth() : hw) / 2;
  }
  if (shape instanceof Konva.Circle || shape instanceof Konva.Ellipse) {
    const rx = r.width / 2 + pad;
    const ry = r.height / 2 + pad;
    if (rx <= 0 || ry <= 0) return false;
    const dx = p.x - (r.x + r.width / 2);
    const dy = p.y - (r.y + r.height / 2);
    return (dx * dx) / (rx * rx) + (dy * dy) / (ry * ry) <= 1;
  }
  return p.x >= r.x - pad && p.x <= r.x + r.width + pad && p.y >= r.y - pad && p.y <= r.y + r.height + pad;
}
