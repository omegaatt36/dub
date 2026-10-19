package domain

import (
	"path/filepath"
	"strings"
)

// Extension returns name's extension including the dot, in the case it has on
// disk. The app derives every stem as TrimSuffix(name, Extension(name)), which
// only works when the extension is an exact suffix, so it must not be
// lowercased here; callers that compare extensions fold case themselves.
//
// Two cases differ from filepath.Ext:
//   - A leading dot is part of the name, not an extension: ".env" has none.
//   - A compressed tarball is one extension: "xxx.tar.gz" yields ".tar.gz", so
//     renaming it cannot leave "yyy.gz". Other multi-dot names such as
//     "IMG.2024.01.jpg" or "file.hidden.txt" are not compound; their last
//     segment is the extension.
func Extension(name string) string {
	ext := filepath.Ext(name)
	if ext == name {
		return ""
	}

	stem := name[:len(name)-len(ext)]
	if len(stem) > len(tarSuffix) && strings.EqualFold(stem[len(stem)-len(tarSuffix):], tarSuffix) {
		return name[len(stem)-len(tarSuffix):]
	}
	return ext
}

const tarSuffix = ".tar"
