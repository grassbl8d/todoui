package main

import (
	"fmt"
	"strings"
	"time"
)

// Local evaluator for the common subset of Todoist filter syntax, so filters
// work offline. Supported: today, overdue, "no date", "no deadline", recurring,
// @label, #project, p1-p4, free text, combined with | , (or) & (and) ! (not)
// and parentheses.

func todayStr() string { return time.Now().In(tz).Format("2006-01-02") }

// dateFmt is the active date display/input format: MDY (default), YMD, or DMY.
var dateFmt = "MDY"

// dateInputHint is the placeholder/label form of the active date format.
func dateInputHint() string {
	switch dateFmt {
	case "YMD":
		return "YYYY-MM-DD"
	case "DMY":
		return "DD-MM-YYYY"
	default:
		return "MM-DD-YYYY"
	}
}

// fmtDate reformats a leading ISO date ("YYYY-MM-DD…") to the active format,
// preserving any trailing time/↻ suffix. Non-date strings pass through.
func fmtDate(s string) string {
	if len(s) < 10 || s[4] != '-' || s[7] != '-' {
		return s
	}
	y, mo, d, rest := s[0:4], s[5:7], s[8:10], s[10:]
	switch dateFmt {
	case "YMD":
		return s
	case "DMY":
		return d + "-" + mo + "-" + y + rest
	default:
		return mo + "-" + d + "-" + y + rest
	}
}

// normalizeDateInput converts a numeric date typed in the active format (e.g.
// MM-DD-YYYY or MM/DD/YYYY) to ISO YYYY-MM-DD. Non-numeric input (natural
// language) is returned unchanged for parseHumanDate to handle.
func normalizeDateInput(s string) string {
	s = strings.TrimSpace(s)
	parts := strings.FieldsFunc(s, func(r rune) bool { return r == '-' || r == '/' || r == '.' })
	if len(parts) != 3 {
		return s
	}
	for _, p := range parts {
		if p == "" || strings.IndexFunc(p, func(r rune) bool { return r < '0' || r > '9' }) >= 0 {
			return s // not all-numeric → leave for natural-language parsing
		}
	}
	var y, mo, d string
	switch dateFmt {
	case "YMD":
		y, mo, d = parts[0], parts[1], parts[2]
	case "DMY":
		d, mo, y = parts[0], parts[1], parts[2]
	default: // MDY
		mo, d, y = parts[0], parts[1], parts[2]
	}
	if len(y) == 2 {
		y = "20" + y
	}
	if len(mo) == 1 {
		mo = "0" + mo
	}
	if len(d) == 1 {
		d = "0" + d
	}
	return y + "-" + mo + "-" + d
}

var weekdayNames = map[string]time.Weekday{
	"sunday": time.Sunday, "sun": time.Sunday,
	"monday": time.Monday, "mon": time.Monday,
	"tuesday": time.Tuesday, "tue": time.Tuesday, "tues": time.Tuesday,
	"wednesday": time.Wednesday, "wed": time.Wednesday,
	"thursday": time.Thursday, "thu": time.Thursday, "thur": time.Thursday, "thurs": time.Thursday,
	"friday": time.Friday, "fri": time.Friday,
	"saturday": time.Saturday, "sat": time.Saturday,
}

// parseHumanDate turns a natural-language phrase (today, tomorrow, next week,
// next month, next friday, in 3 days, mon, …) or a YYYY-MM-DD string into a
// YYYY-MM-DD date. Returns "" if it can't be parsed.
func parseHumanDate(s, today string) string {
	s = strings.ToLower(strings.TrimSpace(s))
	if s == "" {
		return ""
	}
	if len(s) >= 10 && s[4] == '-' && s[7] == '-' {
		return s[:10] // already a date
	}
	t0, err := time.Parse("2006-01-02", today)
	if err != nil {
		return ""
	}
	out := func(d time.Time) string { return d.Format("2006-01-02") }

	switch s {
	case "today", "tod":
		return out(t0)
	case "tomorrow", "tom", "tmr":
		return out(t0.AddDate(0, 0, 1))
	case "yesterday":
		return out(t0.AddDate(0, 0, -1))
	case "next week":
		return out(t0.AddDate(0, 0, 7))
	case "next month":
		return out(t0.AddDate(0, 1, 0))
	case "next year":
		return out(t0.AddDate(1, 0, 0))
	case "end of week", "eow":
		wd := (int(t0.Weekday()) + 6) % 7
		return out(t0.AddDate(0, 0, 6-wd)) // Sunday
	case "end of month", "eom":
		first := time.Date(t0.Year(), t0.Month(), 1, 0, 0, 0, 0, time.UTC)
		return out(first.AddDate(0, 1, -1))
	}
	// "in N day(s)/week(s)/month(s)"
	if strings.HasPrefix(s, "in ") {
		var n int
		var unit string
		if _, e := fmt.Sscanf(s, "in %d %s", &n, &unit); e == nil {
			switch {
			case strings.HasPrefix(unit, "day"):
				return out(t0.AddDate(0, 0, n))
			case strings.HasPrefix(unit, "week"):
				return out(t0.AddDate(0, 0, 7*n))
			case strings.HasPrefix(unit, "month"):
				return out(t0.AddDate(0, n, 0))
			}
		}
	}
	// weekday name (optionally "next <weekday>") → next occurrence
	name := strings.TrimPrefix(s, "next ")
	if wd, ok := weekdayNames[name]; ok {
		delta := (int(wd) - int(t0.Weekday()) + 7) % 7
		if delta == 0 {
			delta = 7 // next, not today
		}
		return out(t0.AddDate(0, 0, delta))
	}
	return ""
}

// dueDateOnly extracts the YYYY-MM-DD prefix from a display due string.
func dueDateOnly(s string) string {
	s = strings.TrimSpace(s)
	if len(s) >= 10 && s[4] == '-' && s[7] == '-' {
		return s[:10]
	}
	return ""
}

// EvalFilter reports whether a task matches a filter expression.
func EvalFilter(expr string, t Task, today string) bool {
	p := &filterParser{tokens: tokenizeFilter(expr)}
	res := p.parseOr(t, today)
	return res
}

type filterParser struct {
	tokens []string
	pos    int
}

func (p *filterParser) peek() string {
	if p.pos < len(p.tokens) {
		return p.tokens[p.pos]
	}
	return ""
}

func (p *filterParser) next() string {
	t := p.peek()
	p.pos++
	return t
}

func (p *filterParser) parseOr(t Task, today string) bool {
	v := p.parseAnd(t, today)
	for p.peek() == "|" || p.peek() == "," {
		p.next()
		r := p.parseAnd(t, today)
		v = v || r
	}
	return v
}

func (p *filterParser) parseAnd(t Task, today string) bool {
	v := p.parseNot(t, today)
	for p.peek() == "&" {
		p.next()
		r := p.parseNot(t, today)
		v = v && r
	}
	return v
}

func (p *filterParser) parseNot(t Task, today string) bool {
	if p.peek() == "!" {
		p.next()
		return !p.parseNot(t, today)
	}
	return p.parsePrimary(t, today)
}

func (p *filterParser) parsePrimary(t Task, today string) bool {
	tok := p.peek()
	if tok == "(" {
		p.next()
		v := p.parseOr(t, today)
		if p.peek() == ")" {
			p.next()
		}
		return v
	}
	if tok == "" || tok == ")" {
		return true
	}
	p.next()
	return matchAtom(tok, t, today)
}

// matchAtom evaluates a single filter atom against a task.
func matchAtom(atom string, t Task, today string) bool {
	a := strings.ToLower(strings.TrimSpace(atom))
	if a == "" {
		return true
	}
	due := dueDateOnly(t.DueDate)
	switch a {
	case "today":
		return due == today
	case "overdue", "od":
		return due != "" && due < today
	case "no date", "no due date", "no due", "nodate":
		return due == ""
	case "no deadline", "!deadline":
		return t.Deadline == ""
	case "deadline":
		return t.Deadline != ""
	case "deadline today":
		return t.Deadline == today
	case "deadline overdue", "deadline past", "deadline due":
		return t.Deadline != "" && t.Deadline <= today
	case "week", "this week", "thisweek", "this+last week":
		due := dueDateOnly(t.DueDate)
		if due == "" {
			return false
		}
		t0, err := time.Parse("2006-01-02", today)
		if err != nil {
			return false
		}
		wd := (int(t0.Weekday()) + 6) % 7 // Monday = 0
		thisMon := t0.AddDate(0, 0, -wd)
		lastMon := thisMon.AddDate(0, 0, -7)
		thisSun := thisMon.AddDate(0, 0, 6)
		return due >= lastMon.Format("2006-01-02") && due <= thisSun.Format("2006-01-02")
	case "this month", "thismonth", "this+last month", "month":
		due := dueDateOnly(t.DueDate)
		if due == "" {
			return false
		}
		t0, err := time.Parse("2006-01-02", today)
		if err != nil {
			return false
		}
		thisFirst := time.Date(t0.Year(), t0.Month(), 1, 0, 0, 0, 0, time.UTC)
		thisLast := thisFirst.AddDate(0, 1, -1)
		start := thisFirst
		if a == "this+last month" || a == "month" {
			start = thisFirst.AddDate(0, -1, 0) // include last month too
		}
		return due >= start.Format("2006-01-02") && due <= thisLast.Format("2006-01-02")
	case "recurring":
		return t.Recurring
	case "p1", "p2", "p3", "p4":
		return t.Priority == a
	}
	if strings.HasPrefix(a, "@") {
		want := strings.TrimPrefix(a, "@")
		for _, l := range strings.Fields(strings.ToLower(t.Labels)) {
			if strings.TrimPrefix(l, "@") == want {
				return true
			}
		}
		return false
	}
	if strings.HasPrefix(a, "#") {
		return strings.EqualFold(t.Project, a) ||
			strings.Contains(strings.ToLower(t.Project), a)
	}
	// fallback: free-text substring over content/project/labels
	hay := strings.ToLower(t.Content + " " + t.Project + " " + t.Labels)
	return strings.Contains(hay, a)
}

// tokenizeFilter splits a filter expression into atoms and operators.
func tokenizeFilter(s string) []string {
	var tokens []string
	var cur strings.Builder
	flush := func() {
		if strings.TrimSpace(cur.String()) != "" {
			tokens = append(tokens, strings.TrimSpace(cur.String()))
		}
		cur.Reset()
	}
	for _, r := range s {
		switch r {
		case '|', '&', ',', '!', '(', ')':
			flush()
			tokens = append(tokens, string(r))
		default:
			cur.WriteRune(r)
		}
	}
	flush()
	return tokens
}
