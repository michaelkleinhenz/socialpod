import { useEffect, useRef, useState } from 'react';
import { ImagePlus, Loader, Palette, RotateCcw, X } from 'lucide-react';
import toast from 'react-hot-toast';
import { api } from '../../services/api';
import { Modal } from '../Common/Modal';
import { ImageEditor } from '../Common/ImageEditor/ImageEditor';

// A reel's cover: an image of its own (an upload, sent as cover_url) or a
// frame of the video (thumb_offset, in ms). Neither leaves it to Instagram,
// which takes the first frame.
export interface ReelCover {
  coverUrl: string;
  thumbOffset: number | null;
}

interface Props {
  // The reel's video, ready to play (an API URL or a local blob URL).
  videoSrc: string;
  apiUrl: string;
  cover: ReelCover;
  onApply: (cover: ReelCover) => void;
  onClose: () => void;
}

const formatSeconds = (t: number) => `${Math.floor(t / 60)}:${(t % 60).toFixed(1).padStart(4, '0')}`;

// Picks a reel's cover: scrub to a frame and use it, or turn that frame (or
// an image file) into a designed cover in the image editor, which is then
// uploaded and sent as the reel's own cover image.
export function ReelCoverPicker({ videoSrc, apiUrl, cover, onApply, onClose }: Props) {
  const videoRef = useRef<HTMLVideoElement>(null);
  const fileRef = useRef<HTMLInputElement>(null);
  const [pending, setPending] = useState<ReelCover>(cover);
  const [time, setTime] = useState((cover.thumbOffset ?? 0) / 1000);
  const [duration, setDuration] = useState(0);
  // The picture the image editor is open on (a frame or a picked file).
  const [designing, setDesigning] = useState<string | null>(null);
  const [uploading, setUploading] = useState(false);

  const coverSrc = pending.coverUrl ? (pending.coverUrl.startsWith('/') ? apiUrl + pending.coverUrl : pending.coverUrl) : '';
  const dirty = pending.coverUrl !== cover.coverUrl || pending.thumbOffset !== cover.thumbOffset;

  useEffect(() => () => { if (designing) URL.revokeObjectURL(designing); }, [designing]);

  const seek = (t: number) => {
    setTime(t);
    if (videoRef.current) videoRef.current.currentTime = t;
  };

  const useFrame = () => setPending({ coverUrl: '', thumbOffset: Math.round(time * 1000) });

  // The frame on show as a JPEG, opened in the image editor.
  const designFromFrame = () => {
    const v = videoRef.current;
    if (!v || !v.videoWidth) return;
    const canvas = document.createElement('canvas');
    canvas.width = v.videoWidth;
    canvas.height = v.videoHeight;
    try {
      canvas.getContext('2d')!.drawImage(v, 0, 0);
      canvas.toBlob(blob => {
        if (blob) setDesigning(URL.createObjectURL(blob));
        else toast.error('Could not take this frame');
      }, 'image/jpeg', 0.92);
    } catch {
      // A video from another origin that does not allow reading it.
      toast.error('This video cannot be read here; use the frame as it is instead');
    }
  };

  const pickFile = (file: File | undefined) => {
    if (!file) return;
    if (!file.type.startsWith('image/')) { toast.error('Pick an image file'); return; }
    setDesigning(URL.createObjectURL(file));
  };

  const saveDesign = async (blob: Blob) => {
    setUploading(true);
    try {
      const { url } = await api.uploadImage(new File([blob], 'reel-cover.jpg', { type: 'image/jpeg' }));
      setPending({ coverUrl: url, thumbOffset: null });
      setDesigning(null);
    } catch (err) {
      toast.error(`Upload failed: ${err instanceof Error ? err.message : err}`);
    } finally {
      setUploading(false);
    }
  };

  const status = pending.coverUrl
    ? 'Your own cover image'
    : pending.thumbOffset !== null
      ? `The frame at ${formatSeconds(pending.thumbOffset / 1000)}`
      : 'Instagram picks (the first frame)';

  return (
    <>
      <Modal onClose={onClose} dirty={dirty} className="reel-cover-modal">
        <div className="reel-cover-head">
          <h2>Reel cover</h2>
          <button className="btn btn-ghost btn-sm" onClick={onClose} aria-label="Close"><X size={18} /></button>
        </div>
        <div className="reel-cover-body">
          <div className="reel-cover-stage">
            <video
              ref={videoRef}
              src={videoSrc}
              crossOrigin={videoSrc.startsWith('blob:') ? undefined : 'anonymous'}
              muted
              playsInline
              preload="auto"
              className={coverSrc ? 'reel-cover-hidden' : ''}
              onLoadedMetadata={e => {
                const v = e.currentTarget;
                setDuration(Number.isFinite(v.duration) ? v.duration : 0);
                v.currentTime = Math.min(time, v.duration || 0);
              }}
            />
            {coverSrc && <img src={coverSrc} alt="Reel cover" />}
          </div>
          <div className="reel-cover-controls">
            <div className="reel-cover-status">Cover: <strong>{status}</strong></div>
            <label className="reel-cover-label" htmlFor="reel-cover-time">Pick a frame</label>
            <input
              id="reel-cover-time"
              type="range"
              min={0}
              max={Math.max(duration - 0.05, 0)}
              step={0.04}
              value={time}
              disabled={!duration}
              onChange={e => {
                // Scrubbing shows the video again over an own cover image.
                if (pending.coverUrl) setPending(p => ({ ...p, coverUrl: '' }));
                seek(Number(e.target.value));
              }}
            />
            <div className="reel-cover-time">{formatSeconds(time)} / {formatSeconds(duration)}</div>
            <button className="btn btn-primary btn-sm" onClick={useFrame} disabled={!duration || !!coverSrc}>
              Use this frame
            </button>
            <button className="btn btn-secondary btn-sm" onClick={designFromFrame} disabled={!duration || !!coverSrc || uploading}>
              <Palette size={14} /> Design a cover from this frame
            </button>
            <button className="btn btn-secondary btn-sm" onClick={() => fileRef.current?.click()} disabled={uploading}>
              {uploading ? <Loader size={14} className="post-editor-spin" /> : <ImagePlus size={14} />} Use an image
            </button>
            <button
              className="btn btn-ghost btn-sm"
              onClick={() => setPending({ coverUrl: '', thumbOffset: null })}
              disabled={!pending.coverUrl && pending.thumbOffset === null}
            >
              <RotateCcw size={14} /> Let Instagram pick
            </button>
            <p className="reel-cover-hint">
              The cover shows on your profile grid, cropped to 3:4 there, so keep titles near the middle.
            </p>
          </div>
        </div>
        <div className="modal-actions">
          <button className="btn btn-secondary" onClick={onClose}>Cancel</button>
          <button className="btn btn-primary" onClick={() => onApply(pending)}>Apply</button>
        </div>
        <input ref={fileRef} type="file" accept="image/*" hidden onChange={e => { pickFile(e.target.files?.[0]); e.target.value = ''; }} />
      </Modal>
      {designing && (
        <ImageEditor
          imageUrl={designing}
          safeZone="reel"
          title="Design reel cover"
          onCancel={() => setDesigning(null)}
          onSave={saveDesign}
        />
      )}
    </>
  );
}
