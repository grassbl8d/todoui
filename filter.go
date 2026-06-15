package main

import (
	"strings"
	"time"
)

// Local evaluator for the common subset of Todoist filter syntax, so filters
// work offline. Supported: today, overdue, "no date", "no deadline", recurring,
// @label, #project, p1-p4, free text, combined with | , (or) & (and) ! (not)
// and parentheses.

func todayStr() string { return time.Now().Format("2006-01-02") }

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
