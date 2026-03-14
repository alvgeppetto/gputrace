package cmd

import (
	"io"
	"testing"

	"github.com/spf13/cobra"
)

func TestCommandHelpRenders(t *testing.T) {
	walkCommands(t, rootCmd)
}

func walkCommands(t *testing.T, command *cobra.Command) {
	t.Helper()

	command.SetOut(io.Discard)
	command.SetErr(io.Discard)
	if err := command.Help(); err != nil {
		t.Fatalf("help failed for %q: %v", command.CommandPath(), err)
	}

	for _, sub := range command.Commands() {
		walkCommands(t, sub)
	}
}
