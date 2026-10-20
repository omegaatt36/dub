# Dub

A clean, cross-platform batch file renamer for your desktop.

Rename many files at once with templates, regular expressions, or your own list. Every rename is previewed first, and one click reverts the last batch.

## Install

Download the latest release for macOS (`.dmg`) or Linux (`.tar.gz`) from [Releases](https://github.com/raiven_kao/dub/releases).

Dub is unsigned, so macOS quarantines it on first launch. Remove the flag:

```sh
xattr -rd com.apple.quarantine /Applications/Dub.app
open /Applications/Dub.app
```

Or click "Cancel" on the warning, then System Settings → Privacy & Security → "Open Anyway".

## Quick Start

- Pick a folder in the **Folders** sidebar, which starts at your home folder, or drag a file or folder into the window. `⌘O` (`Ctrl+O` on Linux) opens any other location.
- The path above the lists is clickable: each parent folder takes you back up, and the buttons beside it hide the sidebar, copy the path or refresh the folder. Dub also refreshes when you switch back to its window.
- Pick a mode: **Template**, **Find & Replace**, or edit the names by hand.
- Check the preview. Duplicate target names are flagged, and the rename stays blocked until you resolve them.
- Press **Rename**, then **Undo** if you change your mind.

Switching folders asks first if you have typed names, since those exist nowhere else; `Esc` answers "keep editing". A template or Find & Replace rule needs no warning. A folder that cannot be read leaves you where you were, and **Undo** keeps working after you move to another folder.

New names must be single filenames. Paths such as `~/photos/image.jpg`, `../image.jpg`, and absolute paths are rejected.
Rename and Undo stay in the original directory and never replace existing files outside the batch.
If the directory identity changes, execution stops before any file moves.

Shortcuts: `⌘O` (`Ctrl+O` on Linux) opens a folder, `⌘R` (`Ctrl+R`) refreshes it, `⌘Enter` starts the rename, `Esc` backs out of the confirmation, and `⌘Z` undoes the last batch. In the folder tree, the arrow keys move and open or close folders, and `Enter` opens one.

## Rename Modes

**Template** builds names from tokens in curly braces. The original extension is appended automatically.

| Token | Result | Example |
|---|---|---|
| `{original}` | Filename without extension | `image01` |
| `{ext}` | Extension without the dot, as it is on disk | `jpg` |
| `{index}` | Counter from 1 | `1`, `2` |
| `{date}` | Modification date | `2023-10-27` |
| `{parent}` | Containing directory name | `Photos` |

Compressed tarballs such as `.tar.gz` and `.tar.xz` count as one extension, so renaming `backup.tar.gz` keeps `.tar.gz`. Any other extra dots stay part of the name: `IMG.2024.jpg` has the stem `IMG.2024`.

Add a format after a colon: `{index:3}` gives `001`, and `{date:20060102}` uses Go's reference time layout. Chain pipes to transform a value: `{original|upper}`, `{original|lower}`, `{original|title}`. Unknown tokens are left as-is.

**Find & Replace** always uses a Go regular expression (RE2) against the filename without its extension. The replacement takes capture groups as `$1`, `$2`. Files that do not match keep their name.

- Search: `IMG_(\d+)`
- Replace: `Photo_$1`

**Manual** lets you type names in place, or drop a `.txt` or `.csv` file on the editor to load a list.

## Filtering

The filter box takes a Go regular expression, matched against the filename without its extension. Shortcuts expand before the pattern compiles:

| Shortcut | Expands to | Matches |
|---|---|---|
| `[serial]`, `[number]` | `(\d+)` | Digits |
| `[word]` | `(\w+)` | Word characters |
| `[alpha]` | `([a-zA-Z]+)` | Letters |
| `[any]` | `(.*)` | Anything |

Files sort naturally by default, so `file_2` comes before `file_10`. The sort menu above the list also orders by modification time or size. `{index}` and manual names follow the order you see.

Hidden files (names starting with a dot) are left out of the list and never renamed. **Show hidden files** lists them, but they stay unticked until you tick them yourself.

Ticking files, changing the filter or the sort re-applies the template or Find & Replace you already set, so the preview stays current. Manual names stay attached to their files.

## Development

Common tasks are defined in `Taskfile.yml` and run through [Task](https://taskfile.dev/).

```sh
task dev      # templ + tailwind watchers, then go run .
task build    # production build via wails3
task package  # .dmg on macOS, .tar.gz on Linux
task test     # Go race tests and frontend regression tests
task check    # go vet, golangci-lint, govulncheck
```

You need [Go](https://go.dev/dl/) 1.27+, [Task](https://taskfile.dev/), the [Wails v3](https://v3.wails.io/getting-started/installation/) CLI (`wails3`), and the Tailwind CSS standalone CLI at `./tailwindcss` in the repository root. Templ, golangci-lint, and govulncheck come from `go.mod`, so `go tool` resolves them for you.

Frontend regression tests run through `go test` with system JavaScriptCore. They use the platform libraries required by Wails.

There is no `wails3 dev` task: that command expects a frontend dev server to proxy to, and dub has no bundler. `task dev` runs the watchers and the app directly instead.

Wails build settings live in `build/config.yml`, with the shared build steps in `build/Taskfile.yml` and the per-platform ones in `build/<goos>/Taskfile.yml`. Run `wails3 task common:update:build-assets` after changing the app metadata.

`go build ./...` and `go test ./...` also work on a fresh clone: `main.go` embeds `web/index.html` and `web/static` directly, and the generated CSS plus vendored htmx are committed.

## License

[MIT](LICENSE)
