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
    showStatus("Sending to SocialPod...", "info");

    try {
      const [tab] = await chrome.tabs.query({ active: true, currentWindow: true });

      const payload = {
        url: tab.url,
        entityType: entityType,
        description: document.getElementById("description").value,
        pageTitle: tab.title,
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

function showStatus(message, type) {
  const status = document.getElementById("status");
  status.textContent = message;
  status.className = "status " + type;
}
