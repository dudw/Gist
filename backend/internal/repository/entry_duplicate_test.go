package repository_test

import (
	"context"
	"testing"
	"time"

	"gist/backend/internal/model"
	"gist/backend/internal/repository"
	"gist/backend/internal/repository/testutil"

	"github.com/stretchr/testify/require"
)

func createDuplicateCandidate(
	t *testing.T,
	repo repository.EntryRepository,
	feedID int64,
	title string,
	author string,
	content string,
	slug string,
	publishedAt time.Time,
	read bool,
	starred bool,
) {
	t.Helper()

	url := "https://example.com/" + slug
	require.NoError(t, repo.CreateOrUpdate(context.Background(), model.Entry{
		FeedID:      feedID,
		Title:       &title,
		Author:      &author,
		Content:     &content,
		URL:         &url,
		Hash:        hashString(url),
		PublishedAt: &publishedAt,
		Read:        read,
		Starred:     starred,
	}))
}

func listFeedEntries(t *testing.T, repo repository.EntryRepository, feedID int64) []model.Entry {
	t.Helper()

	entries, err := repo.List(context.Background(), repository.EntryListFilter{FeedID: &feedID})
	require.NoError(t, err)
	return entries
}

func TestEntryRepository_CreateOrUpdate_KeepsLatestDuplicateByTitleAndAuthor(t *testing.T) {
	db := testutil.NewTestDB(t)
	repo := repository.NewEntryRepository(db)
	feedID := testutil.SeedFeed(t, db, model.Feed{Title: "Feed", URL: "https://example.com/rss"})
	base := time.Date(2025, 1, 1, 12, 0, 0, 0, time.UTC)

	createDuplicateCandidate(t, repo, feedID, "重复标题", "作者A", "<p>旧版本正文</p>", "dup-old", base, false, false)
	createDuplicateCandidate(t, repo, feedID, "重复标题", "作者A", "<p>新版本正文</p>", "dup-new", base.Add(time.Hour), false, false)

	entries := listFeedEntries(t, repo, feedID)
	require.Len(t, entries, 1)
	require.NotNil(t, entries[0].URL)
	require.Equal(t, "https://example.com/dup-new", *entries[0].URL)
}

func TestEntryRepository_CreateOrUpdate_KeepsLatestEvenWhenOlderArrivesLater(t *testing.T) {
	db := testutil.NewTestDB(t)
	repo := repository.NewEntryRepository(db)
	feedID := testutil.SeedFeed(t, db, model.Feed{Title: "Feed", URL: "https://example.com/rss"})
	base := time.Date(2025, 1, 1, 12, 0, 0, 0, time.UTC)

	createDuplicateCandidate(t, repo, feedID, "重复标题", "作者A", "<p>新版本</p>", "dup-new", base.Add(time.Hour), false, false)
	createDuplicateCandidate(t, repo, feedID, "重复标题", "作者A", "<p>旧版本</p>", "dup-old", base, false, false)

	entries := listFeedEntries(t, repo, feedID)
	require.Len(t, entries, 1)
	require.NotNil(t, entries[0].URL)
	require.Equal(t, "https://example.com/dup-new", *entries[0].URL)
}

func TestEntryRepository_CreateOrUpdate_MergesReadAndStarredIntoLatest(t *testing.T) {
	db := testutil.NewTestDB(t)
	repo := repository.NewEntryRepository(db)
	feedID := testutil.SeedFeed(t, db, model.Feed{Title: "Feed", URL: "https://example.com/rss"})
	base := time.Date(2025, 1, 1, 12, 0, 0, 0, time.UTC)

	createDuplicateCandidate(t, repo, feedID, "重复标题", "作者A", "<p>旧版本</p>", "dup-old", base, true, false)
	oldEntries := listFeedEntries(t, repo, feedID)
	require.Len(t, oldEntries, 1)
	// CreateOrUpdate 不写入 starred（订阅抓取不会产生加星），测试中单独设置
	require.NoError(t, repo.UpdateStarredStatus(context.Background(), oldEntries[0].ID, true))

	createDuplicateCandidate(t, repo, feedID, "重复标题", "作者A", "<p>新版本</p>", "dup-new", base.Add(time.Hour), false, false)

	entries := listFeedEntries(t, repo, feedID)
	require.Len(t, entries, 1)
	require.True(t, entries[0].Read, "已读状态应合并到保留的文章")
	require.True(t, entries[0].Starred, "加星状态应合并到保留的文章")
}

func TestEntryRepository_CreateOrUpdate_KeepsDifferentTitleOrAuthor(t *testing.T) {
	db := testutil.NewTestDB(t)
	repo := repository.NewEntryRepository(db)
	feedID := testutil.SeedFeed(t, db, model.Feed{Title: "Feed", URL: "https://example.com/rss"})
	base := time.Date(2025, 1, 1, 12, 0, 0, 0, time.UTC)

	createDuplicateCandidate(t, repo, feedID, "相同标题", "作者A", "<p>一</p>", "same-title-a", base, false, false)
	createDuplicateCandidate(t, repo, feedID, "另一个标题", "作者A", "<p>二</p>", "other-title", base.Add(time.Hour), false, false)
	createDuplicateCandidate(t, repo, feedID, "相同标题", "作者B", "<p>三</p>", "same-title-b", base.Add(2*time.Hour), false, false)

	entries := listFeedEntries(t, repo, feedID)
	require.Len(t, entries, 3)
}

func TestEntryRepository_CreateOrUpdate_SkipsDedupWhenAuthorMissing(t *testing.T) {
	db := testutil.NewTestDB(t)
	repo := repository.NewEntryRepository(db)
	feedID := testutil.SeedFeed(t, db, model.Feed{Title: "Feed", URL: "https://example.com/rss"})
	base := time.Date(2025, 1, 1, 12, 0, 0, 0, time.UTC)

	for i, slug := range []string{"no-author-1", "no-author-2"} {
		title := "相同标题"
		url := "https://example.com/" + slug
		require.NoError(t, repo.CreateOrUpdate(context.Background(), model.Entry{
			FeedID:      feedID,
			Title:       &title,
			URL:         &url,
			Hash:        hashString(url),
			PublishedAt: &base,
			Read:        i == 0,
		}))
	}

	entries := listFeedEntries(t, repo, feedID)
	require.Len(t, entries, 2, "缺少作者时不应按标题去重")
}

func TestEntryRepository_CreateOrUpdate_DedupIsScopedToFeed(t *testing.T) {
	db := testutil.NewTestDB(t)
	repo := repository.NewEntryRepository(db)
	feedA := testutil.SeedFeed(t, db, model.Feed{Title: "A", URL: "https://example.com/a"})
	feedB := testutil.SeedFeed(t, db, model.Feed{Title: "B", URL: "https://example.com/b"})
	base := time.Date(2025, 1, 1, 12, 0, 0, 0, time.UTC)

	createDuplicateCandidate(t, repo, feedA, "相同标题", "作者A", "<p>A</p>", "feed-a", base, false, false)
	createDuplicateCandidate(t, repo, feedB, "相同标题", "作者A", "<p>B</p>", "feed-b", base.Add(time.Hour), false, false)

	require.Len(t, listFeedEntries(t, repo, feedA), 1)
	require.Len(t, listFeedEntries(t, repo, feedB), 1)
}

func TestEntryRepository_CreateOrUpdate_DedupRemovesStaleSearchIndex(t *testing.T) {
	db := testutil.NewTestDB(t)
	repo := repository.NewEntryRepository(db)
	feedID := testutil.SeedFeed(t, db, model.Feed{Title: "Feed", URL: "https://example.com/rss"})
	base := time.Date(2025, 1, 1, 12, 0, 0, 0, time.UTC)

	createDuplicateCandidate(t, repo, feedID, "重复标题", "作者A", "<p>oldkeyword 正文</p>", "dup-old", base, false, false)
	createDuplicateCandidate(t, repo, feedID, "重复标题", "作者A", "<p>newkeyword 正文</p>", "dup-new", base.Add(time.Hour), false, false)

	removed, err := repo.List(context.Background(), repository.EntryListFilter{Search: "oldkeyword"})
	require.NoError(t, err)
	require.Empty(t, removed)

	kept, err := repo.List(context.Background(), repository.EntryListFilter{Search: "newkeyword"})
	require.NoError(t, err)
	require.Len(t, kept, 1)
}
