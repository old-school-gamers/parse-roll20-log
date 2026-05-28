package parser

import (
	"fmt"
	"os"
	"path/filepath"
	"strings"
	"testing"
)

func openFixture(t *testing.T, name string) *os.File {
	t.Helper()
	f, err := os.Open(filepath.Join("testdata", name))
	if err != nil {
		t.Fatalf("open fixture: %v", err)
	}
	t.Cleanup(func() { f.Close() })
	return f
}

func parseFixture(t *testing.T, name string) []Message {
	t.Helper()
	msgs, err := Parse(openFixture(t, name))
	if err != nil {
		t.Fatalf("parse: %v", err)
	}
	return msgs
}

func findByID(msgs []Message, id string) *Message {
	for i := range msgs {
		if msgs[i].ID == id {
			return &msgs[i]
		}
	}
	return nil
}

func TestParse_MessageCount(t *testing.T) {
	msgs := parseFixture(t, "messages.html")
	if got, want := len(msgs), 9; got != want {
		t.Fatalf("message count: got %d, want %d", got, want)
	}
}

func TestParse_StructuredRoll(t *testing.T) {
	msgs := parseFixture(t, "messages.html")
	m := findByID(msgs, "m1-structured")
	if m == nil {
		t.Fatal("m1-structured not found")
	}

	want := Message{
		ID:        "m1-structured",
		Timestamp: "2025-06-10T20:49:00",
		Type:      "general",
		Player:    "alice w.",
		Character: "Helfen Adrick",
		RollName:  "Strength (0)",
	}
	if m.Timestamp != want.Timestamp || m.Player != want.Player ||
		m.Character != want.Character || m.RollName != want.RollName || m.Type != want.Type {
		t.Errorf("structured fields mismatch:\n got %+v\nwant %+v", *m, want)
	}
	if len(m.Results) != 1 {
		t.Fatalf("results: got %d, want 1", len(m.Results))
	}
	r := m.Results[0]
	if r.Value != "8" {
		t.Errorf("value: got %q, want %q", r.Value, "8")
	}
	if !strings.Contains(r.Formula, "1d20+0") {
		t.Errorf("formula missing dice expr: %q", r.Formula)
	}
	if strings.Contains(r.Formula, "&lt;") {
		t.Errorf("formula entities not decoded: %q", r.Formula)
	}
	if r.Crit != "none" {
		t.Errorf("crit: got %q, want none", r.Crit)
	}
	if m.Text != "" {
		t.Errorf("structured-only message leaked text: %q", m.Text)
	}
}

func TestParse_CritFumble(t *testing.T) {
	msgs := parseFixture(t, "messages.html")

	crit := findByID(msgs, "m2-crit")
	if crit == nil || len(crit.Results) != 1 {
		t.Fatalf("m2-crit missing or no results: %+v", crit)
	}
	if crit.Results[0].Crit != "crit" {
		t.Errorf("crit flag: got %q, want crit", crit.Results[0].Crit)
	}

	fumble := findByID(msgs, "m3-fumble")
	if fumble == nil || len(fumble.Results) != 1 {
		t.Fatalf("m3-fumble missing or no results: %+v", fumble)
	}
	if fumble.Results[0].Crit != "fumble" {
		t.Errorf("fumble flag: got %q, want fumble", fumble.Results[0].Crit)
	}
}

func TestParse_PlainText(t *testing.T) {
	msgs := parseFixture(t, "messages.html")
	m := findByID(msgs, "m4-plain")
	if m == nil {
		t.Fatal("m4-plain not found")
	}
	if m.Player != "carol" {
		t.Errorf("player: got %q, want %q", m.Player, "carol")
	}
	if m.Text != "Elf, halfling, elf, halfling" {
		t.Errorf("text: got %q, want %q", m.Text, "Elf, halfling, elf, halfling")
	}
	if len(m.Results) != 0 {
		t.Errorf("plain message has results: %+v", m.Results)
	}
}

func TestParse_EmoteWithInlineRoll(t *testing.T) {
	msgs := parseFixture(t, "messages.html")
	m := findByID(msgs, "m5-emote")
	if m == nil {
		t.Fatal("m5-emote not found")
	}
	if m.Type != "emote" {
		t.Errorf("type: got %q, want emote", m.Type)
	}
	// Emote keeps both: the freeform prefix AND the captured roll.
	if !strings.Contains(m.Text, "Carol Strength") {
		t.Errorf("text missing emote prefix: %q", m.Text)
	}
	if len(m.Results) != 1 || m.Results[0].Value != "9" {
		t.Errorf("emote results: %+v", m.Results)
	}
}

func TestParse_NoTimestampOrPlayer(t *testing.T) {
	msgs := parseFixture(t, "messages.html")
	m := findByID(msgs, "m6-continuation")
	if m == nil {
		t.Fatal("m6-continuation not found")
	}
	// Raw parse: nothing inherited.
	if m.Timestamp != "" {
		t.Errorf("raw timestamp should be empty, got %q", m.Timestamp)
	}
	if m.Player != "" {
		t.Errorf("raw player should be empty, got %q", m.Player)
	}
}

func TestInheritContext(t *testing.T) {
	msgs := parseFixture(t, "messages.html")
	got := InheritContext(msgs)
	if len(got) != len(msgs) {
		t.Fatalf("length: got %d, want %d", len(got), len(msgs))
	}
	m := findByID(got, "m6-continuation")
	if m == nil {
		t.Fatal("m6-continuation not found after inherit")
	}
	if m.Timestamp == "" {
		t.Error("timestamp should have been inherited")
	}
	if m.Player == "" {
		t.Error("player should have been inherited")
	}
	// Should inherit from m4-plain (carol) since m5-emote has no player either.
	if m.Player != "carol" {
		t.Errorf("inherited player: got %q, want carol", m.Player)
	}
}

func TestInheritContext_TimeOnly(t *testing.T) {
	// m8-time-only has tstamp "9:15PM" — InheritContext should stitch
	// the date from the previous full-timestamp message (m7 at June 10, 2025).
	msgs := parseFixture(t, "messages.html")
	got := InheritContext(msgs)
	m := findByID(got, "m8-time-only")
	if m == nil {
		t.Fatal("m8-time-only not found")
	}
	if want := "2025-06-10T21:15:00"; m.Timestamp != want {
		t.Errorf("stitched timestamp: got %q, want %q", m.Timestamp, want)
	}
	if SessionDate(m.Timestamp) != "2025-06-10" {
		t.Errorf("SessionDate after stitch: got %q, want 2025-06-10", SessionDate(m.Timestamp))
	}
}

func TestParse_RollResult(t *testing.T) {
	msgs := parseFixture(t, "messages.html")
	m := findByID(msgs, "m7-rollresult")
	if m == nil {
		t.Fatal("m7-rollresult not found")
	}
	if m.Type != "rollresult" {
		t.Errorf("type: got %q, want rollresult", m.Type)
	}
	if len(m.Results) != 1 {
		t.Fatalf("results: got %d, want 1 (formula+rolled paired)", len(m.Results))
	}
	r := m.Results[0]
	if r.Value != "12" {
		t.Errorf("rolled value: got %q, want 12", r.Value)
	}
	if r.Formula != "rolling 3d6" {
		t.Errorf("formula: got %q, want %q", r.Formula, "rolling 3d6")
	}
	// rollresult messages never carry user prose — Text must be empty.
	if m.Text != "" {
		t.Errorf("rollresult leaked text: %q", m.Text)
	}
}

func TestParse_FormulaEntitiesStripped(t *testing.T) {
	// Roll20 embeds <span class="basicdiceroll">N</span> wrappers inside the
	// inlinerollresult title= attribute. After unescape + tag-strip we want a
	// clean human-readable formula.
	msgs := parseFixture(t, "messages.html")
	m := findByID(msgs, "m9-entity-formula")
	if m == nil || len(m.Results) != 1 {
		t.Fatalf("m9 missing or wrong result count: %+v", m)
	}
	got := m.Results[0].Formula
	want := "Rolling 3d6 = (2+3+4)"
	if got != want {
		t.Errorf("formula: got %q, want %q", got, want)
	}
}

func TestIsTimeOnly(t *testing.T) {
	cases := []struct {
		in   string
		want bool
	}{
		{"21:15:00", true},
		{"09:00:00", true},
		{"2025-06-10T21:15:00", false},
		{"21:15", false}, // no seconds
		{"", false},
		{"garbage", false},
	}
	for _, c := range cases {
		if got := isTimeOnly(c.in); got != c.want {
			t.Errorf("isTimeOnly(%q) = %v, want %v", c.in, got, c.want)
		}
	}
}

func TestStripHTMLTags(t *testing.T) {
	cases := []struct{ in, want string }{
		{"plain", "plain"},
		{`Rolling 3d6 = (<span class="basicdiceroll">2</span>+<span class="basicdiceroll">3</span>+<span class="basicdiceroll">4</span>)`,
			"Rolling 3d6 = (2+3+4)"},
		{"<b>bold</b>", "bold"},
		{"", ""},
	}
	for _, c := range cases {
		if got := stripHTMLTags(c.in); got != c.want {
			t.Errorf("stripHTMLTags(%q) = %q, want %q", c.in, got, c.want)
		}
	}
}

func TestParse_TailBleed(t *testing.T) {
	// Real Roll20 exports often contain trailing JavaScript and template
	// content after the last message. A regex-based parser bleeds that into
	// the final message; this fixture exercises that scenario explicitly.
	msgs := parseFixture(t, "tail-bleed.html")
	if got, want := len(msgs), 2; got != want {
		t.Fatalf("message count: got %d, want %d", got, want)
	}
	last := msgs[len(msgs)-1]
	if last.ID != "real-2" {
		t.Errorf("last message ID: got %q, want real-2", last.ID)
	}
	// The trailing <script>/template content must not have leaked in.
	if strings.Contains(last.Text, "document.write") ||
		strings.Contains(last.Text, "{{#each") ||
		strings.Contains(last.Text, "fake") {
		t.Errorf("tail content leaked into final message: %q", last.Text)
	}
	if len(last.Text) > 200 {
		t.Errorf("final message text suspiciously long (%d chars): %q", len(last.Text), last.Text)
	}
}

func TestNormalizeTimestamp(t *testing.T) {
	cases := []struct{ in, want string }{
		{"June 10, 2025 8:49PM", "2025-06-10T20:49:00"},
		{"February 10, 2026 9:55PM", "2026-02-10T21:55:00"},
		{"  June 10, 2025 8:49PM  ", "2025-06-10T20:49:00"},
		{"9:15PM", "21:15:00"},  // time-only — InheritContext stitches the date
		{"9:00 AM", "09:00:00"}, // with space variant
		{"", ""},
		{"not a date", "not a date"}, // unchanged on parse failure
	}
	for _, c := range cases {
		got := normalizeTimestamp(c.in)
		if got != c.want {
			t.Errorf("normalizeTimestamp(%q) = %q, want %q", c.in, got, c.want)
		}
	}
}

func TestSanityCheck_HealthyInput(t *testing.T) {
	msgs := parseFixture(t, "messages.html")
	// Our fixture has < 100 messages so it's "too small" — but it does
	// contain rolls. Either way, no warning is expected.
	if got := SanityCheck(msgs); got != nil {
		t.Errorf("healthy fixture flagged: %v", got)
	}
}

func TestSanityCheck_NoRollsInLargeBatch(t *testing.T) {
	// 200 plain-chat messages, zero rolls — the signature of a Roll20
	// HTML format change that broke roll extraction.
	msgs := make([]Message, 200)
	for i := range msgs {
		msgs[i] = Message{ID: fmt.Sprintf("m%d", i), Type: "general", Text: "chat"}
	}
	warnings := SanityCheck(msgs)
	if len(warnings) != 1 {
		t.Fatalf("warnings: got %d, want 1", len(warnings))
	}
	if !strings.Contains(warnings[0], "0 rolls") {
		t.Errorf("warning missing key phrase: %q", warnings[0])
	}
	if !strings.Contains(warnings[0], "200 messages") {
		t.Errorf("warning missing message count: %q", warnings[0])
	}
}

func TestSanityCheck_SmallBatchSkipped(t *testing.T) {
	// Small logs could legitimately be chat-only short sessions; don't
	// flag them.
	msgs := make([]Message, 50)
	for i := range msgs {
		msgs[i] = Message{ID: fmt.Sprintf("m%d", i), Type: "general", Text: "chat"}
	}
	if got := SanityCheck(msgs); got != nil {
		t.Errorf("small batch flagged: %v", got)
	}
}

func TestSanityCheck_LargeBatchWithSomeRolls(t *testing.T) {
	// 200 messages, just one has a roll — that's plenty to clear the
	// "zero rolls" check.
	msgs := make([]Message, 200)
	for i := range msgs {
		msgs[i] = Message{ID: fmt.Sprintf("m%d", i), Type: "general", Text: "chat"}
	}
	msgs[42].Results = []RollResult{{Value: "7", Crit: "none"}}
	if got := SanityCheck(msgs); got != nil {
		t.Errorf("batch with one roll flagged: %v", got)
	}
}

func TestSessionDate(t *testing.T) {
	cases := []struct{ in, want string }{
		{"2026-02-10T21:55:00", "2026-02-10"},
		{"2025-06-10T20:49:00", "2025-06-10"},
		{"", ""},
		{"garbage", ""},
		{"2026", ""},
	}
	for _, c := range cases {
		got := SessionDate(c.in)
		if got != c.want {
			t.Errorf("SessionDate(%q) = %q, want %q", c.in, got, c.want)
		}
	}
}
