package clipboard

import (
	"context"
	"fmt"
	"os"
	"os/exec"
	"strings"
)

// Copy writes text only; it never reads or restores the existing clipboard.
func Copy(ctx context.Context, value string) error {
	var candidates [][]string
	if os.Getenv("WAYLAND_DISPLAY") != "" {
		candidates = append(candidates, []string{"wl-copy", "--type", "text/plain;charset=utf-8"})
	}
	if os.Getenv("DISPLAY") != "" {
		candidates = append(candidates, []string{"xclip", "-selection", "clipboard"}, []string{"xsel", "--clipboard", "--input"})
	}
	var last error
	for _, args := range candidates {
		binary, err := exec.LookPath(args[0])
		if err != nil {
			continue
		}
		cmd := exec.CommandContext(ctx, binary, args[1:]...)
		cmd.Stdin = strings.NewReader(value)
		if err = cmd.Run(); err == nil {
			return nil
		}
		last = err
		if ctx.Err() != nil {
			return ctx.Err()
		}
	}
	if last != nil {
		return fmt.Errorf("clipboard unavailable: %w", last)
	}
	return fmt.Errorf("install wl-copy (Wayland) or xclip/xsel (X11) to copy paths")
}
