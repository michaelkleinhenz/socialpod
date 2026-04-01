export function PlatformIcon({ platform, size = 14 }: { platform: string; size?: number }) {
  if (platform === 'bluesky') {
    return (
      <svg width={size} height={size} viewBox="0 0 600 530" fill="var(--bluesky)" style={{ flexShrink: 0 }}>
        <path d="M135.72 44.03C202.216 93.951 273.74 195.86 300 249.834c26.262-53.974 97.784-155.883 164.28-205.804C520.211-1.618 590.2-24.926 590.2 51.664c0 15.318-8.78 128.688-13.923 147.024-17.89 63.78-83.096 80.05-141.37 70.202 101.97 16.066 127.874 69.15 71.846 122.229C398.266 489.881 337.2 392.22 300 327.206c-37.2 65.014-97.674 161.932-206.753 63.913-56.028-53.079-30.124-106.163 71.846-122.23-58.274 9.849-123.48-6.421-141.37-70.201C18.58 180.352 9.8 67.065 9.8 51.747c0-76.59 69.989-53.282 125.92-7.717z" />
      </svg>
    );
  }
  if (platform === 'instagram') {
    return (
      <svg width={size} height={size} viewBox="0 0 24 24" fill="none" stroke="var(--instagram)" strokeWidth="2" strokeLinecap="round" strokeLinejoin="round" style={{ flexShrink: 0 }}>
        <rect x="2" y="2" width="20" height="20" rx="5" ry="5" />
        <circle cx="12" cy="12" r="5" />
        <circle cx="17.5" cy="6.5" r="1.5" fill="var(--instagram)" stroke="none" />
      </svg>
    );
  }
  return <span style={{ width: size, height: size, borderRadius: '50%', background: 'var(--text-muted)', display: 'inline-block', flexShrink: 0 }} />;
}
