package suggest

import (
	"context"
	"encoding/json"
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"regexp"
	"strings"

	"github.com/Rafael-Albernaz-dev/vorq/internal/domain"
)

var modelIDPattern = regexp.MustCompile(`^[a-zA-Z0-9][a-zA-Z0-9._:-]{0,99}$`)

// ListModels uses the official client's local catalog for Codex and explicit
// catalog discovery for Antigravity. It never generates a message.
func (p *CLIProvider) ListModels(ctx context.Context, provider string) ([]domain.AIModel, error) {
	var result []domain.AIModel
	switch provider {
	case "codex":
		result = append(result, domain.AIModel{ID: "", Name: "Padrão do Codex"})
		home, err := os.UserHomeDir()
		if err != nil {
			return nil, err
		}
		data, err := os.ReadFile(filepath.Join(home, ".codex", "models_cache.json"))
		if os.IsNotExist(err) {
			return result, nil
		}
		if err != nil {
			return nil, fmt.Errorf("cannot read Codex model catalog")
		}
		var cache struct {
			Models []struct {
				Slug       string `json:"slug"`
				Name       string `json:"display_name"`
				Visibility string `json:"visibility"`
			} `json:"models"`
		}
		if json.Unmarshal(data, &cache) != nil {
			return nil, fmt.Errorf("invalid Codex model catalog; open Codex once to refresh it")
		}
		for _, m := range cache.Models {
			if m.Visibility != "hide" && strings.HasPrefix(m.Slug, "gpt-") && modelIDPattern.MatchString(m.Slug) {
				result = append(result, domain.AIModel{ID: m.Slug, Name: m.Name})
			}
		}
	case "gemini":
		binary, err := exec.LookPath("agy")
		if err != nil {
			return nil, fmt.Errorf("Antigravity CLI agy not found")
		}
		dir, err := os.MkdirTemp("", "vorq-models-")
		if err != nil {
			return nil, err
		}
		defer os.RemoveAll(dir)
		cmd := exec.CommandContext(ctx, binary, "models")
		cmd.Env = subscriptionEnv(os.Environ())
		cmd.Dir = dir
		out := &limitedBuffer{limit: 64 * 1024}
		cmd.Stdout = out
		if err = cmd.Run(); err != nil {
			if ctx.Err() != nil {
				return nil, ctx.Err()
			}
			return nil, fmt.Errorf("cannot list Gemini models; check agy login and network")
		}
		for _, line := range strings.Split(out.String(), "\n") {
			fields := strings.Fields(line)
			if len(fields) == 0 || !strings.HasPrefix(fields[0], "gemini-") || !modelIDPattern.MatchString(fields[0]) {
				continue
			}
			name := strings.TrimSpace(strings.TrimPrefix(line, fields[0]))
			if name == "" {
				name = fields[0]
			}
			result = append(result, domain.AIModel{ID: fields[0], Name: name})
		}
		if len(result) == 0 {
			return nil, fmt.Errorf("no Gemini models available in Antigravity")
		}
	default:
		return nil, fmt.Errorf("choose codex or gemini")
	}
	if len(result) > 64 {
		result = result[:64]
	}
	return result, nil
}
