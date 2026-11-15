//go:generate mockgen -source=$GOFILE -destination=mock/$GOFILE -package=mock
package service

import (
	"bytes"
	"context"
	"errors"
	"fmt"
	"io"
	"net/http"
	"strings"
	"sync"
	"time"

	"github.com/mmcdole/gofeed"
	"golang.org/x/sync/semaphore"

	"gist/backend/internal/config"
	"gist/backend/internal/model"
	"gist/backend/internal/repository"
	"gist/backend/pkg/logger"
	"gist/backend/pkg/network"
)

const refreshTimeout = 30 * time.Second

const (
	// maxConcurrentRefresh limits parallel feed refreshes to avoid overwhelming
	// the network and remote servers.
	maxConcurrentRefresh = 8
	// maxConcurrentPerHost limits parallel requests to the same host to be polite.
	maxConcurrentPerHost = 1
)

// hostRateLimiter manages per-host concurrency and rate limits.
type hostRateLimiter struct {
	mu          sync.Mutex
	semaphores  map[string]*semaphore.Weighted
	lastRequest map[string]time.Time
	getInterval func(host string) time.Duration
}

func newHostRateLimiter(getInterval func(host string) time.Duration) *hostRateLimiter {
	return &hostRateLimiter{
		semaphores:  make(map[string]*semaphore.Weighted),
		lastRequest: make(map[string]time.Time),
		getInterval: getInterval,
	}
}

// acquireSemaphore acquires the per-host semaphore to ensure serial execution for the same host.
// This does NOT occupy global concurrency slots, allowing different hosts to queue in parallel.
func (h *hostRateLimiter) acquireSemaphore(ctx context.Context, host string) error {
	h.mu.Lock()
	sem, ok := h.semaphores[host]
	if !ok {
		sem = semaphore.NewWeighted(maxConcurrentPerHost)
		h.semaphores[host] = sem
	}
	h.mu.Unlock()

	return sem.Acquire(ctx, 1)
}

// releaseSemaphore releases the per-host semaphore.
func (h *hostRateLimiter) releaseSemaphore(host string) {
	h.mu.Lock()
	if sem, ok := h.semaphores[host]; ok {
		sem.Release(1)
	}
	h.mu.Unlock()
}

// waitForInterval waits until the configured interval has passed since the last request.
// This should be called AFTER acquiring the per-host semaphore to ensure serial waiting.
func (h *hostRateLimiter) waitForInterval(ctx context.Context, host string) error {
	interval := h.getInterval(host)
	if interval <= 0 {
		return nil
	}

	h.mu.Lock()
	lastReq, exists := h.lastRequest[host]
	h.mu.Unlock()

	if exists {
		elapsed := time.Since(lastReq)
		if elapsed < interval {
			waitTime := interval - elapsed
			select {
			case <-ctx.Done():
				return ctx.Err()
			case <-time.After(waitTime):
			}
		}
	}
	return nil
}

// recordRequest records the current time as the last request time for the host.
func (h *hostRateLimiter) recordRequest(host string) {
	h.mu.Lock()
	h.lastRequest[host] = time.Now()
	h.mu.Unlock()
}

// processParsedFeed handles the common logic after successfully parsing a feed.
// It clears error messages, updates ETag/LastModified, saves entries, and fetches icons.
func (s *refreshService) processParsedFeed(ctx context.Context, feed model.Feed, parsed *gofeed.Feed, resp *http.Response) error {
	// Clear error message on successful refresh
	feed.ErrorMessage = nil
	_ = s.feeds.UpdateErrorMessage(ctx, feed.ID, nil)

	// Update feed ETag and LastModified (only update non-empty values)
	newETag := strings.TrimSpace(resp.Header.Get("ETag"))
	newLastModified := strings.TrimSpace(resp.Header.Get("Last-Modified"))
	if newETag != "" || newLastModified != "" {
		if newETag != "" {
			feed.ETag = &newETag
		}
		if newLastModified != "" {
			feed.LastModified = &newLastModified
		}
		if _, err := s.feeds.Update(ctx, feed); err != nil {
			logger.Warn("update feed etag failed", "module", "service", "action", "update", "resource", "feed", "result", "failed", "feed_id", feed.ID, "feed_title", feed.Title, "error", err)
		}
	}

	// Save entries
	newCount, updatedCount, duplicateCount := s.saveEntries(ctx, feed.ID, parsed.Items)
	if newCount > 0 || updatedCount > 0 || duplicateCount > 0 {
		logger.Info("feed refreshed", "module", "service", "action", "refresh", "resource", "feed", "result", "ok", "feed_id", feed.ID, "feed_title", feed.Title, "new", newCount, "updated", updatedCount, "duplicate", duplicateCount)
	}

	// Backfill siteURL if empty (for feeds added before siteURL was implemented)
	if (feed.SiteURL == nil || *feed.SiteURL == "") && parsed.Link != "" {
		newSiteURL := strings.TrimSpace(parsed.Link)
		if newSiteURL != "" {
			_ = s.feeds.UpdateSiteURL(ctx, feed.ID, newSiteURL)
			feed.SiteURL = &newSiteURL
		}
	}

	// Fetch icon if feed doesn't have one
	if s.icons != nil && (feed.IconPath == nil || *feed.IconPath == "") {
		imageURL := ""
		if parsed.Image != nil {
			imageURL = strings.TrimSpace(parsed.Image.URL)
		}
		siteURL := feed.URL
		if feed.SiteURL != nil && *feed.SiteURL != "" {
			siteURL = *feed.SiteURL
		}
		if iconPath, err := s.icons.FetchAndSaveIcon(ctx, imageURL, siteURL); err == nil && iconPath != "" {
			_ = s.feeds.UpdateIconPath(ctx, feed.ID, iconPath)
		}
	}

	return nil
}

// saveEntries saves parsed feed items to the database.
// Returns the count of new, updated and skipped-duplicate entries.
func (s *refreshService) saveEntries(ctx context.Context, feedID int64, items []*gofeed.Item) (newCount, updatedCount, duplicateCount int) {
	dynamicTime := hasDynamicTime(items)
	compiledRules := s.enabledRulesForFeed(ctx, feedID)
	for _, item := range items {
		entry := itemToEntry(feedID, item, dynamicTime)
		if entry.URL == nil || *entry.URL == "" {
			continue
		}

		exists, err := s.entries.ExistsByHash(ctx, feedID, entry.Hash)
		if err != nil {
			logger.Warn("check entry exists failed", "module", "service", "action", "list", "resource", "entry", "result", "failed", "error", err)
			continue
		}
		if !exists {
			legacyExists, err := s.entries.ExistsByLegacyURL(ctx, feedID, *entry.URL, entry.Hash)
			if err != nil {
				logger.Warn("check legacy entry exists failed", "module", "service", "action", "list", "resource", "entry", "result", "failed", "error", err)
				continue
			}
			exists = legacyExists
		}
		if !exists && entry.ContentHash != "" {
			// 链接不稳定时（同一篇文章每次刷新给出不同链接）用内容指纹兜底判重。
			// 命中后直接跳过写入，否则会插入一条内容完全相同的第二行。
			duplicate, err := s.entries.ExistsByContentHash(ctx, feedID, entry.ContentHash)
			if err != nil {
				logger.Warn("check duplicated content failed", "module", "service", "action", "list", "resource", "entry", "result", "failed", "error", err)
				continue
			}
			if duplicate {
				duplicateCount++
				continue
			}
		}

		if !exists && matchesAnyRule(compiledRules, entry) {
			entry.Read = true
		}

		if err := s.entries.CreateOrUpdate(ctx, entry); err != nil {
			logger.Warn("save entry failed", "module", "service", "action", "save", "resource", "entry", "result", "failed", "error", err)
			continue
		}

		if exists {
			updatedCount++
		} else {
			newCount++
		}
	}
	return
}

func (s *refreshService) enabledRulesForFeed(ctx context.Context, feedID int64) []*CompiledArticleRule {
	if s.rules == nil {
		return nil
	}
	rules, err := s.rules.ListEnabledForFeed(ctx, feedID)
	if err != nil {
		logger.Warn("load article rules failed", "module", "service", "action", "list", "resource", "rule", "result", "failed", "feed_id", feedID, "error", err)
		return nil
	}

	compiled := make([]*CompiledArticleRule, 0, len(rules))
	for _, rule := range rules {
		item, err := CompileArticleRule(rule)
		if err != nil {
			logger.Warn("compile article rule failed", "module", "service", "action", "compile", "resource", "rule", "result", "failed", "rule_id", rule.ID, "error", err)
			continue
		}
		compiled = append(compiled, item)
	}
	return compiled
}

func matchesAnyRule(rules []*CompiledArticleRule, entry model.Entry) bool {
	for _, rule := range rules {
		if rule.Matches(entry) {
			return true
		}
	}
	return false
}

var ErrAlreadyRefreshing = errors.New("refresh already in progress")

// RefreshStatus holds the current state of the feed refresh process.
type RefreshStatus struct {
	IsRefreshing    bool
	LastRefreshedAt *time.Time
}

// RuleProvider is the narrow rule dependency needed by the refresh pipeline.
type RuleProvider interface {
	ListEnabledForFeed(ctx context.Context, feedID int64) ([]model.ArticleRule, error)
}

type RefreshService interface {
	RefreshAll(ctx context.Context) error
	RefreshFeed(ctx context.Context, feedID int64) error
	RefreshFeeds(ctx context.Context, feedIDs []int64) error
	IsRefreshing() bool
	GetRefreshStatus() RefreshStatus
}

type refreshService struct {
	feeds           repository.FeedRepository
	entries         repository.EntryRepository
	settings        SettingsService
	icons           IconService
	clientFactory   *network.ClientFactory
	anubis          AnubisSolver
	rateLimitSvc    DomainRateLimitService
	rules           RuleProvider
	mu              sync.Mutex
	isRefreshing    bool
	lastRefreshedAt *time.Time
}

func NewRefreshService(feeds repository.FeedRepository, entries repository.EntryRepository, settings SettingsService, icons IconService, clientFactory *network.ClientFactory, anubisSolver AnubisSolver, rateLimitSvc DomainRateLimitService, rules RuleProvider) RefreshService {
	return &refreshService{
		feeds:         feeds,
		entries:       entries,
		settings:      settings,
		icons:         icons,
		clientFactory: clientFactory,
		anubis:        anubisSolver,
		rateLimitSvc:  rateLimitSvc,
		rules:         rules,
	}
}

func (s *refreshService) RefreshAll(ctx context.Context) error {
	s.mu.Lock()
	if s.isRefreshing {
		s.mu.Unlock()
		return ErrAlreadyRefreshing
	}
	s.isRefreshing = true
	s.mu.Unlock()

	defer func() {
		s.mu.Lock()
		s.isRefreshing = false
		s.mu.Unlock()
	}()

	feeds, err := s.feeds.List(ctx, nil)
	if err != nil {
		logger.Error("refresh list feeds", "module", "service", "action", "list", "resource", "feed", "result", "failed", "error", err)
		return err
	}

	logger.Info("refresh started", "module", "service", "action", "refresh", "resource", "feed", "result", "ok", "count", len(feeds))
	s.refreshFeedsWithRateLimit(ctx, feeds)
	logger.Info("refresh completed", "module", "service", "action", "refresh", "resource", "feed", "result", "ok", "count", len(feeds))

	now := time.Now()
	s.mu.Lock()
	s.lastRefreshedAt = &now
	s.mu.Unlock()

	return nil
}

func (s *refreshService) IsRefreshing() bool {
	s.mu.Lock()
	defer s.mu.Unlock()
	return s.isRefreshing
}

func (s *refreshService) GetRefreshStatus() RefreshStatus {
	s.mu.Lock()
	defer s.mu.Unlock()
	return RefreshStatus{
		IsRefreshing:    s.isRefreshing,
		LastRefreshedAt: s.lastRefreshedAt,
	}
}

func (s *refreshService) RefreshFeed(ctx context.Context, feedID int64) error {
	feed, err := s.feeds.GetByID(ctx, feedID)
	if err != nil {
		return err
	}
	if err := s.refreshFeedInternal(ctx, feed); err != nil {
		return err
	}

	// Single-feed refreshes are user initiated, so surface feed-level failures
	// (for example HTTP 500 responses) instead of silently returning success.
	updated, err := s.feeds.GetByID(ctx, feedID)
	if err != nil {
		return err
	}
	if updated.ErrorMessage != nil && *updated.ErrorMessage != "" {
		return fmt.Errorf("%w: %s", ErrFeedFetch, *updated.ErrorMessage)
	}
	return nil
}

func (s *refreshService) RefreshFeeds(ctx context.Context, feedIDs []int64) error {
	if len(feedIDs) == 0 {
		return nil
	}

	// Get all feeds by IDs in a single query
	feeds, err := s.feeds.GetByIDs(ctx, feedIDs)
	if err != nil {
		logger.Error("get feeds by ids", "module", "service", "action", "list", "resource", "feed", "result", "failed", "error", err)
		return err
	}

	if len(feeds) == 0 {
		return nil
	}

	s.refreshFeedsWithRateLimit(ctx, feeds)
	return nil
}

// refreshFeedsWithRateLimit refreshes multiple feeds with rate limiting and concurrency control.
func (s *refreshService) refreshFeedsWithRateLimit(ctx context.Context, feeds []model.Feed) {
	globalSem := semaphore.NewWeighted(maxConcurrentRefresh)

	hl := newHostRateLimiter(func(host string) time.Duration {
		if s.rateLimitSvc != nil {
			return s.rateLimitSvc.GetIntervalDuration(ctx, host)
		}
		return 0
	})

	var wg sync.WaitGroup
	for _, feed := range feeds {
		feed := feed
		wg.Add(1)
		go func() {
			defer wg.Done()

			host := network.ExtractHost(feed.URL)

			if host != "" {
				if err := hl.acquireSemaphore(ctx, host); err != nil {
					logger.Debug("refresh host acquire cancelled", "module", "service", "action", "refresh", "resource", "feed", "result", "cancelled", "host", host, "error", err)
					return
				}
				defer hl.releaseSemaphore(host)

				if err := hl.waitForInterval(ctx, host); err != nil {
					logger.Debug("refresh host wait cancelled", "module", "service", "action", "refresh", "resource", "feed", "result", "cancelled", "host", host, "error", err)
					return
				}
			}

			if err := globalSem.Acquire(ctx, 1); err != nil {
				logger.Debug("refresh global acquire cancelled", "module", "service", "action", "refresh", "resource", "feed", "result", "cancelled", "host", host, "error", err)
				return
			}
			defer globalSem.Release(1)

			if host != "" {
				hl.recordRequest(host)
			}

			if err := s.refreshFeedInternal(ctx, feed); err != nil {
				logger.Error("refresh feed failed", "module", "service", "action", "refresh", "resource", "feed", "result", "failed", "feed_id", feed.ID, "feed_title", feed.Title, "error", err)
			}
		}()
	}

	wg.Wait()
}

func (s *refreshService) refreshFeedInternal(ctx context.Context, feed model.Feed) error {
	return s.refreshFeedWithUA(ctx, feed, config.DefaultUserAgent, true)
}

func (s *refreshService) refreshFeedWithUA(ctx context.Context, feed model.Feed, userAgent string, allowFallback bool) error {
	return s.refreshFeedWithCookie(ctx, feed, userAgent, "", allowFallback, 0)
}

func (s *refreshService) refreshFeedWithCookie(ctx context.Context, feed model.Feed, userAgent string, cookie string, allowFallback bool, retryCount int) error {
	req, err := http.NewRequestWithContext(ctx, http.MethodGet, feed.URL, nil)
	if err != nil {
		errMsg := err.Error()
		_ = s.feeds.UpdateErrorMessage(ctx, feed.ID, &errMsg)
		return err
	}
	req.Header.Set("User-Agent", userAgent)

	// Add cached Anubis cookie if available
	if cookie == "" {
		host := network.ExtractHost(feed.URL)
		if cachedCookie := getCachedAnubisCookie(ctx, s.anubis, host, req.Header); cachedCookie != "" {
			cookie = cachedCookie
		}
	}

	if cookie != "" {
		req.Header.Set("Cookie", cookie)
	}

	// Conditional GET
	if feed.ETag != nil && *feed.ETag != "" {
		req.Header.Set("If-None-Match", *feed.ETag)
	}
	if feed.LastModified != nil && *feed.LastModified != "" {
		req.Header.Set("If-Modified-Since", *feed.LastModified)
	}

	httpClient := s.clientFactory.NewHTTPClient(ctx, refreshTimeout)
	resp, err := httpClient.Do(req)
	if err != nil {
		errMsg := err.Error()
		_ = s.feeds.UpdateErrorMessage(ctx, feed.ID, &errMsg)
		return err
	}
	defer resp.Body.Close()

	// Not modified, skip parsing but clear any previous error
	if resp.StatusCode == http.StatusNotModified {
		logger.Debug("feed not modified", "module", "service", "action", "refresh", "resource", "feed", "result", "skipped", "feed_id", feed.ID, "host", network.ExtractHost(feed.URL))
		_ = s.feeds.UpdateErrorMessage(ctx, feed.ID, nil)
		return nil
	}

	// On HTTP error, try fallback UA if available
	if resp.StatusCode >= http.StatusBadRequest && allowFallback && s.settings != nil {
		fallbackUA := s.settings.GetFallbackUserAgent(ctx)
		if fallbackUA != "" {
			logger.Warn("retrying with fallback ua", "module", "service", "action", "refresh", "resource", "feed", "result", "failed", "feed_id", feed.ID, "feed_title", feed.Title, "status_code", resp.StatusCode)
			return s.refreshFeedWithCookie(ctx, feed, fallbackUA, cookie, false, retryCount)
		}
	}

	if resp.StatusCode >= http.StatusBadRequest {
		logger.Error("feed http error", "module", "service", "action", "refresh", "resource", "feed", "result", "failed", "feed_id", feed.ID, "feed_title", feed.Title, "status_code", resp.StatusCode)
		errMsg := fmt.Sprintf("HTTP %d", resp.StatusCode)
		_ = s.feeds.UpdateErrorMessage(ctx, feed.ID, &errMsg)
		return nil
	}

	// Read body into memory for Anubis detection and RSS parsing
	body, err := io.ReadAll(resp.Body)
	if err != nil {
		errMsg := err.Error()
		_ = s.feeds.UpdateErrorMessage(ctx, feed.ID, &errMsg)
		return err
	}

	parser := gofeed.NewParser()
	parsed, parseErr := parser.Parse(bytes.NewReader(body))
	if parseErr != nil {
		newCookie, anubisErr := trySolveAnubisChallenge(ctx, s.anubis, body, feed.URL, resp.Cookies(), req.Header.Clone(), retryCount)
		switch {
		case anubisErr == nil:
			// Retry with fresh client and same request fingerprint.
			return s.refreshFeedWithFreshClient(ctx, feed, userAgent, newCookie, retryCount+1)
		case errors.Is(anubisErr, errAnubisNotPage):
			// Not an Anubis page; keep original parse error handling.
		case errors.Is(anubisErr, errAnubisRejected):
			errMsg := "upstream rejected"
			_ = s.feeds.UpdateErrorMessage(ctx, feed.ID, &errMsg)
			return errors.New(errMsg)
		case errors.Is(anubisErr, errAnubisRetryExceeded):
			errMsg := fmt.Sprintf("anubis challenge persists after %d retries", retryCount)
			_ = s.feeds.UpdateErrorMessage(ctx, feed.ID, &errMsg)
			return errors.New(errMsg)
		default:
			errMsg := fmt.Sprintf("anubis solve failed: %v", anubisErr)
			_ = s.feeds.UpdateErrorMessage(ctx, feed.ID, &errMsg)
			return anubisErr
		}
		errMsg := parseErr.Error()
		_ = s.feeds.UpdateErrorMessage(ctx, feed.ID, &errMsg)
		return parseErr
	}

	return s.processParsedFeed(ctx, feed, parsed, resp)
}

// refreshFeedWithFreshClient creates a new http.Client to avoid connection reuse after Anubis
func (s *refreshService) refreshFeedWithFreshClient(ctx context.Context, feed model.Feed, userAgent string, cookie string, retryCount int) error {
	req, err := http.NewRequestWithContext(ctx, http.MethodGet, feed.URL, nil)
	if err != nil {
		errMsg := err.Error()
		_ = s.feeds.UpdateErrorMessage(ctx, feed.ID, &errMsg)
		return err
	}
	req.Header.Set("User-Agent", userAgent)
	if cookie != "" {
		req.Header.Set("Cookie", cookie)
	}

	// Use fresh client to avoid connection reuse
	freshClient := s.clientFactory.NewHTTPClient(ctx, refreshTimeout)
	resp, err := freshClient.Do(req)
	if err != nil {
		errMsg := err.Error()
		_ = s.feeds.UpdateErrorMessage(ctx, feed.ID, &errMsg)
		return err
	}
	defer resp.Body.Close()

	if resp.StatusCode >= http.StatusBadRequest {
		logger.Error("feed http error", "module", "service", "action", "refresh", "resource", "feed", "result", "failed", "feed_id", feed.ID, "feed_title", feed.Title, "status_code", resp.StatusCode)
		errMsg := fmt.Sprintf("HTTP %d", resp.StatusCode)
		_ = s.feeds.UpdateErrorMessage(ctx, feed.ID, &errMsg)
		return nil
	}

	body, err := io.ReadAll(resp.Body)
	if err != nil {
		logger.Error("feed refresh read failed", "module", "service", "action", "refresh", "resource", "feed", "result", "failed", "feed_id", feed.ID, "feed_title", feed.Title, "error", err)
		errMsg := err.Error()
		_ = s.feeds.UpdateErrorMessage(ctx, feed.ID, &errMsg)
		return err
	}

	newCookie, anubisErr := trySolveAnubisChallenge(ctx, s.anubis, body, feed.URL, resp.Cookies(), req.Header.Clone(), retryCount)
	switch {
	case anubisErr == nil:
		return s.refreshFeedWithFreshClient(ctx, feed, userAgent, newCookie, retryCount+1)
	case errors.Is(anubisErr, errAnubisNotPage):
		// Not an Anubis page; continue normal parsing.
	case errors.Is(anubisErr, errAnubisRejected):
		errMsg := "upstream rejected"
		_ = s.feeds.UpdateErrorMessage(ctx, feed.ID, &errMsg)
		return errors.New(errMsg)
	case errors.Is(anubisErr, errAnubisRetryExceeded):
		errMsg := fmt.Sprintf("anubis challenge persists after %d retries", retryCount)
		_ = s.feeds.UpdateErrorMessage(ctx, feed.ID, &errMsg)
		return errors.New(errMsg)
	default:
		errMsg := fmt.Sprintf("anubis solve failed: %v", anubisErr)
		_ = s.feeds.UpdateErrorMessage(ctx, feed.ID, &errMsg)
		return anubisErr
	}

	parser := gofeed.NewParser()
	parsed, parseErr := parser.Parse(bytes.NewReader(body))
	if parseErr != nil {
		errMsg := parseErr.Error()
		_ = s.feeds.UpdateErrorMessage(ctx, feed.ID, &errMsg)
		return parseErr
	}

	return s.processParsedFeed(ctx, feed, parsed, resp)
}
