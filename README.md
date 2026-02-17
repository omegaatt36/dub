# Dub

![Go Version](https://img.shields.io/badge/Go-1.26+-00ADD8?style=flat&logo=go)
![Wails](https://img.shields.io/badge/Wails-v2-E30613?style=flat&logo=wails)
![License](https://img.shields.io/badge/license-MIT-blue.svg)

Dub is a modern, cross-platform batch file renamer built with [Go](https://go.dev/) and [Wails](https://wails.io/). It provides a clean, web-based interface for renaming large sets of files using powerful templates, regex patterns, or manual editing.

<p align="center">
  <img src="docs/logo.svg" width="150" alt="Dub Logo">
</p>


## Features

- Cross-Platform: Runs on macOS, Windows, and Linux.
- Flexible Renaming Methods:
  - Template: Use dynamic placeholders like `{index}`, `{date}`, and `{original}` to construct new filenames.
  - Find & Replace: Support for standard text replacement and Regular Expressions.
  - Manual/List: Manually edit names or upload a list of new names (drag & drop supported).
- Real-time Preview: See exactly how your files will be renamed before applying changes.
- Undo Capability: Safely revert the last renaming operation if you make a mistake.
- File Filtering: Filter the file list using glob patterns (e.g., `*.jpg`, `IMG_*`) to target specific files.
- Natural Sort: Files are sorted naturally (e.g., `file_2` comes before `file_10`).
- Drag & Drop: Drag files or folders directly into the application to scan or load name lists.

## Usage Guide

<p align="center">
  <picture>
    <img src="docs/screenshot-light.png" width="49%" alt="Dub Light Mode">
  </picture>
  <picture>
    <img src="docs/screenshot-dark.png" width="49%" alt="Dub Dark Mode">
  </picture>
</p>

### Template Syntax

The template engine allows you to build complex filenames using tokens. Tokens are enclosed in curly braces `{}`.

Available Tokens:

| Token | Description | Example |
| :--- | :--- | :--- |
| `{original}` | The original filename (without extension). | `image01` |
| `{ext}` | The file extension (without dot). | `jpg` |
| `{index}` | A sequential counter (starting from 1). | `1`, `2`, `3` |
| `{date}` | The file's modification date. | `2023-10-27` |
| `{parent}` | The name of the parent directory. | `Photos` |

Formatting:

You can format tokens by adding a colon `:` followed by the format string.

- Index Padding: `{index:3}` results in `001`, `002`, `010`.
- Date Formatting: `{date:2006-01-02}` uses Go's reference time layout.
  - `2006` = Year
  - `01` = Month
  - `02` = Day
  - `15` = Hour (24h)
  - `04` = Minute
  - `05` = Second

Pipes (Modifiers):

You can transform values using pipes `|`.

- `upper`: Convert to uppercase (`{original|upper}`).
- `lower`: Convert to lowercase (`{original|lower}`).
- `title`: Capitalize the first letter of words (`{original|title}`).

Examples:

- `vacation_{index:3}` -> `vacation_001`, `vacation_002`
- `{parent}_{date:20060102}_{index}` -> `Photos_20231027_1`
- `{original|lower}_v2` -> `image01_v2`

### Find & Replace

Use standard string replacement or enable Regular Expressions for advanced matching.

- Search: `IMG_(\d+)`
- Replace: `Photo_$1`

## Development

### Prerequisites

- [Go](https://go.dev/dl/) (v1.26+)
- [Wails](https://wails.io/docs/gettingstarted/installation) CLI
- [Templ](https://templ.guide/quick-start/installation) CLI
- [Task](https://taskfile.dev/) (Build tool)


## Installation

### macOS Installation Notes

If you download a pre-built binary/DMG, you may encounter a security warning. This is normal for unsigned applications. To resolve:

Command Line (Recommended):
```bash
xattr -rd com.apple.quarantine /Applications/Dub.app
open /Applications/Dub.app
```

System Preferences:
1. Click "Cancel" on the warning.
2. Go to System Preferences → Privacy & Security.
3. Click "Open Anyway" for Dub.app.

## License

This project is licensed under the MIT License - see the [LICENSE](LICENSE) file for details.
