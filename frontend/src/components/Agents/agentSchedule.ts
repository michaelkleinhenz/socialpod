import type { AgentRunStatus, AgentSchedule } from '../../types';

export const WEEKDAYS = ['Mon', 'Tue', 'Wed', 'Thu', 'Fri', 'Sat', 'Sun'];

export const browserTimezone = (): string => {
  try {
    return Intl.DateTimeFormat().resolvedOptions().timeZone || 'Europe/Berlin';
  } catch {
    return 'Europe/Berlin';
  }
};

export const timezones = (): string[] => {
  const supported = (Intl as unknown as { supportedValuesOf?: (key: string) => string[] }).supportedValuesOf;
  try {
    return supported ? supported('timeZone') : [];
  } catch {
    return [];
  }
};

function formatInterval(minutes: number): string {
  if (minutes % 1440 === 0) return minutes === 1440 ? 'day' : `${minutes / 1440} days`;
  if (minutes % 60 === 0) return minutes === 60 ? 'hour' : `${minutes / 60} hours`;
  return `${minutes} minutes`;
}

/** A one-line description of when an agent runs. */
export function describeSchedule(s: AgentSchedule): string {
  switch (s.kind) {
    case 'manual':
      return 'Manual runs only';
    case 'interval':
      return `Every ${formatInterval(s.intervalMinutes || 0)}`;
    case 'hourly':
      return `Hourly at :${String(s.minute ?? 0).padStart(2, '0')}`;
    case 'daily': {
      const days = s.weekdays && s.weekdays.length > 0 && s.weekdays.length < 7
        ? s.weekdays.map(d => WEEKDAYS[d]).join(', ')
        : 'Daily';
      return `${days} at ${(s.times || []).join(', ')} (${s.timezone || 'Europe/Berlin'})`;
    }
  }
  return '';
}

export const STATUS_LABEL: Record<AgentRunStatus, string> = {
  running: 'Running',
  ok: 'Done',
  error: 'Failed',
  limit: 'Step limit',
  stopped: 'Stopped',
};

export const STATUS_BADGE: Record<AgentRunStatus, string> = {
  running: 'badge-scheduled',
  ok: 'badge-published',
  error: 'badge-failed',
  limit: 'badge-failed',
  stopped: 'badge-draft',
};

export function formatDateTime(iso?: string): string {
  if (!iso) return '';
  return new Date(iso).toLocaleString(undefined, { weekday: 'short', day: 'numeric', month: 'short', hour: '2-digit', minute: '2-digit' });
}

export function formatCost(cost: number): string {
  if (!cost) return '$0';
  return cost < 0.01 ? `$${cost.toFixed(4)}` : `$${cost.toFixed(2)}`;
}
