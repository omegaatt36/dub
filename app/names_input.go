package app

import (
	"bufio"
	"bytes"
	"fmt"
	"io"
	"strings"
)

const (
	maxNamesBytes = 4 << 20
	maxNamesCount = 100000
	// Permit whitespace and Unicode names without accepting arbitrarily long lines.
	maxNameLineBytes = 4096
)

// namesFromLines commits nothing on failure, including failures after valid
// lines. LimitedReader bounds total bytes even for streams of empty lines.
func namesFromLines(r io.Reader) ([]string, error) {
	limited := &io.LimitedReader{R: r, N: maxNamesBytes + 1}
	scanner := bufio.NewScanner(limited)
	scanner.Buffer(make([]byte, 1024), maxNameLineBytes+2)
	var names []string
	for scanner.Scan() {
		if len(scanner.Bytes()) > maxNameLineBytes {
			return nil, fmt.Errorf("name line exceeds %d bytes", maxNameLineBytes)
		}
		if line := strings.TrimSpace(scanner.Text()); line != "" {
			if len(names) == maxNamesCount {
				return nil, fmt.Errorf("names file exceeds %d names", maxNamesCount)
			}
			names = append(names, line)
		}
	}
	if err := scanner.Err(); err != nil {
		return nil, fmt.Errorf("read names (maximum line %d bytes): %w", maxNameLineBytes, err)
	}
	if limited.N == 0 {
		return nil, fmt.Errorf("names file exceeds %d bytes", maxNamesBytes)
	}
	return names, nil
}

// Production filesystems provide OpenFile so reads remain bounded even if a
// file grows after Stat. The fallback keeps older injected filesystems usable.
func (a *App) readNamesPath(path string) ([]string, error) {
	if fs, ok := a.fs.(interface {
		OpenFile(string) (io.ReadCloser, error)
	}); ok {
		file, err := fs.OpenFile(path)
		if err != nil {
			return nil, err
		}
		defer file.Close()
		return namesFromLines(file)
	}
	info, err := a.fs.Stat(path)
	if err == nil && info != nil && info.Size() > maxNamesBytes {
		return nil, fmt.Errorf("names file exceeds %d bytes", maxNamesBytes)
	}
	content, err := a.fs.ReadFile(path)
	if err != nil {
		return nil, err
	}
	return namesFromLines(bytes.NewReader(content))
}
