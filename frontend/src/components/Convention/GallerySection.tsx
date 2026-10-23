import { useState } from 'react';
import { api } from '../../services/api';
import type { ConventionGalleryPhoto, ConventionQueueItem } from '../../types';
import { AlertTriangle, Crop, GalleryHorizontalEnd, Image as ImageIcon, Images, Loader, Maximize2, Plus, Trash2, X } from 'lucide-react';
import toast from 'react-hot-toast';
import { Modal } from '../Common/Modal';
import { ImageCropper } from '../Common/ImageCropper';

// The queue's photo gallery: photos uploaded without deciding yet what they
// become. Photos are picked (the order they are picked in is the order they
// take in a carousel) and moved into the queue, each as a single post or all
// into a carousel, a new one or one the queue already has.

// Items photos can still be added to as a carousel: those with an image list
// that have not been handed to a post yet.
function carouselTargets(items: ConventionQueueItem[]) {
  return items.filter(i => i.status !== 'scheduled' && (i.imageUrls?.length ?? 0) > 0);
}

// A carousel goes out square: a page of another shape is fitted into the square
// over a blurred fill of itself. The tiles crop to a square, so a picked photo
// that is not square already gets a warning.
const NOT_SQUARE_HINT = 'Not cropped square: in a carousel it is fitted into a square with blurred edges. Crop it to pick the framing.';

function isSquare(img: HTMLImageElement) {
  const { naturalWidth: w, naturalHeight: h } = img;
  return w > 0 && h > 0 && Math.abs(w - h) / Math.max(w, h) < 0.01;
}

function CarouselPickerModal({
  count,
  carousels,
  onPick,
  onClose,
}: {
  count: number;
  carousels: ConventionQueueItem[];
  onPick: (itemId?: string) => void;
  onClose: () => void;
}) {
  return (
    <Modal onClose={onClose} className="conv-carousel-picker">
      <div className="modal-header">
        <h2>Add {count === 1 ? 'photo' : `${count} photos`} to a carousel</h2>
        <button className="btn btn-ghost btn-sm" onClick={onClose}><X size={18} /></button>
      </div>
      <div className="conv-carousel-picker-list">
        <button className="conv-carousel-option new" onClick={() => onPick()}>
          <span className="conv-carousel-option-thumbs"><Plus size={22} /></span>
          <span className="conv-carousel-option-text">
            <strong>New carousel</strong>
            <span>Starts a new carousel post with {count === 1 ? 'this photo' : 'these photos'}</span>
          </span>
        </button>
        {[...carousels].reverse().map(item => {
          const images = item.imageUrls ?? [];
          return (
            <button key={item.id} className="conv-carousel-option" onClick={() => onPick(item.id)}>
              <span className="conv-carousel-option-thumbs">
                {images.slice(0, 4).map((src, i) => <img key={i} src={src} alt="" loading="lazy" />)}
              </span>
              <span className="conv-carousel-option-text">
                <strong>{images.length} photo{images.length !== 1 ? 's' : ''} · {item.status}</strong>
                <span>{item.caption ? item.caption.slice(0, 80) : 'No caption yet'}</span>
              </span>
            </button>
          );
        })}
      </div>
      <div className="modal-footer">
        <button className="btn btn-secondary" onClick={onClose}>Cancel</button>
      </div>
    </Modal>
  );
}

export function GallerySection({
  queueId,
  photos,
  items,
  onPhotosRemoved,
  onPhotoUpdated,
  onItemsChanged,
  overlayImg,
  overlayName,
}: {
  queueId: string;
  photos: ConventionGalleryPhoto[];
  items: ConventionQueueItem[];
  onPhotosRemoved: (ids: string[]) => void;
  onPhotoUpdated: (photo: ConventionGalleryPhoto) => void;
  onItemsChanged: (items: ConventionQueueItem[]) => void;
  overlayImg?: HTMLImageElement | null;
  overlayName?: string;
}) {
  // Picked photo IDs in the order they were picked.
  const [picked, setPicked] = useState<string[]>([]);
  const [viewing, setViewing] = useState<ConventionGalleryPhoto | null>(null);
  // The photos the carousel picker is open for.
  const [carouselFor, setCarouselFor] = useState<string[] | null>(null);
  const [busy, setBusy] = useState(false);
  const [cropping, setCropping] = useState<ConventionGalleryPhoto | null>(null);
  // IDs of the photos whose loaded image is not square.
  const [notSquare, setNotSquare] = useState<Set<string>>(new Set());

  const measure = (id: string, img: HTMLImageElement) => {
    const off = !isSquare(img);
    setNotSquare(prev => {
      if (prev.has(id) === off) return prev;
      const next = new Set(prev);
      if (off) next.add(id);
      else next.delete(id);
      return next;
    });
  };

  // A photo moved or deleted elsewhere (another device) leaves the selection.
  const selection = picked.filter(id => photos.some(p => p.id === id));
  const carousels = carouselTargets(items);
  const uncropped = selection.filter(id => notSquare.has(id)).length;

  const togglePick = (id: string) => {
    setPicked(prev => (prev.includes(id) ? prev.filter(x => x !== id) : [...prev, id]));
  };

  const move = async (ids: string[], target: 'single' | 'carousel', itemId?: string) => {
    if (ids.length === 0) return;
    setBusy(true);
    try {
      const res = await api.moveConventionGalleryPhotos(queueId, ids, target, itemId);
      onPhotosRemoved(ids);
      onItemsChanged(res.items);
      setPicked(prev => prev.filter(id => !ids.includes(id)));
      setViewing(v => (v && ids.includes(v.id) ? null : v));
      const n = ids.length === 1 ? 'Photo' : `${ids.length} photos`;
      if (target === 'single') toast.success(`${n} moved to the queue as single post${ids.length > 1 ? 's' : ''}`);
      else if (itemId) toast.success(`${n} added to the carousel`);
      else toast.success(`${n} moved to a new carousel`);
    } catch (err) {
      toast.error(err instanceof Error ? err.message : 'Failed to move photos');
    } finally {
      setBusy(false);
    }
  };

  const remove = async (photo: ConventionGalleryPhoto) => {
    if (!confirm('Delete this photo from the gallery?')) return;
    try {
      await api.deleteConventionGalleryPhoto(queueId, photo.id);
      onPhotosRemoved([photo.id]);
      setPicked(prev => prev.filter(id => id !== photo.id));
      setViewing(v => (v?.id === photo.id ? null : v));
    } catch (err) {
      toast.error(err instanceof Error ? err.message : 'Failed to delete photo');
    }
  };

  // Like a queue item's crop, the crop is uploaded without the queue's overlay,
  // which the backend stamps on when it posts the image.
  const applyCrop = async (blob: Blob) => {
    const photo = cropping;
    if (!photo) return;
    setCropping(null);
    try {
      const updated = await api.replaceConventionGalleryPhoto(
        queueId, photo.id, new File([blob], 'cropped.jpg', { type: 'image/jpeg' }),
      );
      onPhotoUpdated(updated);
      toast.success('Photo cropped');
    } catch (err) {
      toast.error(err instanceof Error ? err.message : 'Failed to crop the photo');
    }
  };

  // With no carousel to add to, "Carousel" makes a new one straight away.
  const toCarousel = (ids: string[]) => {
    if (carousels.length === 0) move(ids, 'carousel');
    else setCarouselFor(ids);
  };

  if (photos.length === 0) return null;

  return (
    <section className="conv-gallery">
      <div className="conv-gallery-header">
        <GalleryHorizontalEnd size={16} />
        <h2>Gallery</h2>
        <span className="conv-gallery-count">{photos.length} photo{photos.length !== 1 ? 's' : ''}</span>
        {selection.length > 0 && (
          <button className="btn btn-ghost btn-sm conv-gallery-clear" onClick={() => setPicked([])}>
            Clear selection
          </button>
        )}
      </div>
      <p className="conv-upload-hint">
        Tap photos to pick them, then move them into the queue as single posts or into a carousel.
        A carousel takes them in the order you picked them.
      </p>

      <div className="conv-gallery-grid">
        {photos.map(photo => {
          const order = selection.indexOf(photo.id);
          return (
            <div key={photo.id} className={`conv-gallery-tile ${order >= 0 ? 'picked' : ''}`}>
              <button
                type="button"
                className="conv-gallery-pick"
                onClick={() => togglePick(photo.id)}
                aria-pressed={order >= 0}
                title={order >= 0 ? 'Unpick' : 'Pick'}
              >
                <img
                  src={photo.imageUrl}
                  alt=""
                  loading="lazy"
                  onLoad={e => measure(photo.id, e.currentTarget)}
                />
              </button>
              {order >= 0 && <span className="conv-gallery-order">{order + 1}</span>}
              {order >= 0 && notSquare.has(photo.id) && (
                <span className="conv-gallery-not-square" title={NOT_SQUARE_HINT} aria-label={NOT_SQUARE_HINT}>
                  <AlertTriangle size={12} /> Not square
                </span>
              )}
              <button
                type="button"
                className="conv-item-edit-btn conv-item-crop-btn conv-gallery-view-btn"
                onClick={() => setCropping(photo)}
                title="Crop photo"
              >
                <Crop size={12} />
              </button>
              <button
                type="button"
                className="conv-item-edit-btn conv-gallery-view-btn"
                onClick={() => setViewing(photo)}
                title="View photo"
              >
                <Maximize2 size={12} />
              </button>
            </div>
          );
        })}
      </div>

      {selection.length > 0 && (
        <div className="conv-gallery-actions">
          <span className="conv-gallery-selected">
            {selection.length} picked
            {uncropped > 0 && (
              <span className="conv-gallery-not-square-note" title={NOT_SQUARE_HINT}>
                <AlertTriangle size={12} /> {uncropped} not cropped square
              </span>
            )}
          </span>
          <button className="btn btn-sm btn-secondary" disabled={busy} onClick={() => move(selection, 'single')}>
            {busy ? <Loader size={13} className="spin" /> : <ImageIcon size={13} />}
            {selection.length === 1 ? 'Single post' : 'Single posts'}
          </button>
          <button className="btn btn-sm btn-primary" disabled={busy} onClick={() => toCarousel(selection)}>
            {busy ? <Loader size={13} className="spin" /> : <Images size={13} />}
            Carousel…
          </button>
        </div>
      )}

      {viewing && (
        <Modal onClose={() => setViewing(null)} className="conv-gallery-viewer">
          <div className="modal-header">
            <h2>Gallery photo</h2>
            <button className="btn btn-ghost btn-sm" onClick={() => setViewing(null)}><X size={18} /></button>
          </div>
          <div className="conv-gallery-viewer-body">
            <img src={viewing.imageUrl} alt="" />
          </div>
          <div className="modal-footer">
            <button className="icon-btn danger" onClick={() => remove(viewing)} title="Delete from the gallery">
              <Trash2 size={14} />
            </button>
            <button className="btn btn-secondary" onClick={() => { setCropping(viewing); setViewing(null); }}>
              <Crop size={14} /> Crop
            </button>
            <button className="btn btn-secondary" onClick={() => togglePick(viewing.id)}>
              {selection.includes(viewing.id) ? 'Unpick' : 'Pick'}
            </button>
            <button className="btn btn-secondary" disabled={busy} onClick={() => move([viewing.id], 'single')}>
              <ImageIcon size={14} /> Single post
            </button>
            <button className="btn btn-primary" disabled={busy} onClick={() => toCarousel([viewing.id])}>
              <Images size={14} /> Carousel…
            </button>
          </div>
        </Modal>
      )}

      {cropping && (
        <ImageCropper
          imageUrl={cropping.imageUrl}
          onApply={applyCrop}
          onCancel={() => setCropping(null)}
          watermarkImg={overlayImg}
          watermarkName={overlayImg ? overlayName : undefined}
          bakeWatermark={false}
        />
      )}

      {carouselFor && (
        <CarouselPickerModal
          count={carouselFor.length}
          carousels={carousels}
          onPick={itemId => {
            const ids = carouselFor;
            setCarouselFor(null);
            move(ids, 'carousel', itemId);
          }}
          onClose={() => setCarouselFor(null)}
        />
      )}
    </section>
  );
}
