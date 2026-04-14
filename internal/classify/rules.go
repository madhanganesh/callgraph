// Package classify categorizes a call target into a Kind so downstream
// renderers can annotate it with an icon.
package classify

import "regexp"

// Kind is the semantic category of a call.
type Kind int

const (
	KindPlain Kind = iota
	KindAPI
	KindDB
	KindThread
)

// Icon returns a Unicode prefix for the kind, or "" for plain.
func (k Kind) Icon() string {
	switch k {
	case KindAPI:
		return "🌐 "
	case KindDB:
		return "🛢️ "
	case KindThread:
		return "🧵 "
	}
	return ""
}

// Rule pairs a regex pattern with the kind to assign on match.
type Rule struct {
	Kind    Kind
	Pattern *regexp.Regexp
}

// Classify returns the first matching rule's Kind, or KindPlain if none match.
// The target string is the fully qualified call target the caller chooses
// (e.g. "net/http.Get", "*database/sql.DB.Query"). Order matters.
func Classify(target string, rules []Rule) Kind {
	for _, r := range rules {
		if r.Pattern.MatchString(target) {
			return r.Kind
		}
	}
	return KindPlain
}

// MustRule compiles a regex and returns a Rule. Panics on a bad pattern —
// intended for use with static rule tables.
func MustRule(kind Kind, pattern string) Rule {
	return Rule{Kind: kind, Pattern: regexp.MustCompile(pattern)}
}
