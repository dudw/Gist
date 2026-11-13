package handler

import (
	"context"
	"net/http"
	"strconv"
	"time"

	"github.com/labstack/echo/v4"

	"gist/backend/internal/model"
	"gist/backend/internal/service"
	"gist/backend/pkg/logger"
)

type RuleHandler struct {
	service service.RuleService
}

func NewRuleHandler(svc service.RuleService) *RuleHandler {
	return &RuleHandler{service: svc}
}

func (h *RuleHandler) RegisterRoutes(g *echo.Group) {
	g.GET("/rules", h.List)
	g.POST("/rules", h.Create)
	g.PUT("/rules/:id", h.Update)
	g.DELETE("/rules/:id", h.Delete)
	g.PATCH("/rules/:id/enabled", h.SetEnabled)
	g.POST("/rules/:id/run", h.Run)
}

type ruleActionRequest struct {
	Type string `json:"type"`
}

type ruleConditionRequest struct {
	Join     string `json:"join"`
	Field    string `json:"field"`
	Operator string `json:"operator"`
	Value    string `json:"value"`
}

type ruleRequest struct {
	Name       string                 `json:"name"`
	Enabled    *bool                  `json:"enabled"`
	FullWord   bool                   `json:"fullWord"`
	FeedIDs    []string               `json:"feedIds"`
	Action     ruleActionRequest      `json:"action"`
	Conditions []ruleConditionRequest `json:"conditions"`
}

type ruleConditionResponse struct {
	Join     string `json:"join"`
	Field    string `json:"field"`
	Operator string `json:"operator"`
	Value    string `json:"value"`
}

type ruleResponse struct {
	ID         string                  `json:"id"`
	Name       string                  `json:"name"`
	Enabled    bool                    `json:"enabled"`
	FullWord   bool                    `json:"fullWord"`
	FeedIDs    []string                `json:"feedIds"`
	Action     ruleActionRequest       `json:"action"`
	Conditions []ruleConditionResponse `json:"conditions"`
	CreatedAt  string                  `json:"createdAt"`
	UpdatedAt  string                  `json:"updatedAt"`
}

type setRuleEnabledRequest struct {
	Enabled *bool `json:"enabled"`
}

type ruleRunResponse struct {
	Matched int `json:"matched"`
	Updated int `json:"updated"`
}

// List returns all article rules.
// @Summary List article rules
// @Tags rules
// @Produce json
// @Success 200 {array} ruleResponse
// @Router /rules [get]
func (h *RuleHandler) List(c echo.Context) error {
	rules, err := h.service.List(c.Request().Context())
	if err != nil {
		logger.Error("rule list failed", "module", "handler", "action", "list", "resource", "rule", "result", "failed", "error", err)
		return writeServiceError(c, err)
	}
	response := make([]ruleResponse, 0, len(rules))
	for _, rule := range rules {
		response = append(response, toRuleResponse(rule))
	}
	return c.JSON(http.StatusOK, response)
}

// Create creates an article rule.
// @Summary Create an article rule
// @Tags rules
// @Accept json
// @Produce json
// @Param request body ruleRequest true "Rule definition"
// @Success 201 {object} ruleResponse
// @Failure 400 {object} errorResponse
// @Router /rules [post]
func (h *RuleHandler) Create(c echo.Context) error {
	rule, err := parseRuleRequest(c)
	if err != nil {
		return c.JSON(http.StatusBadRequest, errorResponse{Error: "invalid request"})
	}
	created, err := h.service.Create(c.Request().Context(), rule)
	if err != nil {
		logger.Error("rule create failed", "module", "handler", "action", "create", "resource", "rule", "result", "failed", "error", err)
		return writeServiceError(c, err)
	}
	return c.JSON(http.StatusCreated, toRuleResponse(created))
}

// Update updates an article rule.
// @Summary Update an article rule
// @Tags rules
// @Accept json
// @Produce json
// @Param id path int true "Rule ID"
// @Param request body ruleRequest true "Rule definition"
// @Success 200 {object} ruleResponse
// @Failure 400 {object} errorResponse
// @Failure 404 {object} errorResponse
// @Router /rules/{id} [put]
func (h *RuleHandler) Update(c echo.Context) error {
	id, err := parseIDParam(c, "id")
	if err != nil {
		return c.JSON(http.StatusBadRequest, errorResponse{Error: "invalid request"})
	}
	rule, err := parseRuleRequest(c)
	if err != nil {
		return c.JSON(http.StatusBadRequest, errorResponse{Error: "invalid request"})
	}
	rule.ID = id
	updated, err := h.service.Update(c.Request().Context(), rule)
	if err != nil {
		logger.Error("rule update failed", "module", "handler", "action", "update", "resource", "rule", "result", "failed", "rule_id", id, "error", err)
		return writeServiceError(c, err)
	}
	return c.JSON(http.StatusOK, toRuleResponse(updated))
}

// Delete removes an article rule.
// @Summary Delete an article rule
// @Tags rules
// @Param id path int true "Rule ID"
// @Success 204 "No Content"
// @Failure 404 {object} errorResponse
// @Router /rules/{id} [delete]
func (h *RuleHandler) Delete(c echo.Context) error {
	id, err := parseIDParam(c, "id")
	if err != nil {
		return c.JSON(http.StatusBadRequest, errorResponse{Error: "invalid request"})
	}
	if err := h.service.Delete(c.Request().Context(), id); err != nil {
		logger.Error("rule delete failed", "module", "handler", "action", "delete", "resource", "rule", "result", "failed", "rule_id", id, "error", err)
		return writeServiceError(c, err)
	}
	return c.NoContent(http.StatusNoContent)
}

// SetEnabled toggles an article rule.
// @Summary Enable or disable an article rule
// @Tags rules
// @Accept json
// @Param id path int true "Rule ID"
// @Param request body setRuleEnabledRequest true "Enabled state"
// @Success 204 "No Content"
// @Failure 400 {object} errorResponse
// @Failure 404 {object} errorResponse
// @Router /rules/{id}/enabled [patch]
func (h *RuleHandler) SetEnabled(c echo.Context) error {
	id, err := parseIDParam(c, "id")
	if err != nil {
		return c.JSON(http.StatusBadRequest, errorResponse{Error: "invalid request"})
	}
	var req setRuleEnabledRequest
	if err := c.Bind(&req); err != nil || req.Enabled == nil {
		return c.JSON(http.StatusBadRequest, errorResponse{Error: "invalid request"})
	}
	if err := h.service.SetEnabled(c.Request().Context(), id, *req.Enabled); err != nil {
		logger.Error("rule enabled update failed", "module", "handler", "action", "update", "resource", "rule", "result", "failed", "rule_id", id, "error", err)
		return writeServiceError(c, err)
	}
	return c.NoContent(http.StatusNoContent)
}

// Run applies a rule to existing unread entries.
// @Summary Run an article rule
// @Tags rules
// @Produce json
// @Param id path int true "Rule ID"
// @Success 200 {object} ruleRunResponse
// @Failure 404 {object} errorResponse
// @Router /rules/{id}/run [post]
func (h *RuleHandler) Run(c echo.Context) error {
	id, err := parseIDParam(c, "id")
	if err != nil {
		return c.JSON(http.StatusBadRequest, errorResponse{Error: "invalid request"})
	}
	ctx, cancel := context.WithTimeout(c.Request().Context(), 60*time.Second)
	defer cancel()

	result, err := h.service.Run(ctx, id)
	if err != nil {
		logger.Error("rule run failed", "module", "handler", "action", "run", "resource", "rule", "result", "failed", "rule_id", id, "error", err)
		return writeServiceError(c, err)
	}
	return c.JSON(http.StatusOK, ruleRunResponse{Matched: result.Matched, Updated: result.Updated})
}

func parseRuleRequest(c echo.Context) (model.ArticleRule, error) {
	var req ruleRequest
	if err := c.Bind(&req); err != nil {
		return model.ArticleRule{}, err
	}

	rule := model.ArticleRule{
		Name:       req.Name,
		Enabled:    true,
		FullWord:   req.FullWord,
		ActionType: req.Action.Type,
	}
	if req.Enabled != nil {
		rule.Enabled = *req.Enabled
	}
	if rule.ActionType == "" {
		rule.ActionType = model.RuleActionMarkRead
	}

	for _, rawID := range req.FeedIDs {
		id, err := strconv.ParseInt(rawID, 10, 64)
		if err != nil {
			return model.ArticleRule{}, err
		}
		rule.FeedIDs = append(rule.FeedIDs, id)
	}
	for _, condition := range req.Conditions {
		rule.Conditions = append(rule.Conditions, model.RuleCondition{
			JoinType: condition.Join,
			Field:    condition.Field,
			Operator: condition.Operator,
			Value:    condition.Value,
		})
	}
	return rule, nil
}

func toRuleResponse(rule model.ArticleRule) ruleResponse {
	feedIDs := make([]string, 0, len(rule.FeedIDs))
	for _, feedID := range rule.FeedIDs {
		feedIDs = append(feedIDs, idToString(feedID))
	}
	conditions := make([]ruleConditionResponse, 0, len(rule.Conditions))
	for _, condition := range rule.Conditions {
		conditions = append(conditions, ruleConditionResponse{
			Join:     condition.JoinType,
			Field:    condition.Field,
			Operator: condition.Operator,
			Value:    condition.Value,
		})
	}
	return ruleResponse{
		ID:         idToString(rule.ID),
		Name:       rule.Name,
		Enabled:    rule.Enabled,
		FullWord:   rule.FullWord,
		FeedIDs:    feedIDs,
		Action:     ruleActionRequest{Type: rule.ActionType},
		Conditions: conditions,
		CreatedAt:  rule.CreatedAt.UTC().Format(time.RFC3339),
		UpdatedAt:  rule.UpdatedAt.UTC().Format(time.RFC3339),
	}
}
