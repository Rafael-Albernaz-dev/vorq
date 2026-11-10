package git

import "testing"

func TestTerminalRequirementClassifiesPromptFailures(t *testing.T) {
	for stderr, want := range map[string]bool{
		"fatal: could not read Username for 'https://github.com': terminal prompts disabled": true,
		"git@github.com: Permission denied (publickey).":                                     true,
		"Host key verification failed.":                                                      true,
		"error: gpg failed to sign the data\nfatal: failed to write commit object":           true,
		"error: failed to push some refs (non-fast-forward)":                                 false,
		"fatal: repository not found":                                                        false,
	} {
		if got := terminalRequirement(stderr) != ""; got != want {
			t.Errorf("%q: got %v want %v", stderr, got, want)
		}
	}
}
