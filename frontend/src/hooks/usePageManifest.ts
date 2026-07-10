import { useEffect } from 'react';

const BASE_MANIFEST = {
  name: 'SocialPod',
  short_name: 'SocialPod',
  display: 'standalone',
  background_color: '#0f172a',
  theme_color: '#6366f1',
  icons: [
    { src: '/favicon.svg', type: 'image/svg+xml', sizes: 'any', purpose: 'any' },
  ],
};

/**
 * Swaps the page's <link rel="manifest"> for a Blob-backed one whose
 * start_url/scope point at `path`, so "Add to Home Screen" on this page
 * creates a shortcut that reopens this page instead of the app root
 * (the static manifest.json always has start_url "/").
 */
export function usePageManifest(path: string | undefined) {
  useEffect(() => {
    if (!path) return;

    const link = document.querySelector<HTMLLinkElement>('link[rel="manifest"]');
    if (!link) return;
    const originalHref = link.getAttribute('href');

    const manifest = { ...BASE_MANIFEST, start_url: path, scope: path };
    const blob = new Blob([JSON.stringify(manifest)], { type: 'application/manifest+json' });
    const blobUrl = URL.createObjectURL(blob);
    link.setAttribute('href', blobUrl);

    return () => {
      if (originalHref !== null) link.setAttribute('href', originalHref);
      URL.revokeObjectURL(blobUrl);
    };
  }, [path]);
}
