"use strict";

const MIN_GRID = 2;
const MAX_GRID = 300;
const SUPPORTED_TYPES = ["image/jpeg", "image/png"];
const THEME_KEY = "gosaics.theme";
const LANG_KEY = "gosaics.lang";
const MAX_TILE_THUMBS = 12;

const state = {
  inputFile: null,
  tileFiles: [],
  strings: {},
  resultURL: null,
  inputURL: null,
  tileURLs: [],
};

const el = (id) => document.getElementById(id);

/* ---------------- i18n ---------------- */

function t(key) {
  return state.strings[key] || key;
}

async function loadLanguage(code) {
  const res = await fetch(`strings.${code}.json`);
  if (!res.ok) throw new Error(`missing strings for ${code}`);
  state.strings = await res.json();
  applyStrings();
  document.documentElement.lang = code;
  localStorage.setItem(LANG_KEY, code);
}

function applyStrings() {
  document.querySelectorAll("[data-i18n]").forEach((node) => {
    node.textContent = t(node.dataset.i18n);
  });
  document.querySelectorAll("[data-i18n-title]").forEach((node) => {
    node.title = t(node.dataset.i18nTitle);
  });
  // Re-render any dynamic labels that embed translated text.
  renderInputPreview();
  renderTilesPreview();
}

/* ---------------- theme ---------------- */

function initTheme() {
  const saved = localStorage.getItem(THEME_KEY);
  if (saved === "light" || saved === "dark") {
    document.documentElement.dataset.theme = saved;
  }

  el("theme-toggle").addEventListener("click", () => {
    const systemDark = window.matchMedia("(prefers-color-scheme: dark)").matches;
    const current = document.documentElement.dataset.theme || (systemDark ? "dark" : "light");
    const next = current === "dark" ? "light" : "dark";
    document.documentElement.dataset.theme = next;
    localStorage.setItem(THEME_KEY, next);
  });
}

/* ---------------- file collection ---------------- */

function isSupported(file) {
  if (SUPPORTED_TYPES.includes(file.type)) return true;
  // Some browsers report an empty type for files dragged out of a folder,
  // so fall back to the extension.
  return /\.(jpe?g|png)$/i.test(file.name);
}

// Recursively collects every file under a dropped directory entry.
function readEntry(entry) {
  if (entry.isFile) {
    return new Promise((resolve) => {
      entry.file(
        (file) => resolve([file]),
        () => resolve([]),
      );
    });
  }

  if (!entry.isDirectory) return Promise.resolve([]);

  const reader = entry.createReader();
  return new Promise((resolve) => {
    const collected = [];

    // readEntries returns at most 100 entries per call, so keep reading
    // until it yields an empty batch.
    const readBatch = () => {
      reader.readEntries(
        async (entries) => {
          if (entries.length === 0) {
            resolve(collected);
            return;
          }
          for (const child of entries) {
            collected.push(...(await readEntry(child)));
          }
          readBatch();
        },
        () => resolve(collected),
      );
    };

    readBatch();
  });
}

async function filesFromDataTransfer(dataTransfer) {
  const items = Array.from(dataTransfer.items || []);
  const entries = items
    .map((item) => (item.webkitGetAsEntry ? item.webkitGetAsEntry() : null))
    .filter(Boolean);

  if (entries.length === 0) {
    return Array.from(dataTransfer.files || []);
  }

  const files = [];
  for (const entry of entries) {
    files.push(...(await readEntry(entry)));
  }
  return files;
}

/* ---------------- dropzone wiring ---------------- */

function wireDropzone(zone, fileInput, onFiles) {
  const openPicker = () => fileInput.click();

  zone.addEventListener("click", openPicker);
  zone.addEventListener("keydown", (event) => {
    if (event.key === "Enter" || event.key === " ") {
      event.preventDefault();
      openPicker();
    }
  });

  fileInput.addEventListener("change", () => {
    onFiles(Array.from(fileInput.files || []));
  });

  ["dragenter", "dragover"].forEach((type) => {
    zone.addEventListener(type, (event) => {
      event.preventDefault();
      zone.classList.add("dragover");
    });
  });

  ["dragleave", "dragend"].forEach((type) => {
    zone.addEventListener(type, () => zone.classList.remove("dragover"));
  });

  zone.addEventListener("drop", async (event) => {
    event.preventDefault();
    zone.classList.remove("dragover");
    onFiles(await filesFromDataTransfer(event.dataTransfer));
  });
}

/* ---------------- previews ---------------- */

function renderInputPreview() {
  const preview = el("input-preview");
  const zone = el("drop-input");

  // Release the previous preview URL first: this runs on every file change
  // and on every language switch, so re-creating without revoking leaks.
  if (state.inputURL) {
    URL.revokeObjectURL(state.inputURL);
    state.inputURL = null;
  }

  if (!state.inputFile) {
    preview.hidden = true;
    zone.classList.remove("filled");
    return;
  }

  state.inputURL = URL.createObjectURL(state.inputFile);
  el("input-thumb").src = state.inputURL;
  el("input-name").textContent = state.inputFile.name;
  preview.hidden = false;
  zone.classList.add("filled");
}

function renderTilesPreview() {
  const preview = el("tiles-preview");
  const zone = el("drop-tiles");
  const strip = el("tiles-strip");

  // Same reasoning as renderInputPreview: revoke the previous thumbnail URLs
  // before building a new strip.
  for (const url of state.tileURLs) {
    URL.revokeObjectURL(url);
  }
  state.tileURLs = [];

  if (state.tileFiles.length === 0) {
    preview.hidden = true;
    zone.classList.remove("filled");
    strip.replaceChildren();
    return;
  }

  const thumbs = state.tileFiles.slice(0, MAX_TILE_THUMBS).map((file) => {
    const url = URL.createObjectURL(file);
    state.tileURLs.push(url);
    const img = document.createElement("img");
    img.src = url;
    img.alt = "";
    return img;
  });
  strip.replaceChildren(...thumbs);

  el("tiles-count").textContent = `${state.tileFiles.length} ${t("drop.tiles.selected")}`;
  preview.hidden = false;
  zone.classList.add("filled");
}

function refreshGenerateButton() {
  el("generate").disabled = !state.inputFile || state.tileFiles.length === 0;
}

/* ---------------- errors & progress ---------------- */

function showError(message) {
  const node = el("error");
  node.textContent = message;
  node.hidden = false;
}

function clearError() {
  el("error").hidden = true;
}

let progressTimer = null;

function startProgress() {
  const messages = [t("control.generating"), t("status.matching"), t("status.compositing"), t("status.almost")];
  let index = 0;

  el("progress-text").textContent = messages[0];
  el("progress").hidden = false;

  // Purely cosmetic rotation — the server reports no real progress.
  progressTimer = setInterval(() => {
    index = (index + 1) % messages.length;
    el("progress-text").textContent = messages[index];
  }, 2500);
}

function stopProgress() {
  if (progressTimer !== null) {
    clearInterval(progressTimer);
    progressTimer = null;
  }
  el("progress").hidden = true;
}

/* ---------------- generate ---------------- */

function clampGridSize() {
  const field = el("grid-size");
  let size = parseInt(field.value, 10);
  if (Number.isNaN(size)) size = 50;
  size = Math.min(MAX_GRID, Math.max(MIN_GRID, size));
  field.value = String(size);
  return size;
}

async function generate() {
  clearError();

  if (!state.inputFile) {
    showError(t("error.noInput"));
    return;
  }
  if (state.tileFiles.length === 0) {
    showError(t("error.noTiles"));
    return;
  }

  // The server streams this upload part by part and cannot rewind, so
  // gridSize and input must arrive before any tile photo: the tile
  // downscale limit is derived from both of them.
  const form = new FormData();
  form.append("gridSize", String(clampGridSize()));
  form.append("input", state.inputFile);
  for (const file of state.tileFiles) {
    form.append("tiles", file);
  }

  el("generate").disabled = true;
  el("result").hidden = true;
  startProgress();

  try {
    const res = await fetch("/api/generate", { method: "POST", body: form });

    if (!res.ok) {
      let message = t("error.generic");
      try {
        const payload = await res.json();
        if (payload && payload.error) message = payload.error;
      } catch {
        // Non-JSON error body; keep the generic message.
      }
      showError(message);
      return;
    }

    showResult(await res.blob());
  } catch (err) {
    console.error("[gosaics] generate failed", err);
    showError(t("error.network"));
  } finally {
    stopProgress();
    refreshGenerateButton();
  }
}

function showResult(blob) {
  if (state.resultURL) URL.revokeObjectURL(state.resultURL);
  state.resultURL = URL.createObjectURL(blob);

  el("result-image").src = state.resultURL;
  el("download").href = state.resultURL;
  el("result").hidden = false;
  el("result").scrollIntoView({ behavior: "smooth", block: "start" });
}

function reset() {
  state.inputFile = null;
  state.tileFiles = [];
  if (state.resultURL) {
    URL.revokeObjectURL(state.resultURL);
    state.resultURL = null;
  }

  el("file-input").value = "";
  el("file-tiles").value = "";
  el("result").hidden = true;
  clearError();
  renderInputPreview();
  renderTilesPreview();
  refreshGenerateButton();
}

/* ---------------- init ---------------- */

function init() {
  // Re-enable transitions only after everything has loaded and painted.
  window.addEventListener("load", () => {
    document.documentElement.classList.remove("preload");
  });

  initTheme();

  wireDropzone(el("drop-input"), el("file-input"), (files) => {
    const usable = files.filter(isSupported);
    if (usable.length === 0) {
      showError(t("error.noInput"));
      return;
    }
    clearError();
    state.inputFile = usable[0];
    renderInputPreview();
    refreshGenerateButton();
  });

  wireDropzone(el("drop-tiles"), el("file-tiles"), (files) => {
    const usable = files.filter(isSupported);
    if (usable.length === 0) {
      showError(t("error.noTiles"));
      return;
    }
    clearError();
    state.tileFiles = usable;
    renderTilesPreview();
    refreshGenerateButton();
  });

  el("generate").addEventListener("click", generate);
  el("reset").addEventListener("click", reset);
  el("grid-size").addEventListener("change", clampGridSize);

  const dialog = el("help-dialog");
  el("help-open").addEventListener("click", () => dialog.showModal());
  el("help-close").addEventListener("click", () => dialog.close());

  el("language").addEventListener("change", (event) => {
    loadLanguage(event.target.value).catch((err) => {
      console.error("[gosaics] loading language failed", err);
    });
  });

  const savedLang = localStorage.getItem(LANG_KEY) || "en";
  el("language").value = savedLang;
  loadLanguage(savedLang).catch(() => loadLanguage("en"));
}

document.addEventListener("DOMContentLoaded", init);
