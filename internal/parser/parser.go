// Package parser extracts structured messages from Roll20 chat-log HTML exports.
//
// Roll20's "Save chat log" feature produces a self-contained HTML page where
// every chat message is a <div class="message TYPE" data-messageid="...">
// containing structured sub-elements for timestamp, sender, character name,
// roll name, and one or more inline roll results. This package walks that
// tree (no regex) and emits one [Message] per div in document order.
//
// The div's data-messageid is a Firebase push ID whose leading characters
// encode the message's creation time; [InheritContext] uses it as the
// authoritative date, because Roll20's rendered timestamp omits the date for
// the export-day session. See [MessageIDTime] and issue #6.
package parser

import (
	"fmt"
	"html"
	"io"
	"strings"
	"time"

	netHTML "golang.org/x/net/html"
	"golang.org/x/net/html/atom"
)

// Message is one parsed Roll20 chat-log entry.
//
// Fields are populated only when present in the source HTML. Use
// [InheritContext] to fill blank Timestamp and Player from preceding messages,
// matching the way Roll20's own UI collapses repeat senders.
type Message struct {
	ID        string       `json:"id"`
	Timestamp string       `json:"timestamp,omitempty"` // ISO 8601 local time, no zone
	Type      string       `json:"type"`                // "general", "emote", etc.
	Player    string       `json:"player,omitempty"`
	Character string       `json:"character,omitempty"`
	RollName  string       `json:"roll_name,omitempty"`
	Results   []RollResult `json:"results,omitempty"`
	Text      string       `json:"text,omitempty"`
}

// RollResult is one inline-roll inside a message. Formula carries the full
// dice expression Roll20 records in the title= attribute (e.g.
// "Rolling 1d20+0 = (8)+0"), with HTML entities unescaped.
type RollResult struct {
	Value   string `json:"value"`
	Formula string `json:"formula,omitempty"`
	Crit    string `json:"crit"` // "none", "crit", "fumble"
}

// Parse reads a Roll20 chat-log HTML export and returns all messages in
// document order.
func Parse(r io.Reader) ([]Message, error) {
	doc, err := netHTML.Parse(r)
	if err != nil {
		return nil, fmt.Errorf("parse html: %w", err)
	}
	var out []Message
	walk(doc, &out)
	return out, nil
}

// InheritContext returns a copy of msgs in which each entry's Timestamp and
// Player are filled from the most recent preceding message that set them.
// Used because Roll20 omits the sender on consecutive messages from the same
// player.
//
// Timestamps come, in order of preference:
//
//  1. From the message ID. A Roll20 data-messageid is a Firebase push ID whose
//     first eight characters encode the message's creation time in
//     milliseconds (see [MessageIDTime]). This is present on every real Roll20
//     message and is the authoritative date source -- crucially, it is immune
//     to the fact that Roll20's rendered tstamp omits the date entirely for the
//     most recent (export-day) session, which otherwise merges that night into
//     the prior session's date (issue #6).
//
//  2. From the rendered tstamp, for messages whose ID is absent or is not a
//     valid push ID (synthetic fixtures, or a future Roll20 ID format). Here we
//     fall back to the older stitching logic: a full "June 10, 2025 8:49PM"
//     stamp sets the date directly; a bare time-only "8:49PM" stamp inherits
//     the last known date. A backward clock jump of more than
//     sessionResetThreshold marks a session boundary, at which the inherited
//     date and player are cleared so a new night is not stamped with the prior
//     night's date.
func InheritContext(msgs []Message) []Message {
	out := make([]Message, len(msgs))
	var lastDate, lastTS, lastPlayer, lastTimeOfDay string
	for i, m := range msgs {
		if t, ok := MessageIDTime(m.ID); ok {
			// Authoritative time from the message ID. Truncated to the minute
			// to match Roll20's rendered precision and the fallback path's
			// output shape.
			m.Timestamp = t.Truncate(time.Minute).Format("2006-01-02T15:04:05")
			lastTS = m.Timestamp
			lastDate = m.Timestamp[:10]
			lastTimeOfDay = m.Timestamp[11:19]
		} else {
			switch {
			case m.Timestamp == "":
				m.Timestamp = lastTS
			case isTimeOnly(m.Timestamp):
				if isSessionReset(m.Timestamp, lastTimeOfDay) {
					// Clock reset: new session. Clear inherited context so we do
					// not assign the prior session's date to this night's
					// messages.
					lastDate = ""
					lastPlayer = ""
				}
				lastTimeOfDay = m.Timestamp
				if lastDate != "" {
					m.Timestamp = lastDate + "T" + m.Timestamp
				}
				lastTS = m.Timestamp
				if d := SessionDate(m.Timestamp); d != "" {
					lastDate = d
				}
			default:
				lastTS = m.Timestamp
				if d := SessionDate(m.Timestamp); d != "" {
					lastDate = d
				}
				// Extract the time-of-day portion from a full ISO timestamp so
				// that the next time-only stamp can be compared against it for
				// session-boundary detection.
				if len(m.Timestamp) >= 19 && m.Timestamp[10] == 'T' {
					lastTimeOfDay = m.Timestamp[11:19]
				}
			}
		}
		if m.Player != "" {
			lastPlayer = m.Player
		} else {
			m.Player = lastPlayer
		}
		out[i] = m
	}
	return out
}

// pushChars is Firebase's push-ID alphabet: 64 characters ordered so that
// lexicographic string order matches chronological order. Roll20 message IDs
// (data-messageid) are Firebase push IDs.
const pushChars = "-0123456789ABCDEFGHIJKLMNOPQRSTUVWXYZ_abcdefghijklmnopqrstuvwxyz"

// pushIDLen is the fixed length of a Firebase push ID. We require an exact
// match so shorter synthetic / non-Roll20 IDs are never mistaken for a
// timestamped ID and decoded into a bogus date.
const pushIDLen = 20

// pushCharValue maps each byte to its index in pushChars, or -1 if the byte is
// not part of the alphabet.
var pushCharValue = func() [256]int {
	var idx [256]int
	for i := range idx {
		idx[i] = -1
	}
	for i := range len(pushChars) {
		idx[pushChars[i]] = i
	}
	return idx
}()

// pushIDEpochMillis decodes the millisecond timestamp embedded in the first
// eight characters of a Firebase push ID (48 bits, six bits per character). ok
// is false when id is not a well-formed 20-character push ID, so callers fall
// back to Roll20's rendered timestamps.
func pushIDEpochMillis(id string) (ms int64, ok bool) {
	if len(id) != pushIDLen {
		return 0, false
	}
	for i := range len(id) {
		if pushCharValue[id[i]] < 0 {
			return 0, false
		}
	}
	for i := range 8 {
		ms = ms*64 + int64(pushCharValue[id[i]])
	}
	return ms, true
}

// MessageIDTime returns the creation time encoded in a Roll20 message ID (a
// Firebase push ID), in the machine's local timezone -- matching the local
// wall clock Roll20 renders in its own UI. ok is false when id is not a valid
// push ID.
//
// This is the authoritative date source for a message. Roll20's rendered
// tstamp drops the date for the most recent (export-day) session, so a parser
// that trusts the tstamp alone stamps that whole night with the prior
// session's date and merges the two. The message ID carries the true
// timestamp regardless. See issue #6.
func MessageIDTime(id string) (time.Time, bool) {
	ms, ok := pushIDEpochMillis(id)
	if !ok {
		return time.Time{}, false
	}
	return time.UnixMilli(ms).Local(), true
}

// sessionResetThreshold is the minimum backward jump in seconds that we treat
// as a session boundary rather than a harmless within-session clock oddity.
// One hour is conservative: within a continuous session, messages may arrive
// slightly out of order (seconds, maybe a minute or two), but a jump back by
// more than an hour reliably signals a new game night starting. DST "fall
// back" can cause an apparent 1-hour repetition, but that only pushes the
// threshold right at the boundary -- the session-reset path is harmless there
// because the date would already be correct from prior full timestamps.
const sessionResetThreshold = 60 * 60 // 1 hour in seconds

// isSessionReset reports whether the time-only stamp cur is far enough behind
// prev to indicate a new session rather than ordinary message ordering. Both
// cur and prev must be "HH:MM:SS" strings; if either is empty or malformed,
// it returns false (no reset).
func isSessionReset(cur, prev string) bool {
	if prev == "" || cur == "" {
		return false
	}
	cs := timeOnlyToSeconds(cur)
	ps := timeOnlyToSeconds(prev)
	if cs < 0 || ps < 0 {
		return false
	}
	// Negative delta means the clock went backward.
	delta := cs - ps
	return delta < -sessionResetThreshold
}

// timeOnlyToSeconds converts "HH:MM:SS" to total seconds since midnight.
// Returns -1 on parse failure.
func timeOnlyToSeconds(s string) int {
	if len(s) != 8 || s[2] != ':' || s[5] != ':' {
		return -1
	}
	h := atoi2(s[0:2])
	m := atoi2(s[3:5])
	sec := atoi2(s[6:8])
	if h < 0 || m < 0 || sec < 0 {
		return -1
	}
	return h*3600 + m*60 + sec
}

// atoi2 parses a two-character decimal string. Returns -1 on error.
func atoi2(s string) int {
	if len(s) != 2 {
		return -1
	}
	d0 := int(s[0] - '0')
	d1 := int(s[1] - '0')
	if d0 < 0 || d0 > 9 || d1 < 0 || d1 > 9 {
		return -1
	}
	return d0*10 + d1
}

// isTimeOnly reports whether ts is a bare "HH:MM:SS" with no date prefix.
func isTimeOnly(ts string) bool {
	return len(ts) == 8 && ts[2] == ':' && ts[5] == ':'
}

func walk(n *netHTML.Node, out *[]Message) {
	if isMessageDiv(n) {
		*out = append(*out, extractMessage(n))
		return // messages don't nest — skip subtree
	}
	for c := n.FirstChild; c != nil; c = c.NextSibling {
		walk(c, out)
	}
}

func isMessageDiv(n *netHTML.Node) bool {
	if n.Type != netHTML.ElementNode || n.DataAtom != atom.Div {
		return false
	}
	classes := strings.Fields(attr(n, "class"))
	return len(classes) >= 2 && classes[0] == "message"
}

func extractMessage(n *netHTML.Node) Message {
	classes := strings.Fields(attr(n, "class"))
	msg := Message{
		ID:   attr(n, "data-messageid"),
		Type: classes[1],
	}

	// pendingFormula holds the "rolling Xdy" text seen in a <div class="formula">
	// so we can pair it with the next <div class="rolled"> result.
	var pendingFormula string

	visit(n, func(c *netHTML.Node) {
		if c.Type != netHTML.ElementNode {
			return
		}
		switch {
		case hasClass(c, "tstamp"):
			msg.Timestamp = normalizeTimestamp(textContent(c))
		case hasClass(c, "by"):
			msg.Player = strings.TrimRight(strings.TrimSpace(textContent(c)), ":")
		case hasClass(c, "sheet-char-name"), hasClass(c, "sheet-charname"):
			msg.Character = collapseSpaces(textContent(c))
		case hasClass(c, "sheet-roll-name"), hasClass(c, "sheet-rollname"),
			hasClass(c, "sheet-trait-name"), hasClass(c, "sheet-feature-name"):
			// `sheet-roll-name` is the simple rolltemplate's name field.
			// `sheet-trait-name` / `sheet-feature-name` are the same
			// concept for the traits / feature rolltemplates — same
			// semantic slot, different template-specific class.
			msg.RollName = collapseSpaces(textContent(c))
		case hasClass(c, "inlinerollresult"):
			cls := attr(c, "class")
			crit := "none"
			switch {
			case strings.Contains(cls, "fullcrit"):
				crit = "crit"
			case strings.Contains(cls, "fullfail"):
				crit = "fumble"
			}
			msg.Results = append(msg.Results, RollResult{
				Value:   collapseSpaces(textContent(c)),
				Formula: stripHTMLTags(html.UnescapeString(attr(c, "title"))),
				Crit:    crit,
			})
		case hasClass(c, "formula") && !hasClass(c, "formattedformula"):
			// Discord-routed /r rolls land in a "rollresult" message type
			// with a separate formula div and a rolled div for the total.
			pendingFormula = collapseSpaces(textContent(c))
		case hasClass(c, "rolled"):
			msg.Results = append(msg.Results, RollResult{
				Value:   collapseSpaces(textContent(c)),
				Formula: pendingFormula,
				Crit:    "none",
			})
			pendingFormula = ""
		}
	})

	// rollresult messages are pure Discord-routed dice output — the only
	// non-skipped text is structural ("=" between formula and total). Drop it
	// rather than leaking it into the Text field.
	if msg.Type != "rollresult" {
		msg.Text = leftoverText(n)
	}
	return msg
}

// leftoverText returns the text content of the message div minus subtrees
// already captured into structured fields. Empty when a message is purely a
// rolltemplate (no free prose).
func leftoverText(msgDiv *netHTML.Node) string {
	var b strings.Builder
	var walk func(*netHTML.Node)
	walk = func(node *netHTML.Node) {
		switch node.Type {
		case netHTML.TextNode:
			b.WriteString(node.Data)
		case netHTML.ElementNode:
			if hasAnyClass(node,
				"tstamp", "by", "avatar", "spacer",
				"sheet-char-name", "sheet-charname",
				"sheet-roll-name", "sheet-rollname",
				"sheet-trait-name", "sheet-feature-name",
				"inlinerollresult",
				"formula", "rolled", "clear") {
				return
			}
		}
		for c := node.FirstChild; c != nil; c = c.NextSibling {
			walk(c)
		}
	}
	for c := msgDiv.FirstChild; c != nil; c = c.NextSibling {
		walk(c)
	}
	return collapseSpaces(b.String())
}

// --- HTML helpers ---

func attr(n *netHTML.Node, key string) string {
	for _, a := range n.Attr {
		if a.Key == key {
			return a.Val
		}
	}
	return ""
}

func hasClass(n *netHTML.Node, name string) bool {
	for _, c := range strings.Fields(attr(n, "class")) {
		if c == name {
			return true
		}
	}
	return false
}

func hasAnyClass(n *netHTML.Node, names ...string) bool {
	classes := strings.Fields(attr(n, "class"))
	for _, c := range classes {
		for _, name := range names {
			if c == name {
				return true
			}
		}
	}
	return false
}

func visit(n *netHTML.Node, fn func(*netHTML.Node)) {
	fn(n)
	for c := n.FirstChild; c != nil; c = c.NextSibling {
		visit(c, fn)
	}
}

func textContent(n *netHTML.Node) string {
	var b strings.Builder
	visit(n, func(c *netHTML.Node) {
		if c.Type == netHTML.TextNode {
			b.WriteString(c.Data)
		}
	})
	return b.String()
}

func collapseSpaces(s string) string {
	return strings.Join(strings.Fields(s), " ")
}

// --- Timestamps ---

// tsFormats are the Roll20 full date+time shapes we recognize, in order of
// how often they appear in real exports.
var tsFormats = []string{
	"January 2, 2006 3:04PM",
	"January 2, 2006 3:04 PM",
	"January 2, 2006 3PM",
	"January 2, 2006",
}

// timeOnlyFormats are bare time-of-day shapes Roll20 emits when the date
// hasn't changed since the last full stamp.
var timeOnlyFormats = []string{
	"3:04PM",
	"3:04 PM",
	"3PM",
}

// normalizeTimestamp converts Roll20's "June 10, 2025 8:49PM" to ISO 8601
// local time "2025-06-10T20:49:00". Time-only inputs ("8:49PM") return as
// "20:49:00" with no date prefix — see [InheritContext] for how those get
// stitched to the prior message's date. Roll20 carries no timezone, so
// neither does the output. Unparseable inputs are returned unchanged.
func normalizeTimestamp(s string) string {
	s = strings.TrimSpace(s)
	if s == "" {
		return ""
	}
	for _, f := range tsFormats {
		if t, err := time.Parse(f, s); err == nil {
			return t.Format("2006-01-02T15:04:05")
		}
	}
	for _, f := range timeOnlyFormats {
		if t, err := time.Parse(f, s); err == nil {
			return t.Format("15:04:05")
		}
	}
	return s
}

// SanityCheck returns warnings about parsed messages that look suspicious —
// typically a sign Roll20 has changed an HTML element name the parser
// depends on. Returns nil when the input looks healthy or is too small
// to judge.
//
// Current heuristic: warn if we parsed at least 100 messages but found
// zero rolls. A real Roll20 campaign log of that size almost always
// contains at least some dice rolls; the most likely cause of zero is
// that the `inlinerollresult` / `rolled` class names have changed.
func SanityCheck(msgs []Message) []string {
	if len(msgs) < 100 {
		return nil
	}
	var withResults int
	for _, m := range msgs {
		if len(m.Results) > 0 {
			withResults++
		}
	}
	if withResults == 0 {
		return []string{
			fmt.Sprintf("parsed %d messages but found 0 rolls — Roll20 may have changed the inlinerollresult / rolled HTML classes the parser depends on. If this is a real Roll20 export rather than a chat-only campaign, please file an issue with a representative HTML snippet.", len(msgs)),
		}
	}
	return nil
}

// SessionDate extracts the YYYY-MM-DD prefix from a normalized timestamp.
// Returns "" if the timestamp isn't in ISO form (e.g. a bare time-only stamp
// before [InheritContext] has stitched a date onto it, or an unparsed Roll20
// string we couldn't match).
func SessionDate(ts string) string {
	if len(ts) >= 10 && ts[4] == '-' && ts[7] == '-' {
		return ts[:10]
	}
	return ""
}

// stripHTMLTags returns the textual content of an HTML fragment, dropping
// element tags but preserving the text between them. Roll20 embeds little
// <span class="basicdiceroll">N</span> wrappers around per-die values inside
// the inlinerollresult title= attribute; stripping them gives a clean
// human-readable formula like "Rolling 3d6 = (2+3+4)".
func stripHTMLTags(s string) string {
	if !strings.ContainsAny(s, "<>") {
		return s
	}
	body := &netHTML.Node{Type: netHTML.ElementNode, Data: "body", DataAtom: atom.Body}
	nodes, err := netHTML.ParseFragment(strings.NewReader(s), body)
	if err != nil {
		return s
	}
	var b strings.Builder
	for _, n := range nodes {
		visit(n, func(c *netHTML.Node) {
			if c.Type == netHTML.TextNode {
				b.WriteString(c.Data)
			}
		})
	}
	return b.String()
}
