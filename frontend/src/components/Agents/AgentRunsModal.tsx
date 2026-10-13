import { useCallback, useEffect, useState } from 'react';
import { ChevronDown, ChevronRight, MessageSquare, Wrench, X } from 'lucide-react';
import { Modal } from '../Common/Modal';
import { api } from '../../services/api';
import type { Agent, AgentRun, AgentRunStep } from '../../types';
import { STATUS_BADGE, STATUS_LABEL, formatCost, formatDateTime } from './agentSchedule';

function prettyJSON(raw?: string): string {
  if (!raw) return '';
  try {
    return JSON.stringify(JSON.parse(raw), null, 2);
  } catch {
    return raw;
  }
}

function StepRow({ step }: { step: AgentRunStep }) {
  const [open, setOpen] = useState(false);
  if (step.kind === 'message') {
    return (
      <div className="agent-step">
        <MessageSquare size={14} />
        <div className="agent-step-text">{step.text}</div>
      </div>
    );
  }
  return (
    <div className={`agent-step${step.isError ? ' error' : ''}`}>
      <Wrench size={14} />
      <div style={{ flex: 1, minWidth: 0 }}>
        <button className="agent-step-toggle" onClick={() => setOpen(o => !o)}>
          {open ? <ChevronDown size={14} /> : <ChevronRight size={14} />}
          <code>{step.tool}</code>
          {step.isError && <span className="badge badge-failed">error</span>}
        </button>
        {open && (
          <div className="agent-step-detail">
            <div className="agent-step-label">Arguments</div>
            <pre>{prettyJSON(step.args) || '{}'}</pre>
            <div className="agent-step-label">Result</div>
            <pre>{prettyJSON(step.result)}</pre>
          </div>
        )}
      </div>
    </div>
  );
}

/** An agent's recent runs, each with its log of messages and tool calls. */
export function AgentRunsModal({ agent, initialRunId, onClose }: { agent: Agent; initialRunId?: string; onClose: () => void }) {
  const [runs, setRuns] = useState<AgentRun[]>([]);
  const [selectedId, setSelectedId] = useState<string | undefined>(initialRunId);
  const [detail, setDetail] = useState<AgentRun | null>(null);

  const loadRuns = useCallback(() => {
    api.getAgentRuns(agent.id).then(r => {
      setRuns(r);
      setSelectedId(id => id ?? r[0]?.id);
    }).catch(() => {});
  }, [agent.id]);

  useEffect(loadRuns, [loadRuns]);

  useEffect(() => {
    if (!selectedId) return;
    let alive = true;
    let timer: ReturnType<typeof setTimeout>;
    const load = () => {
      api.getAgentRun(agent.id, selectedId).then(r => {
        if (!alive) return;
        setDetail(r);
        // Follow a run in progress.
        if (r.status === 'running') {
          timer = setTimeout(load, 3000);
        } else {
          setRuns(prev => prev.map(x => (x.id === r.id ? { ...r, steps: undefined } : x)));
        }
      }).catch(() => {});
    };
    load();
    return () => { alive = false; clearTimeout(timer); };
  }, [agent.id, selectedId]);

  return (
    <Modal onClose={onClose} className="agent-modal agent-runs-modal">
      <div className="agent-modal-header">
        <h2>{agent.name}: Runs</h2>
        <button className="btn btn-ghost btn-sm" onClick={onClose} title="Close">
          <X size={18} />
        </button>
      </div>

      {runs.length === 0 ? (
        <p className="agent-hint">No runs yet. Runs are kept for 30 days.</p>
      ) : (
        <div className="agent-runs">
          <div className="agent-run-list">
            {runs.map(r => (
              <button
                key={r.id}
                className={`agent-run-item${r.id === selectedId ? ' active' : ''}`}
                onClick={() => setSelectedId(r.id)}
              >
                <span className="agent-run-when">{formatDateTime(r.startedAt)}</span>
                <span className={`badge ${STATUS_BADGE[r.status]}`}>{STATUS_LABEL[r.status]}</span>
                <span className="agent-hint">{r.trigger === 'manual' ? 'manual' : 'scheduled'} · {r.toolCalls} tool calls</span>
              </button>
            ))}
          </div>

          <div className="agent-run-detail">
            {detail && detail.id === selectedId ? (
              <>
                <div className="agent-run-meta">
                  <span className={`badge ${STATUS_BADGE[detail.status]}`}>{STATUS_LABEL[detail.status]}</span>
                  <span>{detail.model}</span>
                  <span>{(detail.promptTokens + detail.completionTokens).toLocaleString()} tokens</span>
                  <span>{formatCost(detail.cost)}</span>
                  {detail.finishedAt && (
                    <span>{Math.max(1, Math.round((new Date(detail.finishedAt).getTime() - new Date(detail.startedAt).getTime()) / 1000))}s</span>
                  )}
                </div>
                {detail.error && <div className="agent-error">{detail.error}</div>}
                {detail.summary && <div className="agent-summary">{detail.summary}</div>}
                <div className="agent-steps">
                  {(detail.steps || []).map((s, i) => <StepRow key={i} step={s} />)}
                  {detail.status === 'running' && <div className="agent-hint">Working…</div>}
                </div>
              </>
            ) : (
              <p className="agent-hint">Loading…</p>
            )}
          </div>
        </div>
      )}
    </Modal>
  );
}
