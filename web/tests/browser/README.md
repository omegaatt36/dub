# Real-browser smoke tests

This opt-in suite runs Chromium, the actual DOM, vendored HTMX, and Dub's HTTP handlers.
The Go host uses the real scanner, pattern service, renamer, and filesystem.
Each test uses temporary files and a separate app instance.
No frontend bundler or production dependency is necessary.

These tests do **not** run the native Wails WebView.
The host serves `web/index.html` and `web/static` without modification.
It supplies an empty response for `/wails/runtime.js`, because no native runtime exists.
The directory picker callback returns a fixture folder instead of a native dialog.
DOM events, HTMX requests, template responses, and disk operations remain real.

## Run the suite

Prerequisites: the Go version from `go.mod`, Node.js 22 or newer, and npm.

From the repository root, run:

```sh
cd web/tests/browser
npm ci
npx playwright install chromium
npm test
```

On Linux CI, use `npx playwright install --with-deps chromium` to install browser system libraries.
The lockfile pins Playwright and its dependencies.
Playwright pins its Chromium build.
The suite builds `cmd/browserhost` with the `browser` build tag.
Normal application builds exclude this command.
The host listens on loopback and serves an ephemeral fixture tree.
Do not use the host with personal files.

For failure analysis, open a retained trace:

```sh
npx playwright show-trace test-results/<failed-test>/trace.zip
```

CI uploads failure traces and host logs in the `browser-results` artifact.

## Coverage

- An immediate sort preserves typed names by file identity.
- An immediate untick preserves the pending edit for the remaining row.
- A folder change flushes pending autosave before the discard decision.
- Keep editing preserves the draft. Discard and open changes the folder.
- The desktop JavaScript callback flushes drafts before its real HTTP drop request.
- Automatic preview and the first confirmation step leave disk contents unchanged.
- Execute renames real files. Undo restores their names and contents.

The JavaScriptCore fixtures in `web/tests` remain separate unit tests.
Those fixtures use DOM and HTMX doubles. This suite does not.
`task test` runs the Go tests, including JavaScriptCore fixtures on supported platforms.
`task check` runs vet, lint, and the vulnerability scan.
CI runs these commands on Linux and macOS.
CI also regenerates templ, CSS, and HTMX assets, then rejects committed-asset differences.

## Why native automation remains separate

The pinned Wails implementation uses WKWebView on macOS and WebKitGTK on Linux.
The relevant upstream files are `pkg/application/webview_window_darwin.go` and `pkg/application/linux_cgo.go`.
Its documented Chromium remote-debugging arguments belong to `WindowsOptions` in `pkg/application/application_options.go`.
Playwright's Chromium connection cannot attach to WKWebView or WebKitGTK.
Playwright's bundled WebKit is also not the application's native WebView.

A native harness needs platform-specific accessibility automation, display sessions, and native-dialog control.
This repository has no such harness. This suite does not claim native automation coverage.
Developer tools alone do not establish repeatable native-drop or IME automation.

## Native release checklist

Use disposable folders on each supported desktop platform.
Record the OS, app revision, and pass/fail result for each item.

1. Open the packaged application.
2. Open a folder with the native picker.
3. Cancel the picker and make sure that the current folder and draft remain unchanged.
4. Type a manual name and immediately drop another folder from Finder or the Linux file manager.
5. Make sure that the discard prompt appears before the folder changes.
6. Select Keep editing and make sure that the draft remains.
7. Repeat the drop, then select Discard and open.
8. Drop a `.txt` or `.csv` name list onto the names editor.
9. Make sure that the editor imports names instead of navigating to the list's folder.
10. Drop multiple files outside the names editor and make sure that the single-folder explanation appears.
11. Use a Chinese, Japanese, or Korean IME in a manual name field.
12. Press Enter to accept composition and make sure that focus stays in the field.
13. Press Enter after composition ends and make sure that focus advances to the next name.
14. Rename files through the two-step confirmation, then use Undo.
15. Make sure that the filenames and contents match the original disposable files.

The automated Chromium results do not replace this checklist.
