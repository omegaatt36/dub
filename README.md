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

- Drag a file or folder into the window to scan its directory.
- Pick a mode: **Template**, **Find & Replace**, or edit the names by hand.
- Check the preview. Duplicate target names are flagged, and the rename stays blocked until you resolve them.
- Press **Rename**, then **Undo** if you change your mind.

## Rename Modes

**Template** builds names from tokens in curly braces. The original extension is appended automatically.

| Token | Result | Example |
|---|---|---|
| `{original}` | Filename without extension | `image01` |
| `{ext}` | Extension without the dot | `jpg` |
| `{index}` | Counter from 1 | `1`, `2` |
| `{date}` | Modification date | `2023-10-27` |
| `{parent}` | Containing directory name | `Photos` |

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

Files sort naturally, so `file_2` comes before `file_10`.

## Development

Common tasks are defined in `Taskfile.yml` and run through [Task](https://taskfile.dev/).

```sh
task dev    # templ + tailwind watchers, then wails dev
task build  # production build
task test   # go test -race -tags webkit2_41 ./...
task check  # go vet, golangci-lint, govulncheck
```

You need [Go](https://go.dev/dl/) 1.27+, [Task](https://taskfile.dev/), the [Wails](https://wails.io/docs/gettingstarted/installation) CLI, and the Tailwind CSS standalone CLI at `./tailwindcss` in the repository root. Templ, golangci-lint, and govulncheck come from `go.mod`, so `go tool` resolves them for you.

`go build ./...` and `go test ./...` also work on a fresh clone: `main.go` embeds `web/index.html` and `web/static` directly, and the generated CSS plus vendored htmx are committed.

## License

[MIT](LICENSE)
