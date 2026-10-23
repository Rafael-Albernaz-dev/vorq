package git

import (
	"strings"

	"github.com/Rafael-Albernaz-dev/vorq/internal/domain"
)

// ParseStatus parses the NUL-delimited output of `git status --porcelain -z`.
func ParseStatus(output string, wsPath string, branch string) (*domain.ChangeSet, error) {
	changeSet := &domain.ChangeSet{
		WorkspacePath: wsPath,
		Branch:        branch,
		Files:         []domain.FileStatus{},
	}

	if len(output) == 0 {
		return changeSet, nil
	}

	tokens := strings.Split(output, "\x00")
	for i := 0; i < len(tokens); i++ {
		entry := tokens[i]
		if len(entry) == 0 {
			continue
		}

		// Minimum porcelain entry: 2 status chars + 1 space + at least 1 path char = 4 chars
		if len(entry) < 3 {
			continue
		}

		staged := string(entry[0])
		unstaged := string(entry[1])

		var path string
		if len(entry) >= 4 && entry[2] == ' ' {
			path = entry[3:]
		} else {
			path = entry[2:]
		}

		// Porcelain -z puts the destination first, then the original path.
		var origPath string
		if staged == "R" || staged == "C" || unstaged == "R" || unstaged == "C" {
			if i+1 < len(tokens) && tokens[i+1] != "" {
				origPath = tokens[i+1]
				i++ // consume the extra token
			}
		}

		changeSet.Files = append(changeSet.Files, domain.FileStatus{
			Path:     path,
			OrigPath: origPath,
			Staged:   staged,
			Unstaged: unstaged,
		})
	}

	return changeSet, nil
}
