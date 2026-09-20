import { useState, useEffect } from 'react';
import { useNavigate } from 'react-router-dom';
import { api } from '../../services/api';
import { Bot, Link, FileText, Send, Loader, CheckCircle, AlertCircle, Newspaper, Mic, MessageSquare, Edit3 } from 'lucide-react';
import toast from 'react-hot-toast';
import './Agent.css';

type EntityType = 'news' | 'episode' | 'post';

interface ImageSourceInfo {
  type: string;
  url?: string;
  status?: string;
}

interface AgentResult {
  entityType: string;
  draft: { id: string; [key: string]: any };
  imageSources?: ImageSourceInfo[];
}

const EDIT_ROUTES: Record<string, string> = {
  news: '/news',
  episode: '/episodes',
  post: '/',
};

export function AgentPage() {
  const navigate = useNavigate();
  const [pluginReady, setPluginReady] = useState<boolean | null>(null);
  const [url, setUrl] = useState('');
  const [description, setDescription] = useState('');
  const [entityType, setEntityType] = useState<EntityType>('news');
  const [loading, setLoading] = useState(false);
  const [result, setResult] = useState<AgentResult | null>(null);
  const [error, setError] = useState<string | null>(null);

  useEffect(() => {
    api.getTeamSettings().then(s => {
      setPluginReady(!!s.enabledPlugins?.includes('agent'));
    }).catch(() => setPluginReady(false));
  }, []);

  const canSubmit = (url.trim() || description.trim()) && !loading;

  const handleGenerate = async () => {
    if (!canSubmit) return;
    setLoading(true);
    setResult(null);
    setError(null);
    try {
      const data: { url?: string; description?: string; entityType: string } = { entityType };
      if (url.trim()) data.url = url.trim();
      if (description.trim()) data.description = description.trim();
      const res = await api.agentGenerate(data);
      setResult(res);
      toast.success(`${entityType.charAt(0).toUpperCase() + entityType.slice(1)} draft created`);
    } catch (err: any) {
      const msg = err.message || 'Failed to generate content';
      setError(msg);
      toast.error(msg);
    } finally {
      setLoading(false);
    }
  };

  const handleReset = () => {
    setUrl('');
    setDescription('');
    setResult(null);
    setError(null);
  };

  if (pluginReady === null) {
    return <div className="agent-page"><div className="spinner" /></div>;
  }
  if (!pluginReady) {
    return (
      <div className="agent-page">
        <div className="agent-unavailable">
          <Bot size={48} />
          <h2>Agent Plugin Not Enabled</h2>
          <p>Ask your team administrator to enable the Agent plugin in team settings.</p>
        </div>
      </div>
    );
  }

  return (
    <div className="agent-page">
      <div className="agent-header">
        <Bot size={24} />
        <h1>Agent</h1>
      </div>
      <p className="agent-subtitle">
        Generate news articles, episode descriptions, or social media posts automatically from a URL or description.
        All content is saved as a draft for your review before publishing.
      </p>

      <div className="agent-form">
        <div className="agent-type-selector">
          <label>What should I create?</label>
          <div className="agent-type-buttons">
            <button
              className={`agent-type-btn ${entityType === 'news' ? 'active' : ''}`}
              onClick={() => setEntityType('news')}
              disabled={loading}
            >
              <Newspaper size={18} />
              <span>News Article</span>
            </button>
            <button
              className={`agent-type-btn ${entityType === 'episode' ? 'active' : ''}`}
              onClick={() => setEntityType('episode')}
              disabled={loading}
            >
              <Mic size={18} />
              <span>Episode</span>
            </button>
            <button
              className={`agent-type-btn ${entityType === 'post' ? 'active' : ''}`}
              onClick={() => setEntityType('post')}
              disabled={loading}
            >
              <MessageSquare size={18} />
              <span>Post</span>
            </button>
          </div>
        </div>

        <div className="form-group">
          <label><Link size={14} /> Source URL</label>
          <input
            type="url"
            className="input"
            placeholder="https://example.com/article..."
            value={url}
            onChange={e => setUrl(e.target.value)}
            disabled={loading}
          />
          <span className="form-hint">
            The agent will extract content from this URL to generate the draft.
          </span>
        </div>

        <div className="form-group">
          <label><FileText size={14} /> Description</label>
          <textarea
            className="input"
            style={{ minHeight: 100, resize: 'vertical', fontFamily: 'inherit' }}
            placeholder="Describe what the content should be about..."
            value={description}
            onChange={e => setDescription(e.target.value)}
            disabled={loading}
          />
          <span className="form-hint">
            Provide additional context or instructions. You can use just a URL, just a description, or both.
          </span>
        </div>

        <div className="agent-actions">
          <button
            className="btn btn-primary"
            onClick={handleGenerate}
            disabled={!canSubmit}
          >
            {loading ? (
              <>
                <Loader size={16} className="spinning" />
                <span>Generating...</span>
              </>
            ) : (
              <>
                <Send size={16} />
                <span>Generate Draft</span>
              </>
            )}
          </button>
        </div>
      </div>

      {loading && (
        <div className="agent-loading">
          <Loader size={32} className="spinning" />
          <p>The agent is analyzing the content and creating your draft. This may take a moment...</p>
        </div>
      )}

      {error && (
        <div className="agent-result agent-error">
          <AlertCircle size={20} />
          <div>
            <strong>Generation Failed</strong>
            <p>{error}</p>
          </div>
        </div>
      )}

      {result && (
        <div className="agent-result agent-success">
          <CheckCircle size={20} />
          <div>
            <strong>Draft Created</strong>
            <p>Your {result.entityType} draft has been saved. You can edit it before publishing.</p>

            {result.imageSources && result.imageSources.length > 0 && (
              <div className="agent-image-info">
                <h4>Image Sources</h4>
                <ul>
                  {result.imageSources.map((src, i) => (
                    <li key={i}>
                      <strong>{src.type}</strong>
                      {src.url && <span className="agent-image-url"> — {src.url}</span>}
                      {src.status && <span className="agent-image-status"> ({src.status})</span>}
                    </li>
                  ))}
                </ul>
              </div>
            )}

            <div style={{ display: 'flex', gap: 8, marginTop: 12 }}>
              <button
                className="btn btn-primary"
                onClick={() => {
                  const route = EDIT_ROUTES[result.entityType] || '/';
                  navigate(`${route}?editDraft=${result.draft.id}`);
                }}
              >
                <Edit3 size={16} /> Edit Draft
              </button>
              <button className="btn btn-secondary" onClick={handleReset}>
                Create Another
              </button>
            </div>
          </div>
        </div>
      )}
    </div>
  );
}
