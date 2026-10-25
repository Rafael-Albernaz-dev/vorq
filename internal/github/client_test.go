package github

import (
	"context"
	"errors"
	"reflect"
	"strings"
	"testing"

	"github.com/Rafael-Albernaz-dev/vorq/internal/domain"
)

func TestSaveStructuredArgumentsBodyAndNoPush(t *testing.T) {
	for _, edit := range []bool{false, true} {
		t.Run(map[bool]string{false: "create", true: "edit"}[edit], func(t *testing.T) {
			var got []string
			var body string
			c := &Client{Run: func(_ context.Context, dir, input string, args ...string) (string, error) {
				got = args
				body = input
				return "https://github.com/o/r/pull/1\n", nil
			}}
			d := domain.PRDraft{Repository: "o/r", Head: "feature", Base: "main", Title: "literal $(touch nope)", Body: "first\n\nsecond `literal`", Draft: true, Labels: []string{"bug"}, Reviewers: []string{"alice"}, RemoveLabels: []string{"old"}}
			if edit {
				d.Number = 1
			}
			if _, e := c.Save(context.Background(), ".", d); e != nil {
				t.Fatal(e)
			}
			if body != d.Body {
				t.Fatal("body lost newlines")
			}
			if !reflect.DeepEqual(got[:2], []string{"pr", map[bool]string{false: "create", true: "edit"}[edit]}) {
				t.Fatal(got)
			}
			joined := strings.Join(got, "\x00")
			for _, value := range []string{"--repo\x00github.com/o/r", "--body-file\x00-", "--title\x00" + d.Title} {
				if !strings.Contains(joined, value) {
					t.Fatal(got)
				}
			}
			if !edit && !strings.Contains(joined, "--head\x00feature") {
				t.Fatal("implicit push possible")
			}
			if edit && !strings.Contains(joined, "--remove-label\x00old") {
				t.Fatal("remove label missing")
			}
		})
	}
}
func TestListAndViewJSONAndExplicitHost(t *testing.T) {
	c := &Client{Run: func(_ context.Context, _, _ string, args ...string) (string, error) {
		if !strings.Contains(strings.Join(args, " "), "--repo github.com/o/r") {
			t.Fatal(args)
		}
		if args[1] == "list" {
			return `[{"number":7,"title":"test","headRefOid":"abc","mergeCommit":{"oid":"def"},"state":"MERGED"}]`, nil
		}
		return `{"number":7,"body":"details","statusCheckRollup":[{"name":"build","conclusion":"SUCCESS"}],"comments":[{"body":"hi","author":{"login":"alice"}}],"reviews":[{"state":"APPROVED"}]}`, nil
	}}
	prs, e := c.List(context.Background(), ".", "o/r", domain.PRListOptions{State: "all", Limit: 100})
	if e != nil || len(prs) != 1 || prs[0].MergeCommit.OID != "def" {
		t.Fatalf("%+v %v", prs, e)
	}
	p, e := c.View(context.Background(), ".", "o/r", 7)
	if e != nil || len(p.Checks) != 1 || len(p.Reviews) != 1 || p.Comments[0].Author.Login != "alice" {
		t.Fatalf("%+v %v", p, e)
	}
}
func TestInvalidJSONAndUncertainSave(t *testing.T) {
	c := &Client{Run: func(context.Context, string, string, ...string) (string, error) { return "{broken", nil }}
	if _, e := c.List(context.Background(), ".", "o/r", domain.PRListOptions{}); e == nil {
		t.Fatal("malformed JSON accepted")
	}
	c.Run = func(context.Context, string, string, ...string) (string, error) {
		return "", errors.New("network disconnected")
	}
	if _, e := c.Save(context.Background(), ".", domain.PRDraft{}); e == nil || !strings.Contains(e.Error(), "parcial") {
		t.Fatal(e)
	}
}
