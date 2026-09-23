package gateway

import (
	"errors"
	"os"
	"path/filepath"
)

var errProjectsAbsent = errors.New("PROJECTS_ABSENT")
var errProjectsUnavailable = errors.New("PROJECTS_UNAVAILABLE")
var errProjectsEscape = errors.New("PROJECTS_PATH_ESCAPE")

// Absence means a missing tree below a real directory, not merely an OS error
// matching ErrNotExist: Windows also uses it for children of a regular file.
// Reads never create the tree. Callers decide what missing evidence means for
// their operation; all access still uses os.Root, including reparse containment.
func (d *delegations) openProjects(relative string) (*os.Root, error) {
	if !filepath.IsLocal(relative) {
		return nil, errProjectsEscape
	}
	if d.projects == "" {
		return nil, errProjectsUnavailable
	}
	root, err := os.OpenRoot(d.projects)
	if err == nil {
		return root, nil
	}
	if !errors.Is(err, os.ErrNotExist) {
		return nil, errProjectsUnavailable
	}
	path := filepath.Clean(d.projects)
	for {
		info, statErr := os.Lstat(path)
		if statErr == nil {
			if path != filepath.Clean(d.projects) && info.IsDir() {
				return nil, errProjectsAbsent
			}
			return nil, errProjectsUnavailable
		}
		parent := filepath.Dir(path)
		if !errors.Is(statErr, os.ErrNotExist) || parent == path {
			return nil, errProjectsUnavailable
		}
		path = parent
	}
}
