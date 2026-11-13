package model

import "time"

const (
	RuleActionMarkRead = "mark_read"

	RuleFieldTitleOrContent = "title_or_content"
	RuleFieldTitle          = "title"
	RuleFieldContent        = "content"
	RuleFieldAuthor         = "author"
	RuleFieldURL            = "url"
	RuleFieldURLWithoutHost = "url_no_domain"

	RuleOperatorContains    = "contains"
	RuleOperatorNotContains = "not_contains"
	RuleOperatorIs          = "is"
	RuleOperatorIsNot       = "is_not"
	RuleOperatorStartsWith  = "starts_with"
	RuleOperatorEndsWith    = "ends_with"
	RuleOperatorRegex       = "regex"
	RuleOperatorNotRegex    = "not_regex"

	RuleJoinAnd = "and"
	RuleJoinOr  = "or"
)

// ArticleRule is a user-defined rule that is evaluated against entries.
type ArticleRule struct {
	ID         int64
	Name       string
	Enabled    bool
	FullWord   bool
	ActionType string
	Conditions []RuleCondition
	FeedIDs    []int64
	CreatedAt  time.Time
	UpdatedAt  time.Time
}

// RuleCondition is one condition row inside an ArticleRule.
type RuleCondition struct {
	ID       int64
	RuleID   int64
	Position int
	JoinType string
	Field    string
	Operator string
	Value    string
}
