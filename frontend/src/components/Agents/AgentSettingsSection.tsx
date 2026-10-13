import { useEffect, useState } from 'react';
import { Link } from 'react-router-dom';
import { Bot, Trash2 } from 'lucide-react';
import toast from 'react-hot-toast';
import { api } from '../../services/api';
import type { AgentSettings } from '../../types';
import { ModelPicker } from './ModelPicker';
import './Agents.css';

/**
 * The Profile page's Agents section: the OpenRouter key and default model
 * the user's agents call. The key is write-only; the server only says
 * whether one is stored and its last characters.
 */
export function AgentSettingsSection() {
  const [settings, setSettings] = useState<AgentSettings | null>(null);
  const [key, setKey] = useState('');
  const [model, setModel] = useState('');
  const [saving, setSaving] = useState(false);

  useEffect(() => {
    api.getAgentSettings()
      .then(s => { setSettings(s); setModel(s.model); })
      .catch(() => {});
  }, []);

  const save = async (data: { openRouterKey?: string; model?: string }, message: string) => {
    setSaving(true);
    try {
      const s = await api.updateAgentSettings(data);
      setSettings(s);
      setModel(s.model);
      setKey('');
      toast.success(message);
    } catch (err) {
      toast.error(err instanceof Error ? err.message : 'Failed to save');
    } finally {
      setSaving(false);
    }
  };

  const saveAll = () => {
    const data: { openRouterKey?: string; model?: string } = { model: model.trim() };
    if (key.trim()) data.openRouterKey = key.trim();
    save(data, 'Agent settings saved');
  };

  const removeKey = () => {
    if (!window.confirm('Remove the OpenRouter key? Your agents stop running until you add one again.')) return;
    save({ openRouterKey: '' }, 'Key removed');
  };

  if (!settings) return null;
  const changed = key.trim() !== '' || model.trim() !== settings.model;

  return (
    <>
      <h3 style={{ marginBottom: 12, display: 'flex', alignItems: 'center', gap: 8 }}>
        <Bot size={18} /> Agents
      </h3>
      <p style={{ color: 'var(--text-secondary)', fontSize: 14, marginBottom: 16 }}>
        <Link to="/agents">Agents</Link> run your prompts on a schedule with the same tools as the MCP server.
        They call the model through your own <a href="https://openrouter.ai/keys" target="_blank" rel="noreferrer">OpenRouter</a> key,
        so their usage is billed to your OpenRouter account.
      </p>

      <div style={{ display: 'flex', flexDirection: 'column', gap: 12, marginBottom: 16 }}>
        <div className="form-group">
          <label>OpenRouter API Key</label>
          <div style={{ display: 'flex', gap: 8 }}>
            <input
              className="input"
              type="password"
              style={{ flex: 1 }}
              placeholder={settings.hasKey ? `Key saved (${settings.keyHint}); enter a new one to replace it` : 'sk-or-v1-...'}
              value={key}
              onChange={e => setKey(e.target.value)}
              autoComplete="off"
            />
            {settings.hasKey && (
              <button className="btn btn-ghost btn-sm btn-danger-text" onClick={removeKey} disabled={saving} title="Remove the key">
                <Trash2 size={14} />
              </button>
            )}
          </div>
        </div>
        <div className="form-group">
          <label>Default Model</label>
          <ModelPicker value={model} onChange={setModel} />
          <span className="agent-hint">Used by every agent that does not pick its own model. It must support tool calling.</span>
        </div>
      </div>

      <button className="btn btn-primary" onClick={saveAll} disabled={saving || !changed}>
        {saving ? 'Saving...' : 'Save Agent Settings'}
      </button>

      <hr style={{ border: 'none', borderTop: '1px solid var(--border)', margin: '24px 0' }} />
    </>
  );
}
