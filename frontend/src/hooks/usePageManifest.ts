import { useEffect } from 'react';

/**
 * Points the page's <link rel="manifest"> at the backend's dynamic manifest
 * endpoint with start_url set to `path`, so "Add to Home Screen" on this page
 * creates a shortcut that reopens this page instead of the app root (the
 * static manifest.json always has start_url "/").
 *
 * A real, fetchable URL is used rather than a Blob/data URL on purpose: iOS
 * Safari's "Add to Home Screen" ignores blob:/data: manifest hrefs and falls
 * back to the static manifest's start_url ("/", the calendar). Android Chrome
 * accepts blob URLs, which is why the previous approach worked there but not
 * on iOS.
 */
export function usePageManifest(path: string | undefined) {
  useEffect(() => {
    if (!path) return;

    const link = document.querySelector<HTMLLinkElement>('link[rel="manifest"]');
    if (!link) return;
    const originalHref = link.getAttribute('href');

    link.setAttribute('href', `/manifest.webmanifest?start_url=${encodeURIComponent(path)}`);

    return () => {
      if (originalHref !== null) link.setAttribute('href', originalHref);
    };
  }, [path]);
}
