package service

import (
	"net/url"
	"regexp"
	"strings"
	"unicode"
	"unicode/utf8"

	"gist/backend/internal/model"
	"gist/backend/pkg/sanitizer"
)

// CompiledArticleRule holds a rule together with its precompiled regexes.
type CompiledArticleRule struct {
	Rule    model.ArticleRule
	regexps []*regexp.Regexp
}

// CompileArticleRule validates regex conditions and prepares a rule for matching.
func CompileArticleRule(rule model.ArticleRule) (*CompiledArticleRule, error) {
	compiled := &CompiledArticleRule{
		Rule:    rule,
		regexps: make([]*regexp.Regexp, len(rule.Conditions)),
	}
	for i, condition := range rule.Conditions {
		if condition.Operator != model.RuleOperatorRegex && condition.Operator != model.RuleOperatorNotRegex {
			continue
		}
		re, err := regexp.Compile(condition.Value)
		if err != nil {
			return nil, err
		}
		compiled.regexps[i] = re
	}
	return compiled, nil
}

// Matches reports whether the entry satisfies all/any conditions of the rule.
func (c *CompiledArticleRule) Matches(entry model.Entry) bool {
	if c == nil || len(c.Rule.Conditions) == 0 {
		return false
	}

	result := false
	for i, condition := range c.Rule.Conditions {
		matched := matchArticleCondition(condition, c.regexps[i], entry, c.Rule.FullWord)
		if i == 0 {
			result = matched
			continue
		}
		if condition.JoinType == model.RuleJoinOr {
			result = result || matched
		} else {
			result = result && matched
		}
	}
	return result
}

// MatchArticleRule compiles and evaluates a single rule.
func MatchArticleRule(rule model.ArticleRule, entry model.Entry) bool {
	compiled, err := CompileArticleRule(rule)
	if err != nil {
		return false
	}
	return compiled.Matches(entry)
}

func matchArticleCondition(condition model.RuleCondition, re *regexp.Regexp, entry model.Entry, fullWord bool) bool {
	haystack := strings.TrimSpace(fieldValue(condition.Field, entry))
	value := strings.TrimSpace(condition.Value)
	if value == "" {
		return false
	}

	switch condition.Operator {
	case model.RuleOperatorRegex:
		return re != nil && re.MatchString(haystack)
	case model.RuleOperatorNotRegex:
		return re != nil && !re.MatchString(haystack)
	case model.RuleOperatorContains:
		return containsWithWordMode(haystack, value, fullWord)
	case model.RuleOperatorNotContains:
		return !containsWithWordMode(haystack, value, fullWord)
	case model.RuleOperatorIs:
		return strings.EqualFold(haystack, value)
	case model.RuleOperatorIsNot:
		return !strings.EqualFold(haystack, value)
	case model.RuleOperatorStartsWith:
		return strings.HasPrefix(strings.ToLower(haystack), strings.ToLower(value))
	case model.RuleOperatorEndsWith:
		return strings.HasSuffix(strings.ToLower(haystack), strings.ToLower(value))
	default:
		return false
	}
}

func fieldValue(field string, entry model.Entry) string {
	switch field {
	case model.RuleFieldTitle:
		return derefString(entry.Title)
	case model.RuleFieldContent:
		return sanitizer.StripTags(derefString(entry.Content))
	case model.RuleFieldTitleOrContent:
		title := strings.TrimSpace(derefString(entry.Title))
		content := strings.TrimSpace(sanitizer.StripTags(derefString(entry.Content)))
		return strings.TrimSpace(title + " " + content)
	case model.RuleFieldAuthor:
		return derefString(entry.Author)
	case model.RuleFieldURL:
		return derefString(entry.URL)
	case model.RuleFieldURLWithoutHost:
		return urlWithoutHost(derefString(entry.URL))
	default:
		return ""
	}
}

func urlWithoutHost(rawURL string) string {
	parsed, err := url.Parse(strings.TrimSpace(rawURL))
	if err != nil {
		return rawURL
	}
	path := parsed.EscapedPath()
	if path == "" {
		path = "/"
	}
	if parsed.RawQuery != "" {
		path += "?" + parsed.RawQuery
	}
	return path
}

func containsWithWordMode(haystack, needle string, fullWord bool) bool {
	if !fullWord {
		return strings.Contains(strings.ToLower(haystack), strings.ToLower(needle))
	}
	if containsCJK(needle) {
		return strings.Contains(strings.ToLower(haystack), strings.ToLower(needle))
	}

	lowerHaystack := strings.ToLower(haystack)
	lowerNeedle := strings.ToLower(needle)
	for offset := 0; offset <= len(lowerHaystack)-len(lowerNeedle); {
		index := strings.Index(lowerHaystack[offset:], lowerNeedle)
		if index == -1 {
			return false
		}
		index += offset
		if hasWordBoundaries(lowerHaystack, index, len(lowerNeedle)) {
			return true
		}
		offset = index + 1
	}
	return false
}

func hasWordBoundaries(haystack string, start, length int) bool {
	if start > 0 {
		before, _ := utf8.DecodeLastRuneInString(haystack[:start])
		if isWordRune(before) {
			return false
		}
	}
	end := start + length
	if end < len(haystack) {
		after, _ := utf8.DecodeRuneInString(haystack[end:])
		if isWordRune(after) {
			return false
		}
	}
	return true
}

func isWordRune(r rune) bool {
	return r == '_' || unicode.IsLetter(r) || unicode.IsDigit(r)
}

func containsCJK(value string) bool {
	for _, r := range value {
		if unicode.Is(unicode.Han, r) {
			return true
		}
	}
	return false
}

func derefString(value *string) string {
	if value == nil {
		return ""
	}
	return *value
}
