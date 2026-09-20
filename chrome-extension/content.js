chrome.runtime.onMessage.addListener((request, sender, sendResponse) => {
  if (request.action !== "extractImages") return false;

  const images = [];
  const seen = new Set();

  document.querySelectorAll("img").forEach((img) => {
    const src = img.currentSrc || img.src;
    if (!src || src.startsWith("data:") || seen.has(src)) return;
    seen.add(src);

    const rect = img.getBoundingClientRect();
    const entry = {
      src: src,
      naturalWidth: img.naturalWidth,
      naturalHeight: img.naturalHeight,
      displayWidth: Math.round(rect.width),
      displayHeight: Math.round(rect.height),
      x: Math.round(rect.x),
      y: Math.round(rect.y),
      alt: (img.alt || "").substring(0, 200),
      classes: (img.className || "").substring(0, 200),
    };

    if (img.srcset) {
      entry.srcset = img.srcset.substring(0, 500);
    }

    images.push(entry);
  });

  document.querySelectorAll("picture source").forEach((source) => {
    const srcset = source.srcset;
    if (!srcset) return;
    const parts = srcset.split(",").map((s) => s.trim().split(/\s+/));
    for (const part of parts) {
      const url = part[0];
      if (url && !url.startsWith("data:") && !seen.has(url)) {
        seen.add(url);
        images.push({
          src: url,
          naturalWidth: 0,
          naturalHeight: 0,
          displayWidth: 0,
          displayHeight: 0,
          x: 0,
          y: 0,
          alt: "",
          classes: "picture-source",
        });
      }
    }
  });

  const ogImage = document.querySelector('meta[property="og:image"]');
  const ogContent = ogImage ? ogImage.getAttribute("content") : "";

  const title =
    document.querySelector('meta[property="og:title"]')?.getAttribute("content") ||
    document.title ||
    "";

  sendResponse({
    images: images,
    ogImage: ogContent,
    pageTitle: title,
    url: window.location.href,
  });

  return true;
});
