package cli

import (
	"github.com/Rafael-Albernaz-dev/vorq/internal/ui/tui"
	"testing"
)

func TestIconModesRejectRemovedNerdBackend(t *testing.T) {
	for _, value := range []string{"emoji", "text"} {
		cmd := NewRootCmd()
		if err := cmd.ParseFlags([]string{"--icons=" + value}); err != nil {
			t.Fatal(err)
		}
		mode, err := tui.ParseIconMode(value)
		if err != nil || string(mode) != value {
			t.Fatalf("mode %q: %s %v", value, mode, err)
		}
	}
	for _, args := range [][]string{{"up", "--icons=nerd"}, {"up", "--nerd-fonts"}} {
		cmd := NewRootCmd()
		cmd.SetArgs(args)
		if err := cmd.Execute(); err == nil {
			t.Fatal("removed Nerd mode accepted")
		}
	}
	if cmd := NewRootCmd(); cmd.PersistentFlags().Lookup("icons").DefValue != "emoji" {
		t.Fatal("unsafe default")
	}
}
