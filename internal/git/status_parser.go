package git

import (
	"fmt"
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
	if !strings.HasSuffix(output, "\x00") {
		return nil, fmt.Errorf("truncated porcelain status output")
	}

	tokens := strings.Split(output, "\x00")
	for i := 0; i < len(tokens); i++ {
		entry := tokens[i]
		if len(entry) == 0 {
			if i != len(tokens)-1 {
				return nil, fmt.Errorf("empty porcelain status record")
			}
			continue
		}

		// Minimum porcelain entry: 2 status chars + 1 space + at least 1 path char = 4 chars
		if len(entry) < 4 || entry[2] != ' ' || !strings.ContainsRune(" MADRCUT?!", rune(entry[0])) || !strings.ContainsRune(" MADRCUT?!", rune(entry[1])) {
			return nil, fmt.Errorf("invalid porcelain status record")
		}
		if entry[:2] == "  " {
			return nil, fmt.Errorf("unchanged file in porcelain status")
		}
		if (entry[0] == '?' || entry[1] == '?') && entry[:2] != "??" {
			return nil, fmt.Errorf("invalid untracked status")
		}
		if (entry[0] == '!' || entry[1] == '!') && entry[:2] != "!!" {
			return nil, fmt.Errorf("invalid ignored status")
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
			} else {
				return nil, fmt.Errorf("rename/copy status missing source path")
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
