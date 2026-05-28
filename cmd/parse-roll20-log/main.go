// parse-roll20-log is a CLI for extracting structured session data from
// Roll20 chat-log HTML exports.
package main

import (
	"fmt"
	"os"
	"runtime/debug"

	"github.com/spf13/cobra"
)

func main() {
	if err := newRootCmd().Execute(); err != nil {
		// cobra already prints the error; just set the exit code.
		os.Exit(1)
	}
}

func newRootCmd() *cobra.Command {
	root := &cobra.Command{
		Use:   "parse-roll20-log",
		Short: "Extract structured session data from Roll20 chat-log HTML exports",
		Long: `parse-roll20-log walks a saved Roll20 chat log and pulls out timestamps,
players, characters, rolls (including full dice formulas), and free-form chat.

Use 'parse' for the main extraction, 'sessions' to list distinct session dates
in a log, and 'stats' for a quick per-player / per-type breakdown.`,
		SilenceUsage: true,
		Version:      buildVersion(),
	}
	root.AddCommand(newParseCmd(), newSessionsCmd(), newStatsCmd(), newVersionCmd())
	return root
}

func newVersionCmd() *cobra.Command {
	return &cobra.Command{
		Use:   "version",
		Short: "Print the binary version",
		Args:  cobra.NoArgs,
		Run: func(cmd *cobra.Command, _ []string) {
			fmt.Fprintln(cmd.OutOrStdout(), buildVersion())
		},
	}
}

// buildVersion returns the module version embedded by the Go toolchain, or
// the VCS commit when built from a working tree.
func buildVersion() string {
	info, ok := debug.ReadBuildInfo()
	if !ok {
		return "unknown"
	}
	if info.Main.Version != "" && info.Main.Version != "(devel)" {
		return info.Main.Version
	}
	var rev, modified string
	for _, s := range info.Settings {
		switch s.Key {
		case "vcs.revision":
			if len(s.Value) > 12 {
				rev = s.Value[:12]
			} else {
				rev = s.Value
			}
		case "vcs.modified":
			if s.Value == "true" {
				modified = "+dirty"
			}
		}
	}
	if rev != "" {
		return fmt.Sprintf("devel-%s%s", rev, modified)
	}
	return "devel"
}
