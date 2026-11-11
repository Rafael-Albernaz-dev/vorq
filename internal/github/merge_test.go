package github

import (
	"context"
	"strings"
	"testing"
)

func TestMergeIsPinnedAndNeverBypasses(t *testing.T) {
	var got []string
	c := &Client{Run: func(_ context.Context, _, _ string, args ...string) (string, error) {
		got = args
		return "", nil
	}}
	if err := c.Merge(context.Background(), ".", "owner/repo", 7, "squash", "abc123"); err != nil {
		t.Fatal(err)
	}
	line := strings.Join(got, " ")
	if line != "pr merge 7 --repo github.com/owner/repo --squash --match-head-commit abc123" {
		t.Fatalf("args %q", line)
	}
	if err := c.Merge(context.Background(), ".", "owner/repo", 7, "--admin", "abc"); err == nil {
		t.Fatal("invalid method accepted")
	}
}

func TestMergeInfoReadsAllowedMethods(t *testing.T) {
	c := &Client{Run: func(_ context.Context, _, _ string, args ...string) (string, error) {
		if args[0] == "repo" {
			return `{"mergeCommitAllowed":false,"squashMergeAllowed":true,"rebaseMergeAllowed":true}`, nil
		}
		return `{"number":7,"state":"OPEN","headRefOid":"abc","mergeable":"MERGEABLE","mergeStateStatus":"CLEAN","statusCheckRollup":[{"name":"ci","status":"COMPLETED","conclusion":"SUCCESS"}]}`, nil
	}}
	info, err := c.MergeInfo(context.Background(), ".", "owner/repo", 7)
	if err != nil {
		t.Fatal(err)
	}
	if strings.Join(info.Methods, ",") != "squash,rebase" || info.Mergeable != "MERGEABLE" || info.PR.HeadOID != "abc" || len(info.PR.Checks) != 1 {
		t.Fatalf("info %+v", info)
	}
}
