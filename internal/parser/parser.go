// Package parser extracts structured messages from Roll20 chat-log HTML exports.
//
// Roll20's "Save chat log" feature produces a self-contained HTML page where
// every chat message is a <div class="message TYPE" data-messageid="...">
// containing structured sub-elements for timestamp, sender, character name,
// roll name, and one or more inline roll results. This package walks that
// tree (no regex) and emits one [Message] per div in document order.
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
// Used because Roll20 omits the timestamp/sender on consecutive messages from
// the same player.
//
// Time-only stamps (e.g. "20:49:00" with no date — Roll20 emits these once
// the date hasn't changed) get the date from the most recent full timestamp
// prefixed onto them. Midnight rollovers within a session will be attributed
// to the prior day; Roll20 doesn't give us enough context to detect them.
func InheritContext(msgs []Message) []Message {
	out := make([]Message, len(msgs))
	var lastDate, lastTS, lastPlayer string
	for i, m := range msgs {
		switch {
		case m.Timestamp == "":
			m.Timestamp = lastTS
		case isTimeOnly(m.Timestamp):
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
		case hasClass(c, "sheet-roll-name"), hasClass(c, "sheet-rollname"):
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
