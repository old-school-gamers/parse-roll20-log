package main

import (
	"fmt"
	"io"
	"os"
	"sort"
	"text/tabwriter"

	"github.com/old-school-gamers/parse-roll20-log/internal/parser"
	"github.com/spf13/cobra"
)

func newSessionsCmd() *cobra.Command {
	return &cobra.Command{
		Use:   "sessions <chat-log.html>",
		Short: "List distinct session dates in a chat log",
		Long: `Sessions groups every message in the log by its date and prints one row
per date with the message count. Useful for picking which date to pass to
'parse --session'.`,
		Args: cobra.ExactArgs(1),
		RunE: func(cmd *cobra.Command, args []string) error {
			return runSessions(cmd.OutOrStdout(), args[0])
		},
	}
}

func runSessions(w io.Writer, path string) error {
	f, err := os.Open(path)
	if err != nil {
		return err
	}
	defer f.Close()

	msgs, err := parser.Parse(f)
	if err != nil {
		return err
	}
	msgs = parser.InheritContext(msgs)

	counts := map[string]int{}
	for _, m := range msgs {
		d := parser.SessionDate(m.Timestamp)
		if d == "" {
			d = "(no date)"
		}
		counts[d]++
	}

	dates := make([]string, 0, len(counts))
	for d := range counts {
		dates = append(dates, d)
	}
	sort.Strings(dates)

	tw := tabwriter.NewWriter(w, 0, 0, 2, ' ', 0)
	fmt.Fprintln(tw, "DATE\tMESSAGES")
	for _, d := range dates {
		fmt.Fprintf(tw, "%s\t%d\n", d, counts[d])
	}
	return tw.Flush()
}
