package comparecmd

import (
	"errors"
	"fmt"
	"regexp"
	"strings"

	"github.com/mayahiro/nexus/internal/api"
)

func validateCompareEndpoint(label string, endpoint compareEndpoint) error {
	switch {
	case endpoint.SessionID == "" && endpoint.URL == "":
		return fmt.Errorf("%s side requires either --%s-session or --%s-url", label, label, label)
	case endpoint.SessionID != "" && endpoint.URL != "":
		return fmt.Errorf("%s side can not use both session and url", label)
	default:
		return nil
	}
}

func compileCompareRegexps(values []string) ([]*regexp.Regexp, error) {
	patterns := make([]*regexp.Regexp, 0, len(values))
	for _, value := range values {
		pattern, err := regexp.Compile(value)
		if err != nil {
			return nil, fmt.Errorf("invalid ignore-text-regex %q: %w", value, err)
		}
		patterns = append(patterns, pattern)
	}
	return patterns, nil
}

func compileCompareSelectorRules(values []string) ([]compareSelectorRule, error) {
	rules := make([]compareSelectorRule, 0, len(values))
	for _, value := range values {
		rule, err := parseCompareSelectorRule(value)
		if err != nil {
			return nil, err
		}
		rules = append(rules, rule)
	}
	return rules, nil
}

func parseCompareSelectorRule(value string) (compareSelectorRule, error) {
	trimmed := strings.TrimSpace(value)
	if trimmed == "" {
		return compareSelectorRule{}, errors.New("compare selector must not be empty")
	}
	if strings.HasPrefix(trimmed, "@e") {
		return compareSelectorRule{All: []compareSelectorTerm{{Kind: "ref", Value: trimmed}}}, nil
	}

	parts := strings.Split(trimmed, "&")
	terms := make([]compareSelectorTerm, 0, len(parts))
	for _, part := range parts {
		term, err := parseCompareSelectorTerm(part, value)
		if err != nil {
			return compareSelectorRule{}, err
		}
		terms = append(terms, term)
	}
	return compareSelectorRule{All: terms}, nil
}

func parseCompareSelectorTerm(value string, rawInput string) (compareSelectorTerm, error) {
	kind, raw, ok := strings.Cut(strings.TrimSpace(value), "=")
	if !ok {
		return compareSelectorTerm{}, fmt.Errorf("invalid compare selector %q: use @eN or role/name/text/testid/href=<value>[&...]", rawInput)
	}
	kind = strings.TrimSpace(kind)
	raw = strings.TrimSpace(raw)
	if raw == "" {
		return compareSelectorTerm{}, fmt.Errorf("invalid compare selector %q: value must not be empty", rawInput)
	}
	switch kind {
	case "role", "name", "text", "testid", "href":
		return compareSelectorTerm{Kind: kind, Value: raw}, nil
	default:
		return compareSelectorTerm{}, fmt.Errorf("invalid compare selector %q: supported kinds are role, name, text, testid, href, or @eN", rawInput)
	}
}

func matchesCompareSelectorRule(node api.Node, rules []compareSelectorRule) bool {
	for _, rule := range rules {
		matched := true
		for _, term := range rule.All {
			switch term.Kind {
			case "ref":
				if strings.TrimSpace(node.Ref) != term.Value {
					matched = false
				}
			case "role":
				if normalizeFindValue(node.Role) != normalizeFindValue(term.Value) {
					matched = false
				}
			case "name":
				if !compareSelectorContains(node.Name, term.Value) {
					matched = false
				}
			case "text":
				if !compareSelectorContains(node.Text, term.Value) {
					matched = false
				}
			case "testid":
				if !compareSelectorContains(firstNonEmpty(node.Attrs["data-testid"], node.Attrs["data-test"]), term.Value) {
					matched = false
				}
			case "href":
				if !compareSelectorContains(node.Attrs["href"], term.Value) {
					matched = false
				}
			}
			if !matched {
				break
			}
		}
		if matched {
			return true
		}
	}
	return false
}

func compareSelectorContains(value string, needle string) bool {
	return strings.Contains(normalizeFindValue(value), normalizeFindValue(needle))
}

func normalizeFindValue(value string) string {
	return strings.Join(strings.Fields(strings.ToLower(strings.TrimSpace(value))), " ")
}

func normalizeCompareString(value string, ignore []*regexp.Regexp) string {
	normalized := value
	for _, pattern := range ignore {
		normalized = pattern.ReplaceAllString(normalized, "")
	}
	return strings.Join(strings.Fields(strings.TrimSpace(normalized)), " ")
}

func compareNodeLabel(values ...string) string {
	for _, value := range values {
		trimmed := strings.TrimSpace(value)
		if trimmed != "" {
			return trimmed
		}
	}
	return ""
}

func firstNonEmpty(values ...string) string {
	for _, value := range values {
		if strings.TrimSpace(value) != "" {
			return value
		}
	}
	return ""
}
