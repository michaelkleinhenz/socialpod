import { useState, useRef, useEffect, useLayoutEffect, useCallback, type ReactNode } from 'react';
import { Crop, X } from 'lucide-react';

export interface CropRect {
  x: number;
  y: number;
  size: number;
}

interface Props {
  imageUrl: string;
  onApply: (croppedBlob: Blob, cropSize: number) => void;
  onCancel: () => void;
  watermarkImg?: HTMLImageElement | null;
  watermarkName?: string;
  maxBytes?: number;
  children?: ReactNode;
}

const DEFAULT_MAX_BYTES = 1_000_000;

export function ImageCropper({ imageUrl, onApply, onCancel, watermarkImg, watermarkName, maxBytes = DEFAULT_MAX_BYTES, children }: Props) {
  const [cropRect, setCropRect] = useState<CropRect | null>(null);
  const [imgNaturalSize, setImgNaturalSize] = useState<{ w: number; h: number } | null>(null);
  const [dragging, setDragging] = useState<{ startX: number; startY: number; origRect: CropRect } | null>(null);
  const [resizing, setResizing] = useState<{ corner: string; startX: number; startY: number; origRect: CropRect } | null>(null);
  const [displayMetrics, setDisplayMetrics] = useState({ scale: 1, offsetX: 0, offsetY: 0 });
  const cropImgRef = useRef<HTMLImageElement>(null);
  const cropContainerRef = useRef<HTMLDivElement>(null);

  const getDisplayScale = () => {
    if (!cropImgRef.current || !imgNaturalSize) return 1;
    return cropImgRef.current.clientWidth / imgNaturalSize.w;
  };

  const updateDisplayMetrics = useCallback(() => {
    const img = cropImgRef.current;
    const container = cropContainerRef.current;
    if (!img || !container || !imgNaturalSize) return;
    const ir = img.getBoundingClientRect();
    const cr = container.getBoundingClientRect();
    setDisplayMetrics({
      scale: img.clientWidth / imgNaturalSize.w,
      offsetX: ir.left - cr.left,
      offsetY: ir.top - cr.top,
    });
  }, [imgNaturalSize]);

  useLayoutEffect(() => {
    updateDisplayMetrics();
    window.addEventListener('resize', updateDisplayMetrics);
    return () => window.removeEventListener('resize', updateDisplayMetrics);
  }, [updateDisplayMetrics]);

  const onCropImageLoad = () => {
    const img = cropImgRef.current;
    if (!img) return;
    const w = img.naturalWidth;
    const h = img.naturalHeight;
    setImgNaturalSize({ w, h });
    const minDim = Math.min(w, h);
    setCropRect({
      x: Math.floor((w - minDim) / 2),
      y: Math.floor((h - minDim) / 2),
      size: minDim,
    });
  };

  const clampRect = (rect: CropRect, natW: number, natH: number): CropRect => {
    let { x, y, size } = rect;
    size = Math.round(Math.max(50, Math.min(size, natW, natH)));
    x = Math.round(Math.max(0, Math.min(x, natW - size)));
    y = Math.round(Math.max(0, Math.min(y, natH - size)));
    return { x, y, size };
  };

  const handleCropStart = (clientX: number, clientY: number) => {
    if (!cropRect || !imgNaturalSize || !cropImgRef.current) return;
    const scale = getDisplayScale();
    const imgBounds = cropImgRef.current.getBoundingClientRect();
    const mx = (clientX - imgBounds.left) / scale;
    const my = (clientY - imgBounds.top) / scale;

    const { x, y, size } = cropRect;
    const handleHit = 28 / scale;

    const corners = [
      { corner: 'tl', cx: x, cy: y },
      { corner: 'tr', cx: x + size, cy: y },
      { corner: 'bl', cx: x, cy: y + size },
      { corner: 'br', cx: x + size, cy: y + size },
    ];
    for (const c of corners) {
      if (Math.abs(mx - c.cx) < handleHit && Math.abs(my - c.cy) < handleHit) {
        setResizing({ corner: c.corner, startX: clientX, startY: clientY, origRect: { ...cropRect } });
        return;
      }
    }

    if (mx >= x && mx <= x + size && my >= y && my <= y + size) {
      setDragging({ startX: clientX, startY: clientY, origRect: { ...cropRect } });
    }
  };

  const onCropMouseDown = (e: React.MouseEvent) => {
    e.preventDefault();
    handleCropStart(e.clientX, e.clientY);
  };

  const onCropTouchStart = (e: React.TouchEvent) => {
    e.preventDefault();
    if (e.touches.length > 0) {
      handleCropStart(e.touches[0].clientX, e.touches[0].clientY);
    }
  };

  const onCropMouseMove = useCallback((clientX: number, clientY: number) => {
    if (!imgNaturalSize) return;
    const scale = getDisplayScale();

    if (dragging) {
      const dx = (clientX - dragging.startX) / scale;
      const dy = (clientY - dragging.startY) / scale;
      setCropRect(clampRect({
        x: dragging.origRect.x + dx,
        y: dragging.origRect.y + dy,
        size: dragging.origRect.size,
      }, imgNaturalSize.w, imgNaturalSize.h));
    }

    if (resizing) {
      const dx = (clientX - resizing.startX) / scale;
      const dy = (clientY - resizing.startY) / scale;
      const { corner, origRect } = resizing;
      let newSize = origRect.size;
      let newX = origRect.x;
      let newY = origRect.y;

      if (corner === 'br') {
        newSize = origRect.size + Math.max(dx, dy);
      } else if (corner === 'tl') {
        const delta = Math.min(dx, dy);
        newSize = origRect.size - delta;
        newX = origRect.x + delta;
        newY = origRect.y + delta;
      } else if (corner === 'tr') {
        const delta = Math.max(dx, -dy);
        newSize = origRect.size + delta;
        newY = origRect.y - delta;
      } else if (corner === 'bl') {
        const delta = Math.max(-dx, dy);
        newSize = origRect.size + delta;
        newX = origRect.x - delta;
      }

      setCropRect(clampRect({ x: newX, y: newY, size: newSize }, imgNaturalSize.w, imgNaturalSize.h));
    }
  }, [dragging, resizing, imgNaturalSize]);

  const onCropMouseUp = useCallback(() => {
    setDragging(null);
    setResizing(null);
  }, []);

  useEffect(() => {
    if (dragging || resizing) {
      const handleMouseMove = (e: MouseEvent) => onCropMouseMove(e.clientX, e.clientY);
      const handleTouchMove = (e: TouchEvent) => {
        e.preventDefault();
        if (e.touches.length > 0) onCropMouseMove(e.touches[0].clientX, e.touches[0].clientY);
      };
      window.addEventListener('mousemove', handleMouseMove);
      window.addEventListener('mouseup', onCropMouseUp);
      window.addEventListener('touchmove', handleTouchMove, { passive: false });
      window.addEventListener('touchend', onCropMouseUp);
      return () => {
        window.removeEventListener('mousemove', handleMouseMove);
        window.removeEventListener('mouseup', onCropMouseUp);
        window.removeEventListener('touchmove', handleTouchMove);
        window.removeEventListener('touchend', onCropMouseUp);
      };
    }
  }, [dragging, resizing, onCropMouseMove, onCropMouseUp]);

  const renderCropToBlob = async (outputSize: number): Promise<Blob> => {
    const canvas = document.createElement('canvas');
    canvas.width = outputSize;
    canvas.height = outputSize;
    const ctx = canvas.getContext('2d')!;

    const img = new window.Image();
    img.crossOrigin = 'anonymous';
    await new Promise<void>((resolve, reject) => {
      img.onload = () => resolve();
      img.onerror = reject;
      img.src = imageUrl;
    });

    ctx.drawImage(img, cropRect!.x, cropRect!.y, cropRect!.size, cropRect!.size, 0, 0, outputSize, outputSize);

    if (watermarkImg) {
      ctx.drawImage(watermarkImg, 0, 0, outputSize, outputSize);
    }

    return new Promise<Blob>((resolve, reject) => {
      canvas.toBlob(b => b ? resolve(b) : reject(new Error('Failed to create blob')), 'image/jpeg', 0.92);
    });
  };

  const applyCrop = async () => {
    if (!cropRect || !imgNaturalSize) return;

    let outputSize = cropRect.size;
    let blob = await renderCropToBlob(outputSize);

    while (blob.size > maxBytes && outputSize > 100) {
      const scale = Math.sqrt(maxBytes / blob.size) * 0.95;
      outputSize = Math.max(100, Math.floor(outputSize * scale));
      blob = await renderCropToBlob(outputSize);
    }

    onApply(blob, outputSize);
  };

  const renderCropOverlay = () => {
    if (!cropRect || !imgNaturalSize) return null;
    const { scale, offsetX, offsetY } = displayMetrics;

    const x = cropRect.x * scale + offsetX;
    const y = cropRect.y * scale + offsetY;
    const size = cropRect.size * scale;
    const handleSz = 14;

    return (
      <div
        style={{ position: 'absolute', top: 0, left: 0, right: 0, bottom: 0, cursor: 'default', touchAction: 'none' }}
        onMouseDown={onCropMouseDown}
        onTouchStart={onCropTouchStart}
      >
        <div style={{ position: 'absolute', top: 0, left: 0, right: 0, height: y, background: 'rgba(0,0,0,0.5)' }} />
        <div style={{ position: 'absolute', top: y, left: 0, width: x, height: size, background: 'rgba(0,0,0,0.5)' }} />
        <div style={{ position: 'absolute', top: y, left: x + size, right: 0, height: size, background: 'rgba(0,0,0,0.5)' }} />
        <div style={{ position: 'absolute', top: y + size, left: 0, right: 0, bottom: 0, background: 'rgba(0,0,0,0.5)' }} />

        <div style={{
          position: 'absolute', left: x, top: y, width: size, height: size,
          border: '2px solid #fff',
          boxShadow: '0 0 0 1px rgba(0,0,0,0.3)',
          cursor: 'move',
        }}>
          <div style={{ position: 'absolute', left: '33.33%', top: 0, bottom: 0, width: 1, background: 'rgba(255,255,255,0.3)' }} />
          <div style={{ position: 'absolute', left: '66.66%', top: 0, bottom: 0, width: 1, background: 'rgba(255,255,255,0.3)' }} />
          <div style={{ position: 'absolute', top: '33.33%', left: 0, right: 0, height: 1, background: 'rgba(255,255,255,0.3)' }} />
          <div style={{ position: 'absolute', top: '66.66%', left: 0, right: 0, height: 1, background: 'rgba(255,255,255,0.3)' }} />
        </div>

        {[
          { corner: 'tl', left: x - handleSz / 2, top: y - handleSz / 2, cursor: 'nw-resize' },
          { corner: 'tr', left: x + size - handleSz / 2, top: y - handleSz / 2, cursor: 'ne-resize' },
          { corner: 'bl', left: x - handleSz / 2, top: y + size - handleSz / 2, cursor: 'sw-resize' },
          { corner: 'br', left: x + size - handleSz / 2, top: y + size - handleSz / 2, cursor: 'se-resize' },
        ].map(h => (
          <div
            key={h.corner}
            style={{
              position: 'absolute', left: h.left, top: h.top,
              width: handleSz, height: handleSz,
              background: '#fff', border: '1px solid rgba(0,0,0,0.3)',
              borderRadius: 2, cursor: h.cursor,
            }}
          />
        ))}

        {watermarkImg && (
          <img
            src={watermarkImg.src}
            alt="watermark"
            style={{
              position: 'absolute', left: x, top: y,
              width: size, height: size,
              pointerEvents: 'none',
            }}
          />
        )}
      </div>
    );
  };

  return (
    <div
      className="image-crop-overlay"
      onClick={onCancel}
    >
      <div
        className="card image-crop-card"
        onClick={e => e.stopPropagation()}
      >
        <div style={{ display: 'flex', justifyContent: 'space-between', alignItems: 'center', marginBottom: 16 }}>
          <h3 style={{ margin: 0, fontSize: 16, display: 'flex', alignItems: 'center', gap: 8 }}>
            <Crop size={18} /> Crop Image
            {cropRect && <span style={{ fontSize: 12, color: 'var(--text-muted)', fontWeight: 400 }}>
              ({cropRect.size} × {cropRect.size} px)
            </span>}
            {watermarkName && <span style={{ fontSize: 12, color: 'var(--text-muted)', fontWeight: 400 }}>
              · watermark: {watermarkName}
            </span>}
          </h3>
          <button className="btn btn-ghost btn-sm" onClick={onCancel}>
            <X size={18} />
          </button>
        </div>
        {children}
        <div
          ref={cropContainerRef}
          style={{
            position: 'relative',
            flex: 1,
            minHeight: 0,
            overflow: 'hidden',
            display: 'flex',
            alignItems: 'center',
            justifyContent: 'center',
            background: 'var(--bg-secondary, #0f172a)',
            borderRadius: 8,
            userSelect: 'none',
            touchAction: 'none',
          }}
        >
          <img
            ref={cropImgRef}
            src={imageUrl}
            alt="crop source"
            onLoad={onCropImageLoad}
            draggable={false}
            style={{
              maxWidth: '100%',
              maxHeight: 'calc(90vh - 140px)',
              display: 'block',
            }}
          />
          {renderCropOverlay()}
        </div>
        <div style={{ display: 'flex', justifyContent: 'flex-end', gap: 8, marginTop: 16, flexWrap: 'wrap' }}>
          <button className="btn btn-secondary" onClick={onCancel}>
            Cancel
          </button>
          <button className="btn btn-primary" onClick={applyCrop} disabled={!cropRect}>
            <Crop size={14} /> Apply Crop{watermarkName ? ' & Watermark' : ''}
          </button>
        </div>
      </div>
    </div>
  );
}
