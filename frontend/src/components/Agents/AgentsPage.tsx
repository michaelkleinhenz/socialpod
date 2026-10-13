import { useCallback, useEffect, useState } from 'react';
import { Link } from 'react-router-dom';
import { AlertTriangle, Bot, History, Pencil, Play, Plus, Square, Trash2 } from 'lucide-react';
import toast from 'react-hot-toast';
import { api } from '../../services/api';
import type { Agent, AgentInput, AgentSettings } from '../../types';
import { AgentFormModal } from './AgentFormModal';
import { AgentRunsModal } from './AgentRunsModal';
import { STATUS_BADGE, STATUS_LABEL, describeSchedule, formatDateTime } from './agentSchedule';
import './Agents.css';

/**
 * Agents: prompts that run on a schedule with the user's MCP tools, through
 * the OpenRouter key and model set on the Profile page. The global prompt
 * applies to every agent on top of its own.
 */
export function AgentsPage() {
  const [agents, setAgents] = useState<Agent[]>([]);
  const [loading, setLoading] = useState(true);
  const [settings, setSettings] = useState<AgentSettings | null>(null);
  const [globalPrompt, setGlobalPrompt] = useState('');
  const [savingGlobal, setSavingGlobal] = useState(false);
  const [editing, setEditing] = useState<Agent | 'new' | null>(null);
  const [history, setHistory] = useState<{ agent: Agent; runId?: string } | null>(null);

  const load = useCallback(() => {
    return api.getAgents()
      .then(setAgents)
      .catch(err => toast.error(err instanceof Error ? err.message : 'Failed to load agents'))
      .finally(() => setLoading(false));
  }, []);

  useEffect(() => {
    load();
    api.getAgentSettings()
      .then(s => { setSettings(s); setGlobalPrompt(s.globalPrompt); })
      .catch(() => {});
  }, [load]);

  // Keep the list fresh while an agent is running.
  const anyRunning = agents.some(a => a.running);
  useEffect(() => {
    if (!anyRunning) return;
    const t = setInterval(load, 5000);
    return () => clearInterval(t);
  }, [anyRunning, load]);

  const saveGlobal = async () => {
    setSavingGlobal(true);
    try {
      const s = await api.updateAgentSettings({ globalPrompt });
      setSettings(s);
      setGlobalPrompt(s.globalPrompt);
      toast.success('Global prompt saved');
    } catch (err) {
      toast.error(err instanceof Error ? err.message : 'Failed to save');
    } finally {
      setSavingGlobal(false);
    }
  };

  const saveAgent = async (data: AgentInput) => {
    try {
      if (editing && editing !== 'new') {
        const updated = await api.updateAgent(editing.id, data);
        setAgents(prev => prev.map(a => (a.id === updated.id ? updated : a)));
        toast.success('Agent saved');
      } else {
        const created = await api.createAgent(data);
        setAgents(prev => [...prev, created].sort((a, b) => a.name.localeCompare(b.name)));
        toast.success('Agent created');
      }
      setEditing(null);
    } catch (err) {
      toast.error(err instanceof Error ? err.message : 'Failed to save');
    }
  };

  const toggleEnabled = async (agent: Agent) => {
    try {
      const updated = await api.updateAgent(agent.id, {
        name: agent.name, prompt: agent.prompt, enabled: !agent.enabled,
        schedule: agent.schedule, model: agent.model, maxSteps: agent.maxSteps,
      });
      setAgents(prev => prev.map(a => (a.id === updated.id ? updated : a)));
    } catch (err) {
      toast.error(err instanceof Error ? err.message : 'Failed to save');
    }
  };

  const runNow = async (agent: Agent) => {
    try {
      const { runId } = await api.runAgent(agent.id);
      toast.success(`${agent.name} started`);
      await load();
      setHistory({ agent: { ...agent, running: true }, runId });
    } catch (err) {
      toast.error(err instanceof Error ? err.message : 'Failed to start');
    }
  };

  const stop = async (agent: Agent) => {
    try {
      await api.stopAgent(agent.id);
      toast.success('Stopping…');
      setTimeout(load, 1500);
    } catch (err) {
      toast.error(err instanceof Error ? err.message : 'Failed to stop');
    }
  };

  const remove = async (agent: Agent) => {
    if (!window.confirm(`Delete the agent "${agent.name}" and its run history? Drafts it created stay.`)) return;
    try {
      await api.deleteAgent(agent.id);
      setAgents(prev => prev.filter(a => a.id !== agent.id));
    } catch (err) {
      toast.error(err instanceof Error ? err.message : 'Failed to delete');
    }
  };

  const notReady = settings && (!settings.hasKey || !settings.model);

  return (
    <div className="page agents-page">
      <div className="page-header">
        <h1><Bot size={26} /> Agents</h1>
        <button className="btn btn-primary" onClick={() => setEditing('new')}>
          <Plus size={16} /> New Agent
        </button>
      </div>

      <p className="agents-intro">
        An agent runs its prompt on a schedule with the same tools the MCP server offers (posts, drafts, open slots, images,
        media, insights), plus web search and page reading. Prompts written for Claude or another MCP client work here
        unchanged. What an agent creates is labelled with its origin and counts towards the team's agent draft limit.
      </p>

      {notReady && (
        <div className="agents-notice">
          <AlertTriangle size={16} />
          <span>
            Agents need {!settings.hasKey ? 'your OpenRouter key' : 'a default model'} before they can run.
            Set it on the <Link to="/profile">Profile page</Link> under Agents.
          </span>
        </div>
      )}

      <div className="card agents-global">
        <div className="form-group">
          <label>Global prompt</label>
          <textarea
            className="input textarea"
            rows={5}
            value={globalPrompt}
            onChange={e => setGlobalPrompt(e.target.value)}
            placeholder="Instructions every agent follows in addition to its own prompt: tone of voice, language, house rules, what never to post."
          />
        </div>
        <div className="agents-global-actions">
          <button
            className="btn btn-primary btn-sm"
            onClick={saveGlobal}
            disabled={savingGlobal || !settings || globalPrompt === settings.globalPrompt}
          >
            {savingGlobal ? 'Saving...' : 'Save Global Prompt'}
          </button>
        </div>
      </div>

      {!loading && agents.length === 0 ? (
        <div className="empty-state">
          <Bot size={32} />
          <p>No agents yet. Create one to fill your draft queue on a schedule.</p>
        </div>
      ) : (
        <div className="agents-list">
          {agents.map(agent => (
            <div key={agent.id} className={`card agent-card${agent.enabled ? '' : ' disabled'}`}>
              <div className="agent-card-head">
                <div className="agent-card-title">
                  <h3>{agent.name}</h3>
                  <span className="agent-origin" title="Origin label of the drafts it creates">{agent.origin}</span>
                </div>
                <label className="agent-switch" title={agent.enabled ? 'Scheduled runs on' : 'Scheduled runs off'}>
                  <input type="checkbox" checked={agent.enabled} onChange={() => toggleEnabled(agent)} />
                  <span>{agent.enabled ? 'On' : 'Off'}</span>
                </label>
              </div>

              <div className="agent-card-meta">
                <span>{describeSchedule(agent.schedule)}</span>
                {agent.model && <span>Model: {agent.model}</span>}
                {agent.running ? (
                  <span className="badge badge-scheduled">Running since {formatDateTime(agent.runStartedAt)}</span>
                ) : agent.enabled && agent.nextRunAt ? (
                  <span>Next run: {formatDateTime(agent.nextRunAt)}</span>
                ) : null}
              </div>

              <p className="agent-card-prompt">{agent.prompt}</p>

              {agent.lastRunAt && !agent.running && (
                <div className="agent-card-last">
                  Last run {formatDateTime(agent.lastRunAt)}
                  {agent.lastStatus && <span className={`badge ${STATUS_BADGE[agent.lastStatus]}`}>{STATUS_LABEL[agent.lastStatus]}</span>}
                  {agent.lastError && <span className="agent-card-error">{agent.lastError}</span>}
                </div>
              )}

              <div className="agent-card-actions">
                {agent.running ? (
                  <button className="btn btn-secondary btn-sm" onClick={() => stop(agent)}>
                    <Square size={14} /> Stop
                  </button>
                ) : (
                  <button className="btn btn-secondary btn-sm" onClick={() => runNow(agent)} disabled={!!notReady && !agent.model}>
                    <Play size={14} /> Run now
                  </button>
                )}
                <button className="btn btn-ghost btn-sm" onClick={() => setHistory({ agent })}>
                  <History size={14} /> Runs
                </button>
                <span style={{ flex: 1 }} />
                <button className="btn btn-ghost btn-sm" onClick={() => setEditing(agent)} title="Edit">
                  <Pencil size={14} />
                </button>
                <button className="btn btn-ghost btn-sm btn-danger-text" onClick={() => remove(agent)} title="Delete">
                  <Trash2 size={14} />
                </button>
              </div>
            </div>
          ))}
        </div>
      )}

      {editing && (
        <AgentFormModal
          agent={editing === 'new' ? undefined : editing}
          onClose={() => setEditing(null)}
          onSave={saveAgent}
        />
      )}
      {history && (
        <AgentRunsModal
          agent={history.agent}
          initialRunId={history.runId}
          onClose={() => { setHistory(null); load(); }}
        />
      )}
    </div>
  );
}
