package main

import (
	"encoding/csv"
	"encoding/json"
	"fmt"
	"io"
	"os"
	"strings"
	"time"

	"github.com/old-school-gamers/parse-roll20-log/internal/parser"
	"github.com/spf13/cobra"
)

func newParseCmd() *cobra.Command {
	var (
		session string
		format  string
	)
	cmd := &cobra.Command{
		Use:   "parse <chat-log.html>",
		Short: "Parse a Roll20 chat log into JSONL or TSV",
		Long: `Parse reads a Roll20 chat-log HTML export and writes one record per
message to stdout. Records inherit timestamp and player from preceding
messages (matching how Roll20's UI collapses repeat senders) and include the
full dice formula for every inline roll.

With --session YYYY-MM-DD, only messages whose timestamp falls on that date
are emitted. Messages with an unparsed or empty timestamp are dropped from
the output when filtering.`,
		Args: cobra.ExactArgs(1),
		RunE: func(cmd *cobra.Command, args []string) error {
			return runParse(cmd.OutOrStdout(), args[0], session, format)
		},
	}
	cmd.Flags().StringVarP(&session, "session", "s", "", "filter to a single session date (YYYY-MM-DD)")
	cmd.Flags().StringVarP(&format, "format", "f", "jsonl", "output format: jsonl or tsv")
	return cmd
}

func runParse(w io.Writer, path, session, format string) error {
	if session != "" {
		if _, err := time.Parse("2006-01-02", session); err != nil {
			return fmt.Errorf("--session must be YYYY-MM-DD: %w", err)
		}
	}
	switch format {
	case "jsonl", "tsv":
	default:
		return fmt.Errorf("unknown --format %q (want jsonl or tsv)", format)
	}

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
	if session != "" {
		msgs = filterBySession(msgs, session)
	}

	switch format {
	case "jsonl":
		return writeJSONL(w, msgs)
	case "tsv":
		return writeTSV(w, msgs)
	}
	return nil
}

func filterBySession(msgs []parser.Message, date string) []parser.Message {
	out := msgs[:0]
	for _, m := range msgs {
		if parser.SessionDate(m.Timestamp) == date {
			out = append(out, m)
		}
	}
	return out
}

func writeJSONL(w io.Writer, msgs []parser.Message) error {
	enc := json.NewEncoder(w)
	enc.SetEscapeHTML(false)
	for _, m := range msgs {
		if err := enc.Encode(m); err != nil {
			return err
		}
	}
	return nil
}

func writeTSV(w io.Writer, msgs []parser.Message) error {
	cw := csv.NewWriter(w)
	cw.Comma = '\t'
	if err := cw.Write([]string{"timestamp", "type", "player", "character", "roll_name", "results", "text"}); err != nil {
		return err
	}
	for _, m := range msgs {
		if err := cw.Write([]string{
			m.Timestamp, m.Type, m.Player, m.Character, m.RollName,
			formatResults(m.Results), m.Text,
		}); err != nil {
			return err
		}
	}
	cw.Flush()
	return cw.Error()
}

func formatResults(rs []parser.RollResult) string {
	if len(rs) == 0 {
		return ""
	}
	parts := make([]string, len(rs))
	for i, r := range rs {
		s := r.Value
		switch r.Crit {
		case "crit":
			s += " (CRIT)"
		case "fumble":
			s += " (FUMBLE)"
		}
		parts[i] = s
	}
	return strings.Join(parts, "; ")
}
