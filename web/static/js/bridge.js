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
        htmx.trigger(triggerTarget, eventName);
        timers.delete(timerKey);
      }
    }, delay);

    timers.set(timerKey, timerId);
  }

  // --- Smart Swap Preservation (Restore cursor & value) ---

  document.addEventListener("htmx:before:swap", (evt) => {
    // Block swap during IME composition
    if (isComposing) {
      evt.preventDefault();
      return;
    }

    savedScrolls = [...document.querySelectorAll("[data-dub-scroll]")].map(
      (el) => [el.getAttribute(SCROLL_KEY), el.scrollTop],
    );

    const activeEl = document.activeElement;
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

    if (!savedInputState) return;

    const input = document.querySelector(`[name="${savedInputState.name}"]`);
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

  // --- Drag & Drop (Wails native API) ---

  // Smart drop: detect WHERE the file was dropped.
  // - Over "From File" upload area + .txt/.csv → import as names
  // - Anywhere else → scan directory (or parent dir if file)
  document.addEventListener("DOMContentLoaded", () => {
    const rt = window.runtime;
    if (rt && rt.OnFileDrop) {
      rt.OnFileDrop((x, y, paths) => {
        if (!paths || paths.length === 0) return;

        const path = paths[0];
        const el = document.elementFromPoint(x, y);
        const isOverUpload = el && el.closest("[data-drop-names]");
        const isNamesFile = /\.(txt|csv)$/i.test(path);

        if (isOverUpload && isNamesFile) {
          htmx.ajax("POST", "/api/names/load", {
            values: { path },
            target: "#main-content",
          });
        } else {
          triggerScan(path);
        }
      }, true);
    }
  });

  // --- Chip Insertion (event delegation) ---
  // The template / find-replace chips are data attributes rather than inline
  // onclick handlers, so the markup stays CSP-friendly and one listener covers
  // every chip regardless of which panel was swapped in.

  document.addEventListener("click", (e) => {
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

    // Arrow keys move between naming-method tabs, per the ARIA tablist pattern.
    if (e.target.matches?.("[data-method-tab]") && ["ArrowLeft", "ArrowRight"].includes(e.key)) {
      e.preventDefault();
      const tabs = [...document.querySelectorAll("[data-method-tab]")];
      const dir = e.key === "ArrowRight" ? 1 : -1;
      tabs[(tabs.indexOf(e.target) + dir + tabs.length) % tabs.length]?.click();
      return;
    }

    if (!isMod) return;

    const isInput = ["INPUT", "TEXTAREA"].includes(document.activeElement?.tagName);

    if (e.key === "Enter") {
      e.preventDefault();
      // Reveal the confirm step rather than executing straight away, so the
      // shortcut cannot skip the guard rail the button enforces.
      window.__dubConfirm(true);
    } else if (e.key === "z" && !isInput) {
      e.preventDefault();
      const undoBtn = document.querySelector('[hx-post="/api/undo"]');
      if (undoBtn) undoBtn.click();
    } else if (e.key === "o" && !isInput) {
      e.preventDefault();
      window.selectDirectory();
    }
  });
})();

// Toggles the two-step confirm for the rename. Both halves are rendered by the
// server; this only flips which one is visible, so the destructive button is
// never the initial state.
window.__dubConfirm = function (show) {
  const offer = document.getElementById("execute-btn");
  const confirm = document.getElementById("execute-confirm");
  if (!offer || !confirm) return;
  offer.hidden = show;
  confirm.hidden = !show;
};

// --- Directory Selection & Helpers ---

window.selectDirectory = async function () {
  const runtime = window.runtime;

  // Prefer Wails 2 runtime
  if (runtime && runtime.OpenDirectoryDialog) {
    try {
      const path = await runtime.OpenDirectoryDialog({
        title: "Select Directory",
      });
      if (path) triggerScan(path);
    } catch (err) {
      console.error("Directory dialog failed:", err);
    }
  } else {
    // Fallback for browser testing
    const path = prompt("Enter directory path (Debug Mode):");
    if (path) triggerScan(path);
  }
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

function triggerScan(path) {
  htmx.ajax("POST", "/api/scan", {
    values: { path },
    target: "#main-content",
  });
}

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
