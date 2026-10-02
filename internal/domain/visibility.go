package domain

import (
	"errors"
	"regexp"
	"strings"
)

func (p Profile) VisibleDatabases(names []string) []string {
	result := make([]string, 0, len(names))
	for _, name := range names {
		if len(p.DatabaseAllow) > 0 && !exactName(p.DatabaseAllow, name) {
			continue
		}
		if len(p.DatabaseInclude) > 0 && !matchName(p.DatabaseInclude, name) {
			continue
		}
		if matchName(p.DatabaseExclude, name) {
			continue
		}
		result = append(result, name)
	}
	return result
}
func exactName(patterns []string, name string) bool {
	for _, p := range patterns {
		if p == name {
			return true
		}
	}
	return false
}
func matchName(patterns []string, name string) bool {
	for _, pattern := range patterns {
		var expression strings.Builder
		expression.WriteString("^")
		escape := false
		for _, r := range pattern {
			if escape {
				expression.WriteString(regexp.QuoteMeta(string(r)))
				escape = false
				continue
			}
			switch r {
			case '\\':
				escape = true
			case '*', '%':
				expression.WriteString(".*")
			case '_':
				expression.WriteString(".")
			default:
				expression.WriteString(regexp.QuoteMeta(string(r)))
			}
		}
		if escape {
			expression.WriteString(`\\`)
		}
		expression.WriteString("$")
		if ok, _ := regexp.MatchString(expression.String(), name); ok {
			return true
		}
	}
	return false
}
func (p Profile) ValidatePresentation() error {
	if p.IconColor != "" {
		ok, _ := regexp.MatchString(`^#[0-9a-fA-F]{6}$`, p.IconColor)
		if !ok {
			return errors.New("invalid connection icon color")
		}
	}
	if p.IconType != "" {
		if _, err := Resolve(p.IconType); err != nil {
			return err
		}
	}
	for _, values := range [][]string{p.DatabaseAllow, p.DatabaseInclude, p.DatabaseExclude} {
		if len(values) > 100 {
			return errors.New("at most 100 database display filters are allowed")
		}
		for _, v := range values {
			if len(v) > 256 {
				return errors.New("database filter exceeds 256 bytes")
			}
		}
	}
	return nil
}
