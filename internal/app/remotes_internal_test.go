package app

import "testing"

// One remote policy for link, PRs, spawn and push without upstream.
func TestSelectRemotePolicy(t *testing.T) {
	for _, c := range []struct {
		remotes  []string
		upstream string
		want     string
	}{
		{nil, "", ""},
		{[]string{"fork", "origin"}, "fork", "fork"},
		{[]string{"fork", "origin"}, "", "origin"},
		{[]string{"origin"}, "gone", "origin"},
		{[]string{"upstream"}, "", "upstream"},
		{[]string{"a", "b"}, "", ""},
		{[]string{"a", "b"}, ".", ""},
	} {
		if got := selectRemote(c.remotes, c.upstream); got != c.want {
			t.Errorf("selectRemote(%v, %q) = %q, want %q", c.remotes, c.upstream, got, c.want)
		}
	}
}
