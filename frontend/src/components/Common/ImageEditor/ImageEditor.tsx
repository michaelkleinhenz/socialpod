import { Fragment, useCallback, useEffect, useLayoutEffect, useMemo, useRef, useState, type ChangeEvent, type ReactNode, type Ref } from 'react';
import Konva from 'konva';
import { Stage, Layer, Group, Image as KImage, Text as KText, Rect, Line, Label, Tag, Transformer, Ellipse, Shape, Circle } from 'react-konva';
import {
  AlignCenter, AlignLeft, AlignRight, Bold, Brush, Circle as CircleIcon, Clapperboard, Crop, Crosshair, FilePen, Film, FolderOpen, Frame, ImageUp, LayoutTemplate, Layers as LayersIcon, Loader2, Magnet,
  Move, Music, Plus, Redo2, Search, Shapes, Smartphone, Sparkles, Square, Stamp, Trash2, Triangle, Type, Undo2, Upload, X, Maximize,
  ALargeSmall, Blend, CaseSensitive, Contrast, PaintBucket, Palette, Pencil, Replace, SquareDashed, SquareRoundCorner, Volume2, type LucideIcon,
} from 'lucide-react';
import toast from 'react-hot-toast';
import { api } from '../../../services/api';
import type { ImageDraft, MediaItem } from '../../../types';
import {
  applyAdjustments, BLEND_MODES, BUILTIN_FONTS, blendAttrs, BORDER_STYLES, CANVAS_FORMATS, centredCrop, DEFAULT_CANVAS, clamp, FILTER_PRESETS, fitLayers, hasAdjustments, imageLayerParts,
  instantiateTemplate, isVideoTemplate, loadImage, loadVideo, measureText, newLayerId, NO_ADJUSTMENTS, paintStrokeAttrs, parseDraft, parseTemplate, renderEdit, resizeText, SHAPE_KINDS,
  shapeLayerParts, sourceSize, textLayerAttrs, toTemplate,
  type Adjustments, type AudioTrack, type BlendMode, type ClipInfo, type BorderStyle, type CropBox, type EditorCanvas, type EditorDraft, type EditorLayer, type EditorTemplate, type ImageLayer, type ImagePart,
  type FontOption, type PaintLayer, type PaintStroke, type ShapeKind, type ShapeLayer, type TextAlign, type TextLayer,
} from './editorModel';
import { snapBox, type SnapResult } from './snapping';
import { geometricHitTest } from './hitTest';
import { safeZoneRects, type SafeZoneKind } from './safeZones';
import { LayerList } from './LayerList';
import {
  animatedTextAttrs, animGroupAttrs, animStateAt, DEFAULT_DURATION, DURATION_PRESETS, ENTER_EFFECTS, EXIT_EFFECTS, formatTime, layerAnim, layersAt, openingTime, settledAt,
  layerSpan, LOOP_EFFECTS, MAX_DURATION, MIN_DURATION, musicSpan, NO_ANIM, type LayerAnim,
} from './animation';
import { soundDuration, stopMedia, syncMedia } from './mediaSync';
import { Timeline } from './Timeline';
import { BottomSheet, MiniToolbar, PhoneBar, PhoneTray, type BarItem, type TraySub } from './PhoneParts';
import { canExportVideo, renderVideo } from './videoExport';
import { nearestFormat, SYSTEM_CATEGORIES, SYSTEM_FORMATS, SYSTEM_TEMPLATES, TEMPLATE_ASSETS, type SystemCategory, type SystemFormat, type SystemTemplate } from './systemTemplates';
import { SystemTemplateTile } from './SystemTemplateTile';
import './ImageEditor.css';

type Tab = 'crop' | 'canvas' | 'filters' | 'templates' | 'media' | 'text' | 'shapes' | 'brush' | 'animate';

// Where the editor switches to its phone layout (PhoneParts.tsx).
const PHONE_QUERY = '(max-width: 768px)';

// The size of the selected object's toolbar on a phone, for placing it.
const MINI_TOOLBAR = { width: 216, height: 48 };

// What a bottom sheet on a phone holds.
type PhoneSheet = 'templates' | 'media' | 'layers' | 'video';

// The parts of a layer's animation, one at a time in the phone's tray.
type AnimPart = 'timing' | 'in' | 'loop' | 'out';

const SHAPE_ICONS: Record<ShapeKind, typeof Square> = { rect: Square, ellipse: CircleIcon, triangle: Triangle };

// The brush's settings; size and feather in pixels of the picture, null
// until set (they then follow the picture's size).
interface BrushSettings {
  size: number | null;
  feather: number | null;
  color: string;
  opacity: number;
}

// A new brush paints soft and see-through in black: the usual job is to
// darken a busy background a little behind a text.
const DEFAULT_BRUSH: BrushSettings = { size: null, feather: null, color: '#000000', opacity: 0.35 };

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
  // The video's length and music; unused on a still.
  duration: number;
  audio: AudioTrack | null;
}

// How close, in screen pixels, a moved layer has to come to snap.
const SNAP_PX = 6;
// How far, in screen pixels, a tap may miss a layer on a touch screen and
// still pick it.
const TOUCH_SLOP = 16;
const GUIDE_COLOR = '#ff2d95';
const SNAP_STORAGE_KEY = 'socialpod.imageEditor.snap';
const LOOP_STORAGE_KEY = 'socialpod.imageEditor.loop';
const SAFE_ZONE_STORAGE_KEY = 'socialpod.imageEditor.safeZone';
const SAFE_ZONE_FILL = 'rgba(255, 45, 85, 0.3)';

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
  // An overlay drawn over the crop area for reference but left out of the
  // output, for images whose overlay is composited later (convention queues
  // stamp theirs when the item is posted).
  guideOverlay?: HTMLImageElement | null;
  // A video (a reel): the design gets a length and a timeline, its layers
  // show for a span of it and come in, move and go out with an effect, and
  // Save exports an MP4 instead of a JPEG.
  video?: boolean;
  // Shades where Instagram's own interface covers a reel or a story (header,
  // icons, caption), switchable in the header and never exported.
  safeZone?: SafeZoneKind;
  // Editing a media library template: for a video, Save hands over the design
  // with the frame at the playhead as its preview instead of exporting the MP4.
  designOnly?: boolean;
  // The rendered JPEG (an MP4 for a video), and the design as a template
  // (the crop as its canvas).
  onSave: (blob: Blob, design: EditorTemplate) => void | Promise<void>;
  onCancel: () => void;
}

const apiBase = import.meta.env.VITE_API_URL || '';
// The built-in templates' placeholder pictures come from the frontend itself.
const resolveUrl = (url: string) => (url.startsWith('/') && !url.startsWith(TEMPLATE_ASSETS) ? apiBase + url : url);

// The library also holds videos and fonts, which cannot go on a picture.
const isImageMedia = (m: MediaItem) =>
  !m.contentType?.startsWith('video/') && !m.contentType?.startsWith('font/') && !/\.(mp4|mov|ttf|otf)$/i.test(m.url);

export function ImageEditor({ imageUrl, canvas: initialCanvas, template, title, guideOverlay, video = false, safeZone, designOnly = false, onSave, onCancel }: Props) {
  // The photo being edited; a draft restored here can bring its own, or
  // none (a blank canvas).
  const [photoUrl, setPhotoUrl] = useState(imageUrl || undefined);
  const canvasMode = !photoUrl;
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
  // A video's length, where its playhead is and whether it plays; and how far
  // an export has got, in percent.
  const [duration, setDuration] = useState(() => template?.duration ?? DEFAULT_DURATION);
  const [time, setTime] = useState(0);
  const timeRef = useRef(0);
  const [playing, setPlaying] = useState(false);
  // Repeat: playback starts over at the end instead of stopping, and the
  // design stays editable while it runs, so changes show as it plays.
  const [loop, setLoop] = useState(() => {
    try { return localStorage.getItem(LOOP_STORAGE_KEY) === 'on'; } catch { return false; }
  });
  const loopRef = useRef(loop);
  loopRef.current = loop;
  const toggleLoop = () => {
    setLoop(on => {
      try { localStorage.setItem(LOOP_STORAGE_KEY, on ? 'off' : 'on'); } catch { /* per-viewer convenience only */ }
      return !on;
    });
  };
  // Playing on repeat, the picture and the timeline stay live.
  const livePlay = playing && loop;
  // Bumped to restart the playback from the playhead while it plays.
  const [playRun, setPlayRun] = useState(0);
  const [exportProgress, setExportProgress] = useState<number | null>(null);
  // A video's music, part of the undo snapshot, and the library's sounds to
  // pick it from, loaded when the music controls are first shown.
  const [audio, setAudio] = useState<AudioTrack | null>(() => template?.audio ?? null);
  const [sounds, setSounds] = useState<MediaItem[] | null>(null);
  const [uploadingSound, setUploadingSound] = useState(false);
  // Clips and sounds uploaded here as temporary files. They are released
  // when the editor closes, which deletes whatever nothing references: a
  // video rendered into a post keeps only the rendered file, a draft keeps
  // the clips it uses.
  const tempUploads = useRef(new Set<string>());
  // An upload kept in the media library instead, for other videos too.
  const [keepInLibrary, setKeepInLibrary] = useState(false);
  // What playback reads between renders: the layers and music as they are,
  // the music's player, and every clip video the preview has seen.
  const layersRef = useRef<EditorLayer[]>([]);
  const audioRef = useRef<AudioTrack | null>(null);
  const musicRef = useRef<HTMLAudioElement | null>(null);
  const knownVideos = useRef(new Set<HTMLVideoElement>());
  // The text layer being typed into on the picture itself.
  const [editingTextId, setEditingTextId] = useState<string | null>(null);
  const editingTextRef = useRef<string | null>(null);
  const [fonts, setFonts] = useState<FontOption[]>(BUILTIN_FONTS);
  // Colour entries of the media library, offered before the built-in swatches.
  const [libraryColors, setLibraryColors] = useState<MediaItem[]>([]);
  // The team's media library, loaded when the Media tab is first opened;
  // the Watermarks filter narrows it to the images marked as watermarks.
  const [media, setMedia] = useState<MediaItem[] | null>(null);
  const [mediaQuery, setMediaQuery] = useState('');
  const [watermarksOnly, setWatermarksOnly] = useState(false);
  // In a video the Media tab also lists the library's clips (its videos).
  const [clipsView, setClipsView] = useState(false);
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
  // The Templates tab's two lists: SocialPod's built-in templates or the
  // user's own (drafts and the library's); null until picked, which shows
  // the user's own when there are any. The built-in list's format (null:
  // the one closest to the picture) and category filters.
  const [templateView, setTemplateView] = useState<'system' | 'own' | null>(null);
  const [systemFormat, setSystemFormat] = useState<SystemFormat | null>(null);
  const [systemCategory, setSystemCategory] = useState<SystemCategory | null>(null);
  const [saving, setSaving] = useState(false);
  // Image drafts: the user's saved work, listed on the Templates tab; the
  // one this edit was saved as or restored from, which Save draft updates;
  // and whether nothing changed since.
  const [drafts, setDrafts] = useState<ImageDraft[] | null>(null);
  const [draft, setDraft] = useState<{ id: string; name: string } | null>(null);
  const [draftClean, setDraftClean] = useState(false);
  const [savingDraft, setSavingDraft] = useState(false);
  const [savingTemplate, setSavingTemplate] = useState(false);
  const [restoringDraft, setRestoringDraft] = useState<string | null>(null);
  const restoredPhoto = useRef<string | null>(null);
  // A local photo (a fresh pick, a blob URL) uploaded for a draft, so later
  // saves reuse it.
  const storedPhoto = useRef<{ from: string; url: string } | null>(null);
  // Smart guides while a layer is moved; Alt (Option) skips them for one drag.
  const [snapOn, setSnapOn] = useState(() => {
    try { return localStorage.getItem(SNAP_STORAGE_KEY) !== 'off'; } catch { return true; }
  });
  const [guides, setGuides] = useState<SnapResult | null>(null);
  const [showSafeZone, setShowSafeZone] = useState(() => {
    try { return localStorage.getItem(SAFE_ZONE_STORAGE_KEY) !== 'off'; } catch { return true; }
  });
  const [brush, setBrush] = useState<BrushSettings>(DEFAULT_BRUSH);
  // The stroke being painted, in picture pixels, and where the brush is.
  const [drawing, setDrawing] = useState<PaintStroke | null>(null);
  const drawingRef = useRef<PaintStroke | null>(null);
  const [brushAt, setBrushAt] = useState<{ x: number; y: number } | null>(null);
  // On the Brush tab the pointer paints, or picks and moves layers.
  const [brushMode, setBrushMode] = useState<'paint' | 'move'>('paint');
  // The object list on the right; open from the start where there is room.
  const [showLayers, setShowLayers] = useState(() => typeof window === 'undefined' || !window.matchMedia(PHONE_QUERY).matches);
  // The phone layout: the tool open in the tray (and which of its controls),
  // the sheet that is open, and where the selected object's toolbar goes.
  const [phone, setPhone] = useState(() => typeof window !== 'undefined' && window.matchMedia(PHONE_QUERY).matches);
  const [phoneTool, setPhoneTool] = useState<string | null>(null);
  const [phoneSub, setPhoneSub] = useState<string | null>(null);
  const [sheet, setSheet] = useState<PhoneSheet | null>(null);
  const [miniPos, setMiniPos] = useState<{ left: number; top: number } | null>(null);
  // A layer being dragged, resized, turned or pinched.
  const [moving, setMoving] = useState(false);
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
  const clipInputRef = useRef<HTMLInputElement>(null);
  const soundInputRef = useRef<HTMLInputElement>(null);
  const placeInputRef = useRef<HTMLInputElement>(null);
  const textAreaRef = useRef<HTMLTextAreaElement>(null);

  const isTouch = useMemo(() => typeof window !== 'undefined' && window.matchMedia('(pointer: coarse)').matches, []);

  useEffect(() => {
    const mq = window.matchMedia(PHONE_QUERY);
    const onChange = () => setPhone(mq.matches);
    mq.addEventListener('change', onChange);
    return () => mq.removeEventListener('change', onChange);
  }, []);

  // Source image.
  useEffect(() => {
    if (!photoUrl) return;
    // A restored draft has loaded its photo and set its crop already.
    if (restoredPhoto.current === photoUrl) { restoredPhoto.current = null; return; }
    let cancelled = false;
    setLoadError(false);
    loadImage(photoUrl)
      .then(img => {
        if (cancelled) return;
        setSource(img);
        setCrop({ x: 0, y: 0, width: img.naturalWidth, height: img.naturalHeight });
      })
      .catch(() => { if (!cancelled) setLoadError(true); });
    return () => { cancelled = true; };
  }, [photoUrl]);

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
    Promise.all([
      api.getMedia({ kind: 'template', limit: 200 }),
      api.getImageDrafts().catch(() => [] as ImageDraft[]),
    ])
      .then(([all, saved]) => {
        if (cancelled) return;
        // A still editor leaves out video templates, whose animations and
        // clips it cannot play.
        const items = video ? all : all.filter(t => !isVideoTemplate(t.template));
        setTemplates(items);
        setDrafts(saved);
      })
      .catch(() => { if (!cancelled) { setTemplates([]); setDrafts(d => d ?? []); toast.error('Failed to load the templates'); } });
    return () => { cancelled = true; };
  }, [templatesOpened, templates, video]);

  // Media library: fetched once the Media tab has been opened, then again as
  // the search changes.
  const mediaOpened = tab === 'media' || media !== null;
  useEffect(() => {
    if (!mediaOpened) return;
    let cancelled = false;
    const t = setTimeout(() => {
      setMediaLoading(true);
      // Everything the library returns (its limit is 200), newest first; the
      // search only narrows it down.
      api.getMedia({ q: mediaQuery.trim(), kind: clipsView ? 'video' : watermarksOnly ? 'watermark' : 'image', limit: 200 })
        .then(items => { if (!cancelled) setMedia(clipsView ? items.filter(m => m.url) : items.filter(isImageMedia)); })
        .catch(() => { if (!cancelled) { setMedia(m => m ?? []); toast.error('Failed to load the media library'); } })
        .finally(() => { if (!cancelled) setMediaLoading(false); });
    }, 250);
    return () => { cancelled = true; clearTimeout(t); };
  }, [mediaOpened, mediaQuery, watermarksOnly, clipsView]);

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
    // None while painting: the brush works over the selected paint layer.
    const node = tab === 'crop'
      ? cropRectRef.current
      : (selectedId && (!playing || livePlay) && selectedId !== editingTextId && !(tab === 'brush' && brushMode === 'paint') ? nodeRefs.current.get(selectedId) ?? null : null);
    tr.nodes(node ? [node] : []);
    tr.getLayer()?.batchDraw();
  }, [tab, selectedId, layers, source, aspectId, brushMode, playing, livePlay, editingTextId]);

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
    const snap: Snapshot = { canvas, crop, aspectId, adjustments, layers, duration, audio };
    setUndoStack(stack => [...stack.slice(-(HISTORY_LIMIT - 1)), snap]);
    setRedoStack([]);
    setDraftClean(false);
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
    setDuration(snap.duration);
    setAudio(snap.audio);
    if (selectedId && !snap.layers.some(l => l.id === selectedId)) setSelectedId(null);
    lastChange.current = null;
    setDraftClean(false);
  };

  const undo = () => {
    const prev = undoStack[undoStack.length - 1];
    if (!prev || !crop) return;
    setUndoStack(stack => stack.slice(0, -1));
    setRedoStack(stack => [...stack, { canvas, crop, aspectId, adjustments, layers, duration, audio }]);
    restore(prev);
  };

  const redo = () => {
    const next = redoStack[redoStack.length - 1];
    if (!next || !crop) return;
    setRedoStack(stack => stack.slice(0, -1));
    setUndoStack(stack => [...stack, { canvas, crop, aspectId, adjustments, layers, duration, audio }]);
    restore(next);
  };

  // Every change to the edit goes through these, which record the undo step.
  const updateLayer = (id: string, patch: Partial<ImageLayer> | Partial<TextLayer> | Partial<ShapeLayer> | Partial<PaintLayer>, key?: string) => {
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

  // --- Video ----------------------------------------------------------------

  // Draws the stage again, for a clip's new frame.
  const redrawStage = useCallback(() => { contentRef.current?.getLayer()?.batchDraw(); }, []);

  const seek = (t: number) => jumpTo(clamp(t, 0, duration));

  // seek for a design just loaded, whose length is not in duration yet.
  const jumpTo = (t: number) => {
    timeRef.current = t;
    setTime(t);
  };

  const togglePlay = () => {
    if (playing) { setPlaying(false); return; }
    // From the start again once it has played to the end.
    if (timeRef.current >= duration - 1e-3) seek(0);
    setPlaying(true);
  };

  // Plays a stretch to show an effect just picked.
  const playFrom = (t: number) => {
    seek(t);
    setPlaying(true);
    setPlayRun(n => n + 1);
  };

  // Plays from the playhead to the end, in real time, or round and round on
  // repeat. Each frame moves on from timeRef, so a seek while it plays takes.
  useEffect(() => {
    if (!playing) return;
    let last = performance.now();
    let raf = 0;
    const tick = (now: number) => {
      let t = timeRef.current + (now - last) / 1000;
      last = now;
      if (t >= duration && loopRef.current) t = 0;
      if (t >= duration) {
        timeRef.current = duration;
        setTime(duration);
        setPlaying(false);
        return;
      }
      timeRef.current = t;
      setTime(t);
      // Clips and music play along; the canvas draws the clips' new frames.
      syncMedia(layersRef.current, audioRef.current, musicRef.current, t, duration, true, knownVideos.current, redrawStage);
      redrawStage();
      raf = requestAnimationFrame(tick);
    };
    raf = requestAnimationFrame(tick);
    return () => cancelAnimationFrame(raf);
  }, [playing, duration, playRun, redrawStage]);

  // Playback reads the latest layers and music.
  useEffect(() => {
    layersRef.current = layers;
    audioRef.current = audio;
  });

  // Paused, every clip shows its frame at the playhead, and nothing plays.
  useEffect(() => {
    if (!video || playing) return;
    syncMedia(layers, audio, musicRef.current, time, duration, false, knownVideos.current, redrawStage);
  }, [video, playing, time, layers, audio, duration, redrawStage]);

  // The music's player, for the preview.
  const musicSrc = audio?.src;
  useEffect(() => {
    if (!musicSrc) return;
    const el = new Audio(resolveUrl(musicSrc));
    el.preload = 'auto';
    musicRef.current = el;
    return () => {
      el.pause();
      if (musicRef.current === el) musicRef.current = null;
    };
  }, [musicSrc]);

  // Closing the editor stops what plays and releases the temporary uploads:
  // the clips and sounds nothing references by now (a post keeps only the
  // rendered video) are deleted.
  useEffect(() => {
    const known = knownVideos.current;
    const temp = tempUploads.current;
    return () => {
      stopMedia(known, musicRef.current);
      if (temp.size > 0) void api.releaseTemp([...temp]).catch(() => {});
    };
  }, []);

  // Clips and sounds uploaded for this video only.
  const uploadTemp = async (file: File) => {
    const { url } = await api.uploadTemp(file);
    tempUploads.current.add(url);
    return url;
  };

  // Puts a clip on the picture at the playhead, shown whole (filling the
  // frame when it has its shape) and for as long as it runs.
  const addClip = async (url: string, busyKey: string) => {
    if (!crop) return;
    setAddingMedia(busyKey);
    try {
      const v = await loadVideo(resolveUrl(url));
      const own = sourceSize(v);
      const r = own.width / own.height;
      let width = crop.width;
      let height = width / r;
      if (height > crop.height) { height = crop.height; width = height * r; }
      const length = Number.isFinite(v.duration) ? v.duration : 0;
      const start = timeRef.current < duration - 0.5 ? Math.round(timeRef.current * 10) / 10 : 0;
      const end = length > 0 ? Math.round((start + length) * 10) / 10 : duration;
      addLayer({
        id: newLayerId(), kind: 'image', origin: 'media', src: url, img: v,
        x: crop.x + crop.width / 2, y: crop.y + crop.height / 2, width, height, rotation: 0, opacity: 1,
        clip: { trimStart: 0, volume: 1, duration: length },
        anim: { ...NO_ANIM, start, ...(end < duration ? { end } : {}) },
      });
    } catch {
      toast.error('Failed to load that clip');
    } finally {
      setAddingMedia(null);
    }
  };

  // A clip from a file: for this video only, or kept in the library.
  const uploadClip = async (e: ChangeEvent<HTMLInputElement>) => {
    const file = e.target.files?.[0];
    e.target.value = '';
    if (!file) return;
    setUploadingMedia(true);
    try {
      if (keepInLibrary) {
        const item = await api.uploadMedia(file);
        setMedia(prev => [item, ...(prev ?? []).filter(m => m.id !== item.id)]);
        await addClip(item.url, item.id);
      } else {
        await addClip(await uploadTemp(file), 'upload');
      }
    } catch (err) {
      toast.error(err instanceof Error ? err.message : 'Upload failed');
    } finally {
      setUploadingMedia(false);
    }
  };

  const changeAudio = (next: AudioTrack | null, key?: string) => {
    checkpoint(key);
    setAudio(next);
  };

  // The library's sounds, once the music controls are shown.
  const soundsWanted = video && tab === 'animate' && !selectedId;
  useEffect(() => {
    if (!soundsWanted || sounds !== null) return;
    let cancelled = false;
    api.getMedia({ kind: 'audio', limit: 200 })
      .then(items => { if (!cancelled) setSounds(items.filter(m => m.url)); })
      .catch(() => { if (!cancelled) setSounds([]); });
    return () => { cancelled = true; };
  }, [soundsWanted, sounds]);

  // Sets the music, from the start of the video and of the file.
  const pickSound = async (url: string, name: string) => {
    const length = await soundDuration(resolveUrl(url));
    changeAudio({ src: url, name, start: 0, trimStart: 0, volume: 0.8, fadeIn: 0, fadeOut: Math.min(2, duration / 4), duration: length });
  };

  const uploadSound = async (e: ChangeEvent<HTMLInputElement>) => {
    const file = e.target.files?.[0];
    e.target.value = '';
    if (!file) return;
    setUploadingSound(true);
    try {
      const name = file.name.replace(/\.[^.]+$/, '');
      if (keepInLibrary) {
        const item = await api.uploadMedia(file, { name });
        setSounds(prev => [item, ...(prev ?? [])]);
        await pickSound(item.url, item.name);
      } else {
        await pickSound(await uploadTemp(file), name);
      }
    } catch (err) {
      toast.error(err instanceof Error ? err.message : 'Upload failed');
    } finally {
      setUploadingSound(false);
    }
  };

  const changeDuration = (d: number, key?: string) => {
    const next = Math.round(clamp(d, MIN_DURATION, MAX_DURATION) * 10) / 10;
    if (next === duration) return;
    checkpoint(key);
    setDuration(next);
    if (timeRef.current > next) seek(next);
  };

  const updateAnim = (l: EditorLayer, patch: Partial<LayerAnim>, key?: string) => {
    updateLayer(l.id, { anim: { ...layerAnim(l), ...patch } }, key);
  };

  // Moves the playhead to where a layer shows in full, unless it already
  // does (not still coming in), so a layer picked while hidden can be seen and changed.
  const seekInto = (l: EditorLayer) => {
    const { end } = layerSpan(l, duration);
    const settled = settledAt(l, duration);
    const t = timeRef.current;
    if (t >= settled && t < end) return;
    seek(settled);
  };

  // Just saved as a draft, or restored from one, nothing is lost by leaving.
  const dirty = draftClean ? false : canvasMode
    ? undoStack.length > 0
    : layers.length > 0 || hasAdjustments(adjustments) || (!!crop && !!source
      && (crop.x !== 0 || crop.y !== 0 || crop.width !== natW || crop.height !== natH));

  const handleCancel = useCallback(() => {
    if (saving) return;
    if (dirty && !window.confirm('Discard your changes to this image?')) return;
    onCancel();
  }, [dirty, saving, onCancel]);

  // Keyboard: Escape leaves, Delete removes the selected layer, the arrow
  // keys move it (Shift for bigger steps), Enter types into a selected text,
  // Ctrl/Cmd+Z and Ctrl/Cmd+Shift+Z (or Ctrl+Y) undo and redo — unless
  // typing, where those keys belong to the field.
  useEffect(() => {
    const onKey = (e: KeyboardEvent) => {
      const t = e.target as HTMLElement | null;
      const typing = !!t && (t.tagName === 'INPUT' || t.tagName === 'TEXTAREA' || t.tagName === 'SELECT' || t.isContentEditable);
      const mod = e.ctrlKey || e.metaKey;
      const k = e.key.toLowerCase();
      if (mod && k === 's') {
        e.preventDefault();
        void saveDraft();
        return;
      }
      if (mod && !typing && (k === 'z' || k === 'y')) {
        e.preventDefault();
        if (k === 'y' || e.shiftKey) redo();
        else undo();
        return;
      }
      if (video && e.key === ' ' && !typing && t?.tagName !== 'BUTTON') {
        e.preventDefault();
        togglePlay();
        return;
      }
      if (e.key === 'Escape') {
        // Ends typing on the picture, keeping the text selected.
        if (editingTextId) finishTextEdit();
        else if (selectedId) setSelectedId(null);
        else handleCancel();
      } else if ((e.key === 'Delete' || e.key === 'Backspace') && !typing && selectedId) {
        e.preventDefault();
        deleteSelected();
      } else if (e.key.startsWith('Arrow') && !typing && !mod && selected && tab !== 'crop' && !painting) {
        e.preventDefault();
        nudgeSelected(e.key, e.shiftKey);
      } else if (e.key === 'Enter' && !typing && t?.tagName !== 'BUTTON' && !mod && selected?.kind === 'text' && tab !== 'crop' && !painting) {
        e.preventDefault();
        startTextEdit(selected.id);
      }
    };
    window.addEventListener('keydown', onKey);
    return () => window.removeEventListener('keydown', onKey);
  });

  const switchTab = (t: Tab) => {
    setTab(t);
    setBrushAt(null);
    if (t === 'crop') setSelectedId(null);
    else if (t === 'text' && selected?.kind !== 'text') setSelectedId(null);
    else if (t === 'media' && selected?.kind !== 'image') setSelectedId(null);
    else if (t === 'shapes' && selected?.kind !== 'shape') setSelectedId(null);
    // The brush paints into the selected paint layer, or starts a new one.
    else if (t === 'brush') {
      setBrushMode('paint');
      if (selected?.kind !== 'paint') setSelectedId(null);
    }
  };

  // Selecting a layer opens the tab that edits it. An image layer keeps the
  // tab it is on when that tab handles images too.
  const selectLayer = (l: EditorLayer) => {
    if (editingTextId && editingTextId !== l.id) finishTextEdit();
    setSelectedId(l.id);
    // A phone has no tabs: the bar follows the selection, and an open tool
    // stays only where the new object has it too.
    if (phone) {
      if (video) seekInto(l);
      if (phoneTool && !phoneToolIds(l).includes(phoneTool)) closePhoneTool();
      if (tab === 'crop') setTab(restTab);
      return;
    }
    if (video) {
      seekInto(l);
      // The Animate tab stays, to set the timing of one layer after another.
      if (tab === 'animate') return;
    }
    if (l.kind === 'text') setTab('text');
    else if (l.kind === 'shape') setTab('shapes');
    else if (l.kind === 'paint') {
      // From another tab, picked to be moved or removed; on the Brush tab
      // (from the Objects list) the mode stays, so while painting the next
      // strokes go into it.
      if (tab !== 'brush') setBrushMode('move');
      setTab('brush');
    }
    else setTab('media');
  };

  // --- Phone ----------------------------------------------------------------

  // The tab the stage rests on while no tool is open: one that neither crops
  // nor paints.
  const restTab: Tab = canvasMode ? 'canvas' : 'filters';

  // The tray's tools for the selected object, or for none.
  const phoneToolIds = (l: EditorLayer | null): string[] => {
    if (!l) return ['crop', 'filters', 'canvas', 'shapes', 'brush'];
    switch (l.kind) {
      case 'text': return ['font', 'size', 'color', 'align', 'style', 'blend', 'animate'];
      case 'image': return ['clip', 'opacity', 'blend', 'corners', 'border', 'animate'];
      case 'shape': return ['shape', 'fill', 'border', 'corners', 'opacity', 'blend', 'animate'];
      default: return ['brush', 'blend', 'animate'];
    }
  };

  const closePhoneTool = () => {
    setPhoneTool(null);
    setPhoneSub(null);
    setBrushAt(null);
    if (tab === 'crop' || tab === 'brush') setTab(restTab);
  };

  // Opens a tool in the tray, or puts it away when it is open already.
  const openPhoneTool = (id: string, sub: string | null = null) => {
    if (phoneTool === id) { closePhoneTool(); return; }
    setPhoneTool(id);
    setPhoneSub(sub);
    if (id === 'crop') {
      setSelectedId(null);
      setTab('crop');
    } else if (id === 'brush') {
      setTab('brush');
      setBrushMode('paint');
      if (selected?.kind !== 'paint') setSelectedId(null);
    } else if (tab === 'crop' || tab === 'brush') {
      setTab(restTab);
    }
  };

  // The tab behind a sheet loads what it lists.
  const openSheet = (s: PhoneSheet) => {
    if (editingTextId) finishTextEdit();
    setPhoneTool(null);
    setPhoneSub(null);
    setSheet(s);
    setTab(s === 'templates' ? 'templates' : s === 'media' ? 'media' : s === 'video' ? 'animate' : restTab);
    if (s === 'video') setSelectedId(null);
  };

  const closeSheet = () => {
    setSheet(null);
    setTab(restTab);
  };

  // Leaving the phone layout, or losing the object a tool was for, puts the
  // tool away; the stage never crops or paints without its tool open.
  useEffect(() => {
    if (phoneTool && (!phone || !phoneToolIds(selected).includes(phoneTool))) closePhoneTool();
    else if (phone && ((tab === 'crop' && phoneTool !== 'crop') || (tab === 'brush' && phoneTool !== 'brush'))) setTab(restTab);
    if (!phone && sheet) setSheet(null);
    // The helpers it calls are new on every render; what they read is listed.
    // eslint-disable-next-line react-hooks/exhaustive-deps
  }, [phone, phoneTool, selected, tab, sheet]);

  // A blank canvas opens on the templates.
  const templatesShown = useRef(false);
  useEffect(() => {
    if (templatesShown.current || !phone || !canvasMode || template) return;
    templatesShown.current = true;
    setSheet('templates');
  }, [phone, canvasMode, template]);

  // The media sheet closes once what was picked (or uploaded) is on the
  // picture, and the video sheet once an uploaded clip is.
  const layerCount = useRef(layers.length);
  useEffect(() => {
    if ((sheet === 'media' || sheet === 'video') && layers.length > layerCount.current) setSheet(null);
    layerCount.current = layers.length;
  }, [layers.length, sheet]);

  // A copy of the selected object, a little offset, right above it. A clip
  // is not copied: two layers cannot play one video.
  const canDuplicate = !!selected && !(selected.kind === 'image' && selected.clip);
  const duplicateSelected = () => {
    if (!crop || !selected || !canDuplicate) return;
    const off = Math.round(Math.min(crop.width, crop.height) * 0.03);
    const copy = { ...selected, id: newLayerId(), x: selected.x + off, y: selected.y + off } as EditorLayer;
    checkpoint();
    setLayers(prev => {
      const i = prev.findIndex(l => l.id === selected.id);
      return [...prev.slice(0, i + 1), copy, ...prev.slice(i + 1)];
    });
    setSelectedId(copy.id);
  };

  // Moves the selected object one place up (1) or down (-1) the stack.
  const selectedIndex = selected ? layers.indexOf(selected) : -1;
  const restackSelected = (dir: 1 | -1) => {
    const i = selectedIndex;
    const j = i + dir;
    if (i < 0 || j < 0 || j >= layers.length) return;
    const next = [...layers];
    [next[i], next[j]] = [next[j], next[i]];
    reorderLayers(next);
  };

  // The selected object's toolbar sits under it, or above it where there is
  // no room below (clear of the rotate handle on top), and hides while
  // anything else is going on. It runs after every render: the object moves
  // with its layer, the crop, the zoom and a video's playhead alike; it
  // settles once the position stops changing.
  // eslint-disable-next-line react-hooks/exhaustive-deps
  useLayoutEffect(() => {
    const show = phone && selected && !phoneTool && !moving && !playing && !editingTextId && tab !== 'crop' && !painting && size.w > 0;
    const node = show ? nodeRefs.current.get(selected.id) : undefined;
    let next: { left: number; top: number } | null = null;
    if (node) {
      const r = node.getClientRect();
      const { width: mw, height: mh } = MINI_TOOLBAR;
      const left = Math.round(clamp(r.x + r.width / 2 - mw / 2, 8, Math.max(8, size.w - mw - 8)));
      let top = r.y + r.height + 14;
      if (top + mh > size.h - 8) top = r.y - mh - 56;
      if (top < 8) top = 8;
      next = { left, top: Math.round(top) };
    }
    if (next?.left !== miniPos?.left || next?.top !== miniPos?.top) setMiniPos(next);
  });

  // Every layer whose box holds a point of the stage, top one first.
  const layersUnderPoint = (pos: { x: number; y: number }): EditorLayer[] => layers.filter(l => {
    const node = nodeRefs.current.get(l.id);
    if (!node || !node.isVisible()) return false;
    const p = node.getAbsoluteTransform().copy().invert().point(pos);
    const r = node.getClientRect({ skipTransform: true });
    return p.x >= r.x && p.x <= r.x + r.width && p.y >= r.y && p.y <= r.y + r.height;
  }).reverse();

  // On a touch screen a second tap on the same spot, after a pause (a quick
  // one is a double tap), selects the next object down under it, so one
  // covered by another can be reached on the picture.
  const press = useRef<{ x: number; y: number; wasSelected: boolean } | null>(null);
  const lastTap = useRef<{ x: number; y: number; at: number } | null>(null);
  const onStagePointerUp = (e: Konva.KonvaEventObject<PointerEvent>) => {
    const p = press.current;
    press.current = null;
    const pos = e.target.getStage()?.getPointerPosition();
    if (!p || !pos || !isTouch || Math.hypot(pos.x - p.x, pos.y - p.y) > 8) return;
    const now = Date.now();
    const last = lastTap.current;
    lastTap.current = { x: pos.x, y: pos.y, at: now };
    if (!p.wasSelected || !last || now - last.at < 400 || now - last.at > 1500 || Math.hypot(pos.x - last.x, pos.y - last.y) > 14) return;
    const under = layersUnderPoint(pos);
    if (under.length < 2) return;
    const i = under.findIndex(l => l.id === selectedId);
    selectLayer(under[(i + 1) % under.length]);
  };

  // Two fingers on the picture resize and turn the selected object, and move
  // it with them.
  const pinch = useRef<{ id: string; dist: number; angle: number; mid: { x: number; y: number }; from: EditorLayer } | null>(null);
  const touchPoints = (e: Konva.KonvaEventObject<TouchEvent>) => {
    const box = e.target.getStage()?.container().getBoundingClientRect();
    if (!box) return null;
    const [a, b] = [e.evt.touches[0], e.evt.touches[1]];
    if (!a || !b) return null;
    const pa = { x: a.clientX - box.left, y: a.clientY - box.top };
    const pb = { x: b.clientX - box.left, y: b.clientY - box.top };
    return {
      dist: Math.hypot(pb.x - pa.x, pb.y - pa.y),
      angle: Math.atan2(pb.y - pa.y, pb.x - pa.x),
      mid: { x: (pa.x + pb.x) / 2, y: (pa.y + pb.y) / 2 },
    };
  };
  const onStageTouchStart = (e: Konva.KonvaEventObject<TouchEvent>) => {
    if (e.evt.touches.length !== 2 || !selected || selected.kind === 'paint' || !layersInteractive) return;
    const t = touchPoints(e);
    if (!t || t.dist < 10) return;
    const node = nodeRefs.current.get(selected.id);
    if (node?.isDragging()) node.stopDrag();
    node?.draggable(false);
    trRef.current?.stopTransform();
    startGesture();
    setGuides(null);
    setMoving(true);
    pinch.current = { id: selected.id, ...t, from: selected };
  };
  const onStageTouchMove = (e: Konva.KonvaEventObject<TouchEvent>) => {
    const p = pinch.current;
    if (!p) return;
    const t = touchPoints(e);
    if (!t) return;
    e.evt.preventDefault();
    const s = t.dist / p.dist;
    const turn = ((t.angle - p.angle) * 180) / Math.PI;
    const shift = { x: (t.mid.x - p.mid.x) / scale, y: (t.mid.y - p.mid.y) / scale };
    updateLayer(p.id, pinchedLayer(p.from, s, turn, shift), `gesture:pinch:${p.id}`);
  };
  const endPinch = () => {
    const p = pinch.current;
    if (!p) return;
    pinch.current = null;
    nodeRefs.current.get(p.id)?.draggable(true);
    setMoving(false);
  };
  const onStageTouchEnd = (e: Konva.KonvaEventObject<TouchEvent>) => {
    if (e.evt.touches.length < 2) endPinch();
  };

  // A phone cancels a touch it takes over (an edge swipe, a long press, a
  // notification). Konva's Transformer only ends a resize or turn on
  // touchend, so after a cancel it would go on transforming: the next
  // finger anywhere resized the object by the old handle, and the hit areas
  // were no longer redrawn, so taps landed where handles had been. A pinch
  // left the object undraggable the same way.
  useEffect(() => {
    const onCancel = () => {
      if (trRef.current?.isTransforming()) trRef.current.stopTransform();
      endPinch();
    };
    window.addEventListener('touchcancel', onCancel, true);
    window.addEventListener('pointercancel', onCancel, true);
    return () => {
      window.removeEventListener('touchcancel', onCancel, true);
      window.removeEventListener('pointercancel', onCancel, true);
    };
  });

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
      if (video && t.duration) {
        const length = clamp(t.duration, MIN_DURATION, MAX_DURATION);
        setDuration(length);
        setPlaying(false);
        jumpTo(openingTime(added, length));
      }
      if (video && t.audio) setAudio(t.audio);
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

  // A built-in template: on a photo without its own backdrop picture, in a
  // video with its layers' timing.
  const pickSystemTemplate = async (s: SystemTemplate) => {
    if (canvasMode && layers.length > 0 && !window.confirm('Replace your design with this template?')) return;
    setApplyingTemplate(s.id);
    try {
      await applyTemplate(s.build({ video, photo: !canvasMode }));
    } finally {
      setApplyingTemplate(null);
    }
  };

  // --- Drafts ---------------------------------------------------------------

  // The photo as the server stores it: our own uploads by their path, a local
  // file (a blob URL) uploaded first.
  const draftPhotoUrl = async (src: string): Promise<string> => {
    for (const origin of [apiBase, window.location.origin]) {
      if (origin && src.startsWith(origin + '/api/uploads/')) return src.slice(origin.length);
    }
    if (!src.startsWith('blob:') && !src.startsWith('data:')) return src;
    if (storedPhoto.current?.from === src) return storedPhoto.current.url;
    const blob = await (await fetch(src)).blob();
    const ext = ({ 'image/png': 'png', 'image/webp': 'webp', 'image/gif': 'gif' } as Record<string, string>)[blob.type] ?? 'jpg';
    const { url } = await api.uploadImage(new File([blob], `photo.${ext}`, { type: blob.type || 'image/jpeg' }));
    storedPhoto.current = { from: src, url };
    return url;
  };

  // Saves the whole edit to come back to: as a new draft the first time
  // (asking for a name), then over the same one.
  const saveDraft = async () => {
    if (!ready || !crop || saving || savingDraft) return;
    let name: string | undefined;
    if (!draft) {
      const fallback = `Draft ${new Date().toLocaleString([], { dateStyle: 'medium', timeStyle: 'short' })}`;
      const answer = window.prompt('Name this draft', fallback);
      if (answer === null) return;
      name = answer.trim() || fallback;
    }
    setSavingDraft(true);
    try {
      // Layers stay in the picture's own pixels, so the draft comes back
      // exactly as it was.
      const template = toTemplate({ x: 0, y: 0, width: natW, height: natH }, layers, canvas?.background ?? '#ffffff', video ? duration : undefined, audio);
      const design: EditorDraft = canvasMode
        ? { version: 1, template }
        : { version: 1, template, photo: { src: await draftPhotoUrl(photoUrl!), crop, aspectId, adjustments } };
      const preview = await renderEdit({
        source: canvasMode ? null : source,
        background: canvas?.background,
        crop,
        adjustments: canvasMode ? NO_ADJUSTMENTS : adjustments,
        // A video's preview is the frame at the playhead.
        layers: video ? layersAt(layers, time, duration) : layers,
        maxSide: 1080,
        quality: 0.85,
      });
      let saved: ImageDraft;
      try {
        saved = await api.saveImageDraft(draft?.id ?? null, preview, design, name);
      } catch (err) {
        // Deleted meanwhile (another tab): keep the work as a new one.
        if (!draft || !(err instanceof Error) || !/not found/i.test(err.message)) throw err;
        saved = await api.saveImageDraft(null, preview, design, draft.name);
      }
      setDraft({ id: saved.id, name: saved.name });
      setDraftClean(true);
      setDrafts(prev => (prev === null ? prev : [saved, ...prev.filter(d => d.id !== saved.id)]));
      toast.success(`Saved as draft "${saved.name}". Open it again under Templates, Your drafts, the next time you create ${video ? 'a video' : 'an image'}.`);
    } catch (err) {
      const e = err instanceof Error ? err : null;
      toast.error(e?.name === 'SecurityError'
        ? 'This image cannot be saved (it was loaded from another site)'
        : (e?.message || 'Failed to save the draft'));
    } finally {
      setSavingDraft(false);
    }
  };

  // Keeps the video's design in the media library as a template, its length,
  // music and animations included, for any video to start from.
  const saveAsTemplate = async () => {
    if (!ready || !crop || saving || savingTemplate) return;
    if (layers.length === 0) { toast.error('Add something to the video first'); return; }
    const answer = window.prompt('Name this template', draft?.name ?? '');
    if (answer === null) return;
    const name = answer.trim() || `Template ${new Date().toLocaleString([], { dateStyle: 'medium', timeStyle: 'short' })}`;
    setSavingTemplate(true);
    try {
      const template = toTemplate(crop, layers, canvas?.background ?? '#ffffff', duration, audio);
      const preview = await renderEdit({
        source: canvasMode ? null : source,
        background: canvas?.background,
        crop,
        adjustments: canvasMode ? NO_ADJUSTMENTS : adjustments,
        // The frame at the playhead, as for a draft.
        layers: layersAt(layers, time, duration),
        maxSide: 1080,
        quality: 0.85,
      });
      const item = await api.createMediaTemplate(preview, template, { name });
      setTemplates(prev => (prev === null ? prev : [item, ...prev]));
      toast.success(`Saved as template "${item.name}". Find it under Templates and in the Media Library.`);
    } catch (err) {
      toast.error(err instanceof Error ? err.message : 'Failed to save the template');
    } finally {
      setSavingTemplate(false);
    }
  };

  // Puts a draft back exactly as it was saved, photo, crop and filters
  // included, replacing the current edit. Undo starts afresh from it.
  const restoreDraft = async (item: ImageDraft) => {
    if (restoringDraft) return;
    if ((dirty || layers.length > 0) && !window.confirm('Replace what you are working on with this draft?')) return;
    setRestoringDraft(item.id);
    try {
      const full = await api.getImageDraft(item.id);
      const d = parseDraft(full.design);
      if (!d) { toast.error('This draft cannot be read'); return; }
      await fontsReady.current;
      const box = { x: 0, y: 0, width: d.template.width, height: d.template.height };
      const img = d.photo ? await loadImage(resolveUrl(d.photo.src)) : null;
      const { layers: restored, missing } = await instantiateTemplate(d.template, box, resolveUrl, { placeholders: false });
      if (d.photo && img) {
        if (d.photo.src !== photoUrl) restoredPhoto.current = d.photo.src;
        setPhotoUrl(d.photo.src);
        setSource(img);
        setLoadError(false);
        setCanvas(null);
        setCrop(d.photo.crop);
        setAspectId(d.photo.aspectId || 'free');
        setAdjustments({ ...NO_ADJUSTMENTS, ...d.photo.adjustments });
      } else {
        setPhotoUrl(undefined);
        setSource(null);
        setCanvas({ width: box.width, height: box.height, background: d.template.background });
        setCrop(box);
        setAspectId('free');
        setAdjustments(NO_ADJUSTMENTS);
      }
      setLayers(restored);
      if (video) {
        const length = clamp(d.template.duration ?? DEFAULT_DURATION, MIN_DURATION, MAX_DURATION);
        setDuration(length);
        setAudio(d.template.audio ?? null);
        setPlaying(false);
        jumpTo(openingTime(restored, length));
      }
      setSelectedId(null);
      setUndoStack([]);
      setRedoStack([]);
      lastChange.current = null;
      setDraft({ id: full.id, name: full.name });
      setDraftClean(missing === 0);
      if (missing) toast.error(missing === 1 ? 'One image of the draft could not be loaded' : `${missing} images of the draft could not be loaded`);
    } catch (err) {
      toast.error(err instanceof Error ? err.message : 'Failed to open the draft');
    } finally {
      setRestoringDraft(null);
    }
  };

  const deleteDraft = async (item: ImageDraft) => {
    if (!window.confirm(`Delete the draft "${item.name}"?`)) return;
    try {
      await api.deleteImageDraft(item.id);
      setDrafts(prev => prev?.filter(d => d.id !== item.id) ?? prev);
      if (draft?.id === item.id) { setDraft(null); setDraftClean(false); }
    } catch (err) {
      toast.error(err instanceof Error ? err.message : 'Failed to delete the draft');
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
      // frame would.
      const fills = Math.abs(r - cropRatio) / cropRatio < 0.03;
      if (item.watermark && !fills) {
        // A watermark logo goes small into the bottom-right corner.
        const side = Math.min(crop.width, crop.height) * 0.3;
        const width = r >= 1 ? side : side * r;
        const height = width / r;
        const margin = Math.min(crop.width, crop.height) * 0.04;
        addLayer({
          id: newLayerId(), kind: 'image', origin: 'watermark', src: item.url, img,
          x: crop.x + crop.width - margin - width / 2, y: crop.y + crop.height - margin - height / 2,
          width, height, rotation: 0, opacity: 1,
        });
        return;
      }
      // Anything else goes in the middle at about half size, ready to be
      // placed.
      const share = fills ? 1 : 0.5;
      let width = crop.width * share;
      let height = width / r;
      if (height > crop.height * share) { height = crop.height * share; width = height * r; }
      addLayer({
        id: newLayerId(), kind: 'image', origin: item.watermark ? 'watermark' : 'media', src: item.url, img,
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
      // An upload in the Watermarks view is marked as one, as on the Media
      // Library page.
      const item = await api.uploadMedia(file, { watermark: watermarksOnly });
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
    const own = sourceSize(selected.img);
    const r = own.width / own.height;
    let width = crop.width;
    let height = width / r;
    if (height > crop.height) { height = crop.height; width = height * r; }
    updateLayer(selected.id, {
      width, height, rotation: 0, srcCrop: undefined,
      x: crop.x + crop.width / 2, y: crop.y + crop.height / 2,
    });
  };

  // --- Pasted images --------------------------------------------------------

  // Puts an image file on the picture. With `replace` and an image layer
  // selected it takes that layer's place: same frame, corners and border,
  // the image cropped to fill it. Otherwise it goes in the middle like a
  // media image, whole. The file is uploaded first, so a design saved as a
  // template refers to it by URL.
  const placeImageFile = async (file: File, { replace }: { replace: boolean }) => {
    if (!crop || placingImage) return;
    const target = replace && selected?.kind === 'image' ? selected : null;
    setPlacingImage(true);
    try {
      const { url } = await api.uploadImage(file);
      const img = await loadImage(resolveUrl(url));
      if (target) {
        updateLayer(target.id, {
          img, src: url, origin: 'media', clip: undefined,
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
      if (tab !== 'media') setTab('media');
    } catch (err) {
      toast.error(err instanceof Error ? err.message : 'Failed to add the image');
    } finally {
      setPlacingImage(false);
    }
  };

  const pickPlaceFile = (e: ChangeEvent<HTMLInputElement>) => {
    const file = e.target.files?.[0];
    e.target.value = '';
    // The Replace button: always the selected layer's place.
    if (file) void placeImageFile(file, { replace: true });
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
        // Only a template's placeholder is swapped out by a paste; an image
        // the user added, still selected from adding it, stays and the
        // paste becomes a layer of its own.
        void placeImageFile(file, { replace: selected?.kind === 'image' && !!selected.placeholder });
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
    // Type straight onto the picture, the placeholder selected so typing
    // replaces it. Not on touch: the keyboard would cover the picture
    // straight away.
    if (!isTouch) startTextEdit(base.id, { selectAll: true });
  };

  // Instagram's interface covers a full-screen 9:16 frame only, so the zone
  // (and its switch) shows while the crop or canvas has that shape.
  const zone = safeZone && crop && Math.abs(crop.width * 16 - crop.height * 9) <= crop.height * 0.2 ? safeZone : undefined;

  const toggleSafeZone = () => {
    setShowSafeZone(on => {
      try { localStorage.setItem(SAFE_ZONE_STORAGE_KEY, on ? 'off' : 'on'); } catch { /* per-viewer convenience only */ }
      return !on;
    });
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

  const startMove = () => {
    startGesture();
    setMoving(true);
  };

  const onLayerDragEnd = (l: EditorLayer, e: Konva.KonvaEventObject<DragEvent>) => {
    setMoving(false);
    setGuides(null);
    updateLayer(l.id, { x: e.target.x(), y: e.target.y() });
  };

  // Re-centres the selected layer on the crop, keeping its rotation.
  const centreSelected = () => {
    if (!crop || !selected) return;
    if (selected.kind === 'image' || selected.kind === 'shape') {
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

  // Moves the selected layer by one screen pixel per arrow key press, ten
  // with Shift, whatever the zoom. Held or repeated presses are one undo step.
  const nudgeSelected = (key: string, big: boolean) => {
    if (!selected) return;
    const step = (big ? 10 : 1) / scale;
    const dx = key === 'ArrowLeft' ? -step : key === 'ArrowRight' ? step : 0;
    const dy = key === 'ArrowUp' ? -step : key === 'ArrowDown' ? step : 0;
    if (!dx && !dy) return;
    updateLayer(selected.id, { x: selected.x + dx, y: selected.y + dy }, `nudge:${selected.id}`);
  };

  // Typing on the picture: the text is swapped for a field drawn in its
  // place, in its font, size, colour and angle. One edit is one undo step.
  const startTextEdit = (id: string, { selectAll = false } = {}) => {
    startGesture();
    editingTextRef.current = id;
    setEditingTextId(id);
    setTimeout(() => {
      const el = textAreaRef.current;
      if (!el) return;
      el.focus();
      if (selectAll) el.select();
      else el.setSelectionRange(el.value.length, el.value.length);
    }, 0);
  };

  // New text keeps the point its lines are aligned on (a centred text grows
  // to both sides), the first line staying where it is.
  const retypeText = (l: TextLayer, text: string): Pick<TextLayer, 'text' | 'x' | 'y'> => {
    const align = l.align ?? 'center';
    const share = align === 'left' ? 0 : align === 'right' ? 1 : 0.5;
    const grow = (measureText({ ...l, text }).width - measureText(l).width) * share;
    const rad = (l.rotation * Math.PI) / 180;
    return { text, x: l.x - grow * Math.cos(rad), y: l.y - grow * Math.sin(rad) };
  };

  // A text left empty is removed, within the same undo step.
  const finishTextEdit = () => {
    // Escape and the field's blur can both end the same edit.
    const id = editingTextRef.current;
    if (!id) return;
    editingTextRef.current = null;
    setEditingTextId(null);
    const l = layers.find(x => x.id === id);
    if (l?.kind === 'text' && !l.text.trim()) {
      checkpoint(`gesture:text:${id}`);
      setLayers(prev => prev.filter(x => x.id !== id));
      setSelectedId(s => (s === id ? null : s));
    }
    startGesture();
  };

  // An undo or a delete can take the text away while it is typed into.
  useEffect(() => {
    if (editingTextId && !layers.some(l => l.id === editingTextId)) {
      editingTextRef.current = null;
      setEditingTextId(null);
    }
  }, [editingTextId, layers]);

  // Folds a finished resize into the layer itself: an image's size, a text's
  // font size, so the text stays crisp and the size controls stay true.
  const onLayerTransformEnd = (l: EditorLayer, node: Konva.Node) => {
    setMoving(false);
    const sx = node.scaleX();
    const sy = node.scaleY();
    node.setAttrs({ scaleX: 1, scaleY: 1 });
    if (l.kind === 'image' || l.kind === 'shape') {
      const width = Math.max(4, l.width * sx);
      const height = Math.max(4, l.height * sy);
      node.setAttrs({ offsetX: width / 2, offsetY: height / 2 });
      updateLayer(l.id, { x: node.x(), y: node.y(), rotation: node.rotation(), width, height });
    } else if (l.kind === 'text') {
      updateLayer(l.id, { x: node.x(), y: node.y(), rotation: node.rotation(), fontSize: Math.max(MIN_FONT_SIZE, Math.round(l.fontSize * sx)) });
    }
  };

  // --- Shapes ---------------------------------------------------------------

  const addShape = (shape: ShapeKind) => {
    if (!crop) return;
    const side = Math.round(Math.min(crop.width, crop.height) * 0.4);
    addLayer({
      id: newLayerId(), kind: 'shape', shape,
      x: crop.x + crop.width / 2, y: crop.y + crop.height / 2,
      width: shape === 'rect' ? Math.round(side * 1.4) : side, height: side, rotation: 0, opacity: 1,
      fill: '#ffffff', stroke: '#000000', strokeWidth: Math.max(2, Math.round(Math.min(crop.width, crop.height) * 0.01)),
    });
  };

  // --- Brush ----------------------------------------------------------------

  const painting = tab === 'brush' && brushMode === 'paint';

  // The brush follows the picture's size until it is set: a tenth of the
  // shorter side, its edge blurred as far again.
  const minSide = crop ? Math.min(crop.width, crop.height) : 1000;
  const maxBrush = Math.max(50, Math.round(minSide * 0.6));
  const brushSize = brush.size ?? Math.max(1, Math.round(minSide * 0.1));
  const brushFeather = brush.feather ?? Math.round(minSide * 0.1);

  // The pointer in picture pixels.
  const pointerInPicture = (stage: Konva.Stage | null) => {
    const p = stage?.getPointerPosition();
    return p ? { x: (p.x - gx) / scale, y: (p.y - gy) / scale } : null;
  };

  const startStroke = (stage: Konva.Stage | null) => {
    const p = pointerInPicture(stage);
    if (!p) return;
    const st: PaintStroke = { points: [p.x, p.y], size: brushSize, feather: brushFeather, color: brush.color, opacity: brush.opacity };
    drawingRef.current = st;
    setDrawing(st);
  };

  const extendStroke = (stage: Konva.Stage | null) => {
    const p = pointerInPicture(stage);
    if (!p) return;
    setBrushAt(p);
    const st = drawingRef.current;
    if (!st) return;
    // Samples closer than a few pixels on screen add nothing but size.
    const n = st.points.length;
    if (Math.hypot(p.x - st.points[n - 2], p.y - st.points[n - 1]) < 3 / scale) return;
    const next = { ...st, points: [...st.points, Math.round(p.x * 10) / 10, Math.round(p.y * 10) / 10] };
    drawingRef.current = next;
    setDrawing(next);
  };

  // A finished stroke goes into the selected paint layer, or a new one on top.
  const finishStroke = () => {
    const st = drawingRef.current;
    drawingRef.current = null;
    setDrawing(null);
    if (!st) return;
    const target = selected?.kind === 'paint' ? selected : null;
    if (target) {
      const own = { ...st, points: st.points.map((v, i) => v - (i % 2 === 0 ? target.x : target.y)) };
      updateLayer(target.id, { strokes: [...target.strokes, own] });
    } else {
      addLayer({ id: newLayerId(), kind: 'paint', x: 0, y: 0, strokes: [st] });
    }
  };

  // A stroke ends wherever the pointer is let go, on the picture or not.
  useEffect(() => {
    if (!drawing) return;
    const end = () => finishStroke();
    window.addEventListener('pointerup', end);
    window.addEventListener('pointercancel', end);
    return () => {
      window.removeEventListener('pointerup', end);
      window.removeEventListener('pointercancel', end);
    };
  });

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
    if (video && !designOnly) { await saveVideo(); return; }
    if (video) setPlaying(false);
    setSaving(true);
    try {
      const blob = await renderEdit({
        source: canvasMode ? null : source,
        background: canvas?.background,
        crop,
        adjustments: canvasMode ? NO_ADJUSTMENTS : adjustments,
        // A video template's preview is the frame at the playhead.
        layers: video ? layersAt(layers, time, duration) : layers,
      });
      await onSave(blob, video
        ? toTemplate(crop, layers, canvas?.background ?? '#ffffff', duration, audio)
        : toTemplate(crop, layers, canvas?.background ?? '#ffffff'));
    } catch (err) {
      const e = err instanceof Error ? err : null;
      toast.error(e?.name === 'SecurityError'
        ? 'This image cannot be exported (it was loaded from another site)'
        : (e?.message || 'Failed to save the image'));
      setSaving(false);
    }
  };

  // Renders the video frame by frame and hands over the MP4.
  const saveVideo = async () => {
    if (!crop) return;
    setPlaying(false);
    setSaving(true);
    try {
      if (!(await canExportVideo(crop))) {
        toast.error('This browser cannot export video. Use a current Chrome, Edge or Safari.');
        setSaving(false);
        return;
      }
      setExportProgress(0);
      const blob = await renderVideo({
        source: canvasMode ? null : source,
        background: canvas?.background,
        crop,
        adjustments: canvasMode ? NO_ADJUSTMENTS : adjustments,
        layers,
        duration,
        audio,
        resolve: resolveUrl,
        onProgress: p => setExportProgress(Math.floor(p * 100)),
      });
      await onSave(blob, toTemplate(crop, layers, canvas?.background ?? '#ffffff', duration, audio));
    } catch (err) {
      const e = err instanceof Error ? err : null;
      toast.error(e?.name === 'SecurityError'
        ? 'This video cannot be exported (an image was loaded from another site)'
        : (e?.message || 'Failed to export the video'));
      setSaving(false);
    } finally {
      setExportProgress(null);
    }
  };

  // --- Render ---------------------------------------------------------------

  // The selected layer's node, when the pointer is inside it: its own box,
  // rotation included, whatever lies on top of it.
  const selectedNodeAtPointer = (stage: Konva.Stage | null): Konva.Node | null => {
    const node = selectedId ? nodeRefs.current.get(selectedId) : undefined;
    const pos = stage?.getPointerPosition();
    if (!node || !pos) return null;
    const p = node.getAbsoluteTransform().copy().invert().point(pos);
    const r = node.getClientRect({ skipTransform: true });
    return p.x >= r.x && p.x <= r.x + r.width && p.y >= r.y && p.y <= r.y + r.height ? node : null;
  };

  // A press inside the selected layer keeps it selected and drags it, even
  // where another layer covers it (a full-frame image on top, say), so a
  // layer picked in the Objects list can be moved on the picture. The layer
  // on top must not drag meanwhile; the drag starts once the pointer moves.
  // A plain tap or click selects the layer on top after all, or nothing
  // inside a large selected layer could be picked on the picture.
  // The layer last picked in the Objects list, while it stays selected.
  const pickedInList = useRef<string | null>(null);
  const pickInList = (l: EditorLayer) => {
    selectLayer(l);
    pickedInList.current = l.id;
  };

  const holdSelected = (node: Konva.Node, cover: Konva.Node, start: PointerEvent, tapped: EditorLayer) => {
    cover.draggable(false);
    const stage = node.getStage();
    const pressedAt = stage?.getPointerPosition();
    let moved = false;
    const move = (ev: PointerEvent) => {
      if (Math.hypot(ev.clientX - start.clientX, ev.clientY - start.clientY) < (isTouch ? 8 : 3)) return;
      moved = true;
      window.removeEventListener('pointermove', move);
      // Catch up with the pointer first, so the layer stays under the point
      // it was pressed at.
      if (stage && pressedAt) {
        stage.setPointersPositions(ev);
        const now = stage.getPointerPosition();
        const at = node.getAbsolutePosition();
        if (now) node.setAbsolutePosition({ x: at.x + now.x - pressedAt.x, y: at.y + now.y - pressedAt.y });
      }
      node.startDrag();
    };
    const end = (ev: PointerEvent) => {
      window.removeEventListener('pointermove', move);
      window.removeEventListener('pointerup', end);
      window.removeEventListener('pointercancel', end);
      cover.draggable(true);
      if (!moved && ev.type === 'pointerup') {
        pickedInList.current = null;
        selectLayer(tapped);
      }
    };
    window.addEventListener('pointermove', move);
    window.addEventListener('pointerup', end);
    window.addEventListener('pointercancel', end);
  };

  const onLayerPointerDown = (l: EditorLayer, e: Konva.KonvaEventObject<PointerEvent>) => {
    // A second finger pinches; it picks nothing.
    if (!e.evt.isPrimary) return;
    const pos = e.target.getStage()?.getPointerPosition();
    press.current = pos ? { ...pos, wasSelected: l.id === selectedId } : null;
    // On a touch screen only a layer picked in the Objects list is held: one
    // selected on the picture (or just added, like a full-frame clip) would
    // otherwise take every drag meant for the objects on top of it.
    const holds = l.id !== selectedId && (!isTouch || pickedInList.current === selectedId);
    const held = holds ? selectedNodeAtPointer(e.target.getStage()) : null;
    if (held) { holdSelected(held, e.currentTarget, e.evt, l); return; }
    pickedInList.current = null;
    selectLayer(l);
  };

  const onStagePointerDown = (e: Konva.KonvaEventObject<PointerEvent>) => {
    if (tab === 'crop' || !e.evt.isPrimary) return;
    if (painting) { startStroke(e.target.getStage()); return; }
    if (e.target === e.target.getStage() || e.target === baseRef.current || e.target === backgroundRef.current) {
      const near = isTouch && layersInteractive ? layerNearPointer(e.target.getStage()) : undefined;
      if (near) selectLayer(near);
      else setSelectedId(null);
    }
  };

  // A finger is wider than a thin text, an outline or a brush stroke: a tap
  // that hits nothing takes the topmost layer whose box, widened by
  // TOUCH_SLOP screen pixels, holds the point.
  const layerNearPointer = (stage: Konva.Stage | null): EditorLayer | undefined => {
    const pos = stage?.getPointerPosition();
    if (!pos) return undefined;
    for (let i = layers.length - 1; i >= 0; i--) {
      const node = nodeRefs.current.get(layers[i].id);
      if (!node || !node.isVisible()) continue;
      const r = node.getClientRect();
      if (pos.x >= r.x - TOUCH_SLOP && pos.x <= r.x + r.width + TOUCH_SLOP && pos.y >= r.y - TOUCH_SLOP && pos.y <= r.y + r.height + TOUCH_SLOP) return layers[i];
    }
    return undefined;
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
        value={(value || '#000000').slice(0, 7)}
        onChange={e => pick(e.target.value, key)}
        title="Custom colour"
      />
    </div>
  );

  // When the selected layer shows, and its effects in, while and out; all of
  // it, or one part (the phone's tray, which names the parts itself).
  const animateControls = (l: EditorLayer, part?: AnimPart) => {
    const a = layerAnim(l);
    const { start, end } = layerSpan(l, duration);
    const secondsField = (value: number, min: number, max: number, set: (v: number) => void, label: string) => (
      <input
        type="number"
        className="ie-number"
        min={min}
        max={max}
        step={0.1}
        value={Math.round(value * 10) / 10}
        onChange={e => { const v = Number(e.target.value); if (e.target.value !== '' && v >= min && v <= max) set(Math.round(v * 10) / 10); }}
        aria-label={label}
      />
    );
    const durationSlider = (value: number, set: (v: number) => void) => (
      <label className="ie-slider">
        <span>Duration</span>
        <input type="range" min={1} max={30} value={Math.round(value * 10)} onChange={e => set(Number(e.target.value) / 10)} />
        <span className="ie-value">{formatTime(value)}</span>
      </label>
    );
    const shows = (p: AnimPart) => !part || part === p;
    return (
      <>
        {shows('timing') && (
          <>
            {!part && <span className="ie-label">Shows</span>}
            <div className="ie-row">
              <span className="ie-unit">from</span>
              {secondsField(start, 0, Math.max(0, end - 0.1), v => updateAnim(l, { start: v }, `anim-start:${l.id}`), 'Shows from, in seconds')}
              <span className="ie-unit">to</span>
              {secondsField(end, start + 0.1, duration, v => updateAnim(l, { end: v >= duration ? undefined : v }, `anim-end:${l.id}`), 'Shows until, in seconds')}
              <span className="ie-unit">s</span>
            </div>
            <div className={`ie-row ${part ? 'ie-scroll' : ''}`}>
              <button className="ie-chip" onClick={() => updateAnim(l, { start: 0, end: undefined })} disabled={start === 0 && end === duration}>Whole video</button>
              <button className="ie-chip" onClick={() => updateAnim(l, { start: Math.min(Math.round(time * 10) / 10, end - 0.1) })} title="Start at the playhead">Start here</button>
              <button className="ie-chip" onClick={() => updateAnim(l, { end: time >= duration ? undefined : Math.max(Math.round(time * 10) / 10, start + 0.1) })} title="End at the playhead">End here</button>
              {part && <button className="ie-chip" onClick={() => { playFrom(start); }}><Clapperboard size={14} /> Preview</button>}
            </div>
          </>
        )}
        {shows('in') && (
          <>
            {!part && <span className="ie-label ie-section">Comes in</span>}
            <div className={`ie-row ${part ? 'ie-scroll' : ''}`}>
              {ENTER_EFFECTS.filter(f => !f.textOnly || l.kind === 'text').map(f => (
                <button key={f.id} className={`ie-chip ${a.enter === f.id ? 'active' : ''}`} onClick={() => { updateAnim(l, { enter: f.id }); playFrom(start); }}>{f.label}</button>
              ))}
            </div>
            {a.enter !== 'none' && durationSlider(a.enterDuration, v => updateAnim(l, { enterDuration: v }, `anim-enter:${l.id}`))}
          </>
        )}
        {shows('loop') && (
          <>
            {!part && <span className="ie-label ie-section">While it shows</span>}
            <div className={`ie-row ${part ? 'ie-scroll' : ''}`}>
              {LOOP_EFFECTS.map(f => (
                <button key={f.id} className={`ie-chip ${a.loop === f.id ? 'active' : ''}`} onClick={() => updateAnim(l, { loop: f.id })}>{f.label}</button>
              ))}
            </div>
          </>
        )}
        {shows('out') && (
          <>
            {!part && <span className="ie-label ie-section">Goes out</span>}
            <div className={`ie-row ${part ? 'ie-scroll' : ''}`}>
              {EXIT_EFFECTS.map(f => (
                <button
                  key={f.id}
                  className={`ie-chip ${a.exit === f.id ? 'active' : ''}`}
                  onClick={() => { updateAnim(l, { exit: f.id }); playFrom(Math.max(start, end - a.exitDuration - 0.5)); }}
                >
                  {f.label}
                </button>
              ))}
            </div>
            {a.exit !== 'none' && durationSlider(a.exitDuration, v => updateAnim(l, { exitDuration: v }, `anim-exit:${l.id}`))}
            {a.exit !== 'none' && end >= duration && <p className="ie-hint">It shows to the end of the video, so it goes out as the video ends.</p>}
          </>
        )}
        {!part && (
          <div className="ie-row">
            <button className="ie-chip" onClick={() => { playFrom(start); }}><Clapperboard size={14} /> Preview</button>
          </div>
        )}
      </>
    );
  };

  // Whether an upload here goes into the media library or is kept for this
  // video only.
  const keepInLibraryToggle = (
    <label className="ie-check">
      <input type="checkbox" checked={keepInLibrary} onChange={e => setKeepInLibrary(e.target.checked)} />
      <span>Keep uploads in the media library</span>
    </label>
  );

  // Where a clip starts playing, how loud, and how long its layer shows.
  const clipControls = (l: ImageLayer, c: ClipInfo) => {
    const { start, end } = layerSpan(l, duration);
    const runs = c.duration > 0 ? c.duration - c.trimStart : 0;
    return (
      <>
        <span className="ie-label"><Film size={14} /> Clip</span>
        {c.duration > 0 && (
          <label className="ie-slider">
            <span>Start at</span>
            <input
              type="range"
              min={0}
              max={Math.max(0, Math.floor((c.duration - 0.1) * 10))}
              value={Math.round(c.trimStart * 10)}
              onChange={e => updateLayer(l.id, { clip: { ...c, trimStart: Number(e.target.value) / 10 } }, `clip-trim:${l.id}`)}
            />
            <span className="ie-value">{formatTime(c.trimStart)}</span>
          </label>
        )}
        <label className="ie-slider">
          <span>Volume</span>
          <input
            type="range"
            min={0}
            max={100}
            value={Math.round(c.volume * 100)}
            onChange={e => updateLayer(l.id, { clip: { ...c, volume: Number(e.target.value) / 100 } }, `clip-volume:${l.id}`)}
          />
          <span className="ie-value">{Math.round(c.volume * 100)}%</span>
        </label>
        <div className="ie-row">
          <button className="ie-chip" onClick={() => updateLayer(l.id, { clip: { ...c, volume: c.volume > 0 ? 0 : 1 } })}>
            {c.volume > 0 ? 'Mute' : 'Unmute'}
          </button>
          {runs > 0 && (
            <button
              className="ie-chip"
              onClick={() => { const to = Math.round((start + runs) * 10) / 10; updateAnim(l, { end: to >= duration ? undefined : to }); }}
              title="Show it for as long as the clip runs from its start point"
            >
              Fit to clip
            </button>
          )}
        </div>
        {runs > 0 && end - start > runs + 0.05 && (
          <p className="ie-hint">The clip runs {formatTime(runs)} from its start point; its last frame stays for the rest of the {formatTime(end - start)}.</p>
        )}
        <span className="ie-label ie-section">Picture</span>
      </>
    );
  };

  // The video's music: a sound from the library or a file, where it starts,
  // how loud, and its fades.
  const musicControls = () => (
    <>
      <span className="ie-label ie-section"><Music size={14} /> Music</span>
      {audio ? (
        <>
          <div className="ie-row">
            <span className="ie-sound-name" title={audio.name}>{audio.name}</span>
            <button className="ie-chip danger" onClick={() => changeAudio(null)}><Trash2 size={14} /> Remove</button>
          </div>
          <label className="ie-slider">
            <span>Starts at</span>
            <input type="range" min={0} max={Math.round(duration * 10)} value={Math.round(audio.start * 10)}
              onChange={e => changeAudio({ ...audio, start: Number(e.target.value) / 10 }, 'music-start')} />
            <span className="ie-value">{formatTime(audio.start)}</span>
          </label>
          {audio.duration > 0 && (
            <label className="ie-slider">
              <span>Skip</span>
              <input type="range" min={0} max={Math.max(0, Math.floor((audio.duration - 1) * 10))} value={Math.round(audio.trimStart * 10)}
                onChange={e => changeAudio({ ...audio, trimStart: Number(e.target.value) / 10 }, 'music-trim')} title="Start this far into the song" />
              <span className="ie-value">{formatTime(audio.trimStart)}</span>
            </label>
          )}
          <label className="ie-slider">
            <span>Volume</span>
            <input type="range" min={0} max={100} value={Math.round(audio.volume * 100)}
              onChange={e => changeAudio({ ...audio, volume: Number(e.target.value) / 100 }, 'music-volume')} />
            <span className="ie-value">{Math.round(audio.volume * 100)}%</span>
          </label>
          <label className="ie-slider">
            <span>Fade in</span>
            <input type="range" min={0} max={50} value={Math.round(audio.fadeIn * 10)}
              onChange={e => changeAudio({ ...audio, fadeIn: Number(e.target.value) / 10 }, 'music-fade-in')} />
            <span className="ie-value">{formatTime(audio.fadeIn)}</span>
          </label>
          <label className="ie-slider">
            <span>Fade out</span>
            <input type="range" min={0} max={50} value={Math.round(audio.fadeOut * 10)}
              onChange={e => changeAudio({ ...audio, fadeOut: Number(e.target.value) / 10 }, 'music-fade-out')} />
            <span className="ie-value">{formatTime(audio.fadeOut)}</span>
          </label>
          {(() => { const m = musicSpan(audio, duration); return m.end < duration - 0.05 ? <p className="ie-hint">The song ends at {formatTime(m.end)}, before the video does.</p> : null; })()}
        </>
      ) : (
        <p className="ie-hint">Add a song or a sound under the video, from the media library or a file (mp3, m4a, wav). Clips play their own sound as well.</p>
      )}
      <div className="ie-row">
        <button className="ie-chip" onClick={() => soundInputRef.current?.click()} disabled={uploadingSound}>
          {uploadingSound ? <Loader2 size={14} className="ie-spin" /> : <Upload size={14} />} {audio ? 'Upload another' : 'Upload a sound'}
        </button>
      </div>
      {keepInLibraryToggle}
      {sounds === null ? (
        <p className="ie-hint"><Loader2 size={14} className="ie-spin" /> Loading the library's sounds…</p>
      ) : sounds.length > 0 && (
        <div className="ie-sound-list">
          {sounds.map(m => (
            <button key={m.id} className={`ie-sound ${audio?.src === m.url ? 'active' : ''}`} onClick={() => void pickSound(m.url, m.name)} title={m.description || m.name}>
              <Music size={14} /> <span>{m.name}</span>
            </button>
          ))}
        </div>
      )}
      <input ref={soundInputRef} type="file" accept="audio/*,.mp3,.m4a,.aac,.wav,.ogg" hidden onChange={uploadSound} />
    </>
  );

  const borderStyleChips = (l: ImageLayer) => (
    <>
      <button className={`ie-chip ${!l.border ? 'active' : ''}`} onClick={() => setBorderStyle(l, null)}>None</button>
      {BORDER_STYLES.map(b => (
        <button
          key={b.id}
          className={`ie-chip ${l.border?.style === b.id ? 'active' : ''}`}
          onClick={() => setBorderStyle(l, b.id)}
        >
          {b.label}
        </button>
      ))}
    </>
  );

  const shapeKindButtons = (l: ShapeLayer) => (
    <div className="ie-segmented" role="group" aria-label="Shape">
      {SHAPE_KINDS.map(k => {
        const Icon = SHAPE_ICONS[k.id];
        return (
          <button key={k.id} className={l.shape === k.id ? 'active' : ''} onClick={() => updateLayer(l.id, { shape: k.id })} title={k.label} aria-label={k.label} aria-pressed={l.shape === k.id}>
            <Icon size={16} />
          </button>
        );
      })}
    </div>
  );

  // How the layer mixes with what lies under it: a list in the desktop
  // panel, chips in the phone's tray.
  const blendSelect = (l: EditorLayer) => (
    <label className="ie-field">
      <span>Blend</span>
      <select
        className="ie-select"
        value={l.blend ?? 'normal'}
        onChange={e => updateLayer(l.id, { blend: e.target.value as BlendMode })}
        aria-label="Blend mode"
      >
        {BLEND_MODES.map(m => <option key={m.id} value={m.id}>{m.label}</option>)}
      </select>
    </label>
  );
  const blendChips = (l: EditorLayer) => (
    <div className="ie-row ie-scroll">
      {BLEND_MODES.map(m => (
        <button key={m.id} className={`ie-chip ${(l.blend ?? 'normal') === m.id ? 'active' : ''}`} onClick={() => updateLayer(l.id, { blend: m.id })}>
          {m.label}
        </button>
      ))}
    </div>
  );

  // Opacity, blend, corners, border, fit, centre, replace and remove for the
  // selected image layer.
  const imageLayerControls = selected?.kind === 'image' && (
    <>
      {selected.clip && clipControls(selected, selected.clip)}
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
      {blendSelect(selected)}
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
        {borderStyleChips(selected)}
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
      {selected.placeholder && <p className="ie-hint">Paste an image (Ctrl+V) to put it in this one's place, filling its frame.</p>}
      {selected.srcCrop && <p className="ie-hint">Only part of this image is shown. Fit shows all of it again.</p>}
    </>
  );

  // Shape, fill, border, corners, opacity and blend of the selected shape.
  const shapeControls = (l: ShapeLayer) => (
    <>
      <span className="ie-label ie-section">Shape</span>
      <div className="ie-row">
        {shapeKindButtons(l)}
      </div>
      <span className="ie-label">Fill</span>
      <div className="ie-row">
        <button className={`ie-chip ${!l.fill ? 'active' : ''}`} onClick={() => updateLayer(l.id, { fill: '' })}>No fill</button>
      </div>
      {colorRow(l.fill, (c, key) => updateLayer(l.id, { fill: c }, key), `shape-fill:${l.id}`)}
      <span className="ie-label">Border</span>
      <label className="ie-slider">
        <span>Width</span>
        <input
          type="range"
          min={0}
          max={Math.max(maxBorder, Math.round(l.strokeWidth))}
          value={Math.round(l.strokeWidth)}
          onChange={e => updateLayer(l.id, { strokeWidth: Number(e.target.value) }, `shape-stroke-width:${l.id}`)}
        />
        <span className="ie-value">{Math.round(l.strokeWidth)}px</span>
      </label>
      {l.strokeWidth > 0 && colorRow(l.stroke, (c, key) => updateLayer(l.id, { stroke: c }, key), `shape-stroke:${l.id}`)}
      {l.shape === 'rect' && (
        <label className="ie-slider">
          <span>Corners</span>
          <input
            type="range"
            min={0}
            max={50}
            value={l.cornerRadius ?? 0}
            onChange={e => updateLayer(l.id, { cornerRadius: Number(e.target.value) }, `shape-corners:${l.id}`)}
            onDoubleClick={() => updateLayer(l.id, { cornerRadius: 0 })}
          />
          <span className="ie-value">{l.cornerRadius ?? 0}%</span>
        </label>
      )}
      <label className="ie-slider">
        <span>Opacity</span>
        <input
          type="range"
          min={5}
          max={100}
          value={Math.round(l.opacity * 100)}
          onChange={e => updateLayer(l.id, { opacity: Number(e.target.value) / 100 }, `shape-opacity:${l.id}`)}
        />
        <span className="ie-value">{Math.round(l.opacity * 100)}%</span>
      </label>
      {blendSelect(l)}
      <div className="ie-row ie-scroll">
        <button className="ie-chip" onClick={centreSelected}><Crosshair size={14} /> Centre</button>
        <button className="ie-chip danger" onClick={deleteSelected}><Trash2 size={14} /> Remove</button>
      </div>
    </>
  );

  const editingText = layers.find((l): l is TextLayer => l.id === editingTextId && l.kind === 'text') ?? null;

  const anchorSize = isTouch ? 22 : 11;
  const layerTransformer = tab !== 'crop';
  // Layers are picked and moved on every tab but Crop and a painting Brush,
  // where the pointer crops or paints.
  // Nor while a video plays, unless on repeat.
  const layersInteractive = tab !== 'crop' && !painting && (!playing || livePlay);
  const ratio = aspectRatio(aspectId);

  // --- Controls shared by the desktop panel and the phone's tray and sheets

  // The crop's aspect presets.
  const cropControls = (
    <div className="ie-row ie-scroll">
      {ASPECTS.map(a => (
        <button key={a.id} className={`ie-chip ${aspectId === a.id ? 'active' : ''}`} onClick={() => pickAspect(a.id)}>
          {a.label}
        </button>
      ))}
      <button className="ie-chip" onClick={resetCrop}>Reset</button>
    </div>
  );

  // A blank canvas's formats.
  const formatChips = canvas && (
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
  );

  // The Templates tab: SocialPod's built-in templates, and the user's own
  // (drafts and the library's templates), each a tab of its own.
  const ownCount = (templates?.length ?? 0) + (drafts?.length ?? 0);
  const shownView = templateView ?? (templates === null || ownCount > 0 ? 'own' : 'system');
  const viewFormat = systemFormat ?? (crop ? nearestFormat(crop.width, crop.height) : '4:5');
  // On a photo only the designs laid over a full-frame picture, which the
  // photo takes the place of.
  const systemShown = SYSTEM_TEMPLATES.filter(t =>
    t.format === viewFormat && (canvasMode || t.overPhoto) && (!systemCategory || t.category === systemCategory));
  const systemCategories = SYSTEM_CATEGORIES.filter(c => SYSTEM_TEMPLATES.some(t => t.category === c.id && (canvasMode || t.overPhoto)));
  const systemList = (
    <>
      <div className="ie-row ie-scroll">
        {SYSTEM_FORMATS.map(f => (
          <button key={f.id} className={`ie-chip ${viewFormat === f.id ? 'active' : ''}`} onClick={() => setSystemFormat(f.id)} title={`${f.width} × ${f.height}`}>
            {f.label}
          </button>
        ))}
      </div>
      <label className="ie-field">
        <span>Category</span>
        <select
          className="ie-select"
          value={systemCategory ?? ''}
          onChange={e => setSystemCategory((e.target.value || null) as SystemCategory | null)}
          aria-label="Template category"
        >
          <option value="">All</option>
          {systemCategories.map(c => <option key={c.id} value={c.id}>{c.label}</option>)}
        </select>
      </label>
      <p className="ie-hint">
        {canvasMode
          ? `Tap a template to start from it. It replaces the current design${video ? ', and its layers come in one after the other' : ''}. Replace or paste over a picture to put in yours.`
          : 'Tap a template to lay its texts and shapes over the picture.'}
      </p>
      {systemShown.length === 0 ? (
        <p className="ie-hint">No templates in this format and category.</p>
      ) : (
        <div className="ie-grid">
          {systemShown.map(t => (
            <SystemTemplateTile
              key={t.id}
              template={t}
              loading={applyingTemplate === t.id}
              disabled={applyingTemplate !== null}
              onPick={() => void pickSystemTemplate(t).then(() => { if (phone) closeSheet(); })}
            />
          ))}
        </div>
      )}
    </>
  );
  const templateList = (
    <>
      <div className="ie-segmented ie-segmented-wide" role="tablist" aria-label="Templates">
        <button role="tab" aria-selected={shownView === 'system'} className={shownView === 'system' ? 'active' : ''} onClick={() => setTemplateView('system')}>SocialPod</button>
        <button role="tab" aria-selected={shownView === 'own'} className={shownView === 'own' ? 'active' : ''} onClick={() => setTemplateView('own')}>Yours</button>
      </div>
      {shownView === 'system' ? systemList : (
        <>
          {drafts !== null && drafts.length > 0 && (
            <>
              <span className="ie-label">Your drafts</span>
              <p className="ie-hint">Tap a draft to carry on with it. It replaces what you are working on.</p>
              <div className="ie-grid">
                {drafts.map(d => (
                  <div key={d.id} className={`ie-tile-wrap ${draft?.id === d.id ? 'current' : ''}`}>
                    <button
                      className="ie-tile"
                      onClick={() => void restoreDraft(d).then(() => { if (phone) closeSheet(); })}
                      disabled={restoringDraft !== null}
                      title={`${d.name}, saved ${new Date(d.updatedAt).toLocaleString()}`}
                    >
                      <img src={resolveUrl(d.url)} alt="" loading="lazy" />
                      <span>{restoringDraft === d.id ? 'Loading…' : d.name}</span>
                    </button>
                    <button className="ie-tile-delete" onClick={() => void deleteDraft(d)} title="Delete this draft" aria-label={`Delete the draft ${d.name}`}>
                      <Trash2 size={12} />
                    </button>
                  </div>
                ))}
              </div>
              <span className="ie-label ie-section">Templates</span>
            </>
          )}
          {templates === null ? (
            <p className="ie-hint"><Loader2 size={14} className="ie-spin" /> Loading…</p>
          ) : templates.length === 0 ? (
            <p className="ie-hint">No templates of your own yet. Create them in the Media Library, or start from one of SocialPod's.</p>
          ) : (
            <>
              <p className="ie-hint">
                {canvasMode
                  ? 'Tap a template to start from it. It replaces the current design.'
                  : 'Tap a template to lay its texts and images over the picture.'}
              </p>
              <div className="ie-grid">
                {templates.map(t => (
                  <button
                    key={t.id}
                    className="ie-tile"
                    onClick={() => void pickTemplate(t).then(() => { if (phone) closeSheet(); })}
                    disabled={applyingTemplate !== null}
                    title={t.description ? `${t.name}: ${t.description}` : t.name}
                  >
                    <img src={resolveUrl(t.url)} alt="" loading="lazy" />
                    {video && isVideoTemplate(t.template) && <Film size={14} className="ie-tile-badge" aria-label="Video template" />}
                    <span>{applyingTemplate === t.id ? 'Loading…' : t.name}</span>
                  </button>
                ))}
              </div>
            </>
          )}
        </>
      )}
    </>
  );

  const filterPresets = (
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
  );

  // The media library (and in a video its clips): filters, search, upload
  // and the grid to add from.
  const mediaLibrary = (
    <>
      <div className="ie-row">
        <button className={`ie-chip ${!watermarksOnly && !clipsView ? 'active' : ''}`} onClick={() => { setWatermarksOnly(false); setClipsView(false); }}>All images</button>
        <button className={`ie-chip ${watermarksOnly && !clipsView ? 'active' : ''}`} onClick={() => { setWatermarksOnly(true); setClipsView(false); }}><Stamp size={14} /> Watermarks</button>
        {video && (
          <button className={`ie-chip ${clipsView ? 'active' : ''}`} onClick={() => { setClipsView(true); setWatermarksOnly(false); }}><Film size={14} /> Clips</button>
        )}
      </div>
      {clipsView && keepInLibraryToggle}
      <div className="ie-row ie-sticky">
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
          onClick={() => (clipsView ? clipInputRef : mediaInputRef).current?.click()}
          disabled={uploadingMedia}
          title={clipsView ? (keepInLibrary ? 'Upload a clip into the library' : 'Upload a clip for this video') : 'Add an image to the library'}
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
            : clipsView
              ? 'No clips in the library yet. Upload one with the button: it is kept for this video only, unless you keep it in the library.'
              : watermarksOnly
              ? 'No watermarks yet. Upload one here, or mark images as watermarks in the Media Library.'
              : 'The media library has no images yet. Add them here or on the Media Library page.'}
        </p>
      ) : (
        <div className="ie-grid">
          {media.map(m => (
            <button
              key={m.id}
              className="ie-tile"
              onClick={() => (clipsView ? addClip(m.url, m.id) : addMedia(m))}
              disabled={addingMedia !== null}
              title={m.description ? `${m.name}: ${m.description}` : `Add ${m.name}`}
            >
              {clipsView
                ? <video src={resolveUrl(m.url)} muted preload="metadata" />
                : <img src={resolveUrl(m.url)} alt="" loading="lazy" />}
              <span>{addingMedia === m.id ? 'Adding…' : m.name}</span>
              {m.watermark && !watermarksOnly && <Stamp size={14} className="ie-tile-badge" aria-label="Watermark" />}
            </button>
          ))}
        </div>
      )}
      <input ref={mediaInputRef} type="file" accept="image/png,image/jpeg,image/gif,image/webp" hidden onChange={uploadMedia} />
    </>
  );

  const addShapeChips = (
    <div className="ie-row">
      {SHAPE_KINDS.map(k => {
        const Icon = SHAPE_ICONS[k.id];
        return <button key={k.id} className="ie-chip" onClick={() => addShape(k.id)}><Icon size={14} /> {k.label}</button>;
      })}
    </div>
  );

  // A video's length and music, with nothing selected.
  const videoSettings = (
    <>
      <span className="ie-label">Video length</span>
      <div className="ie-row ie-scroll">
        {DURATION_PRESETS.map(d => (
          <button key={d} className={`ie-chip ${duration === d ? 'active' : ''}`} onClick={() => changeDuration(d)}>{d}s</button>
        ))}
        <input
          type="number"
          className="ie-number"
          min={MIN_DURATION}
          max={MAX_DURATION}
          step={0.5}
          value={duration}
          onChange={e => { const v = Number(e.target.value); if (v >= MIN_DURATION && v <= MAX_DURATION) changeDuration(v, 'duration'); }}
          aria-label="Video length in seconds"
        />
        <span className="ie-unit">s</span>
      </div>
      <p className="ie-hint">
        {layers.length > 0
          ? 'Pick an object on the picture or in the timeline to set when it shows and how it comes in and goes out. Drag its bar in the timeline to move it in time.'
          : 'Add texts, images and shapes, then animate them here. Each shows the whole video until you change it.'}
      </p>
      <span className="ie-label ie-section"><Film size={14} /> Clips</span>
      <p className="ie-hint">Put a video clip on the picture at the playhead, from a file (mp4, mov, webm) or from the library under Media. An upload is kept for this video only, unless you keep uploads in the media library below.</p>
      <div className="ie-row">
        <button className="ie-chip" onClick={() => clipInputRef.current?.click()} disabled={uploadingMedia || addingMedia !== null}>
          {uploadingMedia ? <Loader2 size={14} className="ie-spin" /> : <Upload size={14} />} Upload a clip
        </button>
      </div>
      {musicControls()}
    </>
  );

  const alignButtons = (l: TextLayer) => (
    <div className="ie-segmented" role="group" aria-label="Text alignment">
      {TEXT_ALIGNS.map(a => (
        <button
          key={a.id}
          className={(l.align ?? 'center') === a.id ? 'active' : ''}
          onClick={() => updateLayer(l.id, { align: a.id })}
          title={a.label}
          aria-label={a.label}
          aria-pressed={(l.align ?? 'center') === a.id}
        >
          <a.icon size={16} />
        </button>
      ))}
    </div>
  );

  const textStyleChips = (l: TextLayer) => (
    <>
      <button className={`ie-chip ${l.bold ? 'active' : ''}`} onClick={() => updateLayer(l.id, { bold: !l.bold })}>
        <Bold size={14} /> Bold
      </button>
      <button className={`ie-chip ${l.outline ? 'active' : ''}`} onClick={() => updateLayer(l.id, { outline: !l.outline })}>
        Outline
      </button>
      <button className={`ie-chip ${l.shadow ? 'active' : ''}`} onClick={() => updateLayer(l.id, { shadow: !l.shadow })}>
        Shadow
      </button>
    </>
  );


  // --- Phone layout ---------------------------------------------------------

  const traySlider = (label: string, min: number, max: number, value: number, onChange: (v: number) => void, shown: string) => (
    <label className="ie-slider">
      <span>{label}</span>
      <input type="range" min={min} max={max} value={value} onChange={e => onChange(Number(e.target.value))} />
      <span className="ie-value">{shown}</span>
    </label>
  );

  // A tool opens in the tray (on its first control, or sub); an action acts.
  const toolItem = (id: string, label: string, icon: LucideIcon, sub: string | null = null): BarItem => (
    { id, label, icon, active: phoneTool === id, onClick: () => openPhoneTool(id, sub) }
  );
  const actionItem = (id: string, label: string, icon: LucideIcon, onClick: () => void, disabled = false): BarItem => (
    { id, label, icon, onClick, disabled }
  );

  const deselect = () => {
    if (editingTextId) finishTextEdit();
    setSelectedId(null);
  };

  // The add tools with nothing selected; the selected object's tools
  // otherwise.
  const phoneBarItems = (): BarItem[] => {
    const l = selected;
    const animate = video ? [toolItem('animate', 'Animate', Clapperboard, 'timing')] : [];
    const centre = actionItem('centre', 'Centre', Crosshair, centreSelected);
    if (!l) {
      return [
        ...(canvasMode
          ? [toolItem('canvas', 'Canvas', Frame, 'format')]
          : [toolItem('crop', 'Crop', Crop), toolItem('filters', 'Filters', Sparkles, 'presets')]),
        actionItem('templates', 'Templates', LayoutTemplate, () => openSheet('templates')),
        actionItem('media', 'Media', FolderOpen, () => openSheet('media')),
        actionItem('text', 'Text', Type, addText),
        toolItem('shapes', 'Shapes', Shapes),
        toolItem('brush', 'Brush', Brush, 'size'),
        ...(video ? [actionItem('video', 'Video', Film, () => openSheet('video'))] : []),
      ];
    }
    switch (l.kind) {
      case 'text':
        return [
          actionItem('edit', 'Edit', Pencil, () => startTextEdit(l.id)),
          toolItem('font', 'Font', CaseSensitive),
          toolItem('size', 'Size', ALargeSmall),
          toolItem('color', 'Colour', Palette),
          toolItem('align', 'Align', AlignCenter),
          toolItem('style', 'Style', Bold),
          toolItem('blend', 'Blend', Contrast),
          ...animate,
          centre,
        ];
      case 'image':
        return [
          actionItem('replace', 'Replace', Replace, () => placeInputRef.current?.click(), placingImage),
          ...(l.clip ? [toolItem('clip', 'Clip', Volume2, 'volume')] : []),
          toolItem('opacity', 'Opacity', Blend),
          toolItem('blend', 'Blend', Contrast),
          toolItem('corners', 'Corners', SquareRoundCorner),
          toolItem('border', 'Border', SquareDashed, 'style'),
          actionItem('fit', 'Fit', Maximize, fitSelectedImage),
          ...animate,
          centre,
        ];
      case 'shape':
        return [
          toolItem('shape', 'Shape', Shapes),
          toolItem('fill', 'Fill', PaintBucket),
          toolItem('border', 'Border', SquareDashed, 'width'),
          ...(l.shape === 'rect' ? [toolItem('corners', 'Corners', SquareRoundCorner)] : []),
          toolItem('opacity', 'Opacity', Blend),
          toolItem('blend', 'Blend', Contrast),
          ...animate,
          centre,
        ];
      default:
        return [toolItem('brush', 'Brush', Brush, 'size'), toolItem('blend', 'Blend', Contrast), ...animate];
    }
  };

  // What the tray shows for the open tool: the control, the chips that pick
  // one of several (subs), and controls ahead of those (lead).
  const phoneTray = (): { subs?: TraySub[]; lead?: ReactNode; body: ReactNode } | null => {
    const l = selected;
    const pick = (subs: TraySub[]) => (phoneSub && subs.some(x => x.id === phoneSub) ? phoneSub : subs[0].id);
    switch (phoneTool) {
      case 'crop':
        return { body: cropControls };
      case 'filters': {
        const subs = [
          { id: 'presets', label: 'Presets' }, { id: 'brightness', label: 'Brightness' },
          { id: 'contrast', label: 'Contrast' }, { id: 'saturation', label: 'Saturation' },
        ];
        const sub = pick(subs);
        if (sub === 'presets') return { subs, body: filterPresets };
        const k = sub as 'brightness' | 'contrast' | 'saturation';
        return { subs, body: traySlider(subs.find(x => x.id === k)!.label, -100, 100, adjustments[k], v => changeAdjustments({ [k]: v }, `adjust:${k}`), String(adjustments[k])) };
      }
      case 'canvas': {
        if (!canvas) return null;
        const subs = [{ id: 'format', label: 'Format' }, { id: 'background', label: 'Background' }];
        return { subs, body: pick(subs) === 'background' ? colorRow(canvas.background, changeBackground, 'background') : formatChips };
      }
      case 'shapes':
        return { body: addShapeChips };
      case 'brush': {
        const subs = [{ id: 'size', label: 'Size' }, { id: 'feather', label: 'Feather' }, { id: 'opacity', label: 'Opacity' }, { id: 'colour', label: 'Colour' }];
        const sub = pick(subs);
        const lead = (
          <>
            <button className={`ie-tray-sub ${brushMode === 'paint' ? 'active' : ''}`} onClick={() => setBrushMode('paint')} aria-pressed={brushMode === 'paint'}><Brush size={13} /> Paint</button>
            <button className={`ie-tray-sub ${brushMode === 'move' ? 'active' : ''}`} onClick={() => setBrushMode('move')} aria-pressed={brushMode === 'move'}><Move size={13} /> Move</button>
            {l?.kind === 'paint' && (
              <button className="ie-tray-sub" onClick={() => { setSelectedId(null); setBrushMode('paint'); }} title="Keep the next strokes apart"><Plus size={13} /> New layer</button>
            )}
            <span className="ie-tray-divider" />
          </>
        );
        const body = sub === 'size'
          ? traySlider('Size', 1, Math.max(maxBrush, brushSize), brushSize, v => setBrush(b => ({ ...b, size: v })), `${brushSize}px`)
          : sub === 'feather'
            ? traySlider('Feather', 0, Math.max(maxBrush, brushFeather), brushFeather, v => setBrush(b => ({ ...b, feather: v })), `${brushFeather}px`)
            : sub === 'opacity'
              ? traySlider('Opacity', 2, 100, Math.round(brush.opacity * 100), v => setBrush(b => ({ ...b, opacity: v / 100 })), `${Math.round(brush.opacity * 100)}%`)
              : colorRow(brush.color, c => setBrush(b => ({ ...b, color: c })), 'brush-color');
        return { lead, subs, body };
      }
    }
    if (!l) return null;
    if (phoneTool === 'animate') {
      const subs = [{ id: 'timing', label: 'Timing' }, { id: 'in', label: 'Comes in' }, { id: 'loop', label: 'While shown' }, { id: 'out', label: 'Goes out' }];
      return { subs, body: animateControls(l, pick(subs) as AnimPart) };
    }
    if (phoneTool === 'blend') return { body: blendChips(l) };
    if (l.kind === 'text') {
      switch (phoneTool) {
        case 'font':
          return {
            body: (
              <div className="ie-row ie-scroll">
                {fonts.map(f => (
                  <button key={f.family} className={`ie-chip ${l.fontFamily === f.family ? 'active' : ''}`} style={{ fontFamily: f.family }} onClick={() => updateLayer(l.id, { fontFamily: f.family })}>
                    {f.label}
                  </button>
                ))}
              </div>
            ),
          };
        case 'size': {
          const max = Math.max(Math.round(minSide * 0.5), 200, Math.round(l.fontSize));
          return { body: traySlider('Size', MIN_FONT_SIZE, max, Math.round(l.fontSize), v => updateLayer(l.id, resizeText(l, v), `font-size:${l.id}`), `${Math.round(l.fontSize)}px`) };
        }
        case 'color':
          return { body: colorRow(l.fill, (c, key) => updateLayer(l.id, { fill: c }, key), `fill:${l.id}`) };
        case 'align':
          return { body: <div className="ie-row">{alignButtons(l)}</div> };
        case 'style':
          return { body: <div className="ie-row ie-scroll">{textStyleChips(l)}</div> };
      }
    }
    if (l.kind === 'image') {
      switch (phoneTool) {
        case 'opacity':
          return { body: traySlider('Opacity', 10, 100, Math.round(l.opacity * 100), v => updateLayer(l.id, { opacity: v / 100 }, `opacity:${l.id}`), `${Math.round(l.opacity * 100)}%`) };
        case 'corners':
          return { body: traySlider('Corners', 0, 50, l.cornerRadius ?? 0, v => updateLayer(l.id, { cornerRadius: v }, `corners:${l.id}`), `${l.cornerRadius ?? 0}%`) };
        case 'border': {
          const subs = l.border ? [{ id: 'style', label: 'Style' }, { id: 'width', label: 'Width' }, { id: 'colour', label: 'Colour' }] : [{ id: 'style', label: 'Style' }];
          const sub = pick(subs);
          const border = l.border;
          const body = !border || sub === 'style'
            ? <div className="ie-row ie-scroll">{borderStyleChips(l)}</div>
            : sub === 'width'
              ? traySlider('Width', 1, Math.max(maxBorder, Math.round(border.width)), Math.round(border.width), v => updateLayer(l.id, { border: { ...border, width: v } }, `border-width:${l.id}`), `${Math.round(border.width)}px`)
              : colorRow(border.color, (c, key) => updateLayer(l.id, { border: { ...border, color: c } }, key), `border-color:${l.id}`);
          return { subs, body };
        }
        case 'clip': {
          const c = l.clip;
          if (!c) return null;
          const subs = c.duration > 0 ? [{ id: 'volume', label: 'Volume' }, { id: 'start', label: 'Start at' }] : [{ id: 'volume', label: 'Volume' }];
          const body = pick(subs) === 'start'
            ? traySlider('Start at', 0, Math.max(0, Math.floor((c.duration - 0.1) * 10)), Math.round(c.trimStart * 10), v => updateLayer(l.id, { clip: { ...c, trimStart: v / 10 } }, `clip-trim:${l.id}`), formatTime(c.trimStart))
            : (
              <div className="ie-row ie-tray-line">
                {traySlider('Volume', 0, 100, Math.round(c.volume * 100), v => updateLayer(l.id, { clip: { ...c, volume: v / 100 } }, `clip-volume:${l.id}`), `${Math.round(c.volume * 100)}%`)}
                <button className="ie-chip" onClick={() => updateLayer(l.id, { clip: { ...c, volume: c.volume > 0 ? 0 : 1 } })}>{c.volume > 0 ? 'Mute' : 'Unmute'}</button>
              </div>
            );
          return { subs, body };
        }
      }
    }
    if (l.kind === 'shape') {
      switch (phoneTool) {
        case 'shape':
          return { body: <div className="ie-row">{shapeKindButtons(l)}</div> };
        case 'fill':
          return {
            body: (
              <>
                <div className="ie-row"><button className={`ie-chip ${!l.fill ? 'active' : ''}`} onClick={() => updateLayer(l.id, { fill: '' })}>No fill</button></div>
                {colorRow(l.fill, (c, key) => updateLayer(l.id, { fill: c }, key), `shape-fill:${l.id}`)}
              </>
            ),
          };
        case 'border': {
          const subs = [{ id: 'width', label: 'Width' }, { id: 'colour', label: 'Colour' }];
          const body = pick(subs) === 'width'
            ? traySlider('Width', 0, Math.max(maxBorder, Math.round(l.strokeWidth)), Math.round(l.strokeWidth), v => updateLayer(l.id, { strokeWidth: v }, `shape-stroke-width:${l.id}`), `${Math.round(l.strokeWidth)}px`)
            : l.strokeWidth > 0
              ? colorRow(l.stroke, (c, key) => updateLayer(l.id, { stroke: c }, key), `shape-stroke:${l.id}`)
              : <p className="ie-hint">Give the border a width first.</p>;
          return { subs, body };
        }
        case 'corners':
          return { body: traySlider('Corners', 0, 50, l.cornerRadius ?? 0, v => updateLayer(l.id, { cornerRadius: v }, `shape-corners:${l.id}`), `${l.cornerRadius ?? 0}%`) };
        case 'opacity':
          return { body: traySlider('Opacity', 5, 100, Math.round(l.opacity * 100), v => updateLayer(l.id, { opacity: v / 100 }, `shape-opacity:${l.id}`), `${Math.round(l.opacity * 100)}%`) };
      }
    }
    return null;
  };
  const tray = phone && phoneTool ? phoneTray() : null;

  const sheetTitles: Record<PhoneSheet, string> = { templates: 'Templates', media: video ? 'Media and clips' : 'Media', layers: 'Objects', video: 'Video' };

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
        {/* A phone's header has room for the essentials only: no redo and
            no safe-zone switch (the zone always shows there). */}
        {!phone && (
          <button className="ie-icon-btn" onClick={redo} title="Redo" aria-label="Redo" disabled={saving || redoStack.length === 0}>
            <Redo2 size={18} />
          </button>
        )}
        <button
          className={`ie-icon-btn ${snapOn ? 'active' : ''}`}
          onClick={toggleSnap}
          title={snapOn ? 'Smart guides on (hold Alt to move freely)' : 'Smart guides off'}
          aria-label="Smart guides"
          aria-pressed={snapOn}
        >
          <Magnet size={18} />
        </button>
        {zone && !phone && (
          <button
            className={`ie-icon-btn ${showSafeZone ? 'active' : ''}`}
            onClick={toggleSafeZone}
            title={showSafeZone ? `Hide the ${zone} safe zone` : `Show the ${zone} safe zone (where Instagram's text and icons cover the frame)`}
            aria-label="Safe zone"
            aria-pressed={showSafeZone}
          >
            <Smartphone size={18} />
          </button>
        )}
        <button
          className={`ie-icon-btn ${(phone ? sheet === 'layers' : showLayers) ? 'active' : ''}`}
          onClick={() => (phone ? openSheet('layers') : setShowLayers(v => !v))}
          title={phone ? 'Objects' : showLayers ? 'Hide the object list' : 'Show the object list'}
          aria-label="Objects"
          aria-pressed={phone ? sheet === 'layers' : showLayers}
        >
          <LayersIcon size={18} />
        </button>
        {/* On a phone the draft and template buttons are icon buttons like
            the others: a labelled button with its label hidden came out
            empty in Safari. */}
        {video && (
          <button
            className={phone ? 'ie-icon-btn' : 'btn btn-secondary ie-draft'}
            onClick={() => void saveAsTemplate()}
            disabled={!ready || saving || savingTemplate}
            title="Keep this design in the media library as a template for other videos"
            aria-label="Save template"
          >
            {savingTemplate ? <Loader2 size={phone ? 18 : 16} className="ie-spin" /> : <LayoutTemplate size={phone ? 18 : 16} />}
            {!phone && <span>Save template</span>}
          </button>
        )}
        <button
          className={phone ? `ie-icon-btn ${draftClean ? 'active' : ''}` : 'btn btn-secondary ie-draft'}
          onClick={() => void saveDraft()}
          disabled={!ready || saving || savingDraft}
          title={draft ? `Save your work to the draft "${draft.name}" (Ctrl+S)` : 'Keep your work as a draft to finish later (Ctrl+S)'}
          aria-label={draftClean ? 'Draft saved' : 'Save draft'}
        >
          {savingDraft ? <Loader2 size={phone ? 18 : 16} className="ie-spin" /> : <FilePen size={phone ? 18 : 16} />}
          {!phone && <span>{draftClean ? 'Draft saved' : 'Save draft'}</span>}
        </button>
        <button className="btn btn-primary ie-save" onClick={save} disabled={!ready || saving}>
          {saving
            ? <><Loader2 size={16} className="ie-spin" /> {exportProgress !== null ? (phone ? `${exportProgress}%` : `Exporting ${exportProgress}%`) : 'Saving…'}</>
            : video && !designOnly ? (phone ? 'Export' : 'Export video') : 'Save'}
        </button>
      </header>

      {/* Desktop: tool tabs, then their controls, in columns on the left of
          the picture, so the picture keeps its size whatever the controls
          hold. Phone: picture, controls, tabs from top to bottom. */}
      <div className="ie-workspace">
        {!phone && (
        <>
        <nav className="ie-tabs" aria-label="Tools">
          {canvasMode ? (
            <button className={tab === 'canvas' ? 'active' : ''} onClick={() => switchTab('canvas')}><Frame size={20} /><span>Canvas</span></button>
          ) : (
            <>
              <button className={tab === 'crop' ? 'active' : ''} onClick={() => switchTab('crop')}><Crop size={20} /><span>Crop</span></button>
              <button className={tab === 'filters' ? 'active' : ''} onClick={() => switchTab('filters')}><Sparkles size={20} /><span>Filters</span></button>
            </>
          )}
          <button className={tab === 'templates' ? 'active' : ''} onClick={() => switchTab('templates')}><LayoutTemplate size={20} /><span>Templates</span></button>
          <button className={tab === 'media' ? 'active' : ''} onClick={() => switchTab('media')}><FolderOpen size={20} /><span>Media</span></button>
          <button className={tab === 'text' ? 'active' : ''} onClick={() => switchTab('text')}><Type size={20} /><span>Text</span></button>
          <button className={tab === 'shapes' ? 'active' : ''} onClick={() => switchTab('shapes')}><Shapes size={20} /><span>Shapes</span></button>
          <button className={tab === 'brush' ? 'active' : ''} onClick={() => switchTab('brush')}><Brush size={20} /><span>Brush</span></button>
          {video && (
            <button className={tab === 'animate' ? 'active' : ''} onClick={() => switchTab('animate')}><Clapperboard size={20} /><span>Animate</span></button>
          )}
        </nav>
        <div className="ie-panel">
          {tab === 'crop' && cropControls}

          {tab === 'canvas' && canvas && (
            <>
              {formatChips}
              <span className="ie-label">Background</span>
              {colorRow(canvas.background, changeBackground, 'background')}
            </>
          )}

          {tab === 'templates' && templateList}

          {tab === 'filters' && (
            <>
              {filterPresets}
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

          {tab === 'media' && (
            <>
              {selected?.kind === 'image' ? (
                <>
                  {imageLayerControls}
                  <span className="ie-label ie-section">Media library</span>
                </>
              ) : clipsView ? (
                <p className="ie-hint">Tap a clip to add it at the playhead, or upload one. Move, resize and turn it like an image; its sound plays along.</p>
              ) : media && media.length > 0 && (
                <p className="ie-hint">Tap an image to add it, or paste one (Ctrl+V), then drag it, pull its corners or turn it on the picture.</p>
              )}
              {mediaLibrary}
            </>
          )}

          {tab === 'shapes' && (
            <>
              <span className="ie-label">Add a shape</span>
              {addShapeChips}
              {selected?.kind === 'shape' ? shapeControls(selected) : (
                <p className="ie-hint">
                  {layers.some(l => l.kind === 'shape')
                    ? 'Tap a shape on the picture to change it.'
                    : 'Add a shape, then drag it, pull its handles (Shift keeps its proportions) or turn it on the picture.'}
                </p>
              )}
            </>
          )}

          {tab === 'brush' && (
            <>
              <div className="ie-row">
                <button className={`ie-chip ${brushMode === 'paint' ? 'active' : ''}`} onClick={() => setBrushMode('paint')}><Brush size={14} /> Paint</button>
                <button className={`ie-chip ${brushMode === 'move' ? 'active' : ''}`} onClick={() => setBrushMode('move')}><Move size={14} /> Move</button>
              </div>
              <label className="ie-slider">
                <span>Size</span>
                <input
                  type="range"
                  min={1}
                  max={Math.max(maxBrush, brushSize)}
                  value={brushSize}
                  onChange={e => setBrush(b => ({ ...b, size: Number(e.target.value) }))}
                />
                <span className="ie-value">{brushSize}px</span>
              </label>
              <label className="ie-slider">
                <span>Feather</span>
                <input
                  type="range"
                  min={0}
                  max={Math.max(maxBrush, brushFeather)}
                  value={brushFeather}
                  onChange={e => setBrush(b => ({ ...b, feather: Number(e.target.value) }))}
                />
                <span className="ie-value">{brushFeather}px</span>
              </label>
              <label className="ie-slider">
                <span>Opacity</span>
                <input
                  type="range"
                  min={2}
                  max={100}
                  value={Math.round(brush.opacity * 100)}
                  onChange={e => setBrush(b => ({ ...b, opacity: Number(e.target.value) / 100 }))}
                />
                <span className="ie-value">{Math.round(brush.opacity * 100)}%</span>
              </label>
              {colorRow(brush.color, c => setBrush(b => ({ ...b, color: c })), 'brush-color')}
              {selected?.kind === 'paint' ? (
                <>
                  <p className="ie-hint">
                    {brushMode === 'paint'
                      ? 'New strokes go into the selected paint layer. Start a new layer to keep them apart.'
                      : 'Drag the paint to move it.'}
                  </p>
                  {blendSelect(selected)}
                  <div className="ie-row">
                    <button className="ie-chip" onClick={() => { setSelectedId(null); setBrushMode('paint'); }}><Plus size={14} /> New layer</button>
                    <button className="ie-chip danger" onClick={deleteSelected}><Trash2 size={14} /> Remove layer</button>
                  </div>
                </>
              ) : (
                <p className="ie-hint">
                  {brushMode === 'paint'
                    ? 'Paint on the picture. A large, soft, see-through black brush darkens a busy background behind a text.'
                    : 'Tap a layer to pick it, then drag it.'}
                </p>
              )}
            </>
          )}

          {tab === 'animate' && (selected ? animateControls(selected) : videoSettings)}

          {tab === 'text' && (
            selected?.kind === 'text' ? (
              <>
                <p className="ie-hint">
                  {editingTextId === selected.id
                    ? 'Type on the picture. Enter starts a new line; Esc or a click outside finishes.'
                    : 'Double-click the text (or press Enter) to type on the picture. Arrow keys move it, Shift in bigger steps.'}
                </p>
                {editingTextId !== selected.id && (
                  <div className="ie-row">
                    <button className="ie-chip" onClick={() => startTextEdit(selected.id)}><Type size={14} /> Edit text</button>
                  </div>
                )}
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
                  {alignButtons(selected)}
                </div>
                {colorRow(selected.fill, (c, key) => updateLayer(selected.id, { fill: c }, key), `fill:${selected.id}`)}
                {blendSelect(selected)}
                <div className="ie-row ie-scroll">
                  {textStyleChips(selected)}
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
        </>
        )}
        <div className="ie-body">
          <div className={`ie-main ${(phone ? phoneTool === 'animate' : tab === 'animate') ? 'ie-animating' : ''}`}>
            <div className={`ie-canvas ${painting ? 'ie-painting' : ''}`} ref={canvasBoxRef}>
              {loadError && <div className="ie-message">The image could not be loaded.</div>}
              {!ready && !loadError && <div className="ie-message"><Loader2 size={24} className="ie-spin" /></div>}
              {ready && crop && size.w > 0 && (
                <Stage
                  width={size.w}
                  height={size.h}
                  onPointerDown={onStagePointerDown}
                  onPointerUp={onStagePointerUp}
                  onTouchStart={onStageTouchStart}
                  onTouchMove={onStageTouchMove}
                  onTouchEnd={onStageTouchEnd}
                  onPointerMove={e => { if (painting) extendStroke(e.target.getStage()); }}
                  onPointerLeave={() => setBrushAt(null)}
                >
                  <Layer ref={geometricHitTest}>
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
                      {layers.map(l => {
                        // In a video every layer is drawn as it is at the
                        // playhead, inside a group that animates it.
                        const anim = video ? animStateAt(l, time, duration, crop) : null;
                        const wrap = (el: React.JSX.Element) => (anim ? <Group key={l.id} {...animGroupAttrs(l, anim)}>{el}</Group> : el);
                        const common = {
                          ref: (node: Konva.Node | null) => { if (node) nodeRefs.current.set(l.id, node); else nodeRefs.current.delete(l.id); },
                          listening: layersInteractive,
                          draggable: layersInteractive,
                          onPointerDown: (e: Konva.KonvaEventObject<PointerEvent>) => onLayerPointerDown(l, e),
                          onDragStart: startMove,
                          onTransformStart: startMove,
                          onDragMove: (e: Konva.KonvaEventObject<DragEvent>) => onLayerDragMove(l, e),
                          onDragEnd: (e: Konva.KonvaEventObject<DragEvent>) => onLayerDragEnd(l, e),
                        };
                        if (l.kind === 'text') {
                          return wrap(
                            <KText
                              key={l.id}
                              {...common}
                              {...(anim ? animatedTextAttrs(l, anim) : textLayerAttrs(l))}
                              visible={editingTextId !== l.id}
                              onDblClick={() => startTextEdit(l.id)}
                              onDblTap={() => startTextEdit(l.id)}
                              onTransformEnd={e => onLayerTransformEnd(l, e.target)}
                            />,
                          );
                        }
                        if (l.kind === 'paint') {
                          return wrap(
                            <Fragment key={l.id}>
                              <Group {...common} x={l.x} y={l.y} {...blendAttrs(l)}>
                                {l.strokes.map((st, i) => <Shape key={i} {...paintStrokeAttrs(st)} />)}
                              </Group>
                              {drawing && l.id === selectedId && <Shape {...paintStrokeAttrs(drawing)} listening={false} />}
                            </Fragment>,
                          );
                        }
                        const { group, parts } = l.kind === 'image' ? imageLayerParts(l) : shapeLayerParts(l);
                        return wrap(
                          <Group key={l.id} {...common} {...group} onTransformEnd={e => onLayerTransformEnd(l, e.currentTarget)}>
                            {parts.map(renderPart)}
                          </Group>,
                        );
                      })}
                      {/* The stroke being painted shows where it will go: in the
                          selected paint layer's place in the stack (above), or
                          on top, where a new paint layer is added. */}
                      {drawing && selected?.kind !== 'paint' && <Shape {...paintStrokeAttrs(drawing)} listening={false} />}
                      {guideOverlay && (
                        <KImage image={guideOverlay} x={crop.x} y={crop.y} width={crop.width} height={crop.height} listening={false} opacity={tab === 'crop' ? 0.6 : 1} />
                      )}
                    </Group>

                    {zone && (showSafeZone || phone) && tab !== 'crop' && (
                      <Group listening={false}>
                        {safeZoneRects(zone, { x: gx + crop.x * scale, y: gy + crop.y * scale, width: crop.width * scale, height: crop.height * scale }).map((z, i) => (
                          <Group key={`sz${i}`}>
                            <Rect x={z.x} y={z.y} width={z.width} height={z.height} fill={SAFE_ZONE_FILL} stroke="rgba(255, 255, 255, 0.7)" strokeWidth={1} dash={[4, 4]} />
                            {z.width > 60 && (
                              <KText x={z.x + 4} y={z.y + 4} width={z.width - 8} text={z.label} fontSize={11} fill="#ffffff" shadowColor="#000000" shadowBlur={2} shadowOpacity={0.8} />
                            )}
                          </Group>
                        ))}
                      </Group>
                    )}

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

                    {painting && brushAt && (
                      <Group listening={false}>
                        <Circle x={gx + brushAt.x * scale} y={gy + brushAt.y * scale} radius={Math.max(1, (brushSize / 2) * scale)} stroke="#ffffff" strokeWidth={1.5} shadowColor="#000000" shadowBlur={2} shadowOpacity={0.8} />
                        {brushFeather > 0 && (
                          <Circle x={gx + brushAt.x * scale} y={gy + brushAt.y * scale} radius={(brushSize / 2 + brushFeather) * scale} stroke="rgba(255,255,255,0.6)" strokeWidth={1} dash={[4, 4]} />
                        )}
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
                      rotateEnabled={layerTransformer && selected?.kind !== 'paint'}
                      rotateAnchorOffset={isTouch ? 36 : 28}
                      // A finger that misses the small turn handle lands on
                      // the background and deselects: on touch its hit area
                      // is widened to about 46px.
                      anchorStyleFunc={isTouch ? (a: Konva.Rect) => { if (a.hasName('rotater')) a.hitStrokeWidth(24); } : undefined}
                      rotationSnaps={[0, 90, 180, 270]}
                      flipEnabled={false}
                      ignoreStroke
                      // Shapes stretch freely (Shift keeps their proportions);
                      // paint is moved only.
                      keepRatio={layerTransformer ? selected?.kind !== 'shape' : ratio !== null}
                      enabledAnchors={layerTransformer && selected?.kind === 'paint'
                        ? []
                        : (layerTransformer && selected?.kind !== 'shape') || (!layerTransformer && ratio !== null)
                          ? ['top-left', 'top-right', 'bottom-left', 'bottom-right']
                          : ['top-left', 'top-center', 'top-right', 'middle-right', 'middle-left', 'bottom-left', 'bottom-center', 'bottom-right']}
                      boundBoxFunc={(oldBox, newBox) => (layerTransformer
                        ? (newBox.width < 12 || newBox.height < 12 ? oldBox : newBox)
                        : cropBoundBox(oldBox, newBox))}
                    />
                  </Layer>
                </Stage>
              )}
              {editingText && (
                <InlineTextEditor
                  ref={textAreaRef}
                  layer={editingText}
                  scale={scale}
                  origin={{ x: gx, y: gy }}
                  onChange={text => updateLayer(editingText.id, retypeText(editingText, text), `gesture:text:${editingText.id}`)}
                  onDone={finishTextEdit}
                />
              )}
              {tray && (
                <PhoneTray subs={tray.subs} sub={phoneSub ?? tray.subs?.[0]?.id} onSub={setPhoneSub} lead={tray.lead} onDone={closePhoneTool}>
                  {tray.body}
                </PhoneTray>
              )}
              {phone && miniPos && selected && (
                <MiniToolbar
                  left={miniPos.left}
                  top={miniPos.top}
                  canDuplicate={canDuplicate}
                  canForward={selectedIndex < layers.length - 1}
                  canBackward={selectedIndex > 0}
                  onDuplicate={duplicateSelected}
                  onForward={() => restackSelected(1)}
                  onBackward={() => restackSelected(-1)}
                  onLayers={() => openSheet('layers')}
                  onDelete={deleteSelected}
                />
              )}
            </div>
            {video && (
              <Timeline
                layers={layers}
                duration={duration}
                time={time}
                playing={playing}
                selectedId={selectedId}
                loop={loop}
                onSeek={t => { if (!loop) setPlaying(false); seek(t); }}
                onTogglePlay={togglePlay}
                onToggleLoop={toggleLoop}
                onSelect={l => { if (!loop) setPlaying(false); selectLayer(l); }}
                onTiming={(l, a, key) => updateLayer(l.id, { anim: a }, key)}
                onDragStart={startGesture}
                audio={audio}
                onMusicStart={(start, key) => { if (audio) changeAudio({ ...audio, start }, key); }}
                onSelectMusic={() => { if (!loop) setPlaying(false); setSelectedId(null); if (phone) openSheet('video'); else setTab('animate'); }}
              />
            )}
          </div>

          {showLayers && !phone && (
            <aside className="ie-layers" aria-label="Objects">
              <div className="ie-layers-title">Objects</div>
              <LayerList
                layers={layers}
                selectedId={selectedId}
                background={canvas ? { color: canvas.background } : { src: source?.src ?? photoUrl ?? '' }}
                onSelect={pickInList}
                onSelectBackground={() => { setSelectedId(null); if (canvasMode) setTab('canvas'); }}
                onReorder={reorderLayers}
              />
            </aside>
          )}
        </div>
        {phone && <PhoneBar items={phoneBarItems()} back={selected ? deselect : undefined} />}
      </div>

      {phone && sheet && (
        <BottomSheet title={sheetTitles[sheet]} onClose={closeSheet}>
          {sheet === 'templates' && templateList}
          {sheet === 'media' && mediaLibrary}
          {sheet === 'video' && videoSettings}
          {sheet === 'layers' && (
            <LayerList
              layers={layers}
              selectedId={selectedId}
              background={canvas ? { color: canvas.background } : { src: source?.src ?? photoUrl ?? '' }}
              onSelect={l => { closeSheet(); pickInList(l); }}
              onSelectBackground={() => { closeSheet(); setSelectedId(null); if (canvasMode) openPhoneTool('canvas', 'background'); }}
              onReorder={reorderLayers}
            />
          )}
        </BottomSheet>
      )}

      <input ref={placeInputRef} type="file" accept="image/png,image/jpeg,image/gif,image/webp" hidden onChange={pickPlaceFile} />
      <input ref={clipInputRef} type="file" accept="video/mp4,video/quicktime,video/webm,.mp4,.mov,.webm" hidden onChange={uploadClip} />
    </div>
  );
}

// A layer pinched with two fingers, from how it was when the pinch began:
// scaled by s and turned by turn degrees about its centre, and moved by
// shift. A text is scaled through its font size.
function pinchedLayer(l: EditorLayer, s: number, turn: number, shift: { x: number; y: number }): Partial<ImageLayer> | Partial<ShapeLayer> | Partial<TextLayer> {
  if (l.kind === 'image' || l.kind === 'shape') {
    return { x: l.x + shift.x, y: l.y + shift.y, width: Math.max(4, l.width * s), height: Math.max(4, l.height * s), rotation: l.rotation + turn };
  }
  if (l.kind !== 'text') return {};
  const fontSize = clamp(Math.round(l.fontSize * s), MIN_FONT_SIZE, MAX_FONT_SIZE);
  // A text turns about its corner: its centre is the corner plus half its
  // size, turned with it.
  const half = (deg: number, w: number, h: number) => {
    const rad = (deg * Math.PI) / 180;
    return { x: (w / 2) * Math.cos(rad) - (h / 2) * Math.sin(rad), y: (w / 2) * Math.sin(rad) + (h / 2) * Math.cos(rad) };
  };
  const a = measureText(l);
  const b = measureText({ ...l, fontSize });
  const from = half(l.rotation, a.width, a.height);
  const rotation = l.rotation + turn;
  const to = half(rotation, b.width, b.height);
  return { fontSize, rotation, x: l.x + from.x + shift.x - to.x, y: l.y + from.y + shift.y - to.y };
}

// One part of an image or shape layer's group.
function renderPart(p: ImagePart, i: number) {
  switch (p.kind) {
    case 'image': return <KImage key={i} {...p.attrs} image={p.attrs.image} />;
    case 'ellipse': return <Ellipse key={i} {...p.attrs} />;
    case 'line': return <Line key={i} {...p.attrs} />;
    default: return <Rect key={i} {...p.attrs} />;
  }
}

interface BoxLike {
  x: number;
  y: number;
  width: number;
  height: number;
  rotation: number;
}

// The field a text is typed into on the picture: laid over the text layer it
// edits (hidden meanwhile) in its font, size, colour, outline, shadow and
// angle, and grown with what is typed, so the text looks as it will once
// done. Ctrl/Cmd+Enter or a click outside finishes; Escape is the editor's.
function InlineTextEditor({ ref, layer, scale, origin, onChange, onDone }: {
  ref: Ref<HTMLTextAreaElement>;
  layer: TextLayer;
  scale: number;
  origin: { x: number; y: number };
  onChange: (text: string) => void;
  onDone: () => void;
}) {
  const a = textLayerAttrs(layer);
  // An empty text still needs a line to put the caret on.
  const m = measureText(layer.text ? layer : { ...layer, text: ' ' });
  const fontSize = layer.fontSize * scale;
  // The browser may set the letters a little wider than the canvas does;
  // room so they never wrap or clip, taken from the side the alignment
  // leaves free.
  const room = Math.ceil(fontSize * 0.5) + 2;
  const align = layer.align ?? 'center';
  const shift = align === 'left' ? 0 : align === 'right' ? room : room / 2;
  return (
    <textarea
      ref={ref}
      className="ie-inline-text"
      value={layer.text}
      onChange={e => onChange(e.target.value)}
      onBlur={onDone}
      onKeyDown={e => {
        if (e.key === 'Enter' && (e.ctrlKey || e.metaKey)) { e.preventDefault(); onDone(); }
      }}
      spellCheck={false}
      aria-label="Text"
      style={{
        left: origin.x + layer.x * scale,
        top: origin.y + layer.y * scale,
        width: m.width * scale + room,
        height: m.height * scale,
        transform: `rotate(${layer.rotation}deg) translateX(${-shift}px)`,
        fontFamily: layer.fontFamily,
        fontSize,
        fontWeight: layer.bold ? 'bold' : 'normal',
        textAlign: align,
        color: layer.fill,
        caretColor: layer.fill,
        WebkitTextStroke: layer.outline ? `${(a.strokeWidth ?? 0) * scale}px ${a.stroke as string}` : undefined,
        textShadow: layer.shadow
          ? `${(a.shadowOffsetX ?? 0) * scale}px ${(a.shadowOffsetY ?? 0) * scale}px ${(a.shadowBlur ?? 0) * scale}px rgba(0, 0, 0, 0.6)`
          : undefined,
      }}
    />
  );
}
