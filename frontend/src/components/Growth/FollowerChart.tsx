import { useEffect, useRef, useState } from 'react';
import { format, parseISO } from 'date-fns';
import type { GoalProgress } from '../../types';

const DAY = 86_400_000;
const PAD = { top: 16, right: 16, bottom: 28, left: 44 };
const HEIGHT = 260;

function useWidth<T extends HTMLElement>() {
  const ref = useRef<T>(null);
  const [width, setWidth] = useState(600);
  useEffect(() => {
    if (!ref.current) return;
    const ro = new ResizeObserver(entries => setWidth(Math.max(280, entries[0].contentRect.width)));
    ro.observe(ref.current);
    return () => ro.disconnect();
  }, []);
  return [ref, width] as const;
}

function niceTicks(min: number, max: number, count = 4): number[] {
  const span = Math.max(1, max - min);
  const raw = span / count;
  const mag = 10 ** Math.floor(Math.log10(raw));
  const step = [1, 2, 5, 10].map(m => m * mag).find(s => s >= raw) ?? raw;
  const out: number[] = [];
  for (let v = Math.ceil(min / step) * step; v <= max; v += step) out.push(v);
  return out;
}

/**
 * Followers per day against the straight path from the start count to the
 * target. One measure, one axis; the target line is a dashed reference.
 */
export function FollowerChart({ goal }: { goal: GoalProgress }) {
  const [wrapRef, width] = useWidth<HTMLDivElement>();
  const [hover, setHover] = useState<number | null>(null);

  const start = parseISO(goal.goal.startDate).getTime();
  const end = parseISO(goal.goal.endDate).getTime();
  const from = Math.min(start, ...goal.series.map(p => parseISO(p.date).getTime()));
  const startCount = goal.startFollowers ?? goal.series[0]?.followers ?? 0;
  const values = [startCount, goal.goal.target, ...goal.series.map(p => p.followers)];
  const yMin = Math.max(0, Math.floor(Math.min(...values) * 0.95));
  const yMax = Math.ceil(Math.max(...values) * 1.03);

  const innerW = width - PAD.left - PAD.right;
  const innerH = HEIGHT - PAD.top - PAD.bottom;
  const x = (t: number) => PAD.left + ((t - from) / Math.max(1, end - from)) * innerW;
  const y = (v: number) => PAD.top + (1 - (v - yMin) / Math.max(1, yMax - yMin)) * innerH;
  const expectedAt = (t: number) =>
    t < start ? null : Math.round(startCount + (goal.goal.target - startCount) * Math.min(1, (t - start) / (end - start)));

  const points = goal.series.map(p => ({ t: parseISO(p.date).getTime(), v: p.followers, date: p.date }));
  const path = points.map((p, i) => `${i ? 'L' : 'M'}${x(p.t).toFixed(1)},${y(p.v).toFixed(1)}`).join(' ');
  const halfway = goal.halfwayDate ? parseISO(goal.halfwayDate).getTime() : null;
  const yTicks = niceTicks(yMin, yMax);
  const xTicks: number[] = [];
  for (let t = start; t <= end; t += 14 * DAY) xTicks.push(t);

  const onMove = (e: React.PointerEvent<SVGSVGElement>) => {
    if (points.length === 0) return;
    const rect = e.currentTarget.getBoundingClientRect();
    const px = e.clientX - rect.left;
    let best = 0;
    points.forEach((p, i) => { if (Math.abs(x(p.t) - px) < Math.abs(x(points[best].t) - px)) best = i; });
    setHover(best);
  };

  const hp = hover !== null ? points[hover] : null;
  const last = points[points.length - 1];

  return (
    <div className="follower-chart" ref={wrapRef}>
      <div className="chart-legend">
        <span><i className="legend-line followers" /> Followers</span>
        <span><i className="legend-line target" /> Path to {goal.goal.target}</span>
        {halfway && <span><i className="legend-line halfway" /> Halfway review</span>}
      </div>
      <svg width={width} height={HEIGHT} onPointerMove={onMove} onPointerLeave={() => setHover(null)} role="img"
        aria-label={`Followers over time${last ? `, ${last.v} on ${last.date}` : ''}, target ${goal.goal.target} by ${goal.goal.endDate}`}>
        {yTicks.map(v => (
          <g key={v}>
            <line x1={PAD.left} x2={width - PAD.right} y1={y(v)} y2={y(v)} className="chart-grid" />
            <text x={PAD.left - 8} y={y(v) + 4} className="chart-axis" textAnchor="end">{v}</text>
          </g>
        ))}
        {xTicks.map(t => (
          <text key={t} x={x(t)} y={HEIGHT - 8} className="chart-axis" textAnchor="middle">{format(t, 'd MMM')}</text>
        ))}
        {halfway && (
          <line x1={x(halfway)} x2={x(halfway)} y1={PAD.top} y2={PAD.top + innerH} className="chart-halfway" />
        )}
        <line x1={x(start)} y1={y(startCount)} x2={x(end)} y2={y(goal.goal.target)} className="chart-target" />
        {path && <path d={path} className="chart-followers" />}
        {last && <circle cx={x(last.t)} cy={y(last.v)} r={4} className="chart-dot" />}
        {last && (
          <text x={Math.min(x(last.t) + 8, width - PAD.right - 30)} y={y(last.v) - 8} className="chart-label">{last.v}</text>
        )}
        {hp && (
          <>
            <line x1={x(hp.t)} x2={x(hp.t)} y1={PAD.top} y2={PAD.top + innerH} className="chart-crosshair" />
            <circle cx={x(hp.t)} cy={y(hp.v)} r={5} className="chart-dot" />
          </>
        )}
        {/* Wide transparent hit area so the hover works anywhere on the plot */}
        <rect x={PAD.left} y={PAD.top} width={innerW} height={innerH} fill="transparent" />
      </svg>
      {hp && (
        <div className="chart-tooltip" style={{ left: Math.min(x(hp.t) + 12, width - 170), top: PAD.top }}>
          <strong>{format(parseISO(hp.date), 'EEE d MMM')}</strong>
          <span>{hp.v} followers</span>
          {expectedAt(hp.t) !== null && <span className="muted">Plan: {expectedAt(hp.t)}</span>}
        </div>
      )}
    </div>
  );
}
