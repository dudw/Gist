package service_test

import (
	"context"
	"database/sql"
	"testing"

	"github.com/stretchr/testify/require"
	"go.uber.org/mock/gomock"

	"gist/backend/internal/model"
	"gist/backend/internal/repository/mock"
	"gist/backend/internal/service"
)

func testRule() model.ArticleRule {
	return model.ArticleRule{
		Name:       "AI rule",
		Enabled:    true,
		FullWord:   true,
		ActionType: model.RuleActionMarkRead,
		FeedIDs:    []int64{1, 2},
		Conditions: []model.RuleCondition{
			{JoinType: model.RuleJoinOr, Field: model.RuleFieldTitle, Operator: model.RuleOperatorContains, Value: "AI"},
		},
	}
}

func TestRuleService_Create_Success(t *testing.T) {
	ctrl := gomock.NewController(t)
	defer ctrl.Finish()

	ruleRepo := mock.NewMockArticleRuleRepository(ctrl)
	feedRepo := mock.NewMockFeedRepository(ctrl)
	entryRepo := mock.NewMockEntryRepository(ctrl)
	svc := service.NewRuleService(ruleRepo, feedRepo, entryRepo)

	feedRepo.EXPECT().GetByIDs(gomock.Any(), []int64{1, 2}).Return([]model.Feed{{ID: 1}, {ID: 2}}, nil)
	ruleRepo.EXPECT().Create(gomock.Any(), gomock.Any()).DoAndReturn(
		func(_ context.Context, rule model.ArticleRule) (model.ArticleRule, error) {
			require.Equal(t, "AI rule", rule.Name)
			require.Equal(t, model.RuleJoinAnd, rule.Conditions[0].JoinType)
			rule.ID = 10
			return rule, nil
		},
	)

	created, err := svc.Create(context.Background(), testRule())
	require.NoError(t, err)
	require.Equal(t, int64(10), created.ID)
}

func TestRuleService_Create_InvalidRegex(t *testing.T) {
	ctrl := gomock.NewController(t)
	defer ctrl.Finish()

	ruleRepo := mock.NewMockArticleRuleRepository(ctrl)
	feedRepo := mock.NewMockFeedRepository(ctrl)
	entryRepo := mock.NewMockEntryRepository(ctrl)
	svc := service.NewRuleService(ruleRepo, feedRepo, entryRepo)

	feedRepo.EXPECT().GetByIDs(gomock.Any(), []int64{1, 2}).Return([]model.Feed{{ID: 1}, {ID: 2}}, nil)
	rule := testRule()
	rule.Conditions[0].Operator = model.RuleOperatorRegex
	rule.Conditions[0].Value = "("

	_, err := svc.Create(context.Background(), rule)
	require.ErrorIs(t, err, service.ErrInvalid)
}

func TestRuleService_Create_UnknownFeed(t *testing.T) {
	ctrl := gomock.NewController(t)
	defer ctrl.Finish()

	ruleRepo := mock.NewMockArticleRuleRepository(ctrl)
	feedRepo := mock.NewMockFeedRepository(ctrl)
	entryRepo := mock.NewMockEntryRepository(ctrl)
	svc := service.NewRuleService(ruleRepo, feedRepo, entryRepo)

	feedRepo.EXPECT().GetByIDs(gomock.Any(), []int64{1, 2}).Return([]model.Feed{{ID: 1}}, nil)

	_, err := svc.Create(context.Background(), testRule())
	require.ErrorIs(t, err, service.ErrInvalid)
}

func TestRuleService_Update_NotFound(t *testing.T) {
	ctrl := gomock.NewController(t)
	defer ctrl.Finish()

	ruleRepo := mock.NewMockArticleRuleRepository(ctrl)
	feedRepo := mock.NewMockFeedRepository(ctrl)
	entryRepo := mock.NewMockEntryRepository(ctrl)
	svc := service.NewRuleService(ruleRepo, feedRepo, entryRepo)

	feedRepo.EXPECT().GetByIDs(gomock.Any(), []int64{1, 2}).Return([]model.Feed{{ID: 1}, {ID: 2}}, nil)
	ruleRepo.EXPECT().Update(gomock.Any(), gomock.Any()).Return(sql.ErrNoRows)

	rule := testRule()
	rule.ID = 99
	_, err := svc.Update(context.Background(), rule)
	require.ErrorIs(t, err, service.ErrNotFound)
}

func TestRuleService_SetEnabled_NotFound(t *testing.T) {
	ctrl := gomock.NewController(t)
	defer ctrl.Finish()

	ruleRepo := mock.NewMockArticleRuleRepository(ctrl)
	svc := service.NewRuleService(ruleRepo, mock.NewMockFeedRepository(ctrl), mock.NewMockEntryRepository(ctrl))

	ruleRepo.EXPECT().SetEnabled(gomock.Any(), int64(99), false).Return(sql.ErrNoRows)
	require.ErrorIs(t, svc.SetEnabled(context.Background(), 99, false), service.ErrNotFound)
}

func TestRuleService_Run_AppliesToMatchingUnreadEntries(t *testing.T) {
	ctrl := gomock.NewController(t)
	defer ctrl.Finish()

	ruleRepo := mock.NewMockArticleRuleRepository(ctrl)
	feedRepo := mock.NewMockFeedRepository(ctrl)
	entryRepo := mock.NewMockEntryRepository(ctrl)
	svc := service.NewRuleService(ruleRepo, feedRepo, entryRepo)

	ruleRepo.EXPECT().GetByID(gomock.Any(), int64(10)).Return(model.ArticleRule{
		ID:         10,
		Enabled:    true,
		ActionType: model.RuleActionMarkRead,
		FeedIDs:    []int64{1},
		Conditions: []model.RuleCondition{
			{JoinType: model.RuleJoinAnd, Field: model.RuleFieldTitle, Operator: model.RuleOperatorContains, Value: "AI"},
		},
	}, nil)
	entryRepo.EXPECT().ListUnreadForRule(gomock.Any(), []int64{1}, int64(0), 500).Return([]model.Entry{
		{ID: 1, Title: strPtr("AI news")},
		{ID: 2, Title: strPtr("Sports news")},
	}, nil)
	entryRepo.EXPECT().UpdateManyReadStatus(gomock.Any(), []int64{1}, true).Return(nil)

	result, err := svc.Run(context.Background(), 10)
	require.NoError(t, err)
	require.Equal(t, 1, result.Matched)
	require.Equal(t, 1, result.Updated)
}

func TestRuleService_Run_NotFound(t *testing.T) {
	ctrl := gomock.NewController(t)
	defer ctrl.Finish()

	ruleRepo := mock.NewMockArticleRuleRepository(ctrl)
	svc := service.NewRuleService(ruleRepo, mock.NewMockFeedRepository(ctrl), mock.NewMockEntryRepository(ctrl))

	ruleRepo.EXPECT().GetByID(gomock.Any(), int64(99)).Return(model.ArticleRule{}, sql.ErrNoRows)
	_, err := svc.Run(context.Background(), 99)
	require.ErrorIs(t, err, service.ErrNotFound)
}
