import { useCallback, useEffect, useRef, useState } from 'react';
import { Brush, Check, Film, FolderOpen, LayoutTemplate, Palette, Pencil, Search, Trash2, Type, Upload, X } from 'lucide-react';
import toast from 'react-hot-toast';
import { api } from '../../services/api';
import type { MediaItem, Watermark } from '../../types';
import { ImageEditor } from '../Common/ImageEditor/ImageEditor';
import { parseTemplate, type EditorTemplate } from '../Common/ImageEditor/editorModel';
import { ColorField, ColorFormModal } from './ColorFormModal';
import { TemplateFormModal, type NewTemplate } from './TemplateFormModal';
import { HEX_COLOR } from './mediaColor';
import '../Admin/Admin.css';
import './MediaPage.css';

const apiUrl = import.meta.env.VITE_API_URL || '';
const src = (url: string) => (url.startsWith('/') ? apiUrl + url : url);

function parseTags(s: string): string[] {
  return s.split(/[,;]/).map(t => t.trim().replace(/^#/, '').toLowerCase()).filter(Boolean);
}

const isFont = (item: MediaItem) => !!item.contentType?.startsWith('font/');
const isTemplate = (item: MediaItem) => !!item.contentType?.startsWith('template/');

// The template open in the image editor: a new one, or a library entry.
type Designing = { kind: 'new'; info: NewTemplate } | { kind: 'edit'; item: MediaItem; template: EditorTemplate };

/** A library font, shown as a sample set in it once the browser has loaded it. */
function FontPreview({ item }: { item: MediaItem }) {
  const family = `SocialPod Media ${item.id}`;
  const [loaded, setLoaded] = useState(false);

  useEffect(() => {
    let cancelled = false;
    const face = new FontFace(family, `url(${src(item.url)})`);
    face.load()
      .then(f => {
        if (cancelled) return;
        document.fonts.add(f);
        setLoaded(true);
      })
      .catch(() => {});
    return () => { cancelled = true; };
  }, [family, item.url]);

  if (!loaded) return <Type size={40} />;
  return (
    <div className="media-font-sample" style={{ fontFamily: `"${family}"` }}>
      <span>Aa Bb Cc</span>
      <span>ÄÖÜ äöü 0123</span>
    </div>
  );
}

/**
 * The team's media library: photos, artwork, logos and backgrounds with a
 * description and tags. People upload material here; MCP agents search it
 * (list_media) and build post images from it (compose_image).
 */
export function MediaPage() {
  const [items, setItems] = useState<MediaItem[]>([]);
  const [loading, setLoading] = useState(true);
  const [query, setQuery] = useState('');
  const [tagFilter, setTagFilter] = useState<string[]>([]);
  const [uploadTags, setUploadTags] = useState('');
  const [uploading, setUploading] = useState(false);
  const [editing, setEditing] = useState<string | null>(null);
  const [editName, setEditName] = useState('');
  const [editDescription, setEditDescription] = useState('');
  const [editTags, setEditTags] = useState('');
  const [editColor, setEditColor] = useState('');
  const [addingColor, setAddingColor] = useState(false);
  const [newTemplate, setNewTemplate] = useState(false);
  const [designing, setDesigning] = useState<Designing | null>(null);
  // For the editor's Watermark tab, loaded when a template is first opened.
  const [watermarks, setWatermarks] = useState<Watermark[] | null>(null);
  const fileRef = useRef<HTMLInputElement>(null);

  const load = useCallback(() => {
    setLoading(true);
    api.getMedia({ q: query.trim(), tags: tagFilter, limit: 200 })
      .then(setItems)
      .catch(() => toast.error('Failed to load media'))
      .finally(() => setLoading(false));
  }, [query, tagFilter]);

  useEffect(() => {
    const t = setTimeout(load, 250);
    return () => clearTimeout(t);
  }, [load]);

  const handleUpload = async (files: FileList | null) => {
    if (!files || files.length === 0) return;
    setUploading(true);
    let done = 0;
    try {
      for (const file of Array.from(files)) {
        await api.uploadMedia(file, { tags: parseTags(uploadTags) });
        done++;
      }
      toast.success(done === 1 ? 'File added' : `${done} files added`);
      load();
    } catch (err) {
      toast.error(err instanceof Error ? err.message : 'Upload failed');
      if (done > 0) load();
    } finally {
      setUploading(false);
      if (fileRef.current) fileRef.current.value = '';
    }
  };

  const startEdit = (item: MediaItem) => {
    setEditing(item.id);
    setEditName(item.name);
    setEditDescription(item.description || '');
    setEditTags((item.tags || []).join(', '));
    setEditColor(item.color || '');
  };

  const saveEdit = async (item: MediaItem) => {
    const id = item.id;
    if (item.color && !HEX_COLOR.test(editColor.trim())) {
      toast.error('Enter the colour as #rrggbb or #rrggbbaa');
      return;
    }
    try {
      const updated = await api.updateMedia(id, {
        name: editName.trim(),
        description: editDescription.trim(),
        tags: parseTags(editTags),
        ...(item.color ? { color: editColor.trim() } : {}),
      });
      setItems(prev => prev.map(i => (i.id === id ? updated : i)));
      setEditing(null);
    } catch (err) {
      toast.error(err instanceof Error ? err.message : 'Failed to save');
    }
  };

  const handleDelete = async (id: string) => {
    if (!window.confirm('Remove this file from the media library?')) return;
    try {
      await api.deleteMedia(id);
      setItems(prev => prev.filter(i => i.id !== id));
    } catch {
      toast.error('Failed to delete');
    }
  };

  const openDesign = (d: Designing) => {
    setDesigning(d);
    if (watermarks === null) api.getWatermarks().then(setWatermarks).catch(() => setWatermarks([]));
  };

  const editDesign = (item: MediaItem) => {
    const template = parseTemplate(item.template);
    if (!template) { toast.error('This template cannot be read'); return; }
    openDesign({ kind: 'edit', item, template });
  };

  // The editor's render becomes the template's preview. Errors propagate, so
  // the editor stays open with the design.
  const saveDesign = async (blob: Blob, design: EditorTemplate) => {
    if (!designing) return;
    if (designing.kind === 'new') {
      const { name, description, tags } = designing.info;
      const item = await api.createMediaTemplate(blob, design, { name, description, tags });
      setItems(prev => [item, ...prev]);
      toast.success('Template added');
    } else {
      const item = await api.updateMediaTemplate(designing.item.id, blob, design);
      setItems(prev => prev.map(i => (i.id === item.id ? item : i)));
      toast.success('Template saved');
    }
    setDesigning(null);
  };

  const toggleTag = (tag: string) => {
    setTagFilter(prev => (prev.includes(tag) ? prev.filter(t => t !== tag) : [...prev, tag]));
  };

  return (
    <div className="page">
      <div className="page-header">
        <h1>Media Library</h1>
      </div>

      <p className="watermarks-description">
        Photos, artwork, logos, backgrounds and fonts for post images. Describe what each file shows and tag it: MCP agents
        search the library by name, description and tags and compose post images from it together with the watermarks,
        setting their text in the fonts uploaded here (.ttf or .otf) and following the team colours: say in each
        colour's description how and where to use it. Templates are designs for the image editor, which posts and stories
        can start from when an image is created.
      </p>

      <div className="media-toolbar">
        <div className="media-search">
          <Search size={16} />
          <input
            className="input"
            placeholder="Search name, description, tags"
            value={query}
            onChange={e => setQuery(e.target.value)}
          />
        </div>
        <input
          className="input media-upload-tags"
          placeholder="Tags for new uploads (comma-separated)"
          value={uploadTags}
          onChange={e => setUploadTags(e.target.value)}
        />
        <input
          ref={fileRef}
          type="file"
          multiple
          accept="image/png,image/jpeg,image/gif,image/webp,video/mp4,video/quicktime,.ttf,.otf,font/ttf,font/otf"
          style={{ display: 'none' }}
          onChange={e => handleUpload(e.target.files)}
        />
        <button className="btn btn-primary" onClick={() => fileRef.current?.click()} disabled={uploading}>
          <Upload size={16} /> {uploading ? 'Uploading...' : 'Upload'}
        </button>
        <button className="btn btn-secondary" onClick={() => setAddingColor(true)}>
          <Palette size={16} /> Add Colour
        </button>
        <button className="btn btn-secondary" onClick={() => setNewTemplate(true)}>
          <LayoutTemplate size={16} /> New Template
        </button>
      </div>

      {tagFilter.length > 0 && (
        <div className="media-tag-filter">
          {tagFilter.map(t => (
            <button key={t} className="media-tag active" onClick={() => toggleTag(t)} title="Remove filter">
              #{t} <X size={12} />
            </button>
          ))}
        </div>
      )}

      {!loading && items.length === 0 ? (
        <div className="empty-state">
          <FolderOpen size={32} />
          <p>{query || tagFilter.length ? 'Nothing matches the search.' : 'No media yet. Upload material, add colours or design templates to build post images from.'}</p>
        </div>
      ) : (
        <div className="watermark-grid">
          {items.map(item => (
            <div key={item.id} className="watermark-card">
              <div className={`watermark-preview media-preview${isTemplate(item) ? ' media-template' : ''}`}>
                {item.color ? (
                  <div className="media-color-swatch" style={{ background: item.color }}>
                    <span>{item.color}</span>
                  </div>
                ) : item.contentType?.startsWith('video/') ? (
                  <Film size={40} />
                ) : isFont(item) ? (
                  <FontPreview item={item} />
                ) : (
                  <img src={src(item.url)} alt={item.name} loading="lazy" />
                )}
                {isTemplate(item) && (
                  <span className="media-badge"><LayoutTemplate size={12} /> Template</span>
                )}
              </div>
              {editing === item.id ? (
                <div className="watermark-edit">
                  <input className="input" value={editName} onChange={e => setEditName(e.target.value)} placeholder="Name" />
                  {item.color && <ColorField value={editColor} onChange={setEditColor} />}
                  <textarea
                    className="input"
                    rows={3}
                    value={editDescription}
                    onChange={e => setEditDescription(e.target.value)}
                    placeholder={item.color ? 'How to use this colour' : 'What the file shows'}
                  />
                  <input className="input" value={editTags} onChange={e => setEditTags(e.target.value)} placeholder="Tags, comma-separated" />
                  <div className="watermark-edit-actions">
                    <button className="btn btn-ghost btn-sm" onClick={() => setEditing(null)} title="Cancel">
                      <X size={14} />
                    </button>
                    <button className="btn btn-primary btn-sm" onClick={() => saveEdit(item)} title="Save">
                      <Check size={14} />
                    </button>
                  </div>
                </div>
              ) : (
                <>
                  <div className="watermark-info">
                    <span className="watermark-name" title={item.name}>{item.name}</span>
                    <span className="watermark-actions">
                      {isTemplate(item) && (
                        <button className="btn btn-ghost btn-sm" onClick={() => editDesign(item)} title="Edit design">
                          <Brush size={14} />
                        </button>
                      )}
                      <button className="btn btn-ghost btn-sm" onClick={() => startEdit(item)} title="Edit">
                        <Pencil size={14} />
                      </button>
                      <button className="btn btn-ghost btn-sm btn-danger-text" onClick={() => handleDelete(item.id)} title="Delete">
                        <Trash2 size={14} />
                      </button>
                    </span>
                  </div>
                  {item.description && <p className="watermark-desc">{item.description}</p>}
                  {item.tags && item.tags.length > 0 && (
                    <div className="media-tags">
                      {item.tags.map(t => (
                        <button
                          key={t}
                          className={`media-tag${tagFilter.includes(t) ? ' active' : ''}`}
                          onClick={() => toggleTag(t)}
                          title="Filter by this tag"
                        >
                          #{t}
                        </button>
                      ))}
                    </div>
                  )}
                </>
              )}
            </div>
          ))}
        </div>
      )}
      {newTemplate && (
        <TemplateFormModal
          onClose={() => setNewTemplate(false)}
          onContinue={info => {
            setNewTemplate(false);
            openDesign({ kind: 'new', info });
          }}
        />
      )}
      {designing && (
        <ImageEditor
          canvas={designing.kind === 'new'
            ? { width: designing.info.width, height: designing.info.height, background: '#ffffff' }
            : undefined}
          template={designing.kind === 'edit' ? designing.template : undefined}
          title={designing.kind === 'new' ? `New template: ${designing.info.name}` : `Template: ${designing.item.name}`}
          watermarks={watermarks ?? []}
          onSave={saveDesign}
          onCancel={() => setDesigning(null)}
        />
      )}
      {addingColor && (
        <ColorFormModal
          onClose={() => setAddingColor(false)}
          onCreated={item => {
            setAddingColor(false);
            setItems(prev => [item, ...prev]);
            toast.success('Colour added');
          }}
        />
      )}
    </div>
  );
}
