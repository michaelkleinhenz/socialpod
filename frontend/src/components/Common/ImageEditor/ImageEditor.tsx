import { useCallback, useEffect, useLayoutEffect, useMemo, useRef, useState, type ChangeEvent } from 'react';
import Konva from 'konva';
import { Stage, Layer, Group, Image as KImage, Text as KText, Rect, Line, Label, Tag, Transformer } from 'react-konva';
import {
  AlignCenter, AlignLeft, AlignRight, Bold, Crop, Crosshair, FolderOpen, Frame, ImageUp, LayoutTemplate, Layers as LayersIcon, Loader2, Magnet, Plus, Redo2, Search, Sparkles, Stamp,
  Trash2, Type, Undo2, Upload, X, Maximize,
} from 'lucide-react';
import toast from 'react-hot-toast';
import { api } from '../../../services/api';
import type { MediaItem, Watermark } from '../../../types';
import {
  applyAdjustments, BORDER_STYLES, CANVAS_FORMATS, centredCrop, DEFAULT_CANVAS, clamp, FILTER_PRESETS, fitLayers, hasAdjustments, imageLayerParts,
  instantiateTemplate, loadImage, measureText, newLayerId, NO_ADJUSTMENTS, parseTemplate, renderEdit, resizeText, textLayerAttrs,
  toTemplate,
  type Adjustments, type BorderStyle, type CropBox, type EditorCanvas, type EditorLayer, type EditorTemplate, type ImageLayer, type TextAlign,
  type TextLayer,
} from './editorModel';
import { snapBox, type SnapResult } from './snapping';
import { LayerList } from './LayerList';
import './ImageEditor.css';

type Tab = 'crop' | 'canvas' | 'filters' | 'templates' | 'watermark' | 'media' | 'text';

interface FontOption {
  label: string;
  family: string;
}

const BUILTIN_FONTS: FontOption[] = [
  { label: 'Arial', family: 'Arial, Helvetica, sans-serif' },
  { label: 'Helvetica', family: 'Helvetica, Arial, sans-serif' },
  { label: 'Verdana', family: 'Verdana, sans-serif' },
  { label: 'Trebuchet MS', family: 'Trebuchet MS, sans-serif' },
  { label: 'Impact', family: 'Impact, Haettenschweiler, Arial Black, sans-serif' },
  { label: 'Georgia', family: 'Georgia, serif' },
  { label: 'Times New Roman', family: 'Times New Roman, Times, serif' },
  { label: 'Rockwell', family: 'Rockwell, Rockwell Nova, Roboto Slab, DejaVu Serif, Sitka Small, serif' },
  { label: 'Courier New', family: 'Courier New, monospace' },
  { label: 'Comic Sans MS', family: 'Comic Sans MS, cursive' },
];

const ASPECTS: { id: string; label: string; ratio: number | null | 'original' }[] = [
  { id: 'free', label: 'Free', ratio: null },
  { id: 'original', label: 'Original', ratio: 'original' },
  { id: '1:1', label: '1:1', ratio: 1 },
  { id: '4:5', label: '4:5', ratio: 4 / 5 },
  { id: '9:16', label: '9:16', ratio: 9 / 16 },
  { id: '16:9', label: '16:9', ratio: 16 / 9 },
];

const TEXT_ALIGNS: { id: TextAlign; label: string; icon: typeof AlignLeft }[] = [
  { id: 'left', label: 'Align left', icon: AlignLeft },
  { id: 'center', label: 'Align centre', icon: AlignCenter },
  { id: 'right', label: 'Align right', icon: AlignRight },
];

// The font sizes the size field takes, in pixels of the picture.
const MIN_FONT_SIZE = 4;
const MAX_FONT_SIZE = 2000;

const SWATCHES = ['#ffffff', '#000000', '#facc15', '#f97316', '#ef4444', '#ec4899', '#8b5cf6', '#3b82f6', '#22c55e'];

// Undo steps kept; older ones are dropped.
const HISTORY_LIMIT = 50;
// Changes to the same control closer together than this are one undo step,
// so a slider drag or a typed word does not take dozens of undos.
const COALESCE_MS = 1000;

// The part of the editor's state undo covers: the edit itself, not the view
// (tab, selection) or what is loaded (image, fonts).
interface Snapshot {
  canvas: EditorCanvas | null;
  crop: CropBox;
  aspectId: string;
  adjustments: Adjustments;
  layers: EditorLayer[];
}

// How close, in screen pixels, a moved layer has to come to snap.
const SNAP_PX = 6;
const GUIDE_COLOR = '#ff2d95';
const SNAP_STORAGE_KEY = 'socialpod.imageEditor.snap';

// Around the image on screen, so crop and transform handles stay reachable.
const STAGE_PADDING = 20;

interface Props {
  // The photo to edit. Without one the editor draws on a blank canvas: the
  // Crop and Filters tabs give way to a Canvas tab (format, background).
  imageUrl?: string;
  // The blank canvas to start with, when there is no photo and no template.
  canvas?: EditorCanvas;
  // A design to start from (blank canvas only): its canvas and layers.
  template?: EditorTemplate;
  // Header title; "Edit image" by default.
  title?: string;
  // The team's watermarks, offered on the Watermark tab.
  watermarks?: Watermark[];
  // An overlay drawn over the crop area for reference but left out of the
  // output, for images whose overlay is composited later (convention queues
  // stamp theirs when the item is posted).
  guideOverlay?: HTMLImageElement | null;
  // The rendered JPEG, and the design as a template (the crop as its canvas).
  onSave: (blob: Blob, design: EditorTemplate) => void | Promise<void>;
  onCancel: () => void;
}

const apiBase = import.meta.env.VITE_API_URL || '';
const resolveUrl = (url: string) => (url.startsWith('/') ? apiBase + url : url);

// The library also holds videos and fonts, which cannot go on a picture.
const isImageMedia = (m: MediaItem) =>
  !m.contentType?.startsWith('video/') && !m.contentType?.startsWith('font/') && !/\.(mp4|mov|ttf|otf)$/i.test(m.url);

export function ImageEditor({ imageUrl, canvas: initialCanvas, template, title, watermarks = [], guideOverlay, onSave, onCancel }: Props) {
  const canvasMode = !imageUrl;
  const [source, setSource] = useState<HTMLImageElement | null>(null);
  const [loadError, setLoadError] = useState(false);
  // The blank canvas, in canvas mode; null on a photo.
  const [canvas, setCanvas] = useState<EditorCanvas | null>(() => {
    if (!canvasMode) return null;
    if (template) return { width: template.width, height: template.height, background: template.background };
    return initialCanvas ?? DEFAULT_CANVAS;
  });
  // A blank canvas without a design yet opens on the templates to start from.
  const [tab, setTab] = useState<Tab>(canvasMode ? (template ? 'canvas' : 'templates') : 'crop');
  const [crop, setCrop] = useState<CropBox | null>(() => (canvas ? { x: 0, y: 0, width: canvas.width, height: canvas.height } : null));
  const [aspectId, setAspectId] = useState('free');
  const [adjustments, setAdjustments] = useState<Adjustments>(NO_ADJUSTMENTS);
  const [layers, setLayers] = useState<EditorLayer[]>([]);
  const [selectedId, setSelectedId] = useState<string | null>(null);
  const [fonts, setFonts] = useState<FontOption[]>(BUILTIN_FONTS);
  // Colour entries of the media library, offered before the built-in swatches.
  const [libraryColors, setLibraryColors] = useState<MediaItem[]>([]);
  const [addingWatermark, setAddingWatermark] = useState<string | null>(null);
  // The team's media library, loaded when the Media tab is first opened.
  const [media, setMedia] = useState<MediaItem[] | null>(null);
  const [mediaQuery, setMediaQuery] = useState('');
  const [mediaLoading, setMediaLoading] = useState(false);
  const [addingMedia, setAddingMedia] = useState<string | null>(null);
  const [uploadingMedia, setUploadingMedia] = useState(false);
  // A pasted or picked image on its way onto the picture.
  const [placingImage, setPlacingImage] = useState(false);
  // The font size field while it is being typed in, which may not be a valid
  // size yet (cleared, or half a number).
  // Kept with the text it belongs to, so selecting another text drops it.
  const [fontSizeDraft, setFontSizeDraft] = useState<{ id: string; value: string } | null>(null);
  // The media library's templates, loaded when the Templates tab is opened.
  const [templates, setTemplates] = useState<MediaItem[] | null>(null);
  const [applyingTemplate, setApplyingTemplate] = useState<string | null>(null);
  const [saving, setSaving] = useState(false);
  // Smart guides while a layer is moved; Alt (Option) skips them for one drag.
  const [snapOn, setSnapOn] = useState(() => {
    try { return localStorage.getItem(SNAP_STORAGE_KEY) !== 'off'; } catch { return true; }
  });
  const [guides, setGuides] = useState<SnapResult | null>(null);
  // The object list on the right; open from the start where there is room.
  const [showLayers, setShowLayers] = useState(() => typeof window === 'undefined' || !window.matchMedia('(max-width: 768px)').matches);
  const [size, setSize] = useState({ w: 0, h: 0 });
  const [undoStack, setUndoStack] = useState<Snapshot[]>([]);
  const [redoStack, setRedoStack] = useState<Snapshot[]>([]);
  // The control the last undo step was opened for, and when it last changed.
  const lastChange = useRef<{ key: string; at: number } | null>(null);

  const canvasBoxRef = useRef<HTMLDivElement>(null);
  const baseRef = useRef<Konva.Image>(null);
  const backgroundRef = useRef<Konva.Rect>(null);
  // Settles once the media library's fonts are loaded, so a template's texts
  // are measured in their own font.
  const fontsReady = useRef<Promise<void>>(Promise.resolve());
  const cropRectRef = useRef<Konva.Rect>(null);
  const trRef = useRef<Konva.Transformer>(null);
  const contentRef = useRef<Konva.Group>(null);
  const nodeRefs = useRef(new Map<string, Konva.Node>());
  const mediaInputRef = useRef<HTMLInputElement>(null);
  const placeInputRef = useRef<HTMLInputElement>(null);
  const textAreaRef = useRef<HTMLTextAreaElement>(null);

  const isTouch = useMemo(() => typeof window !== 'undefined' && window.matchMedia('(pointer: coarse)').matches, []);

  // Source image.
  useEffect(() => {
    if (!imageUrl) return;
    let cancelled = false;
    setLoadError(false);
    loadImage(imageUrl)
      .then(img => {
        if (cancelled) return;
        setSource(img);
        setCrop({ x: 0, y: 0, width: img.naturalWidth, height: img.naturalHeight });
      })
      .catch(() => { if (!cancelled) setLoadError(true); });
    return () => { cancelled = true; };
  }, [imageUrl]);

  // The media library's fonts are offered first, then the system fonts.
  // A font file the browser cannot load is left out.
  useEffect(() => {
    let cancelled = false;
    fontsReady.current = api.getMedia({ kind: 'font', limit: 200 }).then(async items => {
      const loaded = await Promise.all(items.filter(m => m.url).map(async m => {
        const family = `SocialPod Media ${m.id}`;
        try {
          const face = await new FontFace(family, `url(${resolveUrl(m.url)})`).load();
          document.fonts.add(face);
          return { label: m.name, family };
        } catch {
          return null;
        }
      }));
      if (cancelled) return;
      const library = loaded.filter((f): f is FontOption => f !== null);
      setFonts([...library, ...BUILTIN_FONTS]);
    }).catch(() => {});
    return () => { cancelled = true; };
  }, []);

  // The design to start from. Loading it is not an undo step.
  useEffect(() => {
    if (template && canvasMode) void applyTemplate(template, { record: false });
    // Once, on open.
    // eslint-disable-next-line react-hooks/exhaustive-deps
  }, []);

  // The media library's colours.
  useEffect(() => {
    let cancelled = false;
    api.getMedia({ kind: 'color', limit: 200 })
      .then(items => { if (!cancelled) setLibraryColors(items.filter(m => m.color)); })
      .catch(() => {});
    return () => { cancelled = true; };
  }, []);

  // Templates: fetched once the Templates tab has been opened.
  const templatesOpened = tab === 'templates' || templates !== null;
  useEffect(() => {
    if (!templatesOpened || templates !== null) return;
    let cancelled = false;
    api.getMedia({ kind: 'template', limit: 200 })
      .then(items => {
        if (cancelled) return;
        setTemplates(items);
        // With nothing to start from, a new canvas opens on its own settings.
        if (items.length === 0 && canvasMode) setTab(t => (t === 'templates' ? 'canvas' : t));
      })
      .catch(() => { if (!cancelled) { setTemplates([]); toast.error('Failed to load the templates'); } });
    return () => { cancelled = true; };
  }, [templatesOpened, templates, canvasMode]);

  // Media library: fetched once the Media tab has been opened, then again as
  // the search changes.
  const mediaOpened = tab === 'media' || media !== null;
  useEffect(() => {
    if (!mediaOpened) return;
    let cancelled = false;
    const t = setTimeout(() => {
      setMediaLoading(true);
      api.getMedia({ q: mediaQuery.trim(), kind: 'image', limit: 100 })
        .then(items => { if (!cancelled) setMedia(items.filter(isImageMedia)); })
        .catch(() => { if (!cancelled) { setMedia(m => m ?? []); toast.error('Failed to load the media library'); } })
        .finally(() => { if (!cancelled) setMediaLoading(false); });
    }, 250);
    return () => { cancelled = true; clearTimeout(t); };
  }, [mediaOpened, mediaQuery]);

  // Stage size follows the canvas area.
  useLayoutEffect(() => {
    const el = canvasBoxRef.current;
    if (!el) return;
    const update = () => setSize({ w: el.clientWidth, h: el.clientHeight });
    update();
    const ro = new ResizeObserver(update);
    ro.observe(el);
    return () => ro.disconnect();
  }, []);

  const natW = canvas?.width ?? source?.naturalWidth ?? 1;
  const natH = canvas?.height ?? source?.naturalHeight ?? 1;
  // What the stage draws on: the photo once loaded, or the blank canvas.
  const ready = canvasMode || !!source;

  // What the stage shows: the whole image while cropping, the crop otherwise.
  const view: CropBox = tab === 'crop' || !crop ? { x: 0, y: 0, width: natW, height: natH } : crop;
  const scale = size.w && size.h
    ? Math.max(0.01, Math.min((size.w - STAGE_PADDING * 2) / view.width, (size.h - STAGE_PADDING * 2) / view.height))
    : 1;
  const gx = (size.w - view.width * scale) / 2 - view.x * scale;
  const gy = (size.h - view.height * scale) / 2 - view.y * scale;

  // Filters run on a reduced copy on screen: enough for the display, cheap
  // enough for a slider on a phone.
  const previewRatio = Math.min(1, 1600 / Math.max(natW, natH));
  useEffect(() => {
    const node = baseRef.current;
    if (!node || !source) return;
    const id = requestAnimationFrame(() => {
      applyAdjustments(node, adjustments, previewRatio);
      node.getLayer()?.batchDraw();
    });
    return () => cancelAnimationFrame(id);
  }, [adjustments, source, previewRatio]);

  const selected = layers.find(l => l.id === selectedId) ?? null;

  // Attach the transformer to the crop box or the selected layer.
  useEffect(() => {
    const tr = trRef.current;
    if (!tr) return;
    const node = tab === 'crop'
      ? cropRectRef.current
      : (selectedId ? nodeRefs.current.get(selectedId) ?? null : null);
    tr.nodes(node ? [node] : []);
    tr.getLayer()?.batchDraw();
  }, [tab, selectedId, layers, source, aspectId]);

  // --- Undo -----------------------------------------------------------------

  // Records the current state as an undo step. Called right before a change,
  // while the state still holds what the change replaces. With a key, further
  // changes to the same control in quick succession join that step.
  const checkpoint = (key?: string) => {
    if (!crop) return;
    const now = Date.now();
    const last = lastChange.current;
    lastChange.current = key ? { key, at: now } : null;
    // A gesture key joins its step for as long as the drag lasts; startGesture
    // ends it.
    const sticky = key?.startsWith('gesture:');
    if (key && last?.key === key && (sticky || now - last.at < COALESCE_MS)) return;
    const snap: Snapshot = { canvas, crop, aspectId, adjustments, layers };
    setUndoStack(stack => [...stack.slice(-(HISTORY_LIMIT - 1)), snap]);
    setRedoStack([]);
  };

  // A new drag or resize always starts its own step, however soon after the
  // last one.
  const startGesture = () => { lastChange.current = null; };

  const restore = (snap: Snapshot) => {
    setCanvas(snap.canvas);
    setCrop(snap.crop);
    setAspectId(snap.aspectId);
    setAdjustments(snap.adjustments);
    setLayers(snap.layers);
    if (selectedId && !snap.layers.some(l => l.id === selectedId)) setSelectedId(null);
    lastChange.current = null;
  };

  const undo = () => {
    const prev = undoStack[undoStack.length - 1];
    if (!prev || !crop) return;
    setUndoStack(stack => stack.slice(0, -1));
    setRedoStack(stack => [...stack, { canvas, crop, aspectId, adjustments, layers }]);
    restore(prev);
  };

  const redo = () => {
    const next = redoStack[redoStack.length - 1];
    if (!next || !crop) return;
    setRedoStack(stack => stack.slice(0, -1));
    setUndoStack(stack => [...stack, { canvas, crop, aspectId, adjustments, layers }]);
    restore(next);
  };

  // Every change to the edit goes through these, which record the undo step.
  const updateLayer = (id: string, patch: Partial<ImageLayer> | Partial<TextLayer>, key?: string) => {
    checkpoint(key);
    setLayers(prev => prev.map(l => (l.id === id ? { ...l, ...patch } as EditorLayer : l)));
  };

  const addLayer = (layer: EditorLayer) => {
    checkpoint();
    setLayers(prev => [...prev, layer]);
    setSelectedId(layer.id);
  };

  const changeAdjustments = (patch: Partial<Adjustments>, key?: string) => {
    checkpoint(key);
    setAdjustments(a => ({ ...a, ...patch }));
  };

  const changeCrop = (box: CropBox, nextAspectId = aspectId, key?: string) => {
    checkpoint(key);
    setCrop(box);
    setAspectId(nextAspectId);
  };

  // Restacks the objects: later in the list is drawn on top.
  const reorderLayers = (next: EditorLayer[]) => {
    checkpoint();
    setLayers(next);
  };

  const deleteSelected = () => {
    if (!selectedId) return;
    checkpoint();
    setLayers(prev => prev.filter(l => l.id !== selectedId));
    setSelectedId(null);
  };

  const dirty = canvasMode
    ? undoStack.length > 0
    : layers.length > 0 || hasAdjustments(adjustments) || (!!crop && !!source
      && (crop.x !== 0 || crop.y !== 0 || crop.width !== natW || crop.height !== natH));

  const handleCancel = useCallback(() => {
    if (saving) return;
    if (dirty && !window.confirm('Discard your changes to this image?')) return;
    onCancel();
  }, [dirty, saving, onCancel]);

  // Keyboard: Escape leaves, Delete removes the selected layer, Ctrl/Cmd+Z
  // and Ctrl/Cmd+Shift+Z (or Ctrl+Y) undo and redo — unless typing, where
  // those keys belong to the field.
  useEffect(() => {
    const onKey = (e: KeyboardEvent) => {
      const t = e.target as HTMLElement | null;
      const typing = !!t && (t.tagName === 'INPUT' || t.tagName === 'TEXTAREA' || t.tagName === 'SELECT' || t.isContentEditable);
      const mod = e.ctrlKey || e.metaKey;
      const k = e.key.toLowerCase();
      if (mod && !typing && (k === 'z' || k === 'y')) {
        e.preventDefault();
        if (k === 'y' || e.shiftKey) redo();
        else undo();
        return;
      }
      if (e.key === 'Escape') {
        if (selectedId) setSelectedId(null);
        else handleCancel();
      } else if ((e.key === 'Delete' || e.key === 'Backspace') && !typing && selectedId) {
        e.preventDefault();
        deleteSelected();
      }
    };
    window.addEventListener('keydown', onKey);
    return () => window.removeEventListener('keydown', onKey);
  });

  const switchTab = (t: Tab) => {
    setTab(t);
    if (t === 'crop') setSelectedId(null);
    else if (t === 'text' && selected?.kind !== 'text') setSelectedId(null);
    else if ((t === 'watermark' || t === 'media') && selected?.kind !== 'image') setSelectedId(null);
  };

  const selectLayer = (l: EditorLayer) => {
    setSelectedId(l.id);
    // An image layer keeps the tab it is on when that tab handles images too.
    if (l.kind === 'text') setTab('text');
    else if (tab !== 'watermark' && tab !== 'media') setTab(l.origin);
  };

  // --- Crop -----------------------------------------------------------------

  const aspectRatio = (id: string): number | null => {
    const a = ASPECTS.find(x => x.id === id);
    if (!a || a.ratio === null) return null;
    return a.ratio === 'original' ? natW / natH : a.ratio;
  };

  const pickAspect = (id: string) => {
    if (!crop) return;
    const ratio = aspectRatio(id);
    changeCrop(ratio ? centredCrop(natW, natH, ratio) : crop, id);
  };

  const resetCrop = () => changeCrop({ x: 0, y: 0, width: natW, height: natH }, 'free');

  // The crop box lives in image pixels; Konva reports drags and resizes as
  // position and scale, which are folded back into a plain box.
  const syncCropFromNode = () => {
    const node = cropRectRef.current;
    if (!node) return;
    const width = node.width() * node.scaleX();
    const height = node.height() * node.scaleY();
    node.setAttrs({ width, height, scaleX: 1, scaleY: 1 });
    changeCrop({ x: node.x(), y: node.y(), width, height }, aspectId, 'gesture:crop');
  };

  const cropDragBound = (pos: Konva.Vector2d): Konva.Vector2d => {
    const node = cropRectRef.current;
    if (!node) return pos;
    const w = node.width() * node.scaleX() * scale;
    const h = node.height() * node.scaleY() * scale;
    return {
      x: clamp(pos.x, gx, gx + natW * scale - w),
      y: clamp(pos.y, gy, gy + natH * scale - h),
    };
  };

  // Transformer boxes are in stage pixels.
  const cropBoundBox = (oldBox: BoxLike, newBox: BoxLike): BoxLike => {
    const minSide = 40;
    const eps = 0.5;
    if (
      newBox.width < minSide || newBox.height < minSide
      || newBox.x < gx - eps || newBox.y < gy - eps
      || newBox.x + newBox.width > gx + natW * scale + eps
      || newBox.y + newBox.height > gy + natH * scale + eps
    ) return oldBox;
    return newBox;
  };

  // --- Canvas ---------------------------------------------------------------

  // A new format keeps the layers where they were relative to the canvas.
  const changeFormat = (width: number, height: number) => {
    if (!canvas || (canvas.width === width && canvas.height === height)) return;
    checkpoint();
    const to = { x: 0, y: 0, width, height };
    setLayers(prev => fitLayers(prev, { x: 0, y: 0, width: canvas.width, height: canvas.height }, to));
    setCanvas({ ...canvas, width, height });
    setCrop(to);
  };

  const changeBackground = (background: string, key?: string) => {
    if (!canvas) return;
    checkpoint(key);
    setCanvas({ ...canvas, background });
  };

  // --- Templates ------------------------------------------------------------

  // On a blank canvas a template replaces the design, canvas included; on a
  // photo its layers are laid over the crop.
  const applyTemplate = async (t: EditorTemplate, { record = true } = {}) => {
    if (!crop) return;
    await fontsReady.current;
    const target = canvasMode ? { x: 0, y: 0, width: t.width, height: t.height } : crop;
    const { layers: added, missing } = await instantiateTemplate(t, target, resolveUrl);
    if (record) checkpoint();
    if (canvasMode) {
      setCanvas({ width: t.width, height: t.height, background: t.background });
      setCrop(target);
      setLayers(added);
    } else {
      setLayers(prev => [...prev, ...added]);
    }
    setSelectedId(null);
    if (missing) toast.error(missing === 1 ? 'One image of the template could not be loaded' : `${missing} images of the template could not be loaded`);
  };

  const pickTemplate = async (item: MediaItem) => {
    const t = parseTemplate(item.template);
    if (!t) { toast.error('This template cannot be read'); return; }
    if (canvasMode && layers.length > 0 && !window.confirm('Replace your design with this template?')) return;
    setApplyingTemplate(item.id);
    try {
      await applyTemplate(t);
    } finally {
      setApplyingTemplate(null);
    }
  };

  // --- Watermarks -----------------------------------------------------------

  const addWatermark = async (wm: Watermark) => {
    if (!crop) return;
    setAddingWatermark(wm.id);
    try {
      const img = await loadImage(resolveUrl(wm.url));
      const wmRatio = img.naturalWidth / img.naturalHeight;
      const cropRatio = crop.width / crop.height;
      let width: number;
      let height: number;
      let cx: number;
      let cy: number;
      if (Math.abs(wmRatio - cropRatio) / cropRatio < 0.03) {
        // A frame made for this shape covers the whole picture.
        width = crop.width;
        height = crop.height;
        cx = crop.x + crop.width / 2;
        cy = crop.y + crop.height / 2;
      } else {
        // A logo goes small into the bottom-right corner.
        const side = Math.min(crop.width, crop.height) * 0.3;
        width = wmRatio >= 1 ? side : side * wmRatio;
        height = width / wmRatio;
        const margin = Math.min(crop.width, crop.height) * 0.04;
        cx = crop.x + crop.width - margin - width / 2;
        cy = crop.y + crop.height - margin - height / 2;
      }
      addLayer({ id: newLayerId(), kind: 'image', origin: 'watermark', src: wm.url, img, x: cx, y: cy, width, height, rotation: 0, opacity: 1 });
    } catch {
      toast.error('Failed to load that watermark');
    } finally {
      setAddingWatermark(null);
    }
  };

  // --- Media ----------------------------------------------------------------

  const addMedia = async (item: MediaItem) => {
    if (!crop) return;
    setAddingMedia(item.id);
    try {
      const img = await loadImage(resolveUrl(item.url));
      const r = img.naturalWidth / img.naturalHeight;
      const cropRatio = crop.width / crop.height;
      // A picture of the crop's own shape fills it, as a background or a
      // frame would; anything else goes in the middle at about half size,
      // ready to be placed.
      const share = Math.abs(r - cropRatio) / cropRatio < 0.03 ? 1 : 0.5;
      let width = crop.width * share;
      let height = width / r;
      if (height > crop.height * share) { height = crop.height * share; width = height * r; }
      addLayer({
        id: newLayerId(), kind: 'image', origin: 'media', src: item.url, img,
        x: crop.x + crop.width / 2, y: crop.y + crop.height / 2, width, height, rotation: 0, opacity: 1,
      });
    } catch {
      toast.error('Failed to load that image');
    } finally {
      setAddingMedia(null);
    }
  };

  // Adds a file to the library and puts it on the picture straight away.
  const uploadMedia = async (e: ChangeEvent<HTMLInputElement>) => {
    const file = e.target.files?.[0];
    e.target.value = '';
    if (!file) return;
    setUploadingMedia(true);
    try {
      const item = await api.uploadMedia(file);
      setMedia(prev => [item, ...(prev ?? []).filter(m => m.id !== item.id)]);
      await addMedia(item);
    } catch (err) {
      toast.error(err instanceof Error ? err.message : 'Upload failed');
    } finally {
      setUploadingMedia(false);
    }
  };

  const fitSelectedImage = () => {
    if (!crop || selected?.kind !== 'image') return;
    const r = selected.img.naturalWidth / selected.img.naturalHeight;
    let width = crop.width;
    let height = width / r;
    if (height > crop.height) { height = crop.height; width = height * r; }
    updateLayer(selected.id, {
      width, height, rotation: 0, srcCrop: undefined,
      x: crop.x + crop.width / 2, y: crop.y + crop.height / 2,
    });
  };

  // --- Pasted images --------------------------------------------------------

  // Puts an image file on the picture. With an image layer selected (a
  // template's placeholder, say) it takes that layer's place: same frame,
  // corners and border, the image cropped to fill it. Otherwise it goes in
  // the middle like a media image. The file is uploaded first, so a design
  // saved as a template refers to it by URL.
  const placeImageFile = async (file: File) => {
    if (!crop || placingImage) return;
    const target = selected?.kind === 'image' ? selected : null;
    setPlacingImage(true);
    try {
      const { url } = await api.uploadImage(file);
      const img = await loadImage(resolveUrl(url));
      if (target) {
        updateLayer(target.id, {
          img, src: url, origin: 'media',
          srcCrop: centredCrop(img.naturalWidth, img.naturalHeight, target.width / target.height),
        });
        return;
      }
      const r = img.naturalWidth / img.naturalHeight;
      let width = crop.width * 0.5;
      let height = width / r;
      if (height > crop.height * 0.5) { height = crop.height * 0.5; width = height * r; }
      addLayer({
        id: newLayerId(), kind: 'image', origin: 'media', src: url, img,
        x: crop.x + crop.width / 2, y: crop.y + crop.height / 2, width, height, rotation: 0, opacity: 1,
      });
      if (tab !== 'watermark' && tab !== 'media') setTab('media');
    } catch (err) {
      toast.error(err instanceof Error ? err.message : 'Failed to add the image');
    } finally {
      setPlacingImage(false);
    }
  };

  const pickPlaceFile = (e: ChangeEvent<HTMLInputElement>) => {
    const file = e.target.files?.[0];
    e.target.value = '';
    if (file) void placeImageFile(file);
  };

  // Ctrl/Cmd+V with an image on the clipboard. Caught before the page below
  // (the post editor, the News form) sees it, which would take the image for
  // itself; a text paste is left alone for the field it is headed for.
  useEffect(() => {
    const onPaste = (e: ClipboardEvent) => {
      if (saving) return;
      for (const item of e.clipboardData?.items ?? []) {
        if (item.kind !== 'file' || !item.type.startsWith('image/')) continue;
        const file = item.getAsFile();
        if (!file) continue;
        e.preventDefault();
        e.stopImmediatePropagation();
        void placeImageFile(file);
        return;
      }
    };
    document.addEventListener('paste', onPaste, true);
    return () => document.removeEventListener('paste', onPaste, true);
  });

  // --- Text -----------------------------------------------------------------

  const addText = () => {
    if (!crop) return;
    const base: TextLayer = {
      id: newLayerId(),
      kind: 'text',
      text: 'Your text',
      fontFamily: BUILTIN_FONTS[0].family,
      fontSize: Math.round(Math.min(crop.width, crop.height) * 0.09),
      fill: '#ffffff',
      bold: true,
      outline: true,
      shadow: false,
      x: 0,
      y: 0,
      rotation: 0,
    };
    const m = measureText(base);
    addLayer({ ...base, x: crop.x + (crop.width - m.width) / 2, y: crop.y + (crop.height - m.height) / 2 });
    // Select the placeholder so typing replaces it. Not on touch: the
    // keyboard would cover the picture straight away.
    if (!isTouch) {
      setTimeout(() => { textAreaRef.current?.focus(); textAreaRef.current?.select(); }, 0);
    }
  };

  const toggleSnap = () => {
    setSnapOn(on => {
      try { localStorage.setItem(SNAP_STORAGE_KEY, on ? 'off' : 'on'); } catch { /* per-viewer convenience only */ }
      return !on;
    });
  };

  // Snaps a moving layer to the page and the other layers and shows the
  // guides it lines up on. Konva recomputes the position from the pointer on
  // every move, so the shift never accumulates.
  const onLayerDragMove = (l: EditorLayer, e: Konva.KonvaEventObject<DragEvent>) => {
    const node = e.target;
    const parent = contentRef.current;
    if (!crop || !parent || !snapOn || e.evt?.altKey) { setGuides(null); return; }
    const opts = { relativeTo: parent, skipShadow: true, skipStroke: true };
    const others = layers
      .filter(o => o.id !== l.id)
      .map(o => nodeRefs.current.get(o.id)?.getClientRect(opts))
      .filter((b): b is NonNullable<typeof b> => !!b);
    const r = snapBox(node.getClientRect(opts), crop, others, SNAP_PX / scale);
    if (r.dx || r.dy) node.position({ x: node.x() + r.dx, y: node.y() + r.dy });
    setGuides(r.guides.length || r.gaps.length ? r : null);
  };

  const onLayerDragEnd = (l: EditorLayer, e: Konva.KonvaEventObject<DragEvent>) => {
    setGuides(null);
    updateLayer(l.id, { x: e.target.x(), y: e.target.y() });
  };

  // Re-centres the selected layer on the crop, keeping its rotation.
  const centreSelected = () => {
    if (!crop || !selected) return;
    if (selected.kind === 'image') {
      updateLayer(selected.id, { x: crop.x + crop.width / 2, y: crop.y + crop.height / 2 });
      return;
    }
    const node = nodeRefs.current.get(selected.id);
    const parent = contentRef.current;
    if (!node || !parent) return;
    const box = node.getClientRect({ relativeTo: parent, skipShadow: true, skipStroke: true });
    updateLayer(selected.id, {
      x: selected.x + (crop.x + crop.width / 2) - (box.x + box.width / 2),
      y: selected.y + (crop.y + crop.height / 2) - (box.y + box.height / 2),
    });
  };

  // Folds a finished resize into the layer itself: an image's size, a text's
  // font size, so the text stays crisp and the size controls stay true.
  const onLayerTransformEnd = (l: EditorLayer, node: Konva.Node) => {
    const sx = node.scaleX();
    const sy = node.scaleY();
    node.setAttrs({ scaleX: 1, scaleY: 1 });
    if (l.kind === 'image') {
      const width = Math.max(4, l.width * sx);
      const height = Math.max(4, l.height * sy);
      node.setAttrs({ offsetX: width / 2, offsetY: height / 2 });
      updateLayer(l.id, { x: node.x(), y: node.y(), rotation: node.rotation(), width, height });
    } else {
      updateLayer(l.id, { x: node.x(), y: node.y(), rotation: node.rotation(), fontSize: Math.max(MIN_FONT_SIZE, Math.round(l.fontSize * sx)) });
    }
  };

  // --- Corners and borders -------------------------------------------------

  // A border's default and largest width follow the picture's size, so they
  // look alike on a phone photo and a 4K one.
  const borderScale = crop ? Math.min(crop.width, crop.height) : 1000;
  const maxBorder = Math.max(20, Math.round(borderScale * 0.08));

  const setBorderStyle = (l: ImageLayer, style: BorderStyle | null) => {
    if (!style) { updateLayer(l.id, { border: undefined }); return; }
    updateLayer(l.id, {
      border: {
        style,
        width: l.border?.width ?? Math.max(2, Math.round(borderScale * 0.015)),
        color: l.border?.color ?? '#ffffff',
      },
    });
  };

  // --- Save -----------------------------------------------------------------

  const save = async () => {
    if (!ready || !crop || saving) return;
    setSaving(true);
    try {
      const blob = await renderEdit({
        source: canvasMode ? null : source,
        background: canvas?.background,
        crop,
        adjustments: canvasMode ? NO_ADJUSTMENTS : adjustments,
        layers,
      });
      await onSave(blob, toTemplate(crop, layers, canvas?.background ?? '#ffffff'));
    } catch (err) {
      const e = err instanceof Error ? err : null;
      toast.error(e?.name === 'SecurityError'
        ? 'This image cannot be exported (it was loaded from another site)'
        : (e?.message || 'Failed to save the image'));
      setSaving(false);
    }
  };

  // --- Render ---------------------------------------------------------------

  const onStagePointerDown = (e: Konva.KonvaEventObject<PointerEvent>) => {
    if (tab === 'crop') return;
    if (e.target === e.target.getStage() || e.target === baseRef.current || e.target === backgroundRef.current) setSelectedId(null);
  };

  // The library's colours, the built-in swatches and a custom picker. The
  // picker passes key, so dragging through it is one undo step.
  const colorRow = (value: string, pick: (color: string, key?: string) => void, key: string) => (
    <div className="ie-row ie-scroll">
      {libraryColors.map(m => (
        <button
          key={m.id}
          className={`ie-swatch ${value.toLowerCase() === m.color!.toLowerCase() ? 'active' : ''}`}
          style={{ background: m.color }}
          onClick={() => pick(m.color!)}
          title={m.name}
          aria-label={`Colour ${m.name}`}
        />
      ))}
      {libraryColors.length > 0 && <span className="ie-swatch-divider" />}
      {SWATCHES.map(c => (
        <button
          key={c}
          className={`ie-swatch ${value.toLowerCase() === c ? 'active' : ''}`}
          style={{ background: c }}
          onClick={() => pick(c)}
          aria-label={`Colour ${c}`}
        />
      ))}
      <input
        type="color"
        className="ie-color"
        value={value.slice(0, 7)}
        onChange={e => pick(e.target.value, key)}
        title="Custom colour"
      />
    </div>
  );

  // Opacity, corners, border, fit, centre, replace and remove for the
  // selected watermark or media image.
  const imageLayerControls = selected?.kind === 'image' && (
    <>
      <label className="ie-slider">
        <span>Opacity</span>
        <input
          type="range"
          min={10}
          max={100}
          value={Math.round(selected.opacity * 100)}
          onChange={e => updateLayer(selected.id, { opacity: Number(e.target.value) / 100 }, `opacity:${selected.id}`)}
        />
        <span className="ie-value">{Math.round(selected.opacity * 100)}%</span>
      </label>
      <label className="ie-slider">
        <span>Corners</span>
        <input
          type="range"
          min={0}
          max={50}
          value={selected.cornerRadius ?? 0}
          onChange={e => updateLayer(selected.id, { cornerRadius: Number(e.target.value) }, `corners:${selected.id}`)}
          onDoubleClick={() => updateLayer(selected.id, { cornerRadius: 0 })}
        />
        <span className="ie-value">{selected.cornerRadius ?? 0}%</span>
      </label>
      <span className="ie-label">Border</span>
      <div className="ie-row ie-scroll">
        <button className={`ie-chip ${!selected.border ? 'active' : ''}`} onClick={() => setBorderStyle(selected, null)}>None</button>
        {BORDER_STYLES.map(b => (
          <button
            key={b.id}
            className={`ie-chip ${selected.border?.style === b.id ? 'active' : ''}`}
            onClick={() => setBorderStyle(selected, b.id)}
          >
            {b.label}
          </button>
        ))}
      </div>
      {selected.border && (
        <>
          <label className="ie-slider">
            <span>Width</span>
            <input
              type="range"
              min={1}
              max={Math.max(maxBorder, Math.round(selected.border.width))}
              value={Math.round(selected.border.width)}
              onChange={e => updateLayer(selected.id, { border: { ...selected.border!, width: Number(e.target.value) } }, `border-width:${selected.id}`)}
            />
            <span className="ie-value">{Math.round(selected.border.width)}px</span>
          </label>
          {colorRow(
            selected.border.color,
            (c, key) => updateLayer(selected.id, { border: { ...selected.border!, color: c } }, key),
            `border-color:${selected.id}`,
          )}
        </>
      )}
      <div className="ie-row ie-scroll">
        <button className="ie-chip" onClick={fitSelectedImage}><Maximize size={14} /> Fit</button>
        <button className="ie-chip" onClick={centreSelected}><Crosshair size={14} /> Centre</button>
        <button className="ie-chip" onClick={() => placeInputRef.current?.click()} disabled={placingImage} title="Put another image in this one's place">
          {placingImage ? <Loader2 size={14} className="ie-spin" /> : <ImageUp size={14} />} Replace
        </button>
        <button className="ie-chip danger" onClick={deleteSelected}><Trash2 size={14} /> Remove</button>
      </div>
      <p className="ie-hint">Paste an image (Ctrl+V) to put it in this one's place, filling its frame.</p>
    </>
  );

  const anchorSize = isTouch ? 22 : 11;
  const layerTransformer = tab !== 'crop';
  const ratio = aspectRatio(aspectId);

  return (
    <div className="ie-root" role="dialog" aria-label="Edit image">
      <header className="ie-header">
        <button className="ie-icon-btn" onClick={handleCancel} title="Cancel" disabled={saving}>
          <X size={20} />
        </button>
        <span className="ie-title">{title ?? 'Edit image'}</span>
        <button className="ie-icon-btn" onClick={undo} title="Undo" aria-label="Undo" disabled={saving || undoStack.length === 0}>
          <Undo2 size={18} />
        </button>
        <button className="ie-icon-btn" onClick={redo} title="Redo" aria-label="Redo" disabled={saving || redoStack.length === 0}>
          <Redo2 size={18} />
        </button>
        <button
          className={`ie-icon-btn ${snapOn ? 'active' : ''}`}
          onClick={toggleSnap}
          title={snapOn ? 'Smart guides on (hold Alt to move freely)' : 'Smart guides off'}
          aria-label="Smart guides"
          aria-pressed={snapOn}
        >
          <Magnet size={18} />
        </button>
        <button
          className={`ie-icon-btn ${showLayers ? 'active' : ''}`}
          onClick={() => setShowLayers(v => !v)}
          title={showLayers ? 'Hide the object list' : 'Show the object list'}
          aria-label="Objects"
          aria-pressed={showLayers}
        >
          <LayersIcon size={18} />
        </button>
        <button className="btn btn-primary ie-save" onClick={save} disabled={!ready || saving}>
          {saving ? <><Loader2 size={16} className="ie-spin" /> Saving…</> : 'Save'}
        </button>
      </header>

      <div className="ie-body">
        <div className="ie-canvas" ref={canvasBoxRef}>
          {loadError && <div className="ie-message">The image could not be loaded.</div>}
          {!ready && !loadError && <div className="ie-message"><Loader2 size={24} className="ie-spin" /></div>}
          {ready && crop && size.w > 0 && (
            <Stage width={size.w} height={size.h} onPointerDown={onStagePointerDown}>
              <Layer>
                <Group
                  ref={contentRef}
                  x={gx}
                  y={gy}
                  scaleX={scale}
                  scaleY={scale}
                  {...(tab === 'crop' ? {} : { clipX: crop.x, clipY: crop.y, clipWidth: crop.width, clipHeight: crop.height })}
                >
                  {canvas
                    ? <Rect ref={backgroundRef} x={0} y={0} width={natW} height={natH} fill={canvas.background} />
                    : <KImage ref={baseRef} image={source ?? undefined} width={natW} height={natH} />}
                  {layers.map(l => (l.kind === 'image' ? (() => {
                    const { group, parts } = imageLayerParts(l);
                    return (
                      <Group
                        key={l.id}
                        ref={node => { if (node) nodeRefs.current.set(l.id, node); else nodeRefs.current.delete(l.id); }}
                        {...group}
                        listening={tab !== 'crop'}
                        draggable={tab !== 'crop'}
                        onPointerDown={() => selectLayer(l)}
                        onDragStart={startGesture}
                        onTransformStart={startGesture}
                        onDragMove={e => onLayerDragMove(l, e)}
                        onDragEnd={e => onLayerDragEnd(l, e)}
                        onTransformEnd={e => onLayerTransformEnd(l, e.currentTarget)}
                      >
                        {parts.map((p, i) => (p.kind === 'image'
                          ? <KImage key={i} {...p.attrs} image={p.attrs.image} />
                          : <Rect key={i} {...p.attrs} />))}
                      </Group>
                    );
                  })() : (
                    <KText
                      key={l.id}
                      ref={node => { if (node) nodeRefs.current.set(l.id, node); else nodeRefs.current.delete(l.id); }}
                      {...textLayerAttrs(l)}
                      listening={tab !== 'crop'}
                      draggable={tab !== 'crop'}
                      onPointerDown={() => selectLayer(l)}
                      onDragStart={startGesture}
                      onTransformStart={startGesture}
                      onDblClick={() => textAreaRef.current?.focus()}
                      onDragMove={e => onLayerDragMove(l, e)}
                      onDragEnd={e => onLayerDragEnd(l, e)}
                      onTransformEnd={e => onLayerTransformEnd(l, e.target)}
                    />
                  )))}
                  {guideOverlay && (
                    <KImage image={guideOverlay} x={crop.x} y={crop.y} width={crop.width} height={crop.height} listening={false} opacity={tab === 'crop' ? 0.6 : 1} />
                  )}
                </Group>

                {guides && tab !== 'crop' && (
                  <Group listening={false}>
                    {guides.guides.map((g, i) => (g.orientation === 'v' ? (
                      <Line key={`g${i}`} points={[gx + g.pos * scale, gy + g.from * scale, gx + g.pos * scale, gy + g.to * scale]} stroke={GUIDE_COLOR} strokeWidth={1} />
                    ) : (
                      <Line key={`g${i}`} points={[gx + g.from * scale, gy + g.pos * scale, gx + g.to * scale, gy + g.pos * scale]} stroke={GUIDE_COLOR} strokeWidth={1} />
                    )))}
                    {guides.gaps.map((m, i) => {
                      const a = (m.axis === 'x' ? gx : gy) + m.from * scale;
                      const b = (m.axis === 'x' ? gx : gy) + m.to * scale;
                      const c = (m.axis === 'x' ? gy : gx) + m.at * scale;
                      const pt = (along: number, cross: number) => (m.axis === 'x' ? [along, cross] : [cross, along]);
                      const [lx, ly] = pt((a + b) / 2, c);
                      return (
                        <Group key={`m${i}`}>
                          <Line points={[...pt(a, c), ...pt(b, c)]} stroke={GUIDE_COLOR} strokeWidth={1} />
                          <Line points={[...pt(a, c - 4), ...pt(a, c + 4)]} stroke={GUIDE_COLOR} strokeWidth={1} />
                          <Line points={[...pt(b, c - 4), ...pt(b, c + 4)]} stroke={GUIDE_COLOR} strokeWidth={1} />
                          <Label x={lx} y={ly}>
                            <Tag fill={GUIDE_COLOR} cornerRadius={3} pointerDirection={m.axis === 'x' ? 'down' : 'left'} pointerWidth={0} pointerHeight={4} />
                            <KText text={`${Math.round(m.to - m.from)}`} fontSize={11} fill="#ffffff" padding={3} />
                          </Label>
                        </Group>
                      );
                    })}
                  </Group>
                )}

                {tab === 'crop' && (
                  <Group x={gx} y={gy} scaleX={scale} scaleY={scale}>
                    {/* Shade what the crop leaves out. */}
                    <Rect x={0} y={0} width={natW} height={crop.y} fill="rgba(0,0,0,0.6)" listening={false} />
                    <Rect x={0} y={crop.y + crop.height} width={natW} height={natH - crop.y - crop.height} fill="rgba(0,0,0,0.6)" listening={false} />
                    <Rect x={0} y={crop.y} width={crop.x} height={crop.height} fill="rgba(0,0,0,0.6)" listening={false} />
                    <Rect x={crop.x + crop.width} y={crop.y} width={natW - crop.x - crop.width} height={crop.height} fill="rgba(0,0,0,0.6)" listening={false} />
                    {[1, 2].map(i => (
                      <Line key={`v${i}`} points={[crop.x + crop.width * i / 3, crop.y, crop.x + crop.width * i / 3, crop.y + crop.height]} stroke="rgba(255,255,255,0.45)" strokeWidth={1} strokeScaleEnabled={false} listening={false} />
                    ))}
                    {[1, 2].map(i => (
                      <Line key={`h${i}`} points={[crop.x, crop.y + crop.height * i / 3, crop.x + crop.width, crop.y + crop.height * i / 3]} stroke="rgba(255,255,255,0.45)" strokeWidth={1} strokeScaleEnabled={false} listening={false} />
                    ))}
                    <Rect
                      ref={cropRectRef}
                      x={crop.x}
                      y={crop.y}
                      width={crop.width}
                      height={crop.height}
                      fill="rgba(0,0,0,0)"
                      stroke="#ffffff"
                      strokeWidth={2}
                      strokeScaleEnabled={false}
                      draggable
                      dragBoundFunc={cropDragBound}
                      onDragStart={startGesture}
                      onTransformStart={startGesture}
                      onDragMove={syncCropFromNode}
                      onDragEnd={syncCropFromNode}
                      onTransform={syncCropFromNode}
                      onTransformEnd={syncCropFromNode}
                    />
                  </Group>
                )}

                <Transformer
                  ref={trRef}
                  anchorSize={anchorSize}
                  anchorCornerRadius={anchorSize / 2}
                  anchorStroke="#6366f1"
                  anchorFill="#ffffff"
                  borderStroke={layerTransformer ? '#818cf8' : 'rgba(0,0,0,0)'}
                  borderStrokeWidth={1.5}
                  padding={layerTransformer ? 4 : 0}
                  rotateEnabled={layerTransformer}
                  rotateAnchorOffset={isTouch ? 36 : 28}
                  rotationSnaps={[0, 90, 180, 270]}
                  flipEnabled={false}
                  ignoreStroke
                  keepRatio={layerTransformer || ratio !== null}
                  enabledAnchors={layerTransformer || ratio !== null
                    ? ['top-left', 'top-right', 'bottom-left', 'bottom-right']
                    : ['top-left', 'top-center', 'top-right', 'middle-right', 'middle-left', 'bottom-left', 'bottom-center', 'bottom-right']}
                  boundBoxFunc={(oldBox, newBox) => (layerTransformer
                    ? (newBox.width < 12 || newBox.height < 12 ? oldBox : newBox)
                    : cropBoundBox(oldBox, newBox))}
                />
              </Layer>
            </Stage>
          )}
        </div>

        {showLayers && (
          <aside className="ie-layers" aria-label="Objects">
            <div className="ie-layers-title">Objects</div>
            <LayerList
              layers={layers}
              selectedId={selectedId}
              background={canvas ? { color: canvas.background } : { src: source?.src ?? imageUrl ?? '' }}
              onSelect={selectLayer}
              onSelectBackground={() => { setSelectedId(null); if (canvasMode) setTab('canvas'); }}
              onReorder={reorderLayers}
            />
          </aside>
        )}
      </div>

      <div className="ie-panel">
        {tab === 'crop' && (
          <div className="ie-row ie-scroll">
            {ASPECTS.map(a => (
              <button key={a.id} className={`ie-chip ${aspectId === a.id ? 'active' : ''}`} onClick={() => pickAspect(a.id)}>
                {a.label}
              </button>
            ))}
            <button className="ie-chip" onClick={resetCrop}>Reset</button>
          </div>
        )}

        {tab === 'canvas' && canvas && (
          <>
            <div className="ie-row ie-scroll">
              {CANVAS_FORMATS.map(f => (
                <button
                  key={f.id}
                  className={`ie-chip ${canvas.width === f.width && canvas.height === f.height ? 'active' : ''}`}
                  onClick={() => changeFormat(f.width, f.height)}
                  title={`${f.width} × ${f.height}`}
                >
                  {f.label}
                </button>
              ))}
            </div>
            <span className="ie-label">Background</span>
            {colorRow(canvas.background, changeBackground, 'background')}
          </>
        )}

        {tab === 'templates' && (
          templates === null ? (
            <p className="ie-hint"><Loader2 size={14} className="ie-spin" /> Loading…</p>
          ) : templates.length === 0 ? (
            <p className="ie-hint">No templates yet. Create them in the Media Library.</p>
          ) : (
            <>
              <div className="ie-row ie-scroll">
                {templates.map(t => (
                  <button
                    key={t.id}
                    className="ie-thumb"
                    onClick={() => pickTemplate(t)}
                    disabled={applyingTemplate !== null}
                    title={t.description ? `${t.name}: ${t.description}` : t.name}
                  >
                    <img src={resolveUrl(t.url)} alt="" loading="lazy" />
                    <span>{applyingTemplate === t.id ? 'Loading…' : t.name}</span>
                  </button>
                ))}
              </div>
              <p className="ie-hint">
                {canvasMode
                  ? 'Tap a template to start from it. It replaces the current design.'
                  : 'Tap a template to lay its texts and images over the picture.'}
              </p>
            </>
          )
        )}

        {tab === 'filters' && (
          <>
            <div className="ie-row ie-scroll">
              {FILTER_PRESETS.map(p => (
                <button
                  key={p.id}
                  className={`ie-chip ${adjustments.preset === p.id ? 'active' : ''}`}
                  onClick={() => changeAdjustments({ preset: p.id })}
                >
                  {p.label}
                </button>
              ))}
            </div>
            {(['brightness', 'contrast', 'saturation'] as const).map(k => (
              <label key={k} className="ie-slider">
                <span>{k[0].toUpperCase() + k.slice(1)}</span>
                <input
                  type="range"
                  min={-100}
                  max={100}
                  value={adjustments[k]}
                  onChange={e => changeAdjustments({ [k]: Number(e.target.value) }, `adjust:${k}`)}
                  onDoubleClick={() => changeAdjustments({ [k]: 0 })}
                />
                <span className="ie-value">{adjustments[k]}</span>
              </label>
            ))}
            {hasAdjustments(adjustments) && (
              <div className="ie-row">
                <button className="ie-chip" onClick={() => changeAdjustments(NO_ADJUSTMENTS)}>Reset filters</button>
              </div>
            )}
          </>
        )}

        {tab === 'watermark' && (
          <>
            {watermarks.length === 0 ? (
              <p className="ie-hint">No watermarks yet. Upload them on the Watermarks page.</p>
            ) : (
              <div className="ie-row ie-scroll">
                {watermarks.map(w => (
                  <button
                    key={w.id}
                    className="ie-thumb"
                    onClick={() => addWatermark(w)}
                    disabled={addingWatermark !== null}
                    title={`Add ${w.name || w.filename}`}
                  >
                    <img src={resolveUrl(w.url)} alt="" />
                    <span>{addingWatermark === w.id ? 'Adding…' : (w.name || w.filename)}</span>
                  </button>
                ))}
              </div>
            )}
            {selected?.kind === 'image' ? imageLayerControls : watermarks.length > 0 && (
              <p className="ie-hint">Tap a watermark to add it, then drag it, pull its corners or turn it on the picture.</p>
            )}
          </>
        )}

        {tab === 'media' && (
          <>
            <div className="ie-row">
              <div className="ie-search">
                <Search size={16} />
                <input
                  type="search"
                  value={mediaQuery}
                  onChange={e => setMediaQuery(e.target.value)}
                  placeholder="Search the media library"
                />
              </div>
              <button
                className="ie-icon-btn"
                onClick={() => mediaInputRef.current?.click()}
                disabled={uploadingMedia}
                title="Add an image to the library"
              >
                {uploadingMedia ? <Loader2 size={16} className="ie-spin" /> : <Upload size={16} />}
              </button>
            </div>
            {media === null ? (
              <p className="ie-hint"><Loader2 size={14} className="ie-spin" /> Loading…</p>
            ) : media.length === 0 ? (
              <p className="ie-hint">
                {mediaLoading ? 'Searching…' : mediaQuery.trim()
                  ? 'Nothing in the library matches that.'
                  : 'The media library has no images yet. Add them here or on the Media Library page.'}
              </p>
            ) : (
              <div className="ie-row ie-scroll">
                {media.map(m => (
                  <button
                    key={m.id}
                    className="ie-thumb"
                    onClick={() => addMedia(m)}
                    disabled={addingMedia !== null}
                    title={m.description ? `${m.name}: ${m.description}` : `Add ${m.name}`}
                  >
                    <img src={resolveUrl(m.url)} alt="" loading="lazy" />
                    <span>{addingMedia === m.id ? 'Adding…' : m.name}</span>
                  </button>
                ))}
              </div>
            )}
            {selected?.kind === 'image' ? imageLayerControls : media && media.length > 0 && (
              <p className="ie-hint">Tap an image to add it, or paste one (Ctrl+V), then drag it, pull its corners or turn it on the picture.</p>
            )}
            <input ref={mediaInputRef} type="file" accept="image/png,image/jpeg,image/gif,image/webp" hidden onChange={uploadMedia} />
          </>
        )}

        {tab === 'text' && (
          selected?.kind === 'text' ? (
            <>
              <textarea
                ref={textAreaRef}
                className="ie-textarea"
                rows={2}
                value={selected.text}
                onChange={e => updateLayer(selected.id, { text: e.target.value }, `text:${selected.id}`)}
                placeholder="Text"
              />
              <div className="ie-row">
                <select
                  className="ie-select"
                  value={selected.fontFamily}
                  onChange={e => updateLayer(selected.id, { fontFamily: e.target.value })}
                >
                  {fonts.some(f => f.family === selected.fontFamily) ? null : (
                    <option value={selected.fontFamily}>{selected.fontFamily}</option>
                  )}
                  {fonts.map(f => (
                    <option key={f.family} value={f.family} style={{ fontFamily: f.family }}>{f.label}</option>
                  ))}
                </select>
                <input
                  type="number"
                  className="ie-number"
                  min={MIN_FONT_SIZE}
                  max={MAX_FONT_SIZE}
                  step={1}
                  value={fontSizeDraft?.id === selected.id ? fontSizeDraft.value : Math.round(selected.fontSize)}
                  onChange={e => {
                    setFontSizeDraft({ id: selected.id, value: e.target.value });
                    const v = Math.round(Number(e.target.value));
                    if (v >= MIN_FONT_SIZE && v <= MAX_FONT_SIZE) updateLayer(selected.id, resizeText(selected, v), `font-size:${selected.id}`);
                  }}
                  onBlur={() => setFontSizeDraft(null)}
                  title="Font size in pixels of the picture"
                  aria-label="Font size"
                />
                <span className="ie-unit">px</span>
                <div className="ie-segmented" role="group" aria-label="Text alignment">
                  {TEXT_ALIGNS.map(a => (
                    <button
                      key={a.id}
                      className={(selected.align ?? 'center') === a.id ? 'active' : ''}
                      onClick={() => updateLayer(selected.id, { align: a.id })}
                      title={a.label}
                      aria-label={a.label}
                      aria-pressed={(selected.align ?? 'center') === a.id}
                    >
                      <a.icon size={16} />
                    </button>
                  ))}
                </div>
              </div>
              {colorRow(selected.fill, (c, key) => updateLayer(selected.id, { fill: c }, key), `fill:${selected.id}`)}
              <div className="ie-row ie-scroll">
                <button className={`ie-chip ${selected.bold ? 'active' : ''}`} onClick={() => updateLayer(selected.id, { bold: !selected.bold })}>
                  <Bold size={14} /> Bold
                </button>
                <button className={`ie-chip ${selected.outline ? 'active' : ''}`} onClick={() => updateLayer(selected.id, { outline: !selected.outline })}>
                  Outline
                </button>
                <button className={`ie-chip ${selected.shadow ? 'active' : ''}`} onClick={() => updateLayer(selected.id, { shadow: !selected.shadow })}>
                  Shadow
                </button>
                <button className="ie-chip" onClick={centreSelected}><Crosshair size={14} /> Centre</button>
                <button className="ie-chip" onClick={addText}><Plus size={14} /> New</button>
                <button className="ie-chip danger" onClick={deleteSelected}><Trash2 size={14} /> Remove</button>
              </div>
            </>
          ) : (
            <>
              <div className="ie-row">
                <button className="btn btn-primary" onClick={addText}><Plus size={16} /> Add text</button>
              </div>
              <p className="ie-hint">
                {layers.some(l => l.kind === 'text') ? 'Tap a text on the picture to edit it.' : 'Fonts and colours from the media library are offered alongside the system ones.'}
              </p>
            </>
          )
        )}
      </div>

      <input ref={placeInputRef} type="file" accept="image/png,image/jpeg,image/gif,image/webp" hidden onChange={pickPlaceFile} />

      <nav className="ie-tabs">
        {canvasMode ? (
          <button className={tab === 'canvas' ? 'active' : ''} onClick={() => switchTab('canvas')}><Frame size={20} /><span>Canvas</span></button>
        ) : (
          <>
            <button className={tab === 'crop' ? 'active' : ''} onClick={() => switchTab('crop')}><Crop size={20} /><span>Crop</span></button>
            <button className={tab === 'filters' ? 'active' : ''} onClick={() => switchTab('filters')}><Sparkles size={20} /><span>Filters</span></button>
          </>
        )}
        <button className={tab === 'templates' ? 'active' : ''} onClick={() => switchTab('templates')}><LayoutTemplate size={20} /><span>Templates</span></button>
        <button className={tab === 'watermark' ? 'active' : ''} onClick={() => switchTab('watermark')}><Stamp size={20} /><span>Watermark</span></button>
        <button className={tab === 'media' ? 'active' : ''} onClick={() => switchTab('media')}><FolderOpen size={20} /><span>Media</span></button>
        <button className={tab === 'text' ? 'active' : ''} onClick={() => switchTab('text')}><Type size={20} /><span>Text</span></button>
      </nav>
    </div>
  );
}

interface BoxLike {
  x: number;
  y: number;
  width: number;
  height: number;
  rotation: number;
}
