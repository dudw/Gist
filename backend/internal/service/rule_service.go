//go:generate mockgen -source=$GOFILE -destination=mock/$GOFILE -package=mock
package service

import (
	"context"
	"database/sql"
	"errors"
	"fmt"
	"strings"
	"unicode/utf8"

	"gist/backend/internal/model"
	"gist/backend/internal/repository"
	"gist/backend/pkg/logger"
)

// RuleRunResult reports how many entries matched and were updated.
type RuleRunResult struct {
	Matched int
	Updated int
}

// RuleService manages article rules.
type RuleService interface {
	List(ctx context.Context) ([]model.ArticleRule, error)
	Create(ctx context.Context, rule model.ArticleRule) (model.ArticleRule, error)
	Update(ctx context.Context, rule model.ArticleRule) (model.ArticleRule, error)
	Delete(ctx context.Context, id int64) error
	SetEnabled(ctx context.Context, id int64, enabled bool) error
	Run(ctx context.Context, id int64) (RuleRunResult, error)
	ListEnabledForFeed(ctx context.Context, feedID int64) ([]model.ArticleRule, error)
}

type ruleService struct {
	rules   repository.ArticleRuleRepository
	feeds   repository.FeedRepository
	entries repository.EntryRepository
}

func NewRuleService(
	rules repository.ArticleRuleRepository,
	feeds repository.FeedRepository,
	entries repository.EntryRepository,
) RuleService {
	return &ruleService{rules: rules, feeds: feeds, entries: entries}
}

func (s *ruleService) List(ctx context.Context) ([]model.ArticleRule, error) {
	return s.rules.List(ctx)
}

func (s *ruleService) Create(ctx context.Context, rule model.ArticleRule) (model.ArticleRule, error) {
	prepared, err := s.prepareRule(ctx, rule)
	if err != nil {
		return model.ArticleRule{}, err
	}
	created, err := s.rules.Create(ctx, prepared)
	if err != nil {
		return model.ArticleRule{}, err
	}
	logger.Info("rule created", "module", "service", "action", "create", "resource", "rule", "result", "ok", "rule_id", created.ID)
	return created, nil
}

func (s *ruleService) Update(ctx context.Context, rule model.ArticleRule) (model.ArticleRule, error) {
	prepared, err := s.prepareRule(ctx, rule)
	if err != nil {
		return model.ArticleRule{}, err
	}
	if err := s.rules.Update(ctx, prepared); err != nil {
		if errors.Is(err, sql.ErrNoRows) {
			return model.ArticleRule{}, ErrNotFound
		}
		return model.ArticleRule{}, err
	}
	updated, err := s.rules.GetByID(ctx, prepared.ID)
	if err != nil {
		if errors.Is(err, sql.ErrNoRows) {
			return model.ArticleRule{}, ErrNotFound
		}
		return model.ArticleRule{}, err
	}
	logger.Info("rule updated", "module", "service", "action", "update", "resource", "rule", "result", "ok", "rule_id", updated.ID)
	return updated, nil
}

func (s *ruleService) Delete(ctx context.Context, id int64) error {
	if err := s.rules.Delete(ctx, id); err != nil {
		if errors.Is(err, sql.ErrNoRows) {
			return ErrNotFound
		}
		return err
	}
	logger.Info("rule deleted", "module", "service", "action", "delete", "resource", "rule", "result", "ok", "rule_id", id)
	return nil
}

func (s *ruleService) SetEnabled(ctx context.Context, id int64, enabled bool) error {
	if err := s.rules.SetEnabled(ctx, id, enabled); err != nil {
		if errors.Is(err, sql.ErrNoRows) {
			return ErrNotFound
		}
		return err
	}
	logger.Info("rule enabled changed", "module", "service", "action", "update", "resource", "rule", "result", "ok", "rule_id", id, "enabled", enabled)
	return nil
}

func (s *ruleService) ListEnabledForFeed(ctx context.Context, feedID int64) ([]model.ArticleRule, error) {
	return s.rules.ListEnabledForFeed(ctx, feedID)
}

func (s *ruleService) Run(ctx context.Context, id int64) (RuleRunResult, error) {
	rule, err := s.rules.GetByID(ctx, id)
	if err != nil {
		if errors.Is(err, sql.ErrNoRows) {
			return RuleRunResult{}, ErrNotFound
		}
		return RuleRunResult{}, err
	}
	compiled, err := CompileArticleRule(rule)
	if err != nil {
		return RuleRunResult{}, fmt.Errorf("%w: invalid rule regex", ErrInvalid)
	}

	result := RuleRunResult{}
	var afterID int64
	for {
		entries, err := s.entries.ListUnreadForRule(ctx, rule.FeedIDs, afterID, 500)
		if err != nil {
			return result, err
		}
		if len(entries) == 0 {
			break
		}

		ids := make([]int64, 0, len(entries))
		for _, entry := range entries {
			afterID = entry.ID
			if compiled.Matches(entry) {
				ids = append(ids, entry.ID)
			}
		}
		result.Matched += len(ids)
		if len(ids) > 0 {
			if err := s.entries.UpdateManyReadStatus(ctx, ids, true); err != nil {
				return result, err
			}
			result.Updated += len(ids)
		}
		if len(entries) < 500 {
			break
		}
	}

	logger.Info("rule run completed", "module", "service", "action", "run", "resource", "rule", "result", "ok", "rule_id", id, "matched", result.Matched, "updated", result.Updated)
	return result, nil
}

func (s *ruleService) prepareRule(ctx context.Context, rule model.ArticleRule) (model.ArticleRule, error) {
	rule.Name = strings.TrimSpace(rule.Name)
	if rule.Name == "" || utf8.RuneCountInString(rule.Name) > 100 {
		return model.ArticleRule{}, fmt.Errorf("%w: invalid rule name", ErrInvalid)
	}
	if rule.ActionType == "" {
		rule.ActionType = model.RuleActionMarkRead
	}
	if rule.ActionType != model.RuleActionMarkRead {
		return model.ArticleRule{}, fmt.Errorf("%w: invalid rule action", ErrInvalid)
	}

	feedIDs := uniqueInt64(rule.FeedIDs)
	if len(feedIDs) == 0 {
		return model.ArticleRule{}, fmt.Errorf("%w: at least one feed is required", ErrInvalid)
	}
	feeds, err := s.feeds.GetByIDs(ctx, feedIDs)
	if err != nil {
		return model.ArticleRule{}, err
	}
	if len(feeds) != len(feedIDs) {
		return model.ArticleRule{}, fmt.Errorf("%w: feed not found", ErrInvalid)
	}
	rule.FeedIDs = feedIDs

	if len(rule.Conditions) == 0 || len(rule.Conditions) > 50 {
		return model.ArticleRule{}, fmt.Errorf("%w: invalid condition count", ErrInvalid)
	}
	for i := range rule.Conditions {
		condition := &rule.Conditions[i]
		condition.Field = strings.TrimSpace(condition.Field)
		condition.Operator = strings.TrimSpace(condition.Operator)
		condition.Value = strings.TrimSpace(condition.Value)
		condition.JoinType = strings.TrimSpace(condition.JoinType)

		if i == 0 {
			condition.JoinType = model.RuleJoinAnd
		} else if condition.JoinType != model.RuleJoinAnd && condition.JoinType != model.RuleJoinOr {
			return model.ArticleRule{}, fmt.Errorf("%w: invalid condition join", ErrInvalid)
		}
		if !isValidRuleField(condition.Field) || !isValidRuleOperator(condition.Operator) {
			return model.ArticleRule{}, fmt.Errorf("%w: invalid condition", ErrInvalid)
		}
		if condition.Value == "" || utf8.RuneCountInString(condition.Value) > 1000 {
			return model.ArticleRule{}, fmt.Errorf("%w: invalid condition value", ErrInvalid)
		}
	}

	if _, err := CompileArticleRule(rule); err != nil {
		return model.ArticleRule{}, fmt.Errorf("%w: invalid regular expression", ErrInvalid)
	}
	return rule, nil
}

func uniqueInt64(values []int64) []int64 {
	seen := make(map[int64]struct{}, len(values))
	result := make([]int64, 0, len(values))
	for _, value := range values {
		if value == 0 {
			continue
		}
		if _, ok := seen[value]; ok {
			continue
		}
		seen[value] = struct{}{}
		result = append(result, value)
	}
	return result
}

func isValidRuleField(field string) bool {
	switch field {
	case model.RuleFieldTitleOrContent,
		model.RuleFieldTitle,
		model.RuleFieldContent,
		model.RuleFieldAuthor,
		model.RuleFieldURL,
		model.RuleFieldURLWithoutHost:
		return true
	default:
		return false
	}
}

func isValidRuleOperator(operator string) bool {
	switch operator {
	case model.RuleOperatorContains,
		model.RuleOperatorNotContains,
		model.RuleOperatorIs,
		model.RuleOperatorIsNot,
		model.RuleOperatorStartsWith,
		model.RuleOperatorEndsWith,
		model.RuleOperatorRegex,
		model.RuleOperatorNotRegex:
		return true
	default:
		return false
	}
}
