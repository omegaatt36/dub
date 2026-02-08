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
