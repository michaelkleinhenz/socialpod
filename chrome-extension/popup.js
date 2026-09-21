let entityType = "news";

document.addEventListener("DOMContentLoaded", async () => {
  const serverUrl = document.getElementById("serverUrl");
  const apiToken = document.getElementById("apiToken");
  const settingsToggle = document.getElementById("settingsToggle");
  const settingsPanel = document.getElementById("settingsPanel");
  const settingsArrow = document.getElementById("settingsArrow");
  const mainSection = document.getElementById("mainSection");
  const needsSetup = document.getElementById("needsSetup");
  const captureBtn = document.getElementById("captureBtn");
  const status = document.getElementById("status");
  const pageTitle = document.getElementById("pageTitle");
  const pageUrl = document.getElementById("pageUrl");

  const saved = await chrome.storage.local.get(["serverUrl", "apiToken"]);
  if (saved.serverUrl) serverUrl.value = saved.serverUrl;
  if (saved.apiToken) apiToken.value = saved.apiToken;

  if (!saved.serverUrl || !saved.apiToken) {
    mainSection.style.display = "none";
    needsSetup.style.display = "block";
    settingsPanel.classList.add("show");
    settingsArrow.innerHTML = "&#9660;";
  }

  const [tab] = await chrome.tabs.query({ active: true, currentWindow: true });
  if (tab) {
    pageTitle.textContent = tab.title || "Untitled";
    pageUrl.textContent = tab.url || "";
  }

  settingsToggle.addEventListener("click", () => {
    settingsPanel.classList.toggle("show");
    settingsArrow.innerHTML = settingsPanel.classList.contains("show") ? "&#9660;" : "&#9654;";
  });

  document.getElementById("openSettings")?.addEventListener("click", () => {
    needsSetup.style.display = "none";
    mainSection.style.display = "block";
    settingsPanel.classList.add("show");
    settingsArrow.innerHTML = "&#9660;";
  });

  document.getElementById("saveSettings").addEventListener("click", async () => {
    const cleanUrl = serverUrl.value
      .replace(/\/+$/, "")
      .replace(/\/api(\/mcp)?$/, "");
    serverUrl.value = cleanUrl;
    await chrome.storage.local.set({
      serverUrl: cleanUrl,
      apiToken: apiToken.value,
    });
    needsSetup.style.display = "none";
    mainSection.style.display = "block";
    settingsPanel.classList.remove("show");
    settingsArrow.innerHTML = "&#9654;";
    showStatus("Settings saved.", "success");
  });

  document.querySelectorAll(".type-btn").forEach((btn) => {
    btn.addEventListener("click", () => {
      document.querySelectorAll(".type-btn").forEach((b) => b.classList.remove("active"));
      btn.classList.add("active");
      entityType = btn.dataset.type;
    });
  });

  captureBtn.addEventListener("click", async () => {
    const config = await chrome.storage.local.get(["serverUrl", "apiToken"]);
    if (!config.serverUrl || !config.apiToken) {
      showStatus("Please configure server URL and API token first.", "error");
      return;
    }

    captureBtn.disabled = true;
    captureBtn.textContent = "Capturing...";
    showStatus("Capturing page...", "info");

    try {
      const [tab] = await chrome.tabs.query({ active: true, currentWindow: true });

      // Capture a screenshot of the visible tab
      let screenshot = null;
      try {
        const dataUrl = await chrome.tabs.captureVisibleTab(null, { format: "jpeg", quality: 80 });
        if (dataUrl) {
          const base64 = dataUrl.split(",")[1];
          if (base64) {
            screenshot = { data: base64, filename: "screenshot.jpg" };
          }
        }
      } catch (e) {
        console.warn("Could not capture screenshot:", e);
      }

      // Extract image URLs from the page DOM
      showStatus("Extracting page images...", "info");
      let imageUrls = [];
      try {
        const results = await chrome.scripting.executeScript({
          target: { tabId: tab.id },
          func: extractPageImages,
        });
        if (results && results[0] && results[0].result) {
          imageUrls = results[0].result;
        }
      } catch (e) {
        console.warn("Could not extract images from page:", e);
      }

      // Download all candidate images in the browser
      showStatus("Downloading images...", "info");
      const images = await downloadAllImages(imageUrls);

      // Add the screenshot last (lowest priority for content, but provides context)
      if (screenshot) {
        images.push(screenshot);
      }

      showStatus("Sending to SocialPod...", "info");

      const payload = {
        url: tab.url,
        entityType: entityType,
        description: document.getElementById("description").value,
        pageTitle: tab.title,
        images: images,
      };

      const resp = await fetch(config.serverUrl + "/api/capture", {
        method: "POST",
        headers: {
          "Content-Type": "application/json",
          Authorization: "Bearer " + config.apiToken,
        },
        body: JSON.stringify(payload),
      });

      if (!resp.ok) {
        const err = await resp.json().catch(() => ({ error: resp.statusText }));
        throw new Error(err.error || `HTTP ${resp.status}`);
      }

      const result = await resp.json();
      showStatus(
        `Draft created! (${result.entityType}${result.draft?.newsTagline ? ': ' + result.draft.newsTagline : ''})`,
        "success"
      );
    } catch (err) {
      showStatus("Error: " + err.message, "error");
    } finally {
      captureBtn.disabled = false;
      captureBtn.textContent = "Capture & Create Draft";
    }
  });
});

// Runs inside the active tab to extract image URLs from the page DOM.
function extractPageImages() {
  const images = [];
  const seen = new Set();

  function add(url) {
    if (!url || seen.has(url) || url.startsWith("data:")) return;
    seen.add(url);
    images.push(url);
  }

  // og:image
  const ogImage = document.querySelector('meta[property="og:image"]');
  if (ogImage) add(ogImage.content);

  // twitter:image
  const twImage = document.querySelector('meta[name="twitter:image"], meta[name="twitter:image:src"], meta[property="twitter:image"]');
  if (twImage) add(twImage.content);

  // link rel=image_src
  const linkImage = document.querySelector('link[rel="image_src"]');
  if (linkImage) add(linkImage.href);

  // JSON-LD image
  try {
    document.querySelectorAll('script[type="application/ld+json"]').forEach((el) => {
      try {
        const data = JSON.parse(el.textContent);
        const items = Array.isArray(data) ? data : data["@graph"] ? [data, ...data["@graph"]] : [data];
        for (const item of items) {
          if (!item || typeof item !== "object") continue;
          const img = item.image;
          if (typeof img === "string") add(img);
          else if (Array.isArray(img) && img.length > 0) {
            add(typeof img[0] === "string" ? img[0] : img[0]?.url);
          } else if (img && typeof img === "object") {
            add(img.url);
          }
        }
      } catch (_) {}
    });
  } catch (_) {}

  // Large <img> elements sorted by size
  const imgs = Array.from(document.querySelectorAll("img"))
    .filter((img) => {
      const src = img.src || img.dataset.src || img.dataset.lazySrc;
      if (!src || src.startsWith("data:")) return false;
      const w = img.naturalWidth || parseInt(img.getAttribute("width")) || 0;
      const h = img.naturalHeight || parseInt(img.getAttribute("height")) || 0;
      if (w > 0 && w < 100) return false;
      if (h > 0 && h < 100) return false;
      const lower = src.toLowerCase();
      const negatives = ["logo", "icon", "avatar", "sprite", "pixel", "tracking", "badge", "favicon", "spinner"];
      return !negatives.some((n) => lower.includes(n));
    })
    .sort((a, b) => {
      const aSize = (a.naturalWidth || 0) * (a.naturalHeight || 0);
      const bSize = (b.naturalWidth || 0) * (b.naturalHeight || 0);
      return bSize - aSize;
    })
    .slice(0, 5);

  for (const img of imgs) {
    add(img.src || img.dataset.src || img.dataset.lazySrc);
  }

  return images;
}

// Downloads all candidate images in the browser and returns them as
// {data: base64, filename: string} objects. Skips URLs that fail.
async function downloadAllImages(urls) {
  const images = [];
  for (const url of urls) {
    if (images.length >= 8) break;
    try {
      const resp = await fetch(url);
      if (!resp.ok) continue;

      const blob = await resp.blob();
      if (!blob.type.startsWith("image/")) continue;
      if (blob.size < 1000) continue;

      const ext = blob.type.split("/")[1]?.replace("jpeg", "jpg") || "jpg";
      const base64 = await blobToBase64(blob);
      if (!base64) continue;

      images.push({ data: base64, filename: `image${images.length}.${ext}` });
    } catch (e) {
      console.warn("Failed to download image:", url, e);
    }
  }
  return images;
}

function blobToBase64(blob) {
  return new Promise((resolve) => {
    const reader = new FileReader();
    reader.onloadend = () => {
      const result = reader.result;
      const base64 = result?.split(",")[1] || null;
      resolve(base64);
    };
    reader.onerror = () => resolve(null);
    reader.readAsDataURL(blob);
  });
}

function showStatus(message, type) {
  const status = document.getElementById("status");
  status.textContent = message;
  status.className = "status " + type;
}
