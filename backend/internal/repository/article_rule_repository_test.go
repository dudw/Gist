package repository_test

import (
	"context"
	"database/sql"
	"testing"

	"gist/backend/internal/model"
	"gist/backend/internal/repository"
	"gist/backend/internal/repository/testutil"

	"github.com/stretchr/testify/require"
)

func buildRule(feedIDs []int64) model.ArticleRule {
	return model.ArticleRule{
		Name:       "AI rule",
		Enabled:    true,
		FullWord:   true,
		ActionType: model.RuleActionMarkRead,
		FeedIDs:    feedIDs,
		Conditions: []model.RuleCondition{
			{
				JoinType: model.RuleJoinAnd,
				Field:    model.RuleFieldTitle,
				Operator: model.RuleOperatorContains,
				Value:    "AI",
			},
			{
				JoinType: model.RuleJoinOr,
				Field:    model.RuleFieldAuthor,
				Operator: model.RuleOperatorIs,
				Value:    "Alice",
			},
		},
	}
}

func TestArticleRuleRepository_CRUD(t *testing.T) {
	db := testutil.NewTestDB(t)
	repo := repository.NewArticleRuleRepository(db)
	ctx := context.Background()

	feedOne := testutil.SeedFeed(t, db, model.Feed{Title: "One", URL: "https://one.example.com/feed"})
	feedTwo := testutil.SeedFeed(t, db, model.Feed{Title: "Two", URL: "https://two.example.com/feed"})

	created, err := repo.Create(ctx, buildRule([]int64{feedOne, feedTwo}))
	require.NoError(t, err)
	require.NotZero(t, created.ID)
	require.Len(t, created.Conditions, 2)
	require.NotZero(t, created.Conditions[0].ID)

	fetched, err := repo.GetByID(ctx, created.ID)
	require.NoError(t, err)
	require.Equal(t, "AI rule", fetched.Name)
	require.True(t, fetched.Enabled)
	require.True(t, fetched.FullWord)
	require.Equal(t, model.RuleActionMarkRead, fetched.ActionType)
	require.ElementsMatch(t, []int64{feedOne, feedTwo}, fetched.FeedIDs)
	require.Len(t, fetched.Conditions, 2)
	require.Equal(t, 0, fetched.Conditions[0].Position)
	require.Equal(t, 1, fetched.Conditions[1].Position)
	require.Equal(t, model.RuleJoinOr, fetched.Conditions[1].JoinType)

	enabled, err := repo.ListEnabledForFeed(ctx, feedOne)
	require.NoError(t, err)
	require.Len(t, enabled, 1)
	require.Equal(t, created.ID, enabled[0].ID)

	require.NoError(t, repo.SetEnabled(ctx, created.ID, false))
	enabled, err = repo.ListEnabledForFeed(ctx, feedOne)
	require.NoError(t, err)
	require.Empty(t, enabled)

	updated := fetched
	updated.Name = "Updated rule"
	updated.Enabled = true
	updated.FeedIDs = []int64{feedTwo}
	updated.Conditions = []model.RuleCondition{
		{
			JoinType: model.RuleJoinAnd,
			Field:    model.RuleFieldContent,
			Operator: model.RuleOperatorRegex,
			Value:    `AI\s+news`,
		},
	}
	require.NoError(t, repo.Update(ctx, updated))

	afterUpdate, err := repo.GetByID(ctx, created.ID)
	require.NoError(t, err)
	require.Equal(t, "Updated rule", afterUpdate.Name)
	require.Equal(t, []int64{feedTwo}, afterUpdate.FeedIDs)
	require.Len(t, afterUpdate.Conditions, 1)
	require.Equal(t, model.RuleOperatorRegex, afterUpdate.Conditions[0].Operator)

	list, err := repo.List(ctx)
	require.NoError(t, err)
	require.Len(t, list, 1)

	require.NoError(t, repo.Delete(ctx, created.ID))
	_, err = repo.GetByID(ctx, created.ID)
	require.ErrorIs(t, err, sql.ErrNoRows)
}

func TestArticleRuleRepository_FeedDeletionCascade(t *testing.T) {
	db := testutil.NewTestDB(t)
	repo := repository.NewArticleRuleRepository(db)
	ctx := context.Background()

	feedID := testutil.SeedFeed(t, db, model.Feed{Title: "One", URL: "https://one.example.com/feed"})
	rule, err := repo.Create(ctx, buildRule([]int64{feedID}))
	require.NoError(t, err)

	_, err = db.ExecContext(ctx, `DELETE FROM feeds WHERE id = ?`, feedID)
	require.NoError(t, err)

	fetched, err := repo.GetByID(ctx, rule.ID)
	require.NoError(t, err)
	require.Empty(t, fetched.FeedIDs)
}

func TestEntryRepository_ListUnreadForRule(t *testing.T) {
	db := testutil.NewTestDB(t)
	repo := repository.NewEntryRepository(db)
	ctx := context.Background()

	feedOne := testutil.SeedFeed(t, db, model.Feed{Title: "One", URL: "https://one.example.com/feed"})
	feedTwo := testutil.SeedFeed(t, db, model.Feed{Title: "Two", URL: "https://two.example.com/feed"})

	testutil.SeedEntry(t, db, model.Entry{FeedID: feedOne, Title: strPtrForRule("unread one")})
	testutil.SeedEntry(t, db, model.Entry{FeedID: feedOne, Title: strPtrForRule("read one"), Read: true})
	testutil.SeedEntry(t, db, model.Entry{FeedID: feedTwo, Title: strPtrForRule("unread two")})

	entries, err := repo.ListUnreadForRule(ctx, []int64{feedOne, feedTwo}, 0, 100)
	require.NoError(t, err)
	require.Len(t, entries, 2)
	for _, entry := range entries {
		require.False(t, entry.Read)
	}

	afterID := entries[0].ID
	page, err := repo.ListUnreadForRule(ctx, []int64{feedOne, feedTwo}, afterID, 100)
	require.NoError(t, err)
	require.Len(t, page, 1)
	require.Greater(t, page[0].ID, afterID)
}

func strPtrForRule(value string) *string {
	return &value
}
