import { useEffect, useRef, useState } from 'react';
import { format } from 'date-fns';
import { Clapperboard, FileText, Loader, RefreshCw, Send, Upload, X } from 'lucide-react';
import toast from 'react-hot-toast';
import { api } from '../../services/api';
import type { ConventionQueue } from '../../types';
import { Modal } from '../Common/Modal';
import { parseTemplate, type EditorTemplate } from '../Common/ImageEditor/editorModel';
import { renderQuickReel } from '../Common/ImageEditor/quickReel';
import './Convention.css';

const INSTAGRAM_LIMIT = 2200;
const REEL_DELAY_MS = 30 * 60 * 1000;

// Quick Reel, for a convention floor with no time to spare: pick a clip, say
// in a few words what it shows, and the queue's reel template is rendered
// around it while the AI writes the Instagram caption. The reel itself is
// only previewed, never edited; the caption can be. It is scheduled for
// Instagram half an hour after the clip was uploaded, or kept as a draft to
// finish in the PostEditor.

type Step = 'pick' | 'uploading' | 'hints' | 'working' | 'review' | 'publishing';

const percent = (share: number) => `${Math.round(share * 100)}%`;

function ProgressBar({ share, label }: { share: number; label: string }) {
  return (
    <div className="qr-progress">
      <div className="qr-progress-label">
        <span>{label}</span>
        <span>{percent(share)}</span>
      </div>
      <div className="qr-progress-track">
        <div className="qr-progress-fill" style={{ width: percent(share) }} />
      </div>
    </div>
  );
}

export function QuickReel({ queue, onClose }: { queue: ConventionQueue; onClose: () => void }) {
  const [step, setStep] = useState<Step>('pick');
  const [template, setTemplate] = useState<EditorTemplate | null>(null);
  const [templateError, setTemplateError] = useState('');
  const [uploadShare, setUploadShare] = useState(0);
  const [clipUrl, setClipUrl] = useState('');
  const [uploadedAt, setUploadedAt] = useState<Date | null>(null);
  const [hints, setHints] = useState('');
  const [renderShare, setRenderShare] = useState(0);
  const [reel, setReel] = useState<Blob | null>(null);
  const [reelUrl, setReelUrl] = useState('');
  const reelUrlRef = useRef('');
  const [caption, setCaption] = useState('');
  const [captionBusy, setCaptionBusy] = useState(false);
  const [captionError, setCaptionError] = useState('');
  const [publishShare, setPublishShare] = useState(0);
  const [savingDraft, setSavingDraft] = useState(false);
  const fileRef = useRef<HTMLInputElement>(null);
  const abortRef = useRef<AbortController | null>(null);
  const clipRef = useRef('');
  const uploadAbortRef = useRef<AbortController | null>(null);
  const closedRef = useRef(false);

  // The queue's template, looked up in the media library.
  useEffect(() => {
    let cancelled = false;
    api.getMedia({ kind: 'template', limit: 200 })
      .then(items => {
        if (cancelled) return;
        const t = parseTemplate(items.find(m => m.id === queue.reelTemplateId)?.template);
        if (!t) setTemplateError('The reel template of this convention is gone. Pick another one under Edit.');
        else setTemplate(t);
      })
      .catch(() => { if (!cancelled) setTemplateError('Failed to load the reel template'); });
    return () => { cancelled = true; };
  }, [queue.reelTemplateId]);

  // The uploaded clip is temporary: once the reel is rendered and published
  // nothing needs it, so it is let go when the dialog closes, or when the
  // page is left with the dialog open (closing the tab unmounts nothing).
  // Whatever slips through (no connection at the time) the server's hourly
  // cleanup frees a day later.
  const releaseClip = () => {
    if (!clipRef.current) return;
    void api.releaseTemp([clipRef.current]).catch(() => {});
    clipRef.current = '';
  };
  useEffect(() => {
    const onPageHide = (e: PageTransitionEvent) => { if (!e.persisted) releaseClip(); };
    window.addEventListener('pagehide', onPageHide);
    return () => {
      window.removeEventListener('pagehide', onPageHide);
      closedRef.current = true;
      abortRef.current?.abort();
      uploadAbortRef.current?.abort();
      releaseClip();
      if (reelUrlRef.current) URL.revokeObjectURL(reelUrlRef.current);
    };
  }, []);

  const pickClip = async (e: React.ChangeEvent<HTMLInputElement>) => {
    const file = e.target.files?.[0];
    e.target.value = '';
    if (!file) return;
    if (!file.type.startsWith('video/')) {
      toast.error('Pick a video clip');
      return;
    }
    setUploadShare(0);
    setStep('uploading');
    const controller = new AbortController();
    uploadAbortRef.current = controller;
    try {
      const { url } = await api.uploadTempWithProgress(file, setUploadShare, controller.signal);
      // Closed while the server was storing it: nothing will use it.
      if (closedRef.current) {
        void api.releaseTemp([url]).catch(() => {});
        return;
      }
      clipRef.current = url;
      setClipUrl(url);
      setUploadedAt(new Date());
      setStep('hints');
    } catch (err) {
      if (closedRef.current) return;
      toast.error(err instanceof Error ? err.message : 'Upload failed');
      setStep('pick');
    } finally {
      if (uploadAbortRef.current === controller) uploadAbortRef.current = null;
    }
  };

  const writeCaption = async () => {
    setCaptionBusy(true);
    setCaptionError('');
    try {
      const res = await api.quickReelCaption(queue.id, hints.trim());
      setCaption(res.caption);
    } catch (err) {
      setCaptionError(err instanceof Error ? err.message : 'The AI could not write a caption');
    } finally {
      setCaptionBusy(false);
    }
  };

  // Renders the reel and writes the caption side by side.
  const createReel = async () => {
    if (!template || !clipUrl || !hints.trim()) return;
    setStep('working');
    setRenderShare(0);
    setCaption('');
    const controller = new AbortController();
    abortRef.current = controller;
    const captionDone = writeCaption();
    try {
      const blob = await renderQuickReel({ template, clipUrl, onProgress: setRenderShare, signal: controller.signal });
      await captionDone;
      if (reelUrlRef.current) URL.revokeObjectURL(reelUrlRef.current);
      reelUrlRef.current = URL.createObjectURL(blob);
      setReel(blob);
      setReelUrl(reelUrlRef.current);
      setStep('review');
    } catch (err) {
      if (controller.signal.aborted) return;
      toast.error(err instanceof Error ? err.message : 'Failed to make the reel');
      setStep('hints');
    } finally {
      if (abortRef.current === controller) abortRef.current = null;
    }
  };

  const publishAt = uploadedAt
    ? new Date(Math.max(uploadedAt.getTime() + REEL_DELAY_MS, Date.now()))
    : null;

  // A draft may be unfinished: it needs no caption and is not checked.
  const publish = async (draft: boolean) => {
    if (!reel || !uploadedAt || (!draft && !caption.trim())) return;
    setPublishShare(0);
    setSavingDraft(draft);
    setStep('publishing');
    try {
      const post = await api.publishQuickReel(queue.id, reel, caption.trim(), uploadedAt, setPublishShare, draft);
      toast.success(draft ? 'Reel saved as draft' : `Reel scheduled for ${format(new Date(post.scheduledAt), 'HH:mm')}`);
      onClose();
    } catch (err) {
      toast.error(err instanceof Error ? err.message : draft ? 'Failed to save the draft' : 'Failed to schedule the reel');
      setStep('review');
    }
  };

  const captionLength = caption.length;

  // Once the reel is on its way it cannot be called back, so the dialog
  // stays until it is through.
  const handleClose = () => {
    if (step === 'publishing') return;
    if (step !== 'pick' && !window.confirm('Throw this reel away?')) return;
    onClose();
  };

  return (
    <Modal onClose={handleClose} closeOnBackdrop={false} className="conv-modal qr-modal">
      <div className="modal-header">
        <h2><Clapperboard size={18} /> Quick Reel</h2>
        <button className="btn btn-ghost btn-sm" onClick={handleClose} disabled={step === 'publishing'} aria-label="Close"><X size={18} /></button>
      </div>

      <div className="qr-body">
        {templateError && <p className="qr-error">{templateError}</p>}

        {step === 'pick' && !templateError && (
          <>
            <input ref={fileRef} type="file" accept="video/*" style={{ display: 'none' }} onChange={pickClip} />
            <button className="qr-pick" onClick={() => fileRef.current?.click()} disabled={!template}>
              {template ? <Upload size={32} /> : <Loader size={32} className="spin" />}
              <span className="qr-pick-title">Choose or film a clip</span>
              <span className="qr-pick-hint">It goes into this convention's reel template and out on Instagram 30 minutes after the upload.</span>
            </button>
          </>
        )}

        {step === 'uploading' && <ProgressBar share={uploadShare} label="Uploading the clip" />}

        {step === 'hints' && (
          <>
            <p className="qr-done">Clip uploaded{publishAt && <> · posts at <strong>{format(publishAt, 'HH:mm')}</strong></>}</p>
            <label className="qr-label" htmlFor="qr-hints">What is in the clip?</label>
            <textarea
              id="qr-hints"
              className="input qr-hints"
              value={hints}
              onChange={e => setHints(e.target.value)}
              placeholder="A few words for the AI: game, publisher, people, what is going on"
              rows={3}
              autoFocus
            />
            <button className="btn btn-primary qr-main" onClick={createReel} disabled={!hints.trim()}>
              <Clapperboard size={16} /> Create reel
            </button>
          </>
        )}

        {step === 'working' && (
          <>
            <ProgressBar share={renderShare} label="Rendering the reel" />
            <p className="qr-status">
              {captionBusy ? <><Loader size={14} className="spin" /> Writing the caption…</> : 'Caption ready'}
            </p>
          </>
        )}

        {(step === 'review' || step === 'publishing') && reelUrl && (
          <>
            <video className="qr-video" src={reelUrl} controls playsInline loop autoPlay muted />
            <div className="qr-caption-head">
              <label className="qr-label" htmlFor="qr-caption">Caption</label>
              <button className="btn btn-ghost btn-sm" onClick={writeCaption} disabled={captionBusy || step === 'publishing'}>
                {captionBusy ? <Loader size={13} className="spin" /> : <RefreshCw size={13} />} New text
              </button>
            </div>
            {captionError && <p className="qr-error">{captionError}</p>}
            <textarea
              id="qr-caption"
              className="input qr-caption"
              value={caption}
              onChange={e => setCaption(e.target.value)}
              rows={8}
              disabled={step === 'publishing' || captionBusy}
              placeholder="Write the caption"
            />
            <p className={`qr-count ${captionLength > INSTAGRAM_LIMIT ? 'over' : ''}`}>{captionLength}/{INSTAGRAM_LIMIT}</p>
            {step === 'publishing' ? (
              <ProgressBar share={publishShare} label={savingDraft ? 'Saving the draft' : 'Uploading the reel'} />
            ) : (
              <div className="qr-actions">
                <button className="btn btn-secondary qr-main" onClick={() => publish(true)} disabled={captionBusy}>
                  <FileText size={16} /> Save as draft
                </button>
                <button
                  className="btn btn-primary qr-main"
                  onClick={() => publish(false)}
                  disabled={!caption.trim() || captionLength > INSTAGRAM_LIMIT || captionBusy}
                >
                  <Send size={16} /> Publish{publishAt && <> at {format(publishAt, 'HH:mm')}</>}
                </button>
              </div>
            )}
          </>
        )}
      </div>
    </Modal>
  );
}
