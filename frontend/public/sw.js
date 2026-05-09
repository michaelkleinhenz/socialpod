const SHARE_CACHE = 'share-target-v1';
const SHARE_META_KEY = '/share-cache/meta';
const SHARE_FILE_PREFIX = '/share-cache/file/';

self.addEventListener('install', () => {
  self.skipWaiting();
});

self.addEventListener('activate', (event) => {
  event.waitUntil(clients.claim());
});

self.addEventListener('fetch', (event) => {
  const url = new URL(event.request.url);

  if (url.pathname === '/share-target' && event.request.method === 'POST') {
    event.respondWith(handleShareTarget(event.request));
    return;
  }

  // Serve share-cached files to the page
  if (url.pathname.startsWith(SHARE_FILE_PREFIX) || url.pathname === SHARE_META_KEY) {
    event.respondWith(
      caches.open(SHARE_CACHE).then(cache => cache.match(event.request))
    );
  }
});

async function handleShareTarget(request) {
  let formData;
  try {
    formData = await request.formData();
  } catch {
    return Response.redirect('/share-target?error=parse', 303);
  }

  const title = formData.get('title') || '';
  const text = formData.get('text') || '';
  const url = formData.get('url') || '';
  const mediaFiles = formData.getAll('media');

  const cache = await caches.open(SHARE_CACHE);

  // Clear any previous share data
  const existingKeys = await cache.keys();
  await Promise.all(existingKeys.map(k => cache.delete(k)));

  // Store each file
  const fileInfos = [];
  for (let i = 0; i < mediaFiles.length; i++) {
    const file = mediaFiles[i];
    if (!(file instanceof File)) continue;

    const key = `${SHARE_FILE_PREFIX}${i}_${encodeURIComponent(file.name)}`;
    const fileResponse = new Response(file, {
      headers: { 'Content-Type': file.type || 'application/octet-stream' },
    });
    await cache.put(new Request(key), fileResponse);
    fileInfos.push({ key, name: file.name, type: file.type, size: file.size });
  }

  // Store metadata
  const meta = { title, text, url, files: fileInfos, timestamp: Date.now() };
  await cache.put(
    new Request(SHARE_META_KEY),
    new Response(JSON.stringify(meta), {
      headers: { 'Content-Type': 'application/json' },
    })
  );

  return Response.redirect('/share-target?shared=1', 303);
}
