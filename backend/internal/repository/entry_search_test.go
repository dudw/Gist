package repository_test

import (
	"context"
	"fmt"
	"testing"
	"time"

	"gist/backend/internal/model"
	"gist/backend/internal/repository"
	"gist/backend/internal/repository/testutil"

	"github.com/stretchr/testify/require"
)

func seedSearchEntry(
	t *testing.T,
	repo repository.EntryRepository,
	feedID int64,
	title string,
	content string,
	slug string,
	publishedAt time.Time,
	read bool,
) {
	t.Helper()

	url := "https://example.com/" + slug
	err := repo.CreateOrUpdate(context.Background(), model.Entry{
		FeedID:      feedID,
		Title:       &title,
		URL:         &url,
		Content:     &content,
		Hash:        hashString(url),
		PublishedAt: &publishedAt,
		Read:        read,
	})
	require.NoError(t, err)
}

func listSearch(t *testing.T, repo repository.EntryRepository, filter repository.EntryListFilter) []model.Entry {
	t.Helper()

	entries, err := repo.List(context.Background(), filter)
	require.NoError(t, err)
	return entries
}

func TestEntryRepository_List_SearchByTitleAndContent(t *testing.T) {
	db := testutil.NewTestDB(t)
	repo := repository.NewEntryRepository(db)
	feedID := testutil.SeedFeed(t, db, model.Feed{Title: "Feed", URL: "https://example.com/rss"})
	base := time.Date(2025, 1, 1, 12, 0, 0, 0, time.UTC)

	seedSearchEntry(t, repo, feedID, "Go 并发编程指南", "<p>本文介绍 goroutine 与 channel。</p>", "go", base, false)
	seedSearchEntry(t, repo, feedID, "Rust 所有权", `<div class="summary">关于 borrowing 的说明，以及 channel 通信。</div>`, "rust-ownership", base.Add(time.Hour), false)
	seedSearchEntry(t, repo, feedID, "Rust 生命周期", "<p>lifetime 与 borrowing。</p>", "rust-lifetime", base.Add(2*time.Hour), false)

	// 中文短词（2 个字）回退 LIKE，命中标题
	entries := listSearch(t, repo, repository.EntryListFilter{Search: "并发"})
	require.Len(t, entries, 1)
	require.Equal(t, "Go 并发编程指南", *entries[0].Title)

	// 正文命中走 FTS
	entries = listSearch(t, repo, repository.EntryListFilter{Search: "goroutine"})
	require.Len(t, entries, 1)
	require.Equal(t, "Go 并发编程指南", *entries[0].Title)

	// HTML 标签与属性不参与搜索
	entries = listSearch(t, repo, repository.EntryListFilter{Search: "div class"})
	require.Empty(t, entries)

	// 英文搜索大小写不敏感
	entries = listSearch(t, repo, repository.EntryListFilter{Search: "GOROUTINE"})
	require.Len(t, entries, 1)

	// 多个分词按 AND 组合
	entries = listSearch(t, repo, repository.EntryListFilter{Search: "channel"})
	require.Len(t, entries, 2)

	// 无结果
	entries = listSearch(t, repo, repository.EntryListFilter{Search: "不存在的关键词"})
	require.Empty(t, entries)
}

func TestEntryRepository_List_SearchRanksTitleMatchesFirst(t *testing.T) {
	db := testutil.NewTestDB(t)
	repo := repository.NewEntryRepository(db)
	feedID := testutil.SeedFeed(t, db, model.Feed{Title: "Feed", URL: "https://example.com/rss"})
	base := time.Date(2025, 1, 1, 12, 0, 0, 0, time.UTC)

	// 标题命中但发布时间更早
	seedSearchEntry(t, repo, feedID, "channel 深入解析", "<p>普通正文内容</p>", "title-hit", base, false)
	// 正文命中且发布时间更晚
	seedSearchEntry(t, repo, feedID, "其他主题", "<p>正文里提到 channel 的实现</p>", "content-hit", base.Add(time.Hour), false)

	entries := listSearch(t, repo, repository.EntryListFilter{Search: "channel"})
	require.Len(t, entries, 2)
	require.Equal(t, "channel 深入解析", *entries[0].Title)
	require.Equal(t, "其他主题", *entries[1].Title)
}

func TestEntryRepository_List_SearchCombinedWithFilters(t *testing.T) {
	db := testutil.NewTestDB(t)
	repo := repository.NewEntryRepository(db)
	feedA := testutil.SeedFeed(t, db, model.Feed{Title: "A", URL: "https://example.com/a"})
	feedB := testutil.SeedFeed(t, db, model.Feed{Title: "B", URL: "https://example.com/b"})
	base := time.Date(2025, 1, 1, 12, 0, 0, 0, time.UTC)

	seedSearchEntry(t, repo, feedA, "Channel 深入解析", "<p>chan 的内部实现。</p>", "a-read", base, true)
	seedSearchEntry(t, repo, feedA, "Channel 使用技巧", "<p>chan 的常见用法。</p>", "a-unread", base.Add(time.Hour), false)
	seedSearchEntry(t, repo, feedB, "Channel 在 Rust 中", "<p>std::sync::mpsc。</p>", "b-unread", base.Add(2*time.Hour), false)

	entries := listSearch(t, repo, repository.EntryListFilter{Search: "channel", FeedID: &feedA})
	require.Len(t, entries, 2)

	entries = listSearch(t, repo, repository.EntryListFilter{Search: "channel", FeedID: &feedA, UnreadOnly: true})
	require.Len(t, entries, 1)
	require.Equal(t, "Channel 使用技巧", *entries[0].Title)
}

func TestEntryRepository_List_SearchPagination(t *testing.T) {
	db := testutil.NewTestDB(t)
	repo := repository.NewEntryRepository(db)
	feedID := testutil.SeedFeed(t, db, model.Feed{Title: "Feed", URL: "https://example.com/rss"})
	base := time.Date(2025, 1, 1, 12, 0, 0, 0, time.UTC)

	for i := 0; i < 3; i++ {
		seedSearchEntry(
			t,
			repo,
			feedID,
			fmt.Sprintf("channel 文章 %d", i),
			"<p>channel 正文</p>",
			fmt.Sprintf("channel-%d", i),
			base.Add(time.Duration(i)*time.Hour),
			false,
		)
	}

	firstPage := listSearch(t, repo, repository.EntryListFilter{Search: "channel", Limit: 2})
	require.Len(t, firstPage, 2)

	secondPage := listSearch(t, repo, repository.EntryListFilter{Search: "channel", Limit: 2, Offset: 2})
	require.Len(t, secondPage, 1)

	require.NotEqual(t, firstPage[0].ID, secondPage[0].ID)
}

func TestEntryRepository_List_SearchEscapesLikeWildcards(t *testing.T) {
	db := testutil.NewTestDB(t)
	repo := repository.NewEntryRepository(db)
	feedID := testutil.SeedFeed(t, db, model.Feed{Title: "Feed", URL: "https://example.com/rss"})
	base := time.Date(2025, 1, 1, 12, 0, 0, 0, time.UTC)

	seedSearchEntry(t, repo, feedID, "折扣信息", "<p>限时 100% 返现</p>", "discount", base, false)
	seedSearchEntry(t, repo, feedID, "普通文章", "<p>没有特殊字符</p>", "plain", base.Add(time.Hour), false)

	// 单字符查询走 LIKE，"_" 和 "%" 必须按字面量匹配
	entries := listSearch(t, repo, repository.EntryListFilter{Search: "%"})
	require.Len(t, entries, 1)
	require.Equal(t, "折扣信息", *entries[0].Title)

	entries = listSearch(t, repo, repository.EntryListFilter{Search: "_"})
	require.Empty(t, entries)
}

func TestEntryRepository_CreateOrUpdate_RefreshesSearchText(t *testing.T) {
	db := testutil.NewTestDB(t)
	repo := repository.NewEntryRepository(db)
	feedID := testutil.SeedFeed(t, db, model.Feed{Title: "Feed", URL: "https://example.com/rss"})
	publishedAt := time.Date(2025, 1, 1, 12, 0, 0, 0, time.UTC)
	url := "https://example.com/entry"
	hash := hashString(url)

	oldTitle := "旧标题"
	oldContent := "<p>旧内容</p>"
	require.NoError(t, repo.CreateOrUpdate(context.Background(), model.Entry{
		FeedID:      feedID,
		Title:       &oldTitle,
		URL:         &url,
		Content:     &oldContent,
		Hash:        hash,
		PublishedAt: &publishedAt,
	}))

	newTitle := "新标题"
	newContent := "<p>更新后的正文包含 unique_keyword</p>"
	require.NoError(t, repo.CreateOrUpdate(context.Background(), model.Entry{
		FeedID:      feedID,
		Title:       &newTitle,
		URL:         &url,
		Content:     &newContent,
		Hash:        hash,
		PublishedAt: &publishedAt,
	}))

	entries := listSearch(t, repo, repository.EntryListFilter{Search: "unique_keyword"})
	require.Len(t, entries, 1)
	require.Equal(t, newTitle, *entries[0].Title)

	entries = listSearch(t, repo, repository.EntryListFilter{Search: "旧内容"})
	require.Empty(t, entries)
}
