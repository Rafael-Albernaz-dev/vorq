// Package github adapts the user's authenticated gh CLI. No credentials are
// collected or persisted by vorq, and creating a PR never pushes a branch.
package github

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"os"
	"os/exec"
	"strconv"
	"strings"
	"time"

	"github.com/Rafael-Albernaz-dev/vorq/internal/domain"
)

type Command func(context.Context, string, string, ...string) (string, error)
type Client struct{ Run Command }

func NewClient() *Client { return &Client{Run: run} }

type limitedBuffer struct {
	bytes.Buffer
	overflow bool
}

func (b *limitedBuffer) Write(p []byte) (int, error) {
	n := len(p)
	left := 4*1024*1024 - b.Len()
	if len(p) > left {
		p = p[:left]
		b.overflow = true
	}
	_, _ = b.Buffer.Write(p)
	return n, nil
}
func run(ctx context.Context, dir, input string, args ...string) (string, error) {
	ctx, cancel := context.WithTimeout(ctx, 45*time.Second)
	defer cancel()
	path, e := exec.LookPath("gh")
	if e != nil {
		return "", fmt.Errorf("instale GitHub CLI (gh) e execute gh auth login")
	}
	cmd := exec.CommandContext(ctx, path, args...)
	cmd.Dir = dir
	cmd.WaitDelay = 2 * time.Second
	cmd.Env = append(os.Environ(), "GH_PROMPT_DISABLED=1", "GH_PAGER=cat", "GH_COLOR_LABELS=0", "NO_COLOR=1")
	cmd.Stdin = strings.NewReader(input)
	out, stderr := &limitedBuffer{}, &limitedBuffer{}
	cmd.Stdout = out
	cmd.Stderr = stderr
	if e = cmd.Run(); e != nil {
		if ctx.Err() != nil {
			return "", ctx.Err()
		}
		return "", fmt.Errorf("gh: %s (confira gh auth status): %w", strings.TrimSpace(stderr.String()), e)
	}
	if out.overflow {
		return "", fmt.Errorf("resposta gh excedeu 4 MiB; reduza o limite/filtro")
	}
	return out.String(), nil
}

const listFields = "number,title,url,state,headRefName,baseRefName,headRefOid,isDraft,mergeCommit,reviewDecision"

func (c *Client) List(ctx context.Context, dir, repo string, o domain.PRListOptions) ([]domain.PullRequest, error) {
	args := []string{"pr", "list", "--repo", "github.com/" + repo, "--state", o.State, "--limit", strconv.Itoa(o.Limit), "--json", listFields}
	if o.Search != "" {
		args = append(args, "--search", o.Search)
	}
	raw, e := c.Run(ctx, dir, "", args...)
	if e != nil {
		return nil, e
	}
	var prs []domain.PullRequest
	if e = json.Unmarshal([]byte(raw), &prs); e != nil {
		return nil, fmt.Errorf("JSON de PRs inválido: %w", e)
	}
	return prs, nil
}
func (c *Client) View(ctx context.Context, dir, repo string, number int) (*domain.PullRequest, error) {
	raw, e := c.Run(ctx, dir, "", "pr", "view", strconv.Itoa(number), "--repo", "github.com/"+repo, "--json", listFields+",body,labels,comments,reviews,statusCheckRollup")
	if e != nil {
		return nil, e
	}
	var pr domain.PullRequest
	if e = json.Unmarshal([]byte(raw), &pr); e != nil {
		return nil, fmt.Errorf("JSON de PR inválido: %w", e)
	}
	if pr.Number != number {
		return nil, fmt.Errorf("PR retornado difere do solicitado")
	}
	return &pr, nil
}
func (c *Client) DefaultBranch(ctx context.Context, dir, repo string) (string, error) {
	raw, e := c.Run(ctx, dir, "", "repo", "view", "github.com/"+repo, "--json", "defaultBranchRef")
	if e != nil {
		return "", e
	}
	var data struct {
		DefaultBranchRef struct {
			Name string `json:"name"`
		} `json:"defaultBranchRef"`
	}
	if e = json.Unmarshal([]byte(raw), &data); e != nil {
		return "", e
	}
	if data.DefaultBranchRef.Name == "" {
		return "", fmt.Errorf("GitHub não informou branch padrão")
	}
	return data.DefaultBranchRef.Name, nil
}
func (c *Client) Save(ctx context.Context, dir string, d domain.PRDraft) (string, error) {
	args := []string{"pr", "create"}
	labelFlag, reviewerFlag := "--label", "--reviewer"
	if d.Number > 0 {
		args = []string{"pr", "edit", strconv.Itoa(d.Number)}
		labelFlag, reviewerFlag = "--add-label", "--add-reviewer"
	} else {
		args = append(args, "--head", d.Head)
		if d.Draft {
			args = append(args, "--draft")
		}
	}
	args = append(args, "--repo", "github.com/"+d.Repository, "--base", d.Base, "--title", d.Title, "--body-file", "-")
	for _, label := range d.Labels {
		args = append(args, labelFlag, label)
	}
	for _, reviewer := range d.Reviewers {
		args = append(args, reviewerFlag, reviewer)
	}
	if d.Number > 0 {
		for _, label := range d.RemoveLabels {
			args = append(args, "--remove-label", label)
		}
	}
	raw, e := c.Run(ctx, dir, d.Body, args...)
	if e != nil {
		return "", fmt.Errorf("envio de PR não confirmado; consulte a lista antes de repetir (uma alteração parcial pode ter ocorrido): %w", e)
	}
	return strings.TrimSpace(raw), nil
}
