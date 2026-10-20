// bridge.js - Wails runtime <-> HTMX bridge
(() => {
  "use strict";

  // Using WeakMap to store debounce timers for each element to avoid global pollution.
  // Key: HTMLInputElement | HTMLFormElement, Value: TimerID
  const timers = new WeakMap();

  // Track global IME state (Single focus principle)
  let isComposing = false;

  // Store input state before HTMX Swap
  let savedInputState = null;
  let savedManualDrafts = new Map();

  // Manual-name forms whose debounced save has not fired yet. Leaving the folder
  // before it does would show the server a stale list of typed names, so the
  // navigation flushes these first (see flushPendingSaves).
  const pendingSaves = new Set();
  let manualSaveDrain = null;

  // The element that held focus before a swap is replaced by the swap. Elements
  // that carry data-focus-key are looked up again afterwards, so a keyboard
  // user keeps their place in the folder tree instead of dropping to <body>.
  const FOCUS_KEY = "data-focus-key";
  let savedFocusKey = null;

  // The discard prompt takes focus once per question, not on every re-render.
  let lastPromptKey = "";

  // #main-content is replaced wholesale on every action, so scroll offsets have
  // to be carried across the swap or the file list jumps back to the top
  // mid-edit.
  const SCROLL_KEY = "data-dub-scroll";
  let savedScrolls = [];

  // --- IME Composition Handling ---

  document.addEventListener("compositionstart", () => {
    isComposing = true;
  });

  document.addEventListener("compositionend", (e) => {
    isComposing = false;
    // Trigger input handling immediately when composition ends
    handleSmartInput(e.target);
  });

  // --- Generic Debounced Input Handler ---
  // Reads behavior from HTML attributes instead of hardcoding names.

  document.addEventListener("input", (e) => {
    if (isComposing) return;
    handleSmartInput(e.target);
  });

  function handleSmartInput(target) {
    if (!target) return;

    // 1. Single Input Debounce (e.g., Pattern Search)
    // HTML: <input name="pattern" data-debounce="400" data-event="pattern-changed">
    if (target.dataset.debounce && target.dataset.event) {
      scheduleTrigger(
        target,
        target,
        target.dataset.event,
        parseInt(target.dataset.debounce),
      );
      return;
    }

    // 2. Form Level Debounce (e.g., Manual Names)
    // Look up for a parent form with data-auto-save
    const form = target.closest("form[data-auto-save]");
    if (form) {
      const delay = parseInt(form.dataset.debounce) || 600;
      const eventName = form.dataset.event || "auto-save";
      scheduleTrigger(form, form, eventName, delay);
    }
  }

  /**
   * Generic trigger scheduler
   * @param {HTMLElement} timerKey - The element to bind the timer to (Input or Form)
   * @param {HTMLElement} triggerTarget - The element that will fire the HTMX event
   * @param {string} eventName - The event name to trigger
   * @param {number} delay - Delay in milliseconds
   */
  function scheduleTrigger(timerKey, triggerTarget, eventName, delay) {
    if (timers.has(timerKey)) {
      clearTimeout(timers.get(timerKey));
    }

    const timerId = setTimeout(() => {
      if (!isComposing) {
        timers.delete(timerKey);
        if (triggerTarget.matches?.("form[data-auto-save]")) {
          flushPendingSaves();
        } else {
          htmx.trigger(triggerTarget, eventName);
        }
      }
    }, delay);

    timers.set(timerKey, timerId);
    if (triggerTarget.matches?.("form[data-auto-save]")) {
      pendingSaves.add(triggerTarget);
    }
  }

  /**
   * Sends any unsent manual names now and resolves once the server has them.
   * The server decides whether a folder change would lose typed names, so it has
   * to know about the last keystrokes before it is asked.
   */
  window.__dubFlushImpl = flushPendingSaves;
  function flushPendingSaves() {
    if (manualSaveDrain) return manualSaveDrain;

    const drain = async () => {
      while (pendingSaves.size) {
        const form = pendingSaves.values().next().value;
        pendingSaves.delete(form);
        if (!form.isConnected) continue;
        clearTimeout(timers.get(form));
        timers.delete(form);

        const params = new URLSearchParams(new FormData(form));
        for (const [key, value] of Object.entries(JSON.parse(form.getAttribute("hx-vals") || "{}"))) {
          params.set(key, value);
        }
        await htmx.ajax("POST", `${form.getAttribute("hx-post")}?${params}`, {
          source: "#manual-save-source",
          target: "#main-content",
          swap: "innerHTML",
        });
      }
    };
    manualSaveDrain = drain().then(
      () => { manualSaveDrain = null; },
      (error) => { manualSaveDrain = null; throw error; },
    );
    return manualSaveDrain;
  }

  const draftMutations = new Set([
    "/api/scan", "/api/select-directory", "/api/tick", "/api/tick-all",
    "/api/pattern", "/api/sort", "/api/hidden-files", "/api/names",
    "/api/names/upload", "/api/names/generate", "/api/names/findreplace",
    "/api/extension-policy", "/api/refresh", "/api/preview", "/api/execute", "/api/undo",
  ]);
  document.addEventListener("htmx:before:request", (event) => {
    const ctx = event.detail?.ctx;
    if (!ctx) return;
    const path = ctx.request.action.split("?")[0];
    if (path === "/api/names" && ctx.request.action.includes("?")) return;
    if (!draftMutations.has(path)) return;
    if (!manualSaveDrain && ![...pendingSaves].some((form) => form.isConnected)) return;

    const fetchRequest = ctx.fetch;
    const targetID = ctx.target?.id;
    ctx.fetch = async (...args) => {
      await flushPendingSaves();
      if (targetID) ctx.target = document.getElementById(targetID) || ctx.target;
      return fetchRequest(...args);
    };
  });

  // --- Smart Swap Preservation (Restore cursor & value) ---

  document.addEventListener("htmx:before:swap", (evt) => {
    // Block swap during IME composition
    if (isComposing) {
      evt.preventDefault();
      return;
    }

    savedManualDrafts = new Map();
    for (const form of pendingSaves) {
      if (!form.isConnected) continue;
      for (const input of form.querySelectorAll("input[data-file-path]")) {
        savedManualDrafts.set(input.dataset.filePath, input.value);
      }
      clearTimeout(timers.get(form));
      timers.delete(form);
      pendingSaves.delete(form);
    }

    savedScrolls = [...document.querySelectorAll("[data-dub-scroll]")].map(
      (el) => [el.getAttribute(SCROLL_KEY), el.scrollTop],
    );

    const activeEl = document.activeElement;
    savedFocusKey = activeEl?.closest?.(`[${FOCUS_KEY}]`)?.getAttribute(FOCUS_KEY) ?? null;
    if (
      activeEl &&
      (activeEl.tagName === "INPUT" || activeEl.tagName === "TEXTAREA") &&
      ["text", "search", "url", "tel", "email", "password"].includes(
        activeEl.type,
      ) &&
      activeEl.name
    ) {
      savedInputState = {
        name: activeEl.name,
        filePath: activeEl.dataset.filePath,
        value: activeEl.value,
        selectionStart: activeEl.selectionStart,
        selectionEnd: activeEl.selectionEnd,
      };
    } else {
      savedInputState = null;
    }
  });

  document.addEventListener("htmx:after:settle", () => {
    for (const [key, top] of savedScrolls) {
      const el = document.querySelector(`[${SCROLL_KEY}="${key}"]`);
      // A shorter list can clamp the old offset, which is the correct outcome.
      if (el) el.scrollTop = top;
    }
    savedScrolls = [];

    for (const input of document.querySelectorAll("input[data-file-path]")) {
      if (!savedManualDrafts.has(input.dataset.filePath)) continue;
      const value = savedManualDrafts.get(input.dataset.filePath);
      input.value = value;
      handleSmartInput(input);
    }
    savedManualDrafts.clear();

    restoreFocus();
    focusDiscardPrompt();

    if (!savedInputState) return;

    const input = savedInputState.filePath
      ? [...document.querySelectorAll("input[data-file-path]")].find(
          (el) => el.dataset.filePath === savedInputState.filePath,
        )
      : document.querySelector(`[name="${savedInputState.name}"]`);
    if (input) {
      input.value = savedInputState.value;
      input.focus();
      try {
        input.setSelectionRange(
          savedInputState.selectionStart,
          savedInputState.selectionEnd,
        );
      } catch (e) {
        // Ignore errors for input types that don't support selectionRange
      }
    }
    savedInputState = null;
  });

  function restoreFocus() {
    const key = savedFocusKey;
    savedFocusKey = null;
    if (!key) return;
    // Only fill the gap the swap left; never take focus from somewhere else.
    const active = document.activeElement;
    if (active && active !== document.body) return;
    document.querySelector(`[${FOCUS_KEY}="${CSS.escape(key)}"]`)?.focus({ preventScroll: true });
  }

  function focusDiscardPrompt() {
    const prompt = document.querySelector("[data-autofocus-key]");
    if (!prompt) {
      lastPromptKey = "";
      return;
    }
    if (prompt.dataset.autofocusKey === lastPromptKey) return;
    lastPromptKey = prompt.dataset.autofocusKey;
    prompt.querySelector("[data-autofocus]")?.focus();
  }

  // --- Folder tree keyboard (ARIA tree pattern) ---
  // One item is in the tab order; arrows move between the rest. The buttons in a
  // row serve the mouse, and the keys below press them.

  const treeItems = () => [...document.querySelectorAll("[data-tree-item]")];
  const rowButton = (item, selector) => item.querySelector(`:scope > div ${selector}`);

  document.addEventListener("focusin", (e) => {
    if (!e.target.matches?.("[data-tree-item]")) return;
    for (const item of treeItems()) item.tabIndex = -1;
    e.target.tabIndex = 0;
  });

  function onTreeKey(e) {
    const item = e.target;
    const items = treeItems();
    const at = items.indexOf(item);
    const expanded = item.getAttribute("aria-expanded") === "true";

    switch (e.key) {
      case "ArrowDown":
        items[at + 1]?.focus();
        break;
      case "ArrowUp":
        items[at - 1]?.focus();
        break;
      case "Home":
        items[0]?.focus();
        break;
      case "End":
        items[items.length - 1]?.focus();
        break;
      case "ArrowRight":
        if (!expanded) rowButton(item, "[data-tree-toggle]")?.click();
        else item.querySelector(":scope > ul > [data-tree-item]")?.focus();
        break;
      case "ArrowLeft":
        if (expanded) rowButton(item, "[data-tree-toggle]")?.click();
        else item.parentElement?.closest("[data-tree-item]")?.focus();
        break;
      case "Enter":
      case " ":
        rowButton(item, "[data-tree-open]")?.click();
        break;
      default:
        return;
    }
    e.preventDefault();
  }

  // --- Copy path ---

  async function copyText(text) {
    try {
      await navigator.clipboard.writeText(text);
    } catch {
      // The async clipboard needs a secure context; fall back to the old way.
      const area = document.createElement("textarea");
      area.value = text;
      area.style.position = "fixed";
      area.style.opacity = "0";
      document.body.append(area);
      area.select();
      document.execCommand("copy");
      area.remove();
    }
  }

  // --- Busy indicator ---
  // A request that takes longer than a blink shows a thin bar and a progress
  // cursor, so a slow disk or a big folder reads as work, not as a hang.

  let inflight = 0;
  let busyTimer = null;

  document.addEventListener("htmx:before:request", () => {
    inflight++;
    busyTimer ??= setTimeout(() => {
      document.body.dataset.busy = "true";
    }, 250);
  });

  document.addEventListener("htmx:finally:request", () => {
    inflight = Math.max(0, inflight - 1);
    if (inflight > 0) return;
    clearTimeout(busyTimer);
    busyTimer = null;
    delete document.body.dataset.busy;
  });

  // --- Refresh when the window is used again ---
  // Files change outside the app while it sits in the background. The silent
  // form answers 204 (no swap) when nothing changed.

  let lastFocusRefresh = 0;
  window.addEventListener("focus", () => {
    const now = Date.now();
    if (now - lastFocusRefresh < 2000) return;
    lastFocusRefresh = now;

    const hasFolder = document.querySelector('[hx-post="/api/refresh"]');
    const typing = [...pendingSaves].some((f) => f.isConnected);
    if (!hasFolder || typing || manualSaveDrain || isComposing) return;

    htmx.ajax("POST", "/api/refresh?silent=true", {
      target: "#main-content",
      swap: "innerHTML",
    });
  });

  // --- Chip Insertion (event delegation) ---
  // The template / find-replace chips are data attributes rather than inline
  // onclick handlers, so the markup stays CSP-friendly and one listener covers
  // every chip regardless of which panel was swapped in.

  document.addEventListener("click", (e) => {
    const copy = e.target.closest?.("[data-copy-path]");
    if (copy) {
      copyText(copy.dataset.copyPath).then(() => {
        copy.dataset.copied = "true";
        copy.title = "Copied";
        setTimeout(() => {
          delete copy.dataset.copied;
          copy.title = "Copy path";
        }, 1500);
      });
      return;
    }

    const chip = e.target.closest?.("[data-append-to-template]");
    if (chip) {
      window.appendToTemplate(chip.dataset.appendToTemplate);
      return;
    }

    const frChip = e.target.closest?.("[data-append-field]");
    if (frChip) {
      window.appendToFindReplace(frChip.dataset.appendField, frChip.dataset.appendText);
    }
  });

  // --- Keyboard Shortcuts ---
  document.addEventListener("keydown", (e) => {
    if (isComposing || e.isComposing || e.keyCode === 229) return;

    const isMod = e.metaKey || e.ctrlKey;

    // Enter advances to the next name in the manual editor. Without this,
    // renaming N files means N round trips through Tab.
    if (
      e.key === "Enter" &&
      !e.shiftKey &&
      !isMod &&
      e.target.matches?.("input[data-enter-next]")
    ) {
      e.preventDefault();
      const all = [...document.querySelectorAll("input[data-enter-next]")];
      const next = all[all.indexOf(e.target) + 1];
      if (next) {
        next.focus();
        next.select();
      }
      return;
    }

    if (e.target.matches?.("[data-tree-item]") && !isMod) {
      onTreeKey(e);
      if (e.defaultPrevented) return;
    }

    // Arrow keys move between naming-method tabs, per the ARIA tablist pattern.
    if (e.target.matches?.("[data-method-tab]") && ["ArrowLeft", "ArrowRight"].includes(e.key)) {
      e.preventDefault();
      const tabs = [...document.querySelectorAll("[data-method-tab]")];
      const dir = e.key === "ArrowRight" ? 1 : -1;
      tabs[(tabs.indexOf(e.target) + dir + tabs.length) % tabs.length]?.click();
      return;
    }

    // Escape answers the open question: "keep editing" for the discard prompt,
    // otherwise back out of the rename confirmation like its Cancel button.
    if (e.key === "Escape") {
      const keepEditing = document.querySelector("[data-discard-cancel]");
      if (keepEditing) {
        e.preventDefault();
        keepEditing.click();
        return;
      }
      const confirm = document.getElementById("execute-confirm");
      if (confirm && !confirm.hidden) {
        e.preventDefault();
        window.__dubConfirm(false);
      }
      return;
    }

    if (!isMod) return;

    const isInput = ["INPUT", "TEXTAREA"].includes(document.activeElement?.tagName);
    const key = e.key.toLowerCase();

    if (e.key === "Enter") {
      e.preventDefault();
      // Reveal the confirm step rather than executing straight away, so the
      // shortcut cannot skip the guard rail the button enforces.
      window.__dubConfirm(true);
    } else if (key === "z" && !e.shiftKey && !isInput) {
      // Inside a text field Z is the field's own undo.
      e.preventDefault();
      const undoBtn = document.querySelector('[hx-post="/api/undo"]');
      if (undoBtn) undoBtn.click();
    } else if (key === "o") {
      // Safe in a text field too: it has no editing meaning there.
      e.preventDefault();
      window.selectDirectory();
    } else if (key === "r" && !e.shiftKey) {
      // Claim the key even with no folder open, or the webview reloads the page.
      e.preventDefault();
      document.querySelector('[hx-post="/api/refresh"]')?.click();
    }
  });
})();

// Exposed for the shortcut and the Open button, which live outside the IIFE.
window.__dubFlush = function () {
  return window.__dubFlushImpl ? window.__dubFlushImpl() : Promise.resolve();
};

// Toggles the two-step confirm for the rename. Both halves are rendered by the
// server; this only flips which one is visible, so the destructive button is
// never the initial state. Focus follows the visible half: the button that held
// it has just been hidden, which would otherwise drop keyboard users on <body>.
window.__dubConfirm = function (show) {
  const offer = document.getElementById("execute-btn");
  const confirm = document.getElementById("execute-confirm");
  if (!offer || !confirm) return;
  offer.hidden = show;
  confirm.hidden = !show;
  (show ? document.getElementById("execute-yes") : offer)?.focus();
};

// --- Directory Selection & Helpers ---

// The Go side owns the native folder picker, so the shortcut just posts to the
// same endpoint the Open button uses and takes the rendered result.
window.selectDirectory = function () {
  window.__dubFlush().then(() =>
    htmx.ajax("POST", "/api/select-directory", {
      target: "#main-content",
    }),
  );
};

// Called by the Go side after it applies a desktop drop: no request rendered
// the new state, so pull it.
window.dubRefresh = function () {
  htmx.ajax("GET", "/api/main", {
    target: "#main-content",
  });
};

window.appendShortcut = function (shortcut) {
  const input = document.querySelector('input[name="pattern"]');
  if (!input) return;

  // Use setRangeText for cleaner insertion and cursor management.
  // 'end' mode places cursor after the inserted text.
  input.setRangeText(shortcut, input.selectionStart, input.selectionEnd, "end");

  htmx.trigger(input, "pattern-changed");
  input.focus();
};

window.applyPreset = function (select) {
  const value = select.value;
  if (!value) return;
  const input = document.querySelector('input[name="pattern"]');
  if (!input) return;
  input.value = value;
  select.value = "";
  htmx.trigger(input, "pattern-changed");
  input.focus();
};

window.appendToTemplate = function (text) {
  const input = document.querySelector('input[name="template"]');
  if (!input) return;
  input.setRangeText(text, input.selectionStart ?? input.value.length, input.selectionEnd ?? input.value.length, "end");
  input.focus();
};

window.appendToFindReplace = function (fieldName, text) {
  const input = document.querySelector(`input[name="${fieldName}"]`);
  if (!input) return;
  input.setRangeText(text, input.selectionStart ?? input.value.length, input.selectionEnd ?? input.value.length, "end");
  input.focus();
};

// --- Theme Toggle ---

const THEME_LABELS = { system: "System", light: "Light", dark: "Dark" };

function currentTheme() {
  return localStorage.getItem("dub-theme") || "system";
}

window.initTheme = function () {
  const theme = currentTheme();
  applyTheme(theme);

  window
    .matchMedia("(prefers-color-scheme: dark)")
    .addEventListener("change", () => {
      if (currentTheme() === "system") applyTheme("system");
    });
};

window.setTheme = function (mode) {
  localStorage.setItem("dub-theme", mode);
  applyTheme(mode);
  updateThemeButton(mode);
};

function applyTheme(mode) {
  const isDark =
    mode === "dark" ||
    (mode === "system" &&
      window.matchMedia("(prefers-color-scheme: dark)").matches);
  if (isDark) {
    document.documentElement.classList.add("dark");
  } else {
    document.documentElement.classList.remove("dark");
  }
}

window.cycleTheme = function () {
  const order = ["system", "light", "dark"];
  const next = order[(order.indexOf(currentTheme()) + 1) % order.length];
  window.setTheme(next);
};

// Reveals the icon for the active mode and hides the other two, so repeated
// calls always converge on the same markup.
function updateThemeButton(mode) {
  const btn = document.getElementById("theme-toggle");
  if (!btn) return;

  for (const icon of btn.querySelectorAll("[data-theme-icon]")) {
    icon.classList.toggle("hidden", icon.dataset.themeIcon !== mode);
  }

  const label = "Theme: " + (THEME_LABELS[mode] || THEME_LABELS.system);
  btn.title = label;
  btn.setAttribute("aria-label", label);
}

document.addEventListener("DOMContentLoaded", () => {
  window.initTheme();
  updateThemeButton(currentTheme());
});

// The header is swapped in by htmx after DOMContentLoaded, so the button only
// exists from the first settle onwards.
document.addEventListener("htmx:after:settle", () => {
  updateThemeButton(currentTheme());
});

// A success banner dismisses itself. It is armed once per element, so a later
// settle that leaves the banner in place does not restart the countdown.
document.addEventListener("htmx:after:settle", () => {
  for (const el of document.querySelectorAll("[data-auto-dismiss]")) {
    const ms = parseInt(el.dataset.autoDismiss, 10);
    el.removeAttribute("data-auto-dismiss");
    setTimeout(() => el.remove(), ms);
  }
});
