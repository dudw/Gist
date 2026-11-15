package db_test

import (
	"database/sql"
	"testing"
	"time"

	"gist/backend/internal/db"
	"gist/backend/internal/hashutil"
	"gist/backend/internal/urlutil"

	"github.com/stretchr/testify/require"
	_ "modernc.org/sqlite"
)

func TestMigrate_ArticleRulesTables(t *testing.T) {
	database, err := sql.Open("sqlite", "file::memory:?cache=shared&_pragma=foreign_keys(1)")
	require.NoError(t, err)
	defer database.Close()

	require.NoError(t, db.Migrate(database))

	for _, table := range []string{
		"article_rules",
		"article_rule_conditions",
		"article_rule_feeds",
	} {
		var count int
		require.NoError(t, database.QueryRow(
			`SELECT COUNT(*) FROM sqlite_master WHERE type = 'table' AND name = ?`,
			table,
		).Scan(&count))
		require.Equal(t, 1, count, "missing table %s", table)
	}
}

func TestMigrate_EntryHashDeduplication_MergesHistoricalDuplicates(t *testing.T) {
	database, err := sql.Open("sqlite", "file::memory:?cache=shared&_pragma=foreign_keys(1)")
	require.NoError(t, err)
	defer database.Close()

	createLegacyEntryDedupSchema(t, database)

	_, err = database.Exec(`INSERT INTO feeds (id, title, url, created_at, updated_at) VALUES (1, 'feed', 'https://example.com/rss', '2025-01-01T00:00:00Z', '2025-01-01T00:00:00Z')`)
	require.NoError(t, err)

	_, err = database.Exec(`
		INSERT INTO entries (id, feed_id, title, url, content, author, published_at, read, starred, created_at, updated_at) VALUES
		(1001, 1, 'same', 'https://www.v2ex.com/t/1193191#reply10', 'body', 'author', '2025-01-01T00:00:00Z', 1, 0, '2025-01-01T00:00:00Z', '2025-01-01T00:00:00Z'),
		(1002, 1, 'same', 'https://www.v2ex.com/t/1193191#reply20', 'body', 'author', '2025-01-01T00:00:00Z', 0, 1, '2025-01-01T00:00:00Z', '2025-01-02T00:00:00Z'),
		(1003, 1, NULL, NULL, NULL, NULL, NULL, 0, 0, '2025-01-03T00:00:00Z', '2025-01-03T00:00:00Z')
	`)
	require.NoError(t, err)

	_, err = database.Exec(`
		INSERT INTO ai_summaries (id, entry_id, is_readability, language, summary, created_at) VALUES
		(2001, 1001, 0, 'zh-CN', 'summary from old row', '2025-01-01T00:00:00Z'),
		(2002, 1002, 0, 'zh-CN', 'summary keep row', '2025-01-02T00:00:00Z')
	`)
	require.NoError(t, err)

	_, err = database.Exec(`
		INSERT INTO ai_translations (id, entry_id, is_readability, language, content, created_at) VALUES
		(3001, 1001, 0, 'zh-CN', '<p>translated</p>', '2025-01-01T00:00:00Z')
	`)
	require.NoError(t, err)

	_, err = database.Exec(`
		INSERT INTO ai_list_translations (id, entry_id, language, title, summary, created_at) VALUES
		(4001, 1001, 'zh-CN', 'title', 'summary', '2025-01-01T00:00:00Z')
	`)
	require.NoError(t, err)

	err = db.Migrate(database)
	require.NoError(t, err)

	var reminderColumnCount int
	err = database.QueryRow(`SELECT COUNT(*) FROM pragma_table_info('feeds') WHERE name = 'summary_prompt_reminder'`).Scan(&reminderColumnCount)
	require.NoError(t, err)
	require.Equal(t, 1, reminderColumnCount)

	var oldIndexCount int
	err = database.QueryRow(`SELECT COUNT(*) FROM sqlite_master WHERE type = 'index' AND name = 'idx_entries_feed_url'`).Scan(&oldIndexCount)
	require.NoError(t, err)
	require.Equal(t, 0, oldIndexCount)

	var newIndexCount int
	err = database.QueryRow(`SELECT COUNT(*) FROM sqlite_master WHERE type = 'index' AND name = 'idx_entries_feed_hash'`).Scan(&newIndexCount)
	require.NoError(t, err)
	require.Equal(t, 1, newIndexCount)

	var entryCount int
	err = database.QueryRow(`SELECT COUNT(*) FROM entries`).Scan(&entryCount)
	require.NoError(t, err)
	require.Equal(t, 2, entryCount)

	var url, hash string
	var read, starred int
	err = database.QueryRow(`SELECT url, hash, read, starred FROM entries WHERE id = 1002`).Scan(&url, &hash, &read, &starred)
	require.NoError(t, err)
	require.Equal(t, "https://www.v2ex.com/t/1193191#reply20", url)
	require.Len(t, hash, 64)
	require.Equal(t, 1, read)
	require.Equal(t, 1, starred)

	var hashEmptyEntry string
	err = database.QueryRow(`SELECT hash FROM entries WHERE id = 1003`).Scan(&hashEmptyEntry)
	require.NoError(t, err)
	require.Len(t, hashEmptyEntry, 64)

	err = database.QueryRow(`SELECT COUNT(*) FROM entries WHERE id = 1001`).Scan(&entryCount)
	require.NoError(t, err)
	require.Equal(t, 0, entryCount)

	var summaryRows int
	err = database.QueryRow(`SELECT COUNT(*) FROM ai_summaries WHERE entry_id = 1001`).Scan(&summaryRows)
	require.NoError(t, err)
	require.Equal(t, 0, summaryRows)
	err = database.QueryRow(`SELECT COUNT(*) FROM ai_summaries WHERE entry_id = 1002`).Scan(&summaryRows)
	require.NoError(t, err)
	require.Equal(t, 1, summaryRows)

	var translationRows int
	err = database.QueryRow(`SELECT COUNT(*) FROM ai_translations WHERE entry_id = 1002`).Scan(&translationRows)
	require.NoError(t, err)
	require.Equal(t, 1, translationRows)
	err = database.QueryRow(`SELECT COUNT(*) FROM ai_list_translations WHERE entry_id = 1002`).Scan(&translationRows)
	require.NoError(t, err)
	require.Equal(t, 1, translationRows)

	var duplicateHashGroups int
	err = database.QueryRow(`
		SELECT COUNT(*) FROM (
			SELECT feed_id, hash, COUNT(*) AS c
			FROM entries
			GROUP BY feed_id, hash
			HAVING c > 1
		)
	`).Scan(&duplicateHashGroups)
	require.NoError(t, err)
	require.Equal(t, 0, duplicateHashGroups)

	// Migration should be idempotent.
	err = db.Migrate(database)
	require.NoError(t, err)
}

func TestMigrate_EntrySearchRebuildsLegacyFTS(t *testing.T) {
	database, err := sql.Open("sqlite", "file::memory:?cache=shared&_pragma=foreign_keys(1)")
	require.NoError(t, err)
	defer database.Close()

	// 旧版数据库：没有 search_text，FTS 索引直接建在 HTML 内容上
	_, err = database.Exec(`
		CREATE TABLE feeds (
			id INTEGER PRIMARY KEY,
			folder_id INTEGER,
			title TEXT NOT NULL,
			url TEXT NOT NULL UNIQUE,
			created_at TEXT NOT NULL,
			updated_at TEXT NOT NULL
		);

		CREATE TABLE entries (
			id INTEGER PRIMARY KEY,
			feed_id INTEGER NOT NULL,
			title TEXT,
			url TEXT,
			content TEXT,
			author TEXT,
			published_at TEXT,
			read INTEGER NOT NULL DEFAULT 0,
			starred INTEGER NOT NULL DEFAULT 0,
			created_at TEXT NOT NULL,
			updated_at TEXT NOT NULL
		);

		CREATE VIRTUAL TABLE entries_fts USING fts5(
			title, content, author, url, tokenize = 'unicode61'
		);

		CREATE TRIGGER entries_ai AFTER INSERT ON entries BEGIN
			INSERT INTO entries_fts(rowid, title, content, author, url)
			VALUES (new.id, new.title, new.content, new.author, new.url);
		END;
	`)
	require.NoError(t, err)

	_, err = database.Exec(`
		INSERT INTO feeds (id, title, url, created_at, updated_at)
		VALUES (1, 'feed', 'https://example.com/rss', '2025-01-01T00:00:00Z', '2025-01-01T00:00:00Z');

		INSERT INTO entries (id, feed_id, title, url, content, author, published_at, read, starred, created_at, updated_at)
		VALUES (1, 1, '历史标题', 'https://example.com/entry', '<p>正文里的流星雨现象</p>', NULL, '2025-01-01T00:00:00Z', 0, 0, '2025-01-01T00:00:00Z', '2025-01-01T00:00:00Z');
	`)
	require.NoError(t, err)

	require.NoError(t, db.Migrate(database))

	var searchText string
	require.NoError(t, database.QueryRow(`SELECT search_text FROM entries WHERE id = 1`).Scan(&searchText))
	require.Contains(t, searchText, "历史标题")
	require.Contains(t, searchText, "正文里的流星雨现象")
	require.NotContains(t, searchText, "<p>")

	var ftsColumnCount int
	require.NoError(t, database.QueryRow(
		`SELECT COUNT(*) FROM pragma_table_info('entries_fts') WHERE name = 'search_text'`,
	).Scan(&ftsColumnCount))
	require.Equal(t, 1, ftsColumnCount)

	var matchCount int
	require.NoError(t, database.QueryRow(
		`SELECT COUNT(*) FROM entries_fts WHERE entries_fts MATCH '"流星雨"'`,
	).Scan(&matchCount))
	require.Equal(t, 1, matchCount)

	// 新写入的数据通过触发器同步到索引
	_, err = database.Exec(`
		INSERT INTO entries (id, feed_id, title, url, content, search_text, author, published_at, read, starred, created_at, updated_at)
		VALUES (2, 1, '新文章', 'https://example.com/new', '<p>trigger body</p>', '新文章 trigger body', NULL, '2025-01-02T00:00:00Z', 0, 0, '2025-01-02T00:00:00Z', '2025-01-02T00:00:00Z');
	`)
	require.NoError(t, err)

	require.NoError(t, database.QueryRow(
		`SELECT COUNT(*) FROM entries_fts WHERE entries_fts MATCH '"trigger body"'`,
	).Scan(&matchCount))
	require.Equal(t, 1, matchCount)
}

// createLegacyEntryDedupSchema 构造 hash 迁移之前的旧库结构。
func createLegacyEntryDedupSchema(t *testing.T, database *sql.DB) {
	t.Helper()

	_, err := database.Exec(`
		CREATE TABLE feeds (
			id INTEGER PRIMARY KEY,
			folder_id INTEGER,
			title TEXT NOT NULL,
			url TEXT NOT NULL UNIQUE,
			created_at TEXT NOT NULL,
			updated_at TEXT NOT NULL
		);

		CREATE TABLE entries (
			id INTEGER PRIMARY KEY,
			feed_id INTEGER NOT NULL,
			title TEXT,
			url TEXT,
			content TEXT,
			author TEXT,
			published_at TEXT,
			read INTEGER NOT NULL DEFAULT 0,
			starred INTEGER NOT NULL DEFAULT 0,
			created_at TEXT NOT NULL,
			updated_at TEXT NOT NULL,
			FOREIGN KEY (feed_id) REFERENCES feeds(id) ON DELETE CASCADE
		);

		CREATE UNIQUE INDEX idx_entries_feed_url ON entries(feed_id, url);

		CREATE TABLE ai_summaries (
			id INTEGER PRIMARY KEY,
			entry_id INTEGER NOT NULL,
			is_readability INTEGER NOT NULL DEFAULT 0,
			language TEXT NOT NULL,
			summary TEXT NOT NULL,
			created_at TEXT NOT NULL,
			FOREIGN KEY (entry_id) REFERENCES entries(id) ON DELETE CASCADE
		);
		CREATE UNIQUE INDEX idx_ai_summaries_entry_mode ON ai_summaries(entry_id, is_readability, language);

		CREATE TABLE ai_translations (
			id INTEGER PRIMARY KEY,
			entry_id INTEGER NOT NULL,
			is_readability INTEGER NOT NULL DEFAULT 0,
			language TEXT NOT NULL,
			content TEXT NOT NULL,
			created_at TEXT NOT NULL,
			FOREIGN KEY (entry_id) REFERENCES entries(id) ON DELETE CASCADE
		);
		CREATE UNIQUE INDEX idx_ai_translations_entry_mode ON ai_translations(entry_id, is_readability, language);

		CREATE TABLE ai_list_translations (
			id INTEGER PRIMARY KEY,
			entry_id INTEGER NOT NULL,
			language TEXT NOT NULL,
			title TEXT NOT NULL,
			summary TEXT NOT NULL,
			created_at TEXT NOT NULL,
			FOREIGN KEY (entry_id) REFERENCES entries(id) ON DELETE CASCADE
		);
		CREATE UNIQUE INDEX idx_ai_list_translations_entry_lang ON ai_list_translations(entry_id, language);
	`)
	require.NoError(t, err)
}

func TestMigrate_EntryCanonicalIdentity_MergesEquivalentLinks(t *testing.T) {
	database, err := sql.Open("sqlite", "file::memory:?cache=shared&_pragma=foreign_keys(1)")
	require.NoError(t, err)
	defer database.Close()

	createLegacyEntryDedupSchema(t, database)

	_, err = database.Exec(`INSERT INTO feeds (id, title, url, created_at, updated_at) VALUES (1, 'feed', 'https://example.com/rss', '2025-01-01T00:00:00Z', '2025-01-01T00:00:00Z')`)
	require.NoError(t, err)

	// 同一篇文章因为等价链接（多一个斜杠）被当成两条记录，这是迁移要修复的历史脏数据。
	plainURL := "https://www.t66y.com/htm_data/2611/5/7511015.html"
	doubleSlashURL := "https://www.t66y.com//htm_data/2611/5/7511015.html"
	_, err = database.Exec(`
		INSERT INTO entries (id, feed_id, title, url, content, author, published_at, read, starred, created_at, updated_at) VALUES
		(1001, 1, 'same title', ?, 'same body', NULL, '2026-11-15T00:48:37Z', 1, 0, '2026-11-15T01:10:20Z', '2026-11-15T01:10:20Z'),
		(1002, 1, 'same title', ?, 'same body', NULL, '2026-11-15T00:48:37Z', 0, 1, '2026-11-15T01:24:55Z', '2026-11-15T01:24:55Z')
	`, plainURL, doubleSlashURL)
	require.NoError(t, err)

	require.NoError(t, db.Migrate(database))

	var entryCount int
	require.NoError(t, database.QueryRow(`SELECT COUNT(*) FROM entries`).Scan(&entryCount))
	require.Equal(t, 1, entryCount)

	var hash string
	var read, starred int
	require.NoError(t, database.QueryRow(
		`SELECT hash, read, starred FROM entries WHERE id = 1002`,
	).Scan(&hash, &read, &starred))
	require.Equal(t, hashutil.SHA256Hex(urlutil.NormalizeForHash(plainURL)), hash)
	require.Equal(t, 1, read)
	require.Equal(t, 1, starred)

	var contentHash string
	require.NoError(t, database.QueryRow(
		`SELECT content_hash FROM entries WHERE id = 1002`,
	).Scan(&contentHash))
	publishedAt := time.Date(2026, 11, 15, 0, 48, 37, 0, time.UTC)
	require.Equal(t, hashutil.EntryContentHash("same title", &publishedAt, "same body"), contentHash)

	var contentHashIndexCount int
	require.NoError(t, database.QueryRow(
		`SELECT COUNT(*) FROM sqlite_master WHERE type = 'index' AND name = 'idx_entries_feed_content_hash'`,
	).Scan(&contentHashIndexCount))
	require.Equal(t, 1, contentHashIndexCount)

	// 迁移必须幂等：重复执行不能再删数据。
	require.NoError(t, db.Migrate(database))
	require.NoError(t, database.QueryRow(`SELECT COUNT(*) FROM entries`).Scan(&entryCount))
	require.Equal(t, 1, entryCount)
}
