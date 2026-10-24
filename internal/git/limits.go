package git

import "fmt"

// Native output must never exhaust memory or be parsed as a complete snapshot
// after truncation. File previews have their separate 256 KiB UI limit.
const MaxCommandOutput = 16 * 1024 * 1024

type OutputLimitError struct{ Limit int }

func (e *OutputLimitError) Error() string {
	return fmt.Sprintf("git output exceeds the %d-byte safety limit; operation blocked", e.Limit)
}
