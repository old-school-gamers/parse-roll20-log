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

func newStatsCmd() *cobra.Command {
	return &cobra.Command{
		Use:   "stats <chat-log.html>",
		Short: "Per-player and per-type counts for a chat log",
		Long: `Stats prints message counts grouped by player and by message type, plus
totals for rolls, criticals, and fumbles. Useful as a sanity check after
parsing a new export.`,
		Args: cobra.ExactArgs(1),
		RunE: func(cmd *cobra.Command, args []string) error {
			return runStats(cmd.OutOrStdout(), args[0])
		},
	}
}

func runStats(w io.Writer, path string) error {
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

	byPlayer := map[string]int{}
	byType := map[string]int{}
	var rolls, crits, fumbles int
	for _, m := range msgs {
		p := m.Player
		if p == "" {
			p = "(unknown)"
		}
		byPlayer[p]++
		byType[m.Type]++
		for _, r := range m.Results {
			rolls++
			switch r.Crit {
			case "crit":
				crits++
			case "fumble":
				fumbles++
			}
		}
	}

	fmt.Fprintf(w, "Total messages: %d\n", len(msgs))
	fmt.Fprintf(w, "Total rolls:    %d  (crits: %d, fumbles: %d)\n\n", rolls, crits, fumbles)

	printSortedCounts(w, "By player", byPlayer)
	fmt.Fprintln(w)
	printSortedCounts(w, "By message type", byType)
	return nil
}

func printSortedCounts(w io.Writer, header string, counts map[string]int) {
	type kv struct {
		k string
		v int
	}
	rows := make([]kv, 0, len(counts))
	for k, v := range counts {
		rows = append(rows, kv{k, v})
	}
	sort.Slice(rows, func(i, j int) bool {
		if rows[i].v != rows[j].v {
			return rows[i].v > rows[j].v
		}
		return rows[i].k < rows[j].k
	})

	fmt.Fprintln(w, header+":")
	tw := tabwriter.NewWriter(w, 0, 0, 2, ' ', 0)
	for _, r := range rows {
		fmt.Fprintf(tw, "  %s\t%d\n", r.k, r.v)
	}
	tw.Flush()
}
