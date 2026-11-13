//go:generate mockgen -source=$GOFILE -destination=mock/$GOFILE -package=mock
package repository

import (
	"context"
	"database/sql"
	"strings"
	"time"

	"gist/backend/internal/model"
	"gist/backend/pkg/snowflake"
)

// ArticleRuleRepository defines storage operations for article rules.
type ArticleRuleRepository interface {
	List(ctx context.Context) ([]model.ArticleRule, error)
	GetByID(ctx context.Context, id int64) (model.ArticleRule, error)
	Create(ctx context.Context, rule model.ArticleRule) (model.ArticleRule, error)
	Update(ctx context.Context, rule model.ArticleRule) error
	Delete(ctx context.Context, id int64) error
	SetEnabled(ctx context.Context, id int64, enabled bool) error
	ListEnabledForFeed(ctx context.Context, feedID int64) ([]model.ArticleRule, error)
}

type articleRuleRepository struct {
	db *sql.DB
}

func NewArticleRuleRepository(db *sql.DB) ArticleRuleRepository {
	return &articleRuleRepository{db: db}
}

func (r *articleRuleRepository) List(ctx context.Context) ([]model.ArticleRule, error) {
	rows, err := r.db.QueryContext(ctx, `
		SELECT id, name, enabled, full_word, action_type, created_at, updated_at
		FROM article_rules
		ORDER BY created_at DESC, id DESC
	`)
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	rules, err := scanArticleRules(rows)
	if err != nil {
		return nil, err
	}
	if err := r.loadRelations(ctx, rules); err != nil {
		return nil, err
	}
	return rules, nil
}

func (r *articleRuleRepository) GetByID(ctx context.Context, id int64) (model.ArticleRule, error) {
	row := r.db.QueryRowContext(ctx, `
		SELECT id, name, enabled, full_word, action_type, created_at, updated_at
		FROM article_rules
		WHERE id = ?
	`, id)

	rule, err := scanArticleRule(row)
	if err != nil {
		return model.ArticleRule{}, err
	}

	rules := []model.ArticleRule{rule}
	if err := r.loadRelations(ctx, rules); err != nil {
		return model.ArticleRule{}, err
	}
	return rules[0], nil
}

func (r *articleRuleRepository) Create(ctx context.Context, rule model.ArticleRule) (model.ArticleRule, error) {
	rule.ID = snowflake.NextID()
	now := time.Now().UTC()
	rule.CreatedAt = now
	rule.UpdatedAt = now
	for i := range rule.Conditions {
		rule.Conditions[i].RuleID = rule.ID
		rule.Conditions[i].ID = snowflake.NextID()
		rule.Conditions[i].Position = i
	}

	tx, err := r.db.BeginTx(ctx, nil)
	if err != nil {
		return model.ArticleRule{}, err
	}
	defer tx.Rollback()

	if _, err := tx.ExecContext(ctx, `
		INSERT INTO article_rules (id, name, enabled, full_word, action_type, created_at, updated_at)
		VALUES (?, ?, ?, ?, ?, ?, ?)
	`, rule.ID, rule.Name, boolToInt(rule.Enabled), boolToInt(rule.FullWord), rule.ActionType, formatTime(now), formatTime(now)); err != nil {
		return model.ArticleRule{}, err
	}

	if err := insertRuleRelations(ctx, tx, rule); err != nil {
		return model.ArticleRule{}, err
	}

	if err := tx.Commit(); err != nil {
		return model.ArticleRule{}, err
	}
	return rule, nil
}

func (r *articleRuleRepository) Update(ctx context.Context, rule model.ArticleRule) error {
	tx, err := r.db.BeginTx(ctx, nil)
	if err != nil {
		return err
	}
	defer tx.Rollback()

	result, err := tx.ExecContext(ctx, `
		UPDATE article_rules
		SET name = ?, enabled = ?, full_word = ?, action_type = ?, updated_at = ?
		WHERE id = ?
	`, rule.Name, boolToInt(rule.Enabled), boolToInt(rule.FullWord), rule.ActionType, formatTime(time.Now()), rule.ID)
	if err != nil {
		return err
	}
	affected, err := result.RowsAffected()
	if err != nil {
		return err
	}
	if affected == 0 {
		return sql.ErrNoRows
	}

	if _, err := tx.ExecContext(ctx, `DELETE FROM article_rule_conditions WHERE rule_id = ?`, rule.ID); err != nil {
		return err
	}
	if _, err := tx.ExecContext(ctx, `DELETE FROM article_rule_feeds WHERE rule_id = ?`, rule.ID); err != nil {
		return err
	}

	for i := range rule.Conditions {
		rule.Conditions[i].RuleID = rule.ID
		rule.Conditions[i].ID = snowflake.NextID()
		rule.Conditions[i].Position = i
	}
	if err := insertRuleRelations(ctx, tx, rule); err != nil {
		return err
	}

	return tx.Commit()
}

func (r *articleRuleRepository) Delete(ctx context.Context, id int64) error {
	result, err := r.db.ExecContext(ctx, `DELETE FROM article_rules WHERE id = ?`, id)
	if err != nil {
		return err
	}
	affected, err := result.RowsAffected()
	if err != nil {
		return err
	}
	if affected == 0 {
		return sql.ErrNoRows
	}
	return nil
}

func (r *articleRuleRepository) SetEnabled(ctx context.Context, id int64, enabled bool) error {
	result, err := r.db.ExecContext(ctx, `
		UPDATE article_rules SET enabled = ?, updated_at = ? WHERE id = ?
	`, boolToInt(enabled), formatTime(time.Now()), id)
	if err != nil {
		return err
	}
	affected, err := result.RowsAffected()
	if err != nil {
		return err
	}
	if affected == 0 {
		return sql.ErrNoRows
	}
	return nil
}

func (r *articleRuleRepository) ListEnabledForFeed(ctx context.Context, feedID int64) ([]model.ArticleRule, error) {
	rows, err := r.db.QueryContext(ctx, `
		SELECT r.id, r.name, r.enabled, r.full_word, r.action_type, r.created_at, r.updated_at
		FROM article_rules r
		INNER JOIN article_rule_feeds rf ON rf.rule_id = r.id
		WHERE r.enabled = 1 AND rf.feed_id = ?
		ORDER BY r.id
	`, feedID)
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	rules, err := scanArticleRules(rows)
	if err != nil {
		return nil, err
	}
	if err := r.loadRelations(ctx, rules); err != nil {
		return nil, err
	}
	return rules, nil
}

func (r *articleRuleRepository) loadRelations(ctx context.Context, rules []model.ArticleRule) error {
	if len(rules) == 0 {
		return nil
	}

	indexByID := make(map[int64]int, len(rules))
	args := make([]interface{}, len(rules))
	placeholders := make([]string, len(rules))
	for i, rule := range rules {
		indexByID[rule.ID] = i
		args[i] = rule.ID
		placeholders[i] = "?"
	}
	inClause := strings.Join(placeholders, ",")

	conditionRows, err := r.db.QueryContext(ctx, `
		SELECT id, rule_id, position, join_type, field, operator, value
		FROM article_rule_conditions
		WHERE rule_id IN (`+inClause+`)
		ORDER BY rule_id, position, id
	`, args...)
	if err != nil {
		return err
	}
	defer conditionRows.Close()

	for conditionRows.Next() {
		var condition model.RuleCondition
		if err := conditionRows.Scan(
			&condition.ID,
			&condition.RuleID,
			&condition.Position,
			&condition.JoinType,
			&condition.Field,
			&condition.Operator,
			&condition.Value,
		); err != nil {
			return err
		}
		if index, ok := indexByID[condition.RuleID]; ok {
			rules[index].Conditions = append(rules[index].Conditions, condition)
		}
	}
	if err := conditionRows.Err(); err != nil {
		return err
	}

	feedRows, err := r.db.QueryContext(ctx, `
		SELECT rule_id, feed_id
		FROM article_rule_feeds
		WHERE rule_id IN (`+inClause+`)
		ORDER BY rule_id, feed_id
	`, args...)
	if err != nil {
		return err
	}
	defer feedRows.Close()

	for feedRows.Next() {
		var ruleID, feedID int64
		if err := feedRows.Scan(&ruleID, &feedID); err != nil {
			return err
		}
		if index, ok := indexByID[ruleID]; ok {
			rules[index].FeedIDs = append(rules[index].FeedIDs, feedID)
		}
	}
	return feedRows.Err()
}

func insertRuleRelations(ctx context.Context, tx *sql.Tx, rule model.ArticleRule) error {
	for i, condition := range rule.Conditions {
		position := condition.Position
		if position == 0 && i > 0 {
			position = i
		}
		if _, err := tx.ExecContext(ctx, `
			INSERT INTO article_rule_conditions (id, rule_id, position, join_type, field, operator, value)
			VALUES (?, ?, ?, ?, ?, ?, ?)
		`, condition.ID, rule.ID, position, condition.JoinType, condition.Field, condition.Operator, condition.Value); err != nil {
			return err
		}
	}

	for _, feedID := range rule.FeedIDs {
		if _, err := tx.ExecContext(ctx, `
			INSERT INTO article_rule_feeds (rule_id, feed_id) VALUES (?, ?)
		`, rule.ID, feedID); err != nil {
			return err
		}
	}
	return nil
}

func scanArticleRules(rows *sql.Rows) ([]model.ArticleRule, error) {
	rules := make([]model.ArticleRule, 0)
	for rows.Next() {
		rule, err := scanArticleRule(rows)
		if err != nil {
			return nil, err
		}
		rules = append(rules, rule)
	}
	if err := rows.Err(); err != nil {
		return nil, err
	}
	return rules, nil
}

type articleRuleScanner interface {
	Scan(dest ...interface{}) error
}

func scanArticleRule(scanner articleRuleScanner) (model.ArticleRule, error) {
	var rule model.ArticleRule
	var enabled, fullWord int
	var createdAt, updatedAt string
	if err := scanner.Scan(
		&rule.ID,
		&rule.Name,
		&enabled,
		&fullWord,
		&rule.ActionType,
		&createdAt,
		&updatedAt,
	); err != nil {
		return model.ArticleRule{}, err
	}
	rule.Enabled = enabled != 0
	rule.FullWord = fullWord != 0
	rule.CreatedAt, _ = parseTime(createdAt)
	rule.UpdatedAt, _ = parseTime(updatedAt)
	return rule, nil
}
