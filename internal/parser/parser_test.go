package parser

import (
	"fmt"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"
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

// encodePushID builds a valid 20-character Firebase push ID whose embedded
// timestamp is t (local). Only the leading 8 timestamp characters are decoded
// by the parser; the 12 trailing "random" characters are filled with a fixed
// alphabet character. Encoding and decoding both go through time.Local, so the
// round trip is timezone-independent.
func encodePushID(t time.Time) string {
	ms := t.UnixMilli()
	var b [pushIDLen]byte
	for i := 7; i >= 0; i-- {
		b[i] = pushChars[ms&63]
		ms >>= 6
	}
	for i := 8; i < pushIDLen; i++ {
		b[i] = pushChars[0]
	}
	return string(b[:])
}

func TestParse_MessageCount(t *testing.T) {
	msgs := parseFixture(t, "messages.html")
	if got, want := len(msgs), 10; got != want {
		t.Fatalf("message count: got %d, want %d", got, want)
	}
}

func TestParse_TraitsTemplate(t *testing.T) {
	// sheet-rolltemplate-traits uses sheet-trait-name where the simple
	// template uses sheet-roll-name. Both should land in RollName.
	msgs := parseFixture(t, "messages.html")
	m := findByID(msgs, "m10-traits")
	if m == nil {
		t.Fatal("m10-traits not found")
	}
	if m.Character != "Irulan" {
		t.Errorf("character: got %q, want Irulan", m.Character)
	}
	if m.RollName != "Elf Traits" {
		t.Errorf("roll_name from sheet-trait-name: got %q, want %q", m.RollName, "Elf Traits")
	}
	// The trait-name subtree should also not leak into Text.
	if strings.Contains(m.Text, "Elf Traits") {
		t.Errorf("trait-name leaked into text: %q", m.Text)
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

// TestInheritContext_TwoNights_NoCrossBleed covers the ID-less fallback path
// for issue #6: this fixture's message IDs are synthetic and do not decode to a
// timestamp, so InheritContext must fall back to the rendered tstamps. (The
// primary fix, where real Firebase message IDs supply each night's true date,
// is covered by TestInheritContext_MessageIDDates.)
//
// When two distinct game nights both start with time-only timestamps (Roll20
// omits the date once it hasn't changed, and sometimes omits it entirely for
// sessions past the first in a multi-session export), InheritContext must NOT
// carry the prior session's date forward onto the new night's messages.
//
// The fixture has three sessions:
//   - Night 1 (May 26, 2026): full timestamps throughout -- provides the
//     lastDate that would incorrectly bleed forward.
//   - Night 2 (unknown date, 8:43 PM -> 11:58 PM): time-only stamps only --
//     the 20:43 start is more than 4 hours before the previous 23:58 close,
//     triggering the session-reset heuristic.
//   - Night 3 (unknown date, 8:45 PM -> 11:52 PM): same pattern, second reset.
//
// Expected: the night-2 and night-3 messages must NOT receive "2026-05-26" as
// their date, and the two nights must live in separate date buckets
// (specifically both under "(no date)" rather than merged under May 26).
func TestInheritContext_TwoNights_NoCrossBleed(t *testing.T) {
	msgs := parseFixture(t, "two-nights.html")
	inherited := InheritContext(msgs)

	// Night-1 messages must be correctly dated.
	n1open := findByID(inherited, "n1-s1-open")
	if n1open == nil {
		t.Fatal("n1-s1-open not found")
	}
	if want := "2026-05-26"; SessionDate(n1open.Timestamp) != want {
		t.Errorf("night-1 date: got %q, want %q", SessionDate(n1open.Timestamp), want)
	}

	// Night-2 messages must NOT carry May 26 as their date.
	n2open := findByID(inherited, "n2-s1-open")
	if n2open == nil {
		t.Fatal("n2-s1-open not found")
	}
	if got := SessionDate(n2open.Timestamp); got == "2026-05-26" {
		t.Errorf("night-2 inherited night-1 date (off-by-7 regression): got %q", got)
	}

	// Night-3 messages must NOT carry May 26 as their date either.
	n3open := findByID(inherited, "n3-s1-open")
	if n3open == nil {
		t.Fatal("n3-s1-open not found")
	}
	if got := SessionDate(n3open.Timestamp); got == "2026-05-26" {
		t.Errorf("night-3 inherited night-1 date (session-merge regression): got %q", got)
	}
}

// TestInheritContext_TwoNights_SeparateBuckets verifies that the two time-only
// nights land in different date buckets (i.e. are not merged into one).
//
// Because we cannot know the real calendar date from time-only stamps, both
// nights will be bucketed as "(no date)" individually -- but that still means
// the timestamps should be treated as separate sequences. The key invariant is
// that no night-2 message has the same stitched timestamp as a night-1 message,
// and no night-3 message has the same stitched timestamp as a night-2 message.
func TestInheritContext_TwoNights_SeparateBuckets(t *testing.T) {
	msgs := parseFixture(t, "two-nights.html")
	inherited := InheritContext(msgs)

	// Collect stitched timestamps for each night's first message.
	n2open := findByID(inherited, "n2-s1-open")
	n3open := findByID(inherited, "n3-s1-open")
	if n2open == nil || n3open == nil {
		t.Fatal("night-2 or night-3 open message not found")
	}

	// The two nights start at roughly the same time of day (8:43 PM and 8:45 PM).
	// After the fix, both are bare time-only strings (no date prefix) rather than
	// carrying the prior date -- so their SessionDate() returns "".
	// If they shared a date, they'd be merged -- that's the bug we're preventing.
	n2date := SessionDate(n2open.Timestamp)
	n3date := SessionDate(n3open.Timestamp)

	if n2date != "" {
		t.Errorf("night-2 first message should have no date, got %q", n2date)
	}
	if n3date != "" {
		t.Errorf("night-3 first message should have no date, got %q", n3date)
	}

	// Also confirm night-2 and night-3 player context was cleared at the reset:
	// n2-s1-open has its own "by" tag so it sets its own player.
	if n2open.Player != "carol" {
		t.Errorf("night-2 open player: got %q, want carol", n2open.Player)
	}
	if n3open.Player != "dave" {
		t.Errorf("night-3 open player: got %q, want dave", n3open.Player)
	}
}

func TestPushIDEpochMillis(t *testing.T) {
	// A real Roll20 message ID from a Shadowmaze export -- the first 8
	// characters decode to a known absolute millisecond timestamp (this
	// assertion is timezone-independent).
	if ms, ok := pushIDEpochMillis("-Ox-3OhqL5PV9bVqPaYt"); !ok || ms != 1783486323574 {
		t.Errorf("pushIDEpochMillis(real) = %d, %v; want 1783486323574, true", ms, ok)
	}
	// Non-push-ID inputs must be rejected so InheritContext falls back to the
	// rendered tstamp rather than decoding a bogus date.
	bad := []string{
		"",                      // empty
		"m1-structured",         // synthetic fixture ID (wrong length)
		"n2-s1-open",            // synthetic fixture ID (wrong length)
		"-Ox-3OhqL5PV9bVqPaY",   // 19 chars
		"-Ox-3OhqL5PV9bVqPaYtX", // 21 chars
		"-Ox-3Ohq L5PV9bVqPaY",  // 20 chars but contains a space (not in alphabet)
	}
	for _, id := range bad {
		if ms, ok := pushIDEpochMillis(id); ok {
			t.Errorf("pushIDEpochMillis(%q) = %d, true; want _, false", id, ms)
		}
	}
}

func TestMessageIDTime_RoundTrip(t *testing.T) {
	want := time.Date(2026, 7, 7, 23, 52, 0, 0, time.Local)
	got, ok := MessageIDTime(encodePushID(want))
	if !ok {
		t.Fatal("MessageIDTime returned ok=false for a valid push ID")
	}
	if !got.Truncate(time.Minute).Equal(want) {
		t.Errorf("round-trip: got %v, want %v", got.Truncate(time.Minute), want)
	}
	if _, ok := MessageIDTime("not-a-push-id"); ok {
		t.Error("MessageIDTime accepted an invalid ID")
	}
}

// TestInheritContext_MessageIDDates is the issue-#6 regression test for the
// real Roll20 case: every message carries a Firebase push ID. Night 1 has full
// rendered timestamps; night 2 is the export-day session whose tstamps Roll20
// renders time-only (no date) -- the case that used to merge night 2 into
// night 1's date. Because the message IDs carry the true dates, each night must
// land on its own date, and the time-only minute must survive onto it.
func TestInheritContext_MessageIDDates(t *testing.T) {
	n1 := time.Date(2026, 6, 30, 21, 6, 0, 0, time.Local)
	n2 := time.Date(2026, 7, 7, 21, 5, 0, 0, time.Local)
	msgs := []Message{
		{ID: encodePushID(n1), Type: "general", Player: "alice", Timestamp: "2026-06-30T21:06:00"},
		{ID: encodePushID(n1.Add(time.Hour)), Type: "general", Timestamp: "2026-06-30T22:06:00"},
		// Export-day session: Roll20 rendered these with time-only stamps.
		{ID: encodePushID(n2), Type: "general", Player: "bob", Timestamp: "21:05:00"},
		{ID: encodePushID(n2.Add(90 * time.Minute)), Type: "general", Timestamp: "22:35:00"},
	}
	got := InheritContext(msgs)

	if d := SessionDate(got[0].Timestamp); d != "2026-06-30" {
		t.Errorf("night-1 date: got %q, want 2026-06-30", d)
	}
	if d := SessionDate(got[2].Timestamp); d != "2026-07-07" {
		t.Errorf("night-2 date (issue #6): got %q, want 2026-07-07", d)
	}
	if d := SessionDate(got[3].Timestamp); d != "2026-07-07" {
		t.Errorf("night-2 continuation date: got %q, want 2026-07-07", d)
	}
	// The time-only tstamp's minute lands on the ID-derived date.
	if got[2].Timestamp != "2026-07-07T21:05:00" {
		t.Errorf("night-2 open timestamp: got %q, want 2026-07-07T21:05:00", got[2].Timestamp)
	}
	// Player still inherits onto the continuation line.
	if got[3].Player != "bob" {
		t.Errorf("night-2 continuation player: got %q, want bob", got[3].Player)
	}
}

func TestIsSessionReset(t *testing.T) {
	cases := []struct {
		cur, prev string
		want      bool
	}{
		// Normal forward progression -- not a reset.
		{"21:00:00", "20:45:00", false},
		// Small backward jump (within session, e.g. two players send at the same
		// second and messages arrive slightly out of order) -- not a reset.
		{"20:44:00", "20:45:00", false},
		// Exactly 1 hour backward -- at the boundary, not a reset (delta == -threshold).
		{"22:00:00", "23:00:00", false},
		// More than 1 hour backward -- session reset.
		{"20:43:00", "23:58:00", true},
		// Empty prev -- no reset.
		{"20:43:00", "", false},
		// Empty cur -- no reset.
		{"", "23:58:00", false},
		// Both empty -- no reset.
		{"", "", false},
	}
	for _, c := range cases {
		got := isSessionReset(c.cur, c.prev)
		if got != c.want {
			t.Errorf("isSessionReset(%q, %q) = %v, want %v", c.cur, c.prev, got, c.want)
		}
	}
}

func TestTimeOnlyToSeconds(t *testing.T) {
	cases := []struct {
		in   string
		want int
	}{
		{"00:00:00", 0},
		{"01:00:00", 3600},
		{"20:45:00", 74700},
		{"23:58:00", 86280},
		{"", -1},
		{"garbage", -1},
		{"20:45", -1}, // no seconds
	}
	for _, c := range cases {
		got := timeOnlyToSeconds(c.in)
		if got != c.want {
			t.Errorf("timeOnlyToSeconds(%q) = %d, want %d", c.in, got, c.want)
		}
	}
}
