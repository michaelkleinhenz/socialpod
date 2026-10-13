import { useMemo, useState } from 'react';
import { X } from 'lucide-react';
import { Modal } from '../Common/Modal';
import type { Agent, AgentInput, AgentScheduleKind } from '../../types';
import { ModelPicker } from './ModelPicker';
import { WEEKDAYS, browserTimezone, describeSchedule, timezones } from './agentSchedule';

export const PROMPT_EXAMPLE = `Find 2 interesting board game stories from the last 48 hours and turn them into Instagram post drafts.

1. Call get_draft_queue. If the draft limit is reached, stop.
2. Call get_open_slots for the next 7 days.
3. Use web_search to find news: announcements, crowdfunding launches, awards, release dates.
4. For each candidate, call find_existing with its URL and skip anything already covered.
5. Read the article with fetch_url. Get an image with upload_from_url (the article's image) or import_bgg for a BGG game.
6. Write the caption, then create_post with status "draft", platforms ["instagram"], scheduledAt set to a free slot's "at", and sourceUrl set to the article.
7. Run check_instagram_caption on the text and fix what it reports.`;

type IntervalUnit = 'minutes' | 'hours' | 'days';
const UNIT_MINUTES: Record<IntervalUnit, number> = { minutes: 1, hours: 60, days: 1440 };

interface FormState {
  name: string;
  prompt: string;
  enabled: boolean;
  kind: AgentScheduleKind;
  intervalValue: string;
  intervalUnit: IntervalUnit;
  minute: string;
  times: string[];
  weekdays: number[];
  timezone: string;
  model: string;
  maxSteps: string;
}

function initialState(agent?: Agent): FormState {
  const s = agent?.schedule;
  const minutes = s?.intervalMinutes || 360;
  const unit: IntervalUnit = minutes % 1440 === 0 ? 'days' : minutes % 60 === 0 ? 'hours' : 'minutes';
  return {
    name: agent?.name ?? '',
    prompt: agent?.prompt ?? '',
    enabled: agent?.enabled ?? true,
    kind: s?.kind ?? 'daily',
    intervalValue: String(minutes / UNIT_MINUTES[unit]),
    intervalUnit: unit,
    minute: String(s?.minute ?? 0),
    times: s?.times?.length ? s.times : ['09:00'],
    weekdays: s?.weekdays ?? [],
    timezone: s?.timezone || browserTimezone(),
    model: agent?.model ?? '',
    maxSteps: agent?.maxSteps ? String(agent.maxSteps) : '',
  };
}

function toInput(f: FormState): AgentInput {
  return {
    name: f.name.trim(),
    prompt: f.prompt,
    enabled: f.enabled,
    schedule: {
      kind: f.kind,
      intervalMinutes: f.kind === 'interval' ? Math.round(Number(f.intervalValue) * UNIT_MINUTES[f.intervalUnit]) : undefined,
      minute: f.kind === 'hourly' ? Number(f.minute) : undefined,
      times: f.kind === 'daily' ? f.times.filter(Boolean) : undefined,
      weekdays: f.kind === 'daily' ? f.weekdays : undefined,
      timezone: f.timezone,
    },
    model: f.model.trim(),
    maxSteps: f.maxSteps ? Number(f.maxSteps) : 0,
  };
}

interface Props {
  agent?: Agent;
  onClose: () => void;
  onSave: (data: AgentInput) => Promise<void>;
}

/** Creates or edits an agent: name, prompt, schedule, model and limits. */
export function AgentFormModal({ agent, onClose, onSave }: Props) {
  const initial = useMemo(() => initialState(agent), [agent]);
  const [form, setForm] = useState<FormState>(initial);
  const [saving, setSaving] = useState(false);
  const zones = useMemo(timezones, []);
  const set = <K extends keyof FormState>(key: K, value: FormState[K]) => setForm(f => ({ ...f, [key]: value }));
  // The weekday chips are buttons, which the Modal's own input watch misses.
  const dirty = JSON.stringify(form) !== JSON.stringify(initial);

  // No weekdays means every day, so the first click on a day takes it out.
  const toggleDay = (d: number) => {
    const current = form.weekdays.length === 0 ? [0, 1, 2, 3, 4, 5, 6] : form.weekdays;
    const next = current.includes(d) ? current.filter(x => x !== d) : [...current, d].sort();
    if (next.length === 0) return;
    set('weekdays', next.length === 7 ? [] : next);
  };

  const submit = async () => {
    setSaving(true);
    try {
      await onSave(toInput(form));
    } finally {
      setSaving(false);
    }
  };

  const preview = describeSchedule(toInput(form).schedule);

  return (
    <Modal onClose={onClose} dirty={dirty} className="agent-modal">
      <div className="agent-modal-header">
        <h2>{agent ? 'Edit Agent' : 'New Agent'}</h2>
        <button className="btn btn-ghost btn-sm" onClick={onClose} title="Close">
          <X size={18} />
        </button>
      </div>

      <div className="agent-form">
        <div className="form-group">
          <label>Name</label>
          <input className="input" value={form.name} onChange={e => set('name', e.target.value)} placeholder="e.g. News Scout" maxLength={80} />
        </div>

        <div className="form-group">
          <div className="agent-label-row">
            <label>Prompt</label>
            {!form.prompt.trim() && (
              <button type="button" className="btn btn-ghost btn-sm" onClick={() => set('prompt', PROMPT_EXAMPLE)}>
                Insert example
              </button>
            )}
          </div>
          <textarea
            className="input textarea agent-prompt"
            rows={12}
            value={form.prompt}
            onChange={e => set('prompt', e.target.value)}
            placeholder="What the agent should do on each run. It has the same tools as the MCP server, plus web_search and fetch_url, so prompts written for an MCP client work here too."
          />
        </div>

        <div className="form-group">
          <label>Schedule</label>
          <select className="input" value={form.kind} onChange={e => set('kind', e.target.value as AgentScheduleKind)}>
            <option value="daily">At set times of day</option>
            <option value="hourly">Every hour</option>
            <option value="interval">Every N minutes / hours / days</option>
            <option value="manual">Manual runs only</option>
          </select>
        </div>

        {form.kind === 'interval' && (
          <div className="agent-inline">
            <span>Every</span>
            <input className="input agent-num" type="number" min={1} value={form.intervalValue} onChange={e => set('intervalValue', e.target.value)} />
            <select className="input agent-unit" value={form.intervalUnit} onChange={e => set('intervalUnit', e.target.value as IntervalUnit)}>
              <option value="minutes">minutes</option>
              <option value="hours">hours</option>
              <option value="days">days</option>
            </select>
            <span className="agent-hint">(at least 15 minutes, counted from the last run)</span>
          </div>
        )}

        {form.kind === 'hourly' && (
          <div className="agent-inline">
            <span>At minute</span>
            <input className="input agent-num" type="number" min={0} max={59} value={form.minute} onChange={e => set('minute', e.target.value)} />
            <span>past every hour</span>
          </div>
        )}

        {form.kind === 'daily' && (
          <>
            <div className="agent-inline">
              <span>At</span>
              {form.times.map((t, i) => (
                <span key={i} className="agent-time">
                  <input
                    className="input"
                    type="time"
                    value={t}
                    onChange={e => set('times', form.times.map((x, j) => (j === i ? e.target.value : x)))}
                  />
                  {form.times.length > 1 && (
                    <button type="button" className="btn btn-ghost btn-sm" title="Remove this time"
                      onClick={() => set('times', form.times.filter((_, j) => j !== i))}>
                      <X size={14} />
                    </button>
                  )}
                </span>
              ))}
              <button type="button" className="btn btn-ghost btn-sm" onClick={() => set('times', [...form.times, '18:00'])}>
                + Time
              </button>
            </div>
            <div className="agent-inline">
              <span>On</span>
              {WEEKDAYS.map((d, i) => (
                <button
                  type="button"
                  key={d}
                  className={`agent-day${form.weekdays.length === 0 || form.weekdays.includes(i) ? ' active' : ''}`}
                  onClick={() => toggleDay(i)}
                >
                  {d}
                </button>
              ))}
              <span className="agent-hint">{form.weekdays.length === 0 ? 'every day' : ''}</span>
            </div>
          </>
        )}

        {form.kind !== 'manual' && (
          <div className="form-group">
            <label>Timezone</label>
            {zones.length > 0 ? (
              <select className="input" value={form.timezone} onChange={e => set('timezone', e.target.value)}>
                {!zones.includes(form.timezone) && <option value={form.timezone}>{form.timezone}</option>}
                {zones.map(z => <option key={z} value={z}>{z}</option>)}
              </select>
            ) : (
              <input className="input" value={form.timezone} onChange={e => set('timezone', e.target.value)} />
            )}
            <span className="agent-hint">{preview}. The agent also gets the current time in this zone.</span>
          </div>
        )}

        <div className="agent-form-row">
          <div className="form-group">
            <label>Model (optional)</label>
            <ModelPicker value={form.model} onChange={v => set('model', v)} placeholder="Your default model" />
          </div>
          <div className="form-group">
            <label>Max steps</label>
            <input className="input" type="number" min={1} max={100} value={form.maxSteps} onChange={e => set('maxSteps', e.target.value)} placeholder="30" />
            <span className="agent-hint">Model turns per run before it is stopped.</span>
          </div>
        </div>

        <label className="agent-check">
          <input type="checkbox" checked={form.enabled} onChange={e => set('enabled', e.target.checked)} />
          Run on schedule
        </label>
      </div>

      <div className="modal-actions">
        <button className="btn btn-secondary" onClick={onClose}>Cancel</button>
        <button className="btn btn-primary" onClick={submit} disabled={saving || !form.name.trim() || !form.prompt.trim()}>
          {saving ? 'Saving...' : agent ? 'Save' : 'Create Agent'}
        </button>
      </div>
    </Modal>
  );
}
