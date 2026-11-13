package handler_test

import (
	"context"
	"net/http"
	"testing"
	"time"

	"github.com/stretchr/testify/require"
	"go.uber.org/mock/gomock"

	"gist/backend/internal/handler"
	"gist/backend/internal/model"
	"gist/backend/internal/service"
	"gist/backend/internal/service/mock"
)

func TestRuleHandler_Create_Success(t *testing.T) {
	ctrl := gomock.NewController(t)
	defer ctrl.Finish()

	ruleService := mock.NewMockRuleService(ctrl)
	h := handler.NewRuleHandler(ruleService)
	now := time.Now().UTC()

	ruleService.EXPECT().
		Create(gomock.Any(), gomock.Any()).
		DoAndReturn(func(_ context.Context, rule model.ArticleRule) (model.ArticleRule, error) {
			require.Equal(t, "AI rule", rule.Name)
			require.True(t, rule.Enabled)
			require.True(t, rule.FullWord)
			require.Equal(t, []int64{1, 2}, rule.FeedIDs)
			require.Len(t, rule.Conditions, 1)
			require.Equal(t, model.RuleFieldTitle, rule.Conditions[0].Field)
			rule.ID = 123
			rule.CreatedAt = now
			rule.UpdatedAt = now
			return rule, nil
		})

	e := newTestEcho()
	req := newJSONRequest(http.MethodPost, "/rules", map[string]interface{}{
		"name":     "AI rule",
		"enabled":  true,
		"fullWord": true,
		"feedIds":  []string{"1", "2"},
		"action":   map[string]string{"type": "mark_read"},
		"conditions": []map[string]string{
			{"join": "and", "field": "title", "operator": "contains", "value": "AI"},
		},
	})
	c, rec := newTestContext(e, req)

	require.NoError(t, h.Create(c))
	require.Equal(t, http.StatusCreated, rec.Code)

	var response handler.RuleResponse
	parseJSONResponse(t, rec, &response)
	require.Equal(t, "123", response.ID)
	require.Equal(t, "AI rule", response.Name)
	require.Equal(t, []string{"1", "2"}, response.FeedIDs)
	require.Len(t, response.Conditions, 1)
}

func TestRuleHandler_List_Success(t *testing.T) {
	ctrl := gomock.NewController(t)
	defer ctrl.Finish()

	ruleService := mock.NewMockRuleService(ctrl)
	h := handler.NewRuleHandler(ruleService)
	ruleService.EXPECT().List(gomock.Any()).Return([]model.ArticleRule{
		{ID: 1, Name: "One", Enabled: true, ActionType: model.RuleActionMarkRead},
	}, nil)

	e := newTestEcho()
	req := newJSONRequest(http.MethodGet, "/rules", nil)
	c, rec := newTestContext(e, req)

	require.NoError(t, h.List(c))
	require.Equal(t, http.StatusOK, rec.Code)

	var response []handler.RuleResponse
	parseJSONResponse(t, rec, &response)
	require.Len(t, response, 1)
	require.Equal(t, "1", response[0].ID)
}

func TestRuleHandler_Update_InvalidID(t *testing.T) {
	ctrl := gomock.NewController(t)
	defer ctrl.Finish()

	h := handler.NewRuleHandler(mock.NewMockRuleService(ctrl))
	e := newTestEcho()
	req := newJSONRequest(http.MethodPut, "/rules/bad", map[string]interface{}{})
	c, rec := newTestContext(e, req)
	setPathParams(c, map[string]string{"id": "bad"})

	require.NoError(t, h.Update(c))
	require.Equal(t, http.StatusBadRequest, rec.Code)
}

func TestRuleHandler_SetEnabled_Success(t *testing.T) {
	ctrl := gomock.NewController(t)
	defer ctrl.Finish()

	ruleService := mock.NewMockRuleService(ctrl)
	h := handler.NewRuleHandler(ruleService)
	ruleService.EXPECT().SetEnabled(gomock.Any(), int64(7), false).Return(nil)

	e := newTestEcho()
	req := newJSONRequest(http.MethodPatch, "/rules/7/enabled", map[string]bool{"enabled": false})
	c, rec := newTestContext(e, req)
	setPathParams(c, map[string]string{"id": "7"})

	require.NoError(t, h.SetEnabled(c))
	require.Equal(t, http.StatusNoContent, rec.Code)
}

func TestRuleHandler_Run_Success(t *testing.T) {
	ctrl := gomock.NewController(t)
	defer ctrl.Finish()

	ruleService := mock.NewMockRuleService(ctrl)
	h := handler.NewRuleHandler(ruleService)
	ruleService.EXPECT().Run(gomock.Any(), int64(9)).Return(service.RuleRunResult{Matched: 3, Updated: 3}, nil)

	e := newTestEcho()
	req := newJSONRequest(http.MethodPost, "/rules/9/run", nil)
	c, rec := newTestContext(e, req)
	setPathParams(c, map[string]string{"id": "9"})

	require.NoError(t, h.Run(c))
	require.Equal(t, http.StatusOK, rec.Code)

	var response handler.RuleRunResponse
	parseJSONResponse(t, rec, &response)
	require.Equal(t, 3, response.Matched)
	require.Equal(t, 3, response.Updated)
}
