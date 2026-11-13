package service_test

import (
	"testing"

	"gist/backend/internal/model"
	"gist/backend/internal/service"

	"github.com/stretchr/testify/require"
)

func makeRuleEntry() model.Entry {
	return model.Entry{
		Title:   strPtr("The AI Revolution in 2026"),
		Content: strPtr("<p>Hello <strong>world</strong>, this is about AI.</p>"),
		Author:  strPtr("Alice"),
		URL:     strPtr("https://example.com/posts/ai-2026?ref=feed"),
	}
}

func condition(field, operator, value string) model.RuleCondition {
	return model.RuleCondition{
		JoinType: model.RuleJoinAnd,
		Field:    field,
		Operator: operator,
		Value:    value,
	}
}

func TestMatchArticleRule_Fields(t *testing.T) {
	rule := model.ArticleRule{
		Conditions: []model.RuleCondition{condition(model.RuleFieldTitleOrContent, model.RuleOperatorContains, "world")},
	}
	require.True(t, service.MatchArticleRule(rule, makeRuleEntry()))

	rule.Conditions[0] = condition(model.RuleFieldContent, model.RuleOperatorContains, "strong")
	require.False(t, service.MatchArticleRule(rule, makeRuleEntry()), "HTML tags must be stripped")

	rule.Conditions[0] = condition(model.RuleFieldAuthor, model.RuleOperatorIs, "alice")
	require.True(t, service.MatchArticleRule(rule, makeRuleEntry()))

	rule.Conditions[0] = condition(model.RuleFieldURL, model.RuleOperatorContains, "example.com")
	require.True(t, service.MatchArticleRule(rule, makeRuleEntry()))

	rule.Conditions[0] = condition(model.RuleFieldURLWithoutHost, model.RuleOperatorIs, "/posts/ai-2026?ref=feed")
	require.True(t, service.MatchArticleRule(rule, makeRuleEntry()))
}

func TestMatchArticleRule_Operators(t *testing.T) {
	tests := []struct {
		name     string
		operator string
		value    string
		want     bool
	}{
		{name: "contains", operator: model.RuleOperatorContains, value: "ai", want: true},
		{name: "not contains", operator: model.RuleOperatorNotContains, value: "blockchain", want: true},
		{name: "is", operator: model.RuleOperatorIs, value: "the ai revolution in 2026", want: true},
		{name: "is not", operator: model.RuleOperatorIsNot, value: "other", want: true},
		{name: "starts with", operator: model.RuleOperatorStartsWith, value: "the ai", want: true},
		{name: "ends with", operator: model.RuleOperatorEndsWith, value: "2026", want: true},
		{name: "regex", operator: model.RuleOperatorRegex, value: `AI\s+Revolution`, want: true},
		{name: "not regex", operator: model.RuleOperatorNotRegex, value: `blockchain`, want: true},
	}

	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			rule := model.ArticleRule{
				Conditions: []model.RuleCondition{condition(model.RuleFieldTitle, test.operator, test.value)},
			}
			require.Equal(t, test.want, service.MatchArticleRule(rule, makeRuleEntry()))
		})
	}
}

func TestMatchArticleRule_FullWord(t *testing.T) {
	entry := model.Entry{Title: strPtr("MAIL and AI")}

	rule := model.ArticleRule{
		FullWord: true,
		Conditions: []model.RuleCondition{
			condition(model.RuleFieldTitle, model.RuleOperatorContains, "AI"),
		},
	}
	require.True(t, service.MatchArticleRule(rule, entry))

	rule.Conditions[0].Value = "A"
	require.False(t, service.MatchArticleRule(rule, entry), "single letter inside MAIL is not a whole word")

	rule.FullWord = false
	require.True(t, service.MatchArticleRule(rule, entry))

	cjkEntry := model.Entry{Title: strPtr("这是人工智能新闻")}
	cjkRule := model.ArticleRule{
		FullWord: true,
		Conditions: []model.RuleCondition{
			condition(model.RuleFieldTitle, model.RuleOperatorContains, "人工智能"),
		},
	}
	require.True(t, service.MatchArticleRule(cjkRule, cjkEntry), "CJK should fall back to substring matching")
}

func TestMatchArticleRule_AndOr(t *testing.T) {
	entry := makeRuleEntry()
	rule := model.ArticleRule{
		Conditions: []model.RuleCondition{
			condition(model.RuleFieldTitle, model.RuleOperatorContains, "AI"),
			{JoinType: model.RuleJoinAnd, Field: model.RuleFieldAuthor, Operator: model.RuleOperatorIs, Value: "Alice"},
		},
	}
	require.True(t, service.MatchArticleRule(rule, entry))

	rule.Conditions[1].Value = "Bob"
	require.False(t, service.MatchArticleRule(rule, entry))

	rule.Conditions[1].JoinType = model.RuleJoinOr
	require.True(t, service.MatchArticleRule(rule, entry))

	// The first row's join type is ignored.
	rule.Conditions[0].JoinType = model.RuleJoinOr
	rule.Conditions[0].Value = "blockchain"
	rule.Conditions[1] = condition(model.RuleFieldAuthor, model.RuleOperatorIs, "Bob")
	require.False(t, service.MatchArticleRule(rule, entry))
}

func TestCompileArticleRule_InvalidRegex(t *testing.T) {
	rule := model.ArticleRule{
		Conditions: []model.RuleCondition{
			condition(model.RuleFieldTitle, model.RuleOperatorRegex, "("),
		},
	}
	_, err := service.CompileArticleRule(rule)
	require.Error(t, err)
}

func TestMatchArticleRule_EmptyConditions(t *testing.T) {
	require.False(t, service.MatchArticleRule(model.ArticleRule{}, makeRuleEntry()))
}
