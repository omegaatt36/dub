// bridge.js - Wails runtime <-> HTMX bridge

(function () {
    'use strict';

    var composing = false;
    var patternTimer = null;
    var namesTimer = null;
    var savedInput = null;

    // -----------------------------------------------------------------------
    // 1. IME composition tracking
    // -----------------------------------------------------------------------
    document.addEventListener('compositionstart', function () {
        composing = true;
    });

    document.addEventListener('compositionend', function (e) {
        composing = false;
        // Trigger debounce after composition commits.
        // setTimeout(0) ensures the final input event has been processed.
        setTimeout(function () { handleInput(e.target); }, 0);
    });

    // -----------------------------------------------------------------------
    // 2. IME-aware debounced input → custom HTMX trigger events
    //    Uses event delegation on document so it survives DOM swaps.
    // -----------------------------------------------------------------------
    document.addEventListener('input', function (e) {
        if (composing) return;
        handleInput(e.target);
    });

    function handleInput(target) {
        // Filter Pattern input
        if (target.name === 'pattern') {
            clearTimeout(patternTimer);
            patternTimer = setTimeout(function () {
                var el = document.querySelector('input[name="pattern"]');
                if (el && !composing) htmx.trigger(el, 'pattern-changed');
            }, 400);
            return;
        }
        // Manual Names form
        if (target.closest && target.closest('#manual-names-form')) {
            clearTimeout(namesTimer);
            namesTimer = setTimeout(function () {
                var form = document.getElementById('manual-names-form');
                if (form && !composing) htmx.trigger(form, 'auto-save');
            }, 600);
        }
    }

    // -----------------------------------------------------------------------
    // 3. Swap gate + input value preservation
    //
    //    Problem: a swap replaces the DOM, destroying the user's in-progress
    //    edits in text inputs (value, cursor, IME state).
    //
    //    Solution:
    //    - If composing → block the swap entirely (discard response).
    //      compositionend will fire a new debounce → fresh response later.
    //    - If not composing but a text input is focused → allow the swap
    //      but save the input's current value + cursor, then restore after
    //      settle so the user doesn't lose keystrokes typed between
    //      request-sent and response-received.
    // -----------------------------------------------------------------------
    document.addEventListener('htmx:beforeSwap', function (evt) {
        var target = evt.detail.target;
        if (!target || target.id !== 'main-content') return;

        // Block swap while IME is composing
        if (composing) {
            evt.detail.shouldSwap = false;
            return;
        }

        // Save active text input state before the swap destroys it
        var el = document.activeElement;
        if (el && el.tagName === 'INPUT' && el.type === 'text' && el.name) {
            savedInput = {
                name: el.name,
                value: el.value,
                pos: el.selectionStart,
            };
        } else {
            savedInput = null;
        }
    });

    document.addEventListener('htmx:afterSettle', function () {
        if (!savedInput) return;
        var input = document.querySelector('input[name="' + savedInput.name + '"]');
        if (!input) { savedInput = null; return; }

        // Restore the user's current value (may differ from server response)
        input.value = savedInput.value;
        input.focus();
        var pos = Math.min(savedInput.pos, input.value.length);
        input.setSelectionRange(pos, pos);
        savedInput = null;
    });
})();

// ---------------------------------------------------------------------------
// Directory selection (uses Wails runtime or fallback prompt)
// ---------------------------------------------------------------------------
async function selectDirectory() {
    if (!window.runtime || !window.runtime.OpenDirectoryDialog) {
        var path = prompt('Enter directory path:');
        if (!path) return;
        htmx.ajax('POST', '/api/scan', { values: { path: path }, target: '#main-content' });
        return;
    }
    try {
        var path = await window.runtime.OpenDirectoryDialog({ title: 'Select Directory' });
        if (!path) return;
        htmx.ajax('POST', '/api/scan', { values: { path: path }, target: '#main-content' });
    } catch (err) {
        console.error('Failed to open directory dialog:', err);
    }
}

// ---------------------------------------------------------------------------
// Pattern shortcut insertion (programmatic, no IME concern)
// ---------------------------------------------------------------------------
function appendShortcut(shortcut) {
    var input = document.querySelector('input[name="pattern"]');
    if (!input) return;

    var start = input.selectionStart;
    var end = input.selectionEnd;
    var value = input.value;

    input.value = value.substring(0, start) + shortcut + value.substring(end);
    input.focus();

    var newPos = start + shortcut.length;
    input.setSelectionRange(newPos, newPos);

    htmx.trigger(input, 'pattern-changed');
}
