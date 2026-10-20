//go:build !darwin && !linux

package fs

import (
	"fmt"

	"github.com/omegaatt36/dub/internal/port"
)

func (f *OSFileSystem) OpenRenameDirectory(path string) (port.RenameDirectory, error) {
	return nil, fmt.Errorf("safe rename is unsupported on this platform")
}
