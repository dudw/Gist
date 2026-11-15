package db

import (
	"database/sql"
	"fmt"
	"strings"
	"time"

	"gist/backend/internal/hashutil"
	"gist/backend/internal/searchtext"
	"gist/backend/internal/urlutil"
)

// Base schema - uses Snowflake IDs (no AUTOINCREMENT)
const baseSchema = `
CREATE TABLE IF NOT EXISTS folders (
  id INTEGER PRIMARY KEY,
  name TEXT NOT NULL,
  parent_id INTEGER,
  created_at TEXT NOT NULL,
  updated_at TEXT NOT NULL,
  FOREIGN KEY (parent_id) REFERENCES folders(id) ON DELETE CASCADE
);

CREATE INDEX IF NOT EXISTS idx_folders_parent_id ON folders(parent_id);

CREATE TABLE IF NOT EXISTS feeds (
  id INTEGER PRIMARY KEY,
  folder_id INTEGER,
  title TEXT NOT NULL,
  url TEXT NOT NULL UNIQUE,
  site_url TEXT,
  description TEXT,
  summary_prompt_reminder TEXT,
  etag TEXT,
  last_modified TEXT,
  created_at TEXT NOT NULL,
  updated_at TEXT NOT NULL,
  FOREIGN KEY (folder_id) REFERENCES folders(id) ON DELETE SET NULL
);

CREATE INDEX IF NOT EXISTS idx_feeds_folder_id ON feeds(folder_id);

CREATE TABLE IF NOT EXISTS entries (
  id INTEGER PRIMARY KEY,
  feed_id INTEGER NOT NULL,
  hash TEXT NOT NULL DEFAULT '',
  title TEXT,
  url TEXT,
  content TEXT,
  search_text TEXT NOT NULL DEFAULT '',
  author TEXT,
  published_at TEXT,
  created_at TEXT NOT NULL,
  updated_at TEXT NOT NULL,
  FOREIGN KEY (feed_id) REFERENCES feeds(id) ON DELETE CASCADE
);

CREATE INDEX IF NOT EXISTS idx_entries_feed_id ON entries(feed_id);
`

// entrySearchFTSSchema 定义文章搜索用的 FTS5 索引。
// 使用 trigram 分词以支持中英文子串匹配，内容来自去 HTML 的 search_text。
const entrySearchFTSSchema = `
CREATE VIRTUAL TABLE IF NOT EXISTS entries_fts USING fts5(
  title,
  search_text,
  tokenize = 'trigram'
);
`

const entrySearchTriggersSchema = `
CREATE TRIGGER IF NOT EXISTS entries_ai AFTER INSERT ON entries BEGIN
  INSERT INTO entries_fts(rowid, title, search_text)
  VALUES (new.id, COALESCE(new.title, ''), new.search_text);
END;

CREATE TRIGGER IF NOT EXISTS entries_ad AFTER DELETE ON entries BEGIN
  DELETE FROM entries_fts WHERE rowid = old.id;
END;

CREATE TRIGGER IF NOT EXISTS entries_au AFTER UPDATE OF title, search_text ON entries
WHEN old.title IS NOT new.title OR old.search_text IS NOT new.search_text
BEGIN
  DELETE FROM entries_fts WHERE rowid = old.id;
  INSERT INTO entries_fts(rowid, title, search_text)
  VALUES (new.id, COALESCE(new.title, ''), new.search_text);
END;
`

func Migrate(db *sql.DB) error {
	// Run base schema first (without incremental columns)
	if _, err := db.Exec(baseSchema + entrySearchFTSSchema + entrySearchTriggersSchema); err != nil {
		return fmt.Errorf("migrate base schema: %w", err)
	}

	// Run incremental migrations
	if err := runMigrations(db); err != nil {
		return fmt.Errorf("run migrations: %w", err)
	}

	return nil
}

func runMigrations(db *sql.DB) error {
	// Migration 1: Add read column to entries if not exists
	var count int
	err := db.QueryRow(`
		SELECT COUNT(*) FROM pragma_table_info('entries') WHERE name = 'read'
	`).Scan(&count)
	if err != nil {
		return fmt.Errorf("check read column: %w", err)
	}

	if count == 0 {
		if _, err := db.Exec(`ALTER TABLE entries ADD COLUMN read INTEGER NOT NULL DEFAULT 0`); err != nil {
			return fmt.Errorf("add read column: %w", err)
		}
	}

	// Create indexes (safe to run even if they exist)
	if _, err := db.Exec(`CREATE INDEX IF NOT EXISTS idx_entries_read ON entries(read)`); err != nil {
		return fmt.Errorf("create idx_entries_read: %w", err)
	}
	if _, err := db.Exec(`CREATE INDEX IF NOT EXISTS idx_entries_feed_read ON entries(feed_id, read)`); err != nil {
		return fmt.Errorf("create idx_entries_feed_read: %w", err)
	}

	// Migration 3: Drop the UPDATE trigger (causes issues with FTS5 on read status changes)
	// RSS entries rarely change content after insertion, so we only need INSERT/DELETE triggers
	if _, err := db.Exec(`DROP TRIGGER IF EXISTS entries_au`); err != nil {
		return fmt.Errorf("drop entries_au trigger: %w", err)
	}

	// Migration 4: Add readable_content column to entries for readability-extracted content
	err = db.QueryRow(`
		SELECT COUNT(*) FROM pragma_table_info('entries') WHERE name = 'readable_content'
	`).Scan(&count)
	if err != nil {
		return fmt.Errorf("check readable_content column: %w", err)
	}

	if count == 0 {
		if _, err := db.Exec(`ALTER TABLE entries ADD COLUMN readable_content TEXT`); err != nil {
			return fmt.Errorf("add readable_content column: %w", err)
		}
	}

	// Migration 5: Add icon_path column to feeds for cached icon file path
	err = db.QueryRow(`
		SELECT COUNT(*) FROM pragma_table_info('feeds') WHERE name = 'icon_path'
	`).Scan(&count)
	if err != nil {
		return fmt.Errorf("check icon_path column: %w", err)
	}

	if count == 0 {
		if _, err := db.Exec(`ALTER TABLE feeds ADD COLUMN icon_path TEXT`); err != nil {
			return fmt.Errorf("add icon_path column: %w", err)
		}
	}

	// Migration 6: Add thumbnail_url column to entries for article cover image
	err = db.QueryRow(`
		SELECT COUNT(*) FROM pragma_table_info('entries') WHERE name = 'thumbnail_url'
	`).Scan(&count)
	if err != nil {
		return fmt.Errorf("check thumbnail_url column: %w", err)
	}

	if count == 0 {
		if _, err := db.Exec(`ALTER TABLE entries ADD COLUMN thumbnail_url TEXT`); err != nil {
			return fmt.Errorf("add thumbnail_url column: %w", err)
		}
	}

	// Migration 7: Add starred column to entries for bookmarking
	err = db.QueryRow(`
		SELECT COUNT(*) FROM pragma_table_info('entries') WHERE name = 'starred'
	`).Scan(&count)
	if err != nil {
		return fmt.Errorf("check starred column: %w", err)
	}

	if count == 0 {
		if _, err := db.Exec(`ALTER TABLE entries ADD COLUMN starred INTEGER NOT NULL DEFAULT 0`); err != nil {
			return fmt.Errorf("add starred column: %w", err)
		}
	}

	if _, err := db.Exec(`CREATE INDEX IF NOT EXISTS idx_entries_starred ON entries(starred)`); err != nil {
		return fmt.Errorf("create idx_entries_starred: %w", err)
	}

	// Migration 8: Add error_message column to feeds for tracking fetch/refresh errors
	err = db.QueryRow(`
		SELECT COUNT(*) FROM pragma_table_info('feeds') WHERE name = 'error_message'
	`).Scan(&count)
	if err != nil {
		return fmt.Errorf("check error_message column: %w", err)
	}

	if count == 0 {
		if _, err := db.Exec(`ALTER TABLE feeds ADD COLUMN error_message TEXT`); err != nil {
			return fmt.Errorf("add error_message column: %w", err)
		}
	}

	// Migration 9: Create settings table for key-value configuration storage
	if _, err := db.Exec(`
		CREATE TABLE IF NOT EXISTS settings (
			key TEXT PRIMARY KEY,
			value TEXT NOT NULL,
			updated_at TEXT NOT NULL
		)
	`); err != nil {
		return fmt.Errorf("create settings table: %w", err)
	}

	// Migration 10: Create ai_summaries table for AI summary cache
	if _, err := db.Exec(`
		CREATE TABLE IF NOT EXISTS ai_summaries (
			id INTEGER PRIMARY KEY,
			entry_id INTEGER NOT NULL,
			is_readability INTEGER NOT NULL DEFAULT 0,
			language TEXT NOT NULL,
			summary TEXT NOT NULL,
			created_at TEXT NOT NULL,
			FOREIGN KEY (entry_id) REFERENCES entries(id) ON DELETE CASCADE
		)
	`); err != nil {
		return fmt.Errorf("create ai_summaries table: %w", err)
	}

	if _, err := db.Exec(`CREATE UNIQUE INDEX IF NOT EXISTS idx_ai_summaries_entry_mode ON ai_summaries(entry_id, is_readability, language)`); err != nil {
		return fmt.Errorf("create idx_ai_summaries_entry_mode: %w", err)
	}

	// Migration 11: Create ai_translations table for AI translation cache
	if _, err := db.Exec(`
		CREATE TABLE IF NOT EXISTS ai_translations (
			id INTEGER PRIMARY KEY,
			entry_id INTEGER NOT NULL,
			is_readability INTEGER NOT NULL DEFAULT 0,
			language TEXT NOT NULL,
			content TEXT NOT NULL,
			created_at TEXT NOT NULL,
			FOREIGN KEY (entry_id) REFERENCES entries(id) ON DELETE CASCADE
		)
	`); err != nil {
		return fmt.Errorf("create ai_translations table: %w", err)
	}

	if _, err := db.Exec(`CREATE UNIQUE INDEX IF NOT EXISTS idx_ai_translations_entry_mode ON ai_translations(entry_id, is_readability, language)`); err != nil {
		return fmt.Errorf("create idx_ai_translations_entry_mode: %w", err)
	}

	// Migration 12: Create ai_list_translations table for title/summary translation cache
	if _, err := db.Exec(`
		CREATE TABLE IF NOT EXISTS ai_list_translations (
			id INTEGER PRIMARY KEY,
			entry_id INTEGER NOT NULL,
			language TEXT NOT NULL,
			title TEXT NOT NULL,
			summary TEXT NOT NULL,
			created_at TEXT NOT NULL,
			FOREIGN KEY (entry_id) REFERENCES entries(id) ON DELETE CASCADE
		)
	`); err != nil {
		return fmt.Errorf("create ai_list_translations table: %w", err)
	}

	if _, err := db.Exec(`CREATE UNIQUE INDEX IF NOT EXISTS idx_ai_list_translations_entry_lang ON ai_list_translations(entry_id, language)`); err != nil {
		return fmt.Errorf("create idx_ai_list_translations_entry_lang: %w", err)
	}

	// Migration 13: Add type column to feeds for content type (article/picture/notification)
	err = db.QueryRow(`
		SELECT COUNT(*) FROM pragma_table_info('feeds') WHERE name = 'type'
	`).Scan(&count)
	if err != nil {
		return fmt.Errorf("check feeds type column: %w", err)
	}

	if count == 0 {
		if _, err := db.Exec(`ALTER TABLE feeds ADD COLUMN type TEXT NOT NULL DEFAULT 'article'`); err != nil {
			return fmt.Errorf("add feeds type column: %w", err)
		}
	}

	// Migration 14: Add type column to folders for content type (article/picture/notification)
	err = db.QueryRow(`
		SELECT COUNT(*) FROM pragma_table_info('folders') WHERE name = 'type'
	`).Scan(&count)
	if err != nil {
		return fmt.Errorf("check folders type column: %w", err)
	}

	if count == 0 {
		if _, err := db.Exec(`ALTER TABLE folders ADD COLUMN type TEXT NOT NULL DEFAULT 'article'`); err != nil {
			return fmt.Errorf("add folders type column: %w", err)
		}
	}

	// Migration 15: Fix FTS5 delete trigger (modernc.org/sqlite doesn't support the special insert syntax)
	// Recreate the trigger with direct DELETE syntax
	if _, err := db.Exec(`DROP TRIGGER IF EXISTS entries_ad`); err != nil {
		return fmt.Errorf("drop entries_ad trigger: %w", err)
	}
	if _, err := db.Exec(`CREATE TRIGGER IF NOT EXISTS entries_ad AFTER DELETE ON entries BEGIN
		DELETE FROM entries_fts WHERE rowid = old.id;
	END`); err != nil {
		return fmt.Errorf("create entries_ad trigger: %w", err)
	}

	// Migration 16: Create domain_rate_limits table for per-host rate limiting
	if _, err := db.Exec(`
		CREATE TABLE IF NOT EXISTS domain_rate_limits (
			id INTEGER PRIMARY KEY,
			host TEXT NOT NULL UNIQUE,
			interval_seconds INTEGER NOT NULL,
			created_at TEXT NOT NULL,
			updated_at TEXT NOT NULL
		)
	`); err != nil {
		return fmt.Errorf("create domain_rate_limits table: %w", err)
	}

	if _, err := db.Exec(`CREATE INDEX IF NOT EXISTS idx_domain_rate_limits_host ON domain_rate_limits(host)`); err != nil {
		return fmt.Errorf("create idx_domain_rate_limits_host: %w", err)
	}

	// Migration 17: Switch entry dedup key to hash and merge historical duplicates.
	if err := migrateEntryHashDeduplication(db); err != nil {
		return fmt.Errorf("migrate entry hash deduplication: %w", err)
	}

	// Migration 18: Add summary_prompt_reminder column to feeds for per-feed summarize prompt customization.
	err = db.QueryRow(`
		SELECT COUNT(*) FROM pragma_table_info('feeds') WHERE name = 'summary_prompt_reminder'
	`).Scan(&count)
	if err != nil {
		return fmt.Errorf("check feeds summary_prompt_reminder column: %w", err)
	}

	if count == 0 {
		if _, err := db.Exec(`ALTER TABLE feeds ADD COLUMN summary_prompt_reminder TEXT`); err != nil {
			return fmt.Errorf("add feeds summary_prompt_reminder column: %w", err)
		}
	}

	// Migration 19: Article rules.
	if _, err := db.Exec(`
		CREATE TABLE IF NOT EXISTS article_rules (
			id INTEGER PRIMARY KEY,
			name TEXT NOT NULL,
			enabled INTEGER NOT NULL DEFAULT 1,
			full_word INTEGER NOT NULL DEFAULT 0,
			action_type TEXT NOT NULL DEFAULT 'mark_read',
			created_at TEXT NOT NULL,
			updated_at TEXT NOT NULL
		);
	`); err != nil {
		return fmt.Errorf("create article_rules table: %w", err)
	}

	// Migration 20: Conditions belonging to an article rule.
	if _, err := db.Exec(`
		CREATE TABLE IF NOT EXISTS article_rule_conditions (
			id INTEGER PRIMARY KEY,
			rule_id INTEGER NOT NULL,
			position INTEGER NOT NULL,
			join_type TEXT NOT NULL DEFAULT 'and',
			field TEXT NOT NULL,
			operator TEXT NOT NULL,
			value TEXT NOT NULL,
			FOREIGN KEY (rule_id) REFERENCES article_rules(id) ON DELETE CASCADE
		)
	`); err != nil {
		return fmt.Errorf("create article_rule_conditions table: %w", err)
	}

	if _, err := db.Exec(`
		CREATE INDEX IF NOT EXISTS idx_article_rule_conditions_rule
			ON article_rule_conditions(rule_id, position)
	`); err != nil {
		return fmt.Errorf("create idx_article_rule_conditions_rule: %w", err)
	}

	// Migration 21: Feed bindings for article rules.
	if _, err := db.Exec(`
		CREATE TABLE IF NOT EXISTS article_rule_feeds (
			rule_id INTEGER NOT NULL,
			feed_id INTEGER NOT NULL,
			PRIMARY KEY (rule_id, feed_id),
			FOREIGN KEY (rule_id) REFERENCES article_rules(id) ON DELETE CASCADE,
			FOREIGN KEY (feed_id) REFERENCES feeds(id) ON DELETE CASCADE
		)
	`); err != nil {
		return fmt.Errorf("create article_rule_feeds table: %w", err)
	}

	if _, err := db.Exec(`
		CREATE INDEX IF NOT EXISTS idx_article_rule_feeds_feed
			ON article_rule_feeds(feed_id)
	`); err != nil {
		return fmt.Errorf("create idx_article_rule_feeds_feed: %w", err)
	}

	// Migration 22: Entry full-text search (search_text column + trigram FTS index).
	if err := migrateEntrySearch(db); err != nil {
		return fmt.Errorf("migrate entry search: %w", err)
	}

	// Migration 23: Canonical link hash + content fingerprint for entry deduplication.
	if err := migrateEntryCanonicalIdentity(db); err != nil {
		return fmt.Errorf("migrate entry canonical identity: %w", err)
	}

	return nil
}

// migrateEntrySearch 为文章搜索补齐 search_text 列、trigram FTS 索引与同步触发器。
// 旧版 entries_fts 使用 unicode61 且直接索引 HTML 内容，无法满足中文与纯文本搜索，
// 因此首次升级时整体重建索引并回填历史数据。
func migrateEntrySearch(db *sql.DB) error {
	hasSearchText, err := hasColumn(db, "entries", "search_text")
	if err != nil {
		return fmt.Errorf("check search_text column: %w", err)
	}
	if !hasSearchText {
		if _, err := db.Exec(`ALTER TABLE entries ADD COLUMN search_text TEXT NOT NULL DEFAULT ''`); err != nil {
			return fmt.Errorf("add search_text column: %w", err)
		}
	}

	ftsReady, err := hasColumn(db, "entries_fts", "search_text")
	if err != nil {
		return fmt.Errorf("check entries_fts schema: %w", err)
	}

	if !ftsReady {
		for _, trigger := range []string{"entries_ai", "entries_ad", "entries_au"} {
			if _, err := db.Exec(`DROP TRIGGER IF EXISTS ` + trigger); err != nil {
				return fmt.Errorf("drop %s trigger: %w", trigger, err)
			}
		}
		if _, err := db.Exec(`DROP TABLE IF EXISTS entries_fts`); err != nil {
			return fmt.Errorf("drop entries_fts: %w", err)
		}
		if _, err := db.Exec(entrySearchFTSSchema); err != nil {
			return fmt.Errorf("create entries_fts: %w", err)
		}
		if err := backfillEntrySearchText(db); err != nil {
			return err
		}
		if err := rebuildEntrySearchIndex(db); err != nil {
			return err
		}
	} else {
		// 异常中断后的恢复路径：补回缺失的搜索文本，并在索引不完整时重建
		needsBackfill, err := entrySearchNeedsBackfill(db)
		if err != nil {
			return err
		}
		indexIncomplete, err := entrySearchIndexIncomplete(db)
		if err != nil {
			return err
		}
		if needsBackfill || indexIncomplete {
			if needsBackfill {
				if err := backfillEntrySearchText(db); err != nil {
					return err
				}
			}
			if err := rebuildEntrySearchIndex(db); err != nil {
				return err
			}
		}
	}

	return recreateEntrySearchTriggers(db)
}

// entrySearchIndexIncomplete 判断 FTS 索引行数是否与文章总数不一致。
func entrySearchIndexIncomplete(db *sql.DB) (bool, error) {
	var entryCount, indexCount int64
	if err := db.QueryRow(`SELECT COUNT(*) FROM entries`).Scan(&entryCount); err != nil {
		return false, fmt.Errorf("count entries: %w", err)
	}
	if err := db.QueryRow(`SELECT COUNT(*) FROM entries_fts`).Scan(&indexCount); err != nil {
		return false, fmt.Errorf("count entries_fts: %w", err)
	}
	return entryCount != indexCount, nil
}

// entrySearchNeedsBackfill 判断是否还有带标题或正文但缺少搜索文本的文章。
func entrySearchNeedsBackfill(db *sql.DB) (bool, error) {
	var exists int
	err := db.QueryRow(`
		SELECT EXISTS(
			SELECT 1 FROM entries
			WHERE search_text = ''
			  AND (
			    (title IS NOT NULL AND TRIM(title) <> '')
			    OR (content IS NOT NULL AND TRIM(content) <> '')
			  )
			LIMIT 1
		)`).Scan(&exists)
	if err != nil {
		return false, fmt.Errorf("check entry search backfill: %w", err)
	}
	return exists == 1, nil
}

// rebuildEntrySearchIndex 全量重建文章搜索索引。
func rebuildEntrySearchIndex(db *sql.DB) error {
	tx, err := db.Begin()
	if err != nil {
		return fmt.Errorf("begin entries_fts rebuild tx: %w", err)
	}
	if _, err := tx.Exec(`DELETE FROM entries_fts`); err != nil {
		_ = tx.Rollback()
		return fmt.Errorf("clear entries_fts: %w", err)
	}
	if _, err := tx.Exec(`INSERT INTO entries_fts(rowid, title, search_text)
		SELECT id, COALESCE(title, ''), search_text FROM entries`); err != nil {
		_ = tx.Rollback()
		return fmt.Errorf("rebuild entries_fts: %w", err)
	}
	if err := tx.Commit(); err != nil {
		return fmt.Errorf("commit entries_fts rebuild tx: %w", err)
	}
	return nil
}

// recreateEntrySearchTriggers 重建搜索相关的触发器，保证定义与当前索引结构一致。
func recreateEntrySearchTriggers(db *sql.DB) error {
	for _, trigger := range []string{"entries_ai", "entries_ad", "entries_au"} {
		if _, err := db.Exec(`DROP TRIGGER IF EXISTS ` + trigger); err != nil {
			return fmt.Errorf("drop %s trigger: %w", trigger, err)
		}
	}
	if _, err := db.Exec(entrySearchTriggersSchema); err != nil {
		return fmt.Errorf("create entry search triggers: %w", err)
	}
	return nil
}

// backfillEntrySearchText 按 id 分批回填历史文章的搜索文本。
func backfillEntrySearchText(db *sql.DB) error {
	const batchSize = 500

	lastID := int64(0)
	for {
		rows, err := db.Query(
			`SELECT id, title, content FROM entries WHERE id > ? ORDER BY id LIMIT ?`,
			lastID,
			batchSize,
		)
		if err != nil {
			return fmt.Errorf("query entries for search backfill: %w", err)
		}

		type entryRecord struct {
			id      int64
			title   sql.NullString
			content sql.NullString
		}

		records := make([]entryRecord, 0, batchSize)
		for rows.Next() {
			var record entryRecord
			if err := rows.Scan(&record.id, &record.title, &record.content); err != nil {
				_ = rows.Close()
				return fmt.Errorf("scan entry for search backfill: %w", err)
			}
			records = append(records, record)
		}
		if err := rows.Err(); err != nil {
			_ = rows.Close()
			return fmt.Errorf("iterate entries for search backfill: %w", err)
		}
		_ = rows.Close()

		if len(records) == 0 {
			return nil
		}

		tx, err := db.Begin()
		if err != nil {
			return fmt.Errorf("begin search backfill tx: %w", err)
		}
		for _, record := range records {
			searchText := searchtext.Build(
				nullStringPtr(record.title),
				nullStringPtr(record.content),
			)
			if _, err := tx.Exec(
				`UPDATE entries SET search_text = ? WHERE id = ?`,
				searchText,
				record.id,
			); err != nil {
				_ = tx.Rollback()
				return fmt.Errorf("backfill search_text: %w", err)
			}
			lastID = record.id
		}
		if err := tx.Commit(); err != nil {
			return fmt.Errorf("commit search backfill tx: %w", err)
		}

		if len(records) < batchSize {
			return nil
		}
	}
}

func nullStringPtr(value sql.NullString) *string {
	if !value.Valid {
		return nil
	}
	result := value.String
	return &result
}

type dedupeEntry struct {
	id        int64
	feedID    int64
	url       string
	title     string
	content   string
	read      int
	starred   int
	updatedAt time.Time
}

type dedupeGroup struct {
	keep       dedupeEntry
	hasKeep    bool
	readMax    int
	starredMax int
	duplicate  []int64
}

func migrateEntryHashDeduplication(db *sql.DB) error {
	var hashIndexCount int
	if err := db.QueryRow(
		`SELECT COUNT(*) FROM sqlite_master WHERE type = 'index' AND name = 'idx_entries_feed_hash'`,
	).Scan(&hashIndexCount); err != nil {
		return fmt.Errorf("check idx_entries_feed_hash: %w", err)
	}
	if hashIndexCount > 0 {
		return nil
	}

	exists, err := hasColumn(db, "entries", "hash")
	if err != nil {
		return fmt.Errorf("check hash column: %w", err)
	}
	if !exists {
		if _, err := db.Exec(`ALTER TABLE entries ADD COLUMN hash TEXT NOT NULL DEFAULT ''`); err != nil {
			return fmt.Errorf("add hash column: %w", err)
		}
	}

	tx, err := db.Begin()
	if err != nil {
		return fmt.Errorf("begin tx: %w", err)
	}
	defer func() { _ = tx.Rollback() }()

	if err := mergeHistoricalEntries(tx); err != nil {
		return err
	}
	if err := backfillEntryHash(tx); err != nil {
		return err
	}
	if err := ensureHashUniqueness(tx); err != nil {
		return err
	}
	if _, err := tx.Exec(`DROP INDEX IF EXISTS idx_entries_feed_url`); err != nil {
		return fmt.Errorf("drop idx_entries_feed_url: %w", err)
	}
	if _, err := tx.Exec(`CREATE UNIQUE INDEX IF NOT EXISTS idx_entries_feed_hash ON entries(feed_id, hash)`); err != nil {
		return fmt.Errorf("create idx_entries_feed_hash: %w", err)
	}

	if err := tx.Commit(); err != nil {
		return fmt.Errorf("commit tx: %w", err)
	}
	return nil
}

func hasColumn(db *sql.DB, table string, column string) (bool, error) {
	var count int
	if err := db.QueryRow(
		fmt.Sprintf(`SELECT COUNT(*) FROM pragma_table_info('%s') WHERE name = ?`, table),
		column,
	).Scan(&count); err != nil {
		return false, err
	}
	return count > 0, nil
}

func mergeHistoricalEntries(tx *sql.Tx) error {
	rows, err := tx.Query(`
		SELECT id, feed_id, url, title, content, read, starred, updated_at
		FROM entries
	`)
	if err != nil {
		return fmt.Errorf("query entries for merge: %w", err)
	}
	defer rows.Close()

	groups := make(map[string]*dedupeGroup)
	for rows.Next() {
		var (
			entry        dedupeEntry
			urlStr       sql.NullString
			titleStr     sql.NullString
			contentStr   sql.NullString
			updatedAtStr string
		)
		if err := rows.Scan(
			&entry.id,
			&entry.feedID,
			&urlStr,
			&titleStr,
			&contentStr,
			&entry.read,
			&entry.starred,
			&updatedAtStr,
		); err != nil {
			return fmt.Errorf("scan entry for merge: %w", err)
		}
		entry.url = strings.TrimSpace(urlStr.String)
		entry.title = strings.TrimSpace(titleStr.String)
		entry.content = strings.TrimSpace(contentStr.String)
		entry.updatedAt, _ = time.Parse(time.RFC3339, updatedAtStr)

		groupKey := fmt.Sprintf("%d|%s", entry.feedID, legacyMergeKey(entry))
		group, ok := groups[groupKey]
		if !ok {
			groups[groupKey] = &dedupeGroup{
				keep:       entry,
				hasKeep:    true,
				readMax:    entry.read,
				starredMax: entry.starred,
			}
			continue
		}

		if entry.read > group.readMax {
			group.readMax = entry.read
		}
		if entry.starred > group.starredMax {
			group.starredMax = entry.starred
		}

		if chooseAsKeep(entry, group.keep) {
			group.duplicate = append(group.duplicate, group.keep.id)
			group.keep = entry
		} else {
			group.duplicate = append(group.duplicate, entry.id)
		}
	}
	if err := rows.Err(); err != nil {
		return fmt.Errorf("iterate entries for merge: %w", err)
	}

	for _, group := range groups {
		if !group.hasKeep || len(group.duplicate) == 0 {
			continue
		}

		if group.keep.read != group.readMax || group.keep.starred != group.starredMax {
			if _, err := tx.Exec(
				`UPDATE entries SET read = ?, starred = ? WHERE id = ?`,
				group.readMax,
				group.starredMax,
				group.keep.id,
			); err != nil {
				return fmt.Errorf("update merged entry state: %w", err)
			}
		}

		for _, duplicateID := range group.duplicate {
			if err := moveEntryCaches(tx, duplicateID, group.keep.id); err != nil {
				return err
			}
		}

		if err := deleteEntriesByID(tx, group.duplicate); err != nil {
			return err
		}
	}

	return nil
}

func moveEntryCaches(tx *sql.Tx, fromID int64, toID int64) error {
	tables := []string{"ai_summaries", "ai_translations", "ai_list_translations"}
	for _, table := range tables {
		updateQuery := fmt.Sprintf(`UPDATE OR IGNORE %s SET entry_id = ? WHERE entry_id = ?`, table)
		if _, err := tx.Exec(updateQuery, toID, fromID); err != nil {
			return fmt.Errorf("update %s entry_id: %w", table, err)
		}

		deleteQuery := fmt.Sprintf(`DELETE FROM %s WHERE entry_id = ?`, table)
		if _, err := tx.Exec(deleteQuery, fromID); err != nil {
			return fmt.Errorf("delete %s duplicate rows: %w", table, err)
		}
	}
	return nil
}

func deleteEntriesByID(tx *sql.Tx, ids []int64) error {
	if len(ids) == 0 {
		return nil
	}

	// Keep each statement below SQLite variable limits.
	const batchSize = 500
	for start := 0; start < len(ids); start += batchSize {
		end := start + batchSize
		if end > len(ids) {
			end = len(ids)
		}

		chunk := ids[start:end]
		args := make([]interface{}, 0, len(chunk))
		placeholders := make([]string, 0, len(chunk))
		for _, id := range chunk {
			args = append(args, id)
			placeholders = append(placeholders, "?")
		}

		query := fmt.Sprintf(`DELETE FROM entries WHERE id IN (%s)`, strings.Join(placeholders, ","))
		if _, err := tx.Exec(query, args...); err != nil {
			return fmt.Errorf("delete duplicate entries: %w", err)
		}
	}
	return nil
}

func backfillEntryHash(tx *sql.Tx) error {
	rows, err := tx.Query(`SELECT id, url, title, content FROM entries WHERE hash = ''`)
	if err != nil {
		return fmt.Errorf("query entries for hash backfill: %w", err)
	}
	defer rows.Close()

	for rows.Next() {
		var (
			id       int64
			urlStr   sql.NullString
			titleStr sql.NullString
			bodyStr  sql.NullString
		)
		if err := rows.Scan(&id, &urlStr, &titleStr, &bodyStr); err != nil {
			return fmt.Errorf("scan entry for hash backfill: %w", err)
		}

		link := strings.TrimSpace(urlStr.String)
		title := strings.TrimSpace(titleStr.String)
		content := strings.TrimSpace(bodyStr.String)

		var hash string
		switch {
		case link != "":
			hash = hashHex(link)
		case title != "" || content != "":
			hash = hashHex(title + content)
		default:
			hash = hashHex(fmt.Sprintf("legacy-entry-id:%d", id))
		}

		if _, err := tx.Exec(`UPDATE entries SET hash = ? WHERE id = ?`, hash, id); err != nil {
			return fmt.Errorf("update entry hash: %w", err)
		}
	}

	if err := rows.Err(); err != nil {
		return fmt.Errorf("iterate entries for hash backfill: %w", err)
	}

	return nil
}

func ensureHashUniqueness(tx *sql.Tx) error {
	var duplicateGroups int
	if err := tx.QueryRow(`
		SELECT COUNT(*) FROM (
			SELECT feed_id, hash, COUNT(*) AS c
			FROM entries
			GROUP BY feed_id, hash
			HAVING c > 1
		)
	`).Scan(&duplicateGroups); err != nil {
		return fmt.Errorf("check hash uniqueness: %w", err)
	}
	if duplicateGroups > 0 {
		return fmt.Errorf("found %d duplicate (feed_id, hash) groups after merge", duplicateGroups)
	}
	return nil
}

func legacyMergeKey(entry dedupeEntry) string {
	if entry.url != "" {
		return "url:" + urlutil.StripFragment(entry.url)
	}
	if entry.title != "" || entry.content != "" {
		return "content:" + entry.title + "\n" + entry.content
	}
	return fmt.Sprintf("id:%d", entry.id)
}

func chooseAsKeep(candidate dedupeEntry, current dedupeEntry) bool {
	if candidate.updatedAt.After(current.updatedAt) {
		return true
	}
	if candidate.updatedAt.Equal(current.updatedAt) && candidate.id > current.id {
		return true
	}
	return false
}

func hashHex(input string) string {
	return hashutil.SHA256Hex(input)
}

// entryContentHashIndex 同时是迁移 23 的幂等标记：索引存在说明迁移已完成。
const entryContentHashIndex = "idx_entries_feed_content_hash"

// migrateEntryCanonicalIdentity 归一化链接哈希并补充内容指纹。
// 历史数据里同一篇文章可能因为等价链接（重复斜杠、追踪参数、discuz 多种帖子路由）
// 算出不同哈希而重复入库，这里重算哈希并就地合并重复行。
func migrateEntryCanonicalIdentity(db *sql.DB) error {
	var indexCount int
	if err := db.QueryRow(
		`SELECT COUNT(*) FROM sqlite_master WHERE type = 'index' AND name = ?`,
		entryContentHashIndex,
	).Scan(&indexCount); err != nil {
		return fmt.Errorf("check %s: %w", entryContentHashIndex, err)
	}
	if indexCount > 0 {
		return nil
	}

	hasContentHash, err := hasColumn(db, "entries", "content_hash")
	if err != nil {
		return fmt.Errorf("check content_hash column: %w", err)
	}
	if !hasContentHash {
		if _, err := db.Exec(`ALTER TABLE entries ADD COLUMN content_hash TEXT NOT NULL DEFAULT ''`); err != nil {
			return fmt.Errorf("add content_hash column: %w", err)
		}
	}

	tx, err := db.Begin()
	if err != nil {
		return fmt.Errorf("begin tx: %w", err)
	}
	defer func() { _ = tx.Rollback() }()

	// 唯一索引会阻止哈希重算后的就地合并，先删除并在合并完成后重建。
	if _, err := tx.Exec(`DROP INDEX IF EXISTS idx_entries_feed_hash`); err != nil {
		return fmt.Errorf("drop idx_entries_feed_hash: %w", err)
	}
	if err := rewriteEntryHashes(tx); err != nil {
		return err
	}
	if err := mergeEntryGroups(tx, "hash"); err != nil {
		return fmt.Errorf("merge duplicate hash: %w", err)
	}
	if err := backfillEntryContentHash(tx); err != nil {
		return err
	}
	if err := mergeEntryGroups(tx, "content_hash"); err != nil {
		return fmt.Errorf("merge duplicate content: %w", err)
	}
	if err := ensureHashUniqueness(tx); err != nil {
		return err
	}
	if _, err := tx.Exec(`CREATE UNIQUE INDEX IF NOT EXISTS idx_entries_feed_hash ON entries(feed_id, hash)`); err != nil {
		return fmt.Errorf("create idx_entries_feed_hash: %w", err)
	}
	if _, err := tx.Exec(`CREATE INDEX IF NOT EXISTS ` + entryContentHashIndex + ` ON entries(feed_id, content_hash)`); err != nil {
		return fmt.Errorf("create %s: %w", entryContentHashIndex, err)
	}

	if err := tx.Commit(); err != nil {
		return fmt.Errorf("commit tx: %w", err)
	}
	return nil
}

// rewriteEntryHashes 只重算可证明由原始链接派生的哈希（hash == SHA256(url)），
// GUID 派生等其它来源的哈希保持不动，避免下次刷新因身份键变化而重复插入。
func rewriteEntryHashes(tx *sql.Tx) error {
	rows, err := tx.Query(`SELECT id, url, hash FROM entries WHERE url IS NOT NULL AND url <> ''`)
	if err != nil {
		return fmt.Errorf("query entries for hash rewrite: %w", err)
	}

	type hashUpdate struct {
		id   int64
		hash string
	}
	updates := make([]hashUpdate, 0)

	for rows.Next() {
		var (
			id      int64
			rawURL  string
			oldHash string
		)
		if err := rows.Scan(&id, &rawURL, &oldHash); err != nil {
			_ = rows.Close()
			return fmt.Errorf("scan entry for hash rewrite: %w", err)
		}
		if oldHash != hashHex(rawURL) {
			continue
		}
		normalizedURL := urlutil.NormalizeForHash(rawURL)
		if normalizedURL == "" {
			continue
		}
		newHash := hashHex(normalizedURL)
		if newHash == oldHash {
			continue
		}
		updates = append(updates, hashUpdate{id: id, hash: newHash})
	}
	if err := rows.Err(); err != nil {
		_ = rows.Close()
		return fmt.Errorf("iterate entries for hash rewrite: %w", err)
	}
	_ = rows.Close()

	for _, update := range updates {
		if _, err := tx.Exec(`UPDATE entries SET hash = ? WHERE id = ?`, update.hash, update.id); err != nil {
			return fmt.Errorf("rewrite entry hash: %w", err)
		}
	}
	return nil
}

// mergeEntryGroups 合并同一订阅源内判重键相同的重复文章，
// 保留最新一条，并把已读/加星状态与 AI 缓存迁移到保留行。
// keyColumn 只允许本文件内的常量列名（hash / content_hash），不接受外部输入。
func mergeEntryGroups(tx *sql.Tx, keyColumn string) error {
	rows, err := tx.Query(fmt.Sprintf(`
		SELECT feed_id, %s
		FROM entries
		WHERE %s <> ''
		GROUP BY feed_id, %s
		HAVING COUNT(*) > 1
	`, keyColumn, keyColumn, keyColumn))
	if err != nil {
		return fmt.Errorf("query duplicate %s groups: %w", keyColumn, err)
	}

	type duplicateKey struct {
		feedID int64
		hash   string
	}
	keys := make([]duplicateKey, 0)

	for rows.Next() {
		var key duplicateKey
		if err := rows.Scan(&key.feedID, &key.hash); err != nil {
			_ = rows.Close()
			return fmt.Errorf("scan duplicate %s group: %w", keyColumn, err)
		}
		keys = append(keys, key)
	}
	if err := rows.Err(); err != nil {
		_ = rows.Close()
		return fmt.Errorf("iterate duplicate %s groups: %w", keyColumn, err)
	}
	_ = rows.Close()

	for _, key := range keys {
		if err := mergeEntryGroup(tx, key.feedID, keyColumn, key.hash); err != nil {
			return err
		}
	}
	return nil
}

func mergeEntryGroup(tx *sql.Tx, feedID int64, keyColumn string, keyValue string) error {
	rows, err := tx.Query(
		fmt.Sprintf(`SELECT id, read, starred, updated_at FROM entries WHERE feed_id = ? AND %s = ?`, keyColumn),
		feedID,
		keyValue,
	)
	if err != nil {
		return fmt.Errorf("query duplicate %s group entries: %w", keyColumn, err)
	}

	var (
		keeper       dedupeEntry
		hasKeeper    bool
		readMax      int
		starredMax   int
		duplicateIDs []int64
	)

	for rows.Next() {
		var (
			entry        dedupeEntry
			updatedAtStr string
		)
		if err := rows.Scan(&entry.id, &entry.read, &entry.starred, &updatedAtStr); err != nil {
			_ = rows.Close()
			return fmt.Errorf("scan duplicate %s group entry: %w", keyColumn, err)
		}
		entry.updatedAt, _ = time.Parse(time.RFC3339Nano, updatedAtStr)

		if !hasKeeper {
			keeper = entry
			hasKeeper = true
			readMax = entry.read
			starredMax = entry.starred
			continue
		}

		if entry.read > readMax {
			readMax = entry.read
		}
		if entry.starred > starredMax {
			starredMax = entry.starred
		}
		if chooseAsKeep(entry, keeper) {
			duplicateIDs = append(duplicateIDs, keeper.id)
			keeper = entry
			continue
		}
		duplicateIDs = append(duplicateIDs, entry.id)
	}
	if err := rows.Err(); err != nil {
		_ = rows.Close()
		return fmt.Errorf("iterate duplicate %s group entries: %w", keyColumn, err)
	}
	_ = rows.Close()

	if len(duplicateIDs) == 0 {
		return nil
	}

	if keeper.read != readMax || keeper.starred != starredMax {
		if _, err := tx.Exec(
			`UPDATE entries SET read = ?, starred = ? WHERE id = ?`,
			readMax,
			starredMax,
			keeper.id,
		); err != nil {
			return fmt.Errorf("update merged entry state: %w", err)
		}
	}

	for _, duplicateID := range duplicateIDs {
		if err := moveEntryCaches(tx, duplicateID, keeper.id); err != nil {
			return err
		}
	}

	return deleteEntriesByID(tx, duplicateIDs)
}

// backfillEntryContentHash 为历史文章补齐内容指纹，空正文保持为空串。
func backfillEntryContentHash(tx *sql.Tx) error {
	rows, err := tx.Query(`SELECT id, title, content, published_at FROM entries WHERE content_hash = ''`)
	if err != nil {
		return fmt.Errorf("query entries for content hash backfill: %w", err)
	}

	type contentHashUpdate struct {
		id   int64
		hash string
	}
	updates := make([]contentHashUpdate, 0)

	for rows.Next() {
		var (
			id          int64
			title       sql.NullString
			content     sql.NullString
			publishedAt sql.NullString
		)
		if err := rows.Scan(&id, &title, &content, &publishedAt); err != nil {
			_ = rows.Close()
			return fmt.Errorf("scan entry for content hash backfill: %w", err)
		}
		contentHash := hashutil.EntryContentHash(title.String, parsePublishedAt(publishedAt), content.String)
		if contentHash == "" {
			continue
		}
		updates = append(updates, contentHashUpdate{id: id, hash: contentHash})
	}
	if err := rows.Err(); err != nil {
		_ = rows.Close()
		return fmt.Errorf("iterate entries for content hash backfill: %w", err)
	}
	_ = rows.Close()

	for _, update := range updates {
		if _, err := tx.Exec(`UPDATE entries SET content_hash = ? WHERE id = ?`, update.hash, update.id); err != nil {
			return fmt.Errorf("update entry content hash: %w", err)
		}
	}
	return nil
}

// parsePublishedAt 解析历史文章的发布时间，用于计算内容指纹。
// 解析失败按「没有发布时间」处理，与运行期缺失发布时间的语义一致。
func parsePublishedAt(value sql.NullString) *time.Time {
	if !value.Valid || strings.TrimSpace(value.String) == "" {
		return nil
	}
	parsed, err := time.Parse(time.RFC3339Nano, value.String)
	if err != nil {
		return nil
	}
	utc := parsed.UTC()
	return &utc
}
