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

  document.addEventListener("htmx:beforeSwap", (evt) => {
    // Block swap during IME composition
    if (isComposing) {
      evt.detail.shouldSwap = false;
      return;
    }

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

  document.addEventListener("htmx:afterSettle", () => {
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
})();

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

function triggerScan(path) {
  htmx.ajax("POST", "/api/scan", {
    values: { path },
    target: "#main-content",
  });
}

// --- Theme Toggle ---

window.initTheme = function() {
  const theme = localStorage.getItem("dub-theme") || "system";
  applyTheme(theme);

  window
    .matchMedia("(prefers-color-scheme: dark)")
    .addEventListener("change", () => {
      const current = localStorage.getItem("dub-theme") || "system";
      if (current === "system") applyTheme("system");
    });
};

window.setTheme = function(mode) {
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

window.cycleTheme = function() {
  const current = localStorage.getItem("dub-theme") || "system";
  const order = ["system", "light", "dark"];
  const next = order[(order.indexOf(current) + 1) % order.length];
  window.setTheme(next);
};

function updateThemeButton(mode) {
  const btn = document.getElementById("theme-toggle");
  if (!btn) return;
  const icons = { system: "\u{1F4BB}", light: "\u2600\uFE0F", dark: "\u{1F319}" };
  const labels = { system: "System", light: "Light", dark: "Dark" };
  btn.textContent = icons[mode] || icons.system;
  btn.title = "Theme: " + (labels[mode] || "System");
}

document.addEventListener("DOMContentLoaded", () => {
  window.initTheme();
  updateThemeButton(localStorage.getItem("dub-theme") || "system");
});
