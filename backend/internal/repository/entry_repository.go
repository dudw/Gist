//go:generate mockgen -source=$GOFILE -destination=mock/$GOFILE -package=mock
package repository

import (
	"context"
	"database/sql"
	"strings"
	"time"
	"unicode/utf8"

	"gist/backend/internal/model"
	"gist/backend/internal/searchtext"
	"gist/backend/internal/urlutil"
	"gist/backend/pkg/snowflake"
)

// entrySearchMinFTSTermRunes trigram 索引要求查询词至少包含 3 个字符，
// 更短的查询词回退到 LIKE 子串匹配。
const entrySearchMinFTSTermRunes = 3

type EntryListFilter struct {
	FeedID       *int64
	FolderID     *int64
	ContentType  *string
	UnreadOnly   bool
	StarredOnly  bool
	HasThumbnail bool
	Search       string
	Limit        int
	Offset       int
}

type UnreadCount struct {
	FeedID int64
	Count  int
}

type EntryRepository interface {
	GetByID(ctx context.Context, id int64) (model.Entry, error)
	List(ctx context.Context, filter EntryListFilter) ([]model.Entry, error)
	ListUnreadForRule(ctx context.Context, feedIDs []int64, afterID int64, limit int) ([]model.Entry, error)
	UpdateReadStatus(ctx context.Context, id int64, read bool) error
	UpdateManyReadStatus(ctx context.Context, ids []int64, read bool) error
	UpdateStarredStatus(ctx context.Context, id int64, starred bool) error
	UpdateReadableContent(ctx context.Context, id int64, content string) error
	MarkAllAsRead(ctx context.Context, feedID *int64, folderID *int64, contentType *string) error
	GetAllUnreadCounts(ctx context.Context) ([]UnreadCount, error)
	GetStarredCount(ctx context.Context) (int, error)
	CreateOrUpdate(ctx context.Context, entry model.Entry) error
	ExistsByHash(ctx context.Context, feedID int64, hash string) (bool, error)
	ExistsByLegacyURL(ctx context.Context, feedID int64, rawURL string, hash string) (bool, error)
	ClearAllReadableContent(ctx context.Context) (int64, error)
	DeleteUnstarred(ctx context.Context) (int64, error)
}

type entryRepository struct {
	db dbtx
}

func NewEntryRepository(db dbtx) EntryRepository {
	return &entryRepository{db: db}
}

func (r *entryRepository) GetByID(ctx context.Context, id int64) (model.Entry, error) {
	row := r.db.QueryRowContext(
		ctx,
		`SELECT id, feed_id, hash, title, url, content, readable_content, thumbnail_url, author, published_at, read, starred, created_at, updated_at
		 FROM entries WHERE id = ?`,
		id,
	)
	return scanEntry(row)
}

func (r *entryRepository) List(ctx context.Context, filter EntryListFilter) ([]model.Entry, error) {
	var args []interface{}
	search := strings.TrimSpace(filter.Search)

	useFTS := false
	var ftsQuery string
	if search != "" {
		ftsQuery, useFTS = buildEntryFTSQuery(search)
	}

	query := `
		SELECT e.id, e.feed_id, e.hash, e.title, e.url, e.content, e.readable_content, e.thumbnail_url, e.author,
		       e.published_at, e.read, e.starred, e.created_at, e.updated_at
		FROM entries e
	`

	if useFTS {
		query += " INNER JOIN entries_fts ON entries_fts.rowid = e.id"
	}

	var conditions []string
	needFeedsJoin := filter.FolderID != nil || filter.ContentType != nil

	if needFeedsJoin {
		query += " INNER JOIN feeds f ON e.feed_id = f.id"
	}

	if filter.FolderID != nil {
		conditions = append(conditions, "f.folder_id = ?")
		args = append(args, *filter.FolderID)
	}

	if filter.ContentType != nil {
		conditions = append(conditions, "f.type = ?")
		args = append(args, *filter.ContentType)
	}

	if filter.FeedID != nil {
		conditions = append(conditions, "e.feed_id = ?")
		args = append(args, *filter.FeedID)
	}

	if filter.UnreadOnly {
		conditions = append(conditions, "e.read = 0")
	}

	if filter.StarredOnly {
		conditions = append(conditions, "e.starred = 1")
	}

	if filter.HasThumbnail {
		conditions = append(conditions, "e.thumbnail_url IS NOT NULL AND e.thumbnail_url != ''")
	}

	if search != "" {
		if useFTS {
			conditions = append(conditions, "entries_fts MATCH ?")
			args = append(args, ftsQuery)
		} else {
			// 短查询无法使用 trigram 索引，退化为子串匹配
			for _, term := range strings.Fields(search) {
				pattern := entrySearchLikePattern(term)
				conditions = append(conditions, "(e.title LIKE ? ESCAPE '\\' OR e.search_text LIKE ? ESCAPE '\\')")
				args = append(args, pattern, pattern)
			}
		}
	}

	if len(conditions) > 0 {
		query += " WHERE " + strings.Join(conditions, " AND ")
	}

	switch {
	case useFTS:
		query += " ORDER BY bm25(entries_fts, 10.0, 1.0), e.published_at DESC, e.id DESC"
	case search != "":
		query += " ORDER BY CASE WHEN e.title LIKE ? ESCAPE '\\' THEN 0 ELSE 1 END, e.published_at DESC, e.id DESC"
		args = append(args, entrySearchLikePattern(search))
	default:
		query += " ORDER BY e.published_at DESC, e.id DESC"
	}

	if filter.Limit > 0 {
		query += " LIMIT ?"
		args = append(args, filter.Limit)
	}
	if filter.Offset > 0 {
		query += " OFFSET ?"
		args = append(args, filter.Offset)
	}

	rows, err := r.db.QueryContext(ctx, query, args...)
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	var entries []model.Entry
	for rows.Next() {
		entry, err := scanEntry(rows)
		if err != nil {
			return nil, err
		}
		entries = append(entries, entry)
	}

	if err := rows.Err(); err != nil {
		return nil, err
	}

	return entries, nil
}

func boolToInt(value bool) int {
	if value {
		return 1
	}
	return 0
}

// buildEntryFTSQuery 将用户输入转换为 FTS5 查询表达式。
// 每个空白分词都会加双引号按字面量匹配；任一分词少于 3 个字符时返回 false，
// 由调用方回退到 LIKE 子串匹配（trigram 索引无法命中短查询）。
func buildEntryFTSQuery(search string) (string, bool) {
	terms := strings.Fields(search)
	if len(terms) == 0 {
		return "", false
	}

	quoted := make([]string, 0, len(terms))
	for _, term := range terms {
		if utf8.RuneCountInString(term) < entrySearchMinFTSTermRunes {
			return "", false
		}
		quoted = append(quoted, `"`+strings.ReplaceAll(term, `"`, `""`)+`"`)
	}

	return strings.Join(quoted, " AND "), true
}

// entrySearchLikePattern 将搜索词转义为安全的 LIKE 子串模式。
func entrySearchLikePattern(term string) string {
	replacer := strings.NewReplacer(`\`, `\\`, `%`, `\%`, `_`, `\_`)
	return "%" + replacer.Replace(term) + "%"
}

func (r *entryRepository) ListUnreadForRule(ctx context.Context, feedIDs []int64, afterID int64, limit int) ([]model.Entry, error) {
	if len(feedIDs) == 0 {
		return []model.Entry{}, nil
	}
	if limit <= 0 {
		limit = 500
	}

	placeholders := make([]string, len(feedIDs))
	args := make([]interface{}, 0, len(feedIDs)+2)
	for i, feedID := range feedIDs {
		placeholders[i] = "?"
		args = append(args, feedID)
	}
	args = append(args, afterID, limit)

	rows, err := r.db.QueryContext(ctx, `
		SELECT id, feed_id, hash, title, url, content, readable_content, thumbnail_url, author,
		       published_at, read, starred, created_at, updated_at
		FROM entries
		WHERE feed_id IN (`+strings.Join(placeholders, ",")+`)
		  AND read = 0
		  AND id > ?
		ORDER BY id
		LIMIT ?
	`, args...)
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	entries := make([]model.Entry, 0)
	for rows.Next() {
		entry, err := scanEntry(rows)
		if err != nil {
			return nil, err
		}
		entries = append(entries, entry)
	}
	if err := rows.Err(); err != nil {
		return nil, err
	}
	return entries, nil
}

func (r *entryRepository) UpdateReadStatus(ctx context.Context, id int64, read bool) error {
	readInt := boolToInt(read)

	_, err := r.db.ExecContext(
		ctx,
		`UPDATE entries SET read = ?, updated_at = ? WHERE id = ?`,
		readInt,
		formatTime(time.Now()),
		id,
	)
	return err
}

func (r *entryRepository) UpdateManyReadStatus(ctx context.Context, ids []int64, read bool) error {
	if len(ids) == 0 {
		return nil
	}

	readInt := boolToInt(read)

	args := make([]interface{}, 0, len(ids)+2)
	args = append(args, readInt, formatTime(time.Now()))
	placeholders := make([]string, len(ids))
	for i, id := range ids {
		placeholders[i] = "?"
		args = append(args, id)
	}

	_, err := r.db.ExecContext(
		ctx,
		`UPDATE entries SET read = ?, updated_at = ? WHERE id IN (`+strings.Join(placeholders, ",")+")",
		args...,
	)
	return err
}

func (r *entryRepository) MarkAllAsRead(ctx context.Context, feedID *int64, folderID *int64, contentType *string) error {
	now := formatTime(time.Now())

	if folderID != nil {
		_, err := r.db.ExecContext(
			ctx,
			`UPDATE entries SET read = 1, updated_at = ?
			 WHERE feed_id IN (SELECT id FROM feeds WHERE folder_id = ?) AND read = 0`,
			now,
			*folderID,
		)
		return err
	}

	if feedID != nil {
		_, err := r.db.ExecContext(
			ctx,
			`UPDATE entries SET read = 1, updated_at = ? WHERE feed_id = ? AND read = 0`,
			now,
			*feedID,
		)
		return err
	}

	// Mark all as read with optional content type filter
	if contentType != nil {
		_, err := r.db.ExecContext(
			ctx,
			`UPDATE entries SET read = 1, updated_at = ?
			 WHERE feed_id IN (SELECT id FROM feeds WHERE type = ?) AND read = 0`,
			now,
			*contentType,
		)
		return err
	}

	// Mark all as read without filter
	_, err := r.db.ExecContext(
		ctx,
		`UPDATE entries SET read = 1, updated_at = ? WHERE read = 0`,
		now,
	)
	return err
}

func (r *entryRepository) GetAllUnreadCounts(ctx context.Context) ([]UnreadCount, error) {
	rows, err := r.db.QueryContext(
		ctx,
		`SELECT feed_id, COUNT(*) as count FROM entries WHERE read = 0 GROUP BY feed_id`,
	)
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	var counts []UnreadCount
	for rows.Next() {
		var uc UnreadCount
		if err := rows.Scan(&uc.FeedID, &uc.Count); err != nil {
			return nil, err
		}
		counts = append(counts, uc)
	}

	if err := rows.Err(); err != nil {
		return nil, err
	}

	return counts, nil
}

// entryScanner is an interface for scanning entry rows.
type entryScanner interface {
	Scan(dest ...interface{}) error
}

func scanEntry(s entryScanner) (model.Entry, error) {
	var e model.Entry
	var publishedAt sql.NullString
	var createdAt, updatedAt string
	var readInt, starredInt int

	err := s.Scan(
		&e.ID, &e.FeedID, &e.Hash, &e.Title, &e.URL, &e.Content, &e.ReadableContent, &e.ThumbnailURL, &e.Author,
		&publishedAt, &readInt, &starredInt, &createdAt, &updatedAt,
	)
	if err != nil {
		return model.Entry{}, err
	}

	e.Read = readInt == 1
	e.Starred = starredInt == 1
	if publishedAt.Valid {
		e.PublishedAt = parseTimePtr(publishedAt.String)
	}
	e.CreatedAt, _ = parseTime(createdAt)
	e.UpdatedAt, _ = parseTime(updatedAt)

	return e, nil
}

func parseTimePtr(s string) *time.Time {
	if s == "" {
		return nil
	}
	t, _ := parseTime(s)
	return &t
}

func (r *entryRepository) CreateOrUpdate(ctx context.Context, entry model.Entry) error {
	id := snowflake.NextID()
	now := formatTime(time.Now())
	searchText := searchtext.Build(entry.Title, entry.Content)

	var publishedAt interface{}
	if entry.PublishedAt != nil {
		publishedAt = formatTime(*entry.PublishedAt)
	}

	// Compatibility path:
	// legacy databases might still carry URL-derived hashes after migration.
	// If we receive the same URL with a new GUID-derived hash, upgrade that row in place
	// so the first refresh after migration doesn't create duplicates.
	if entry.URL != nil && *entry.URL != "" && entry.Hash != "" {
		normalizedURL := urlutil.StripFragment(*entry.URL)
		result, err := r.db.ExecContext(
			ctx,
			`UPDATE entries SET
			   hash = ?,
			   title = ?,
			   url = ?,
			   content = ?,
			   search_text = ?,
			   thumbnail_url = ?,
			   author = ?,
			   published_at = COALESCE(entries.published_at, ?),
			   updated_at = ?
			 WHERE id = (
			   SELECT id
			   FROM entries
			   WHERE feed_id = ?
			     AND hash <> ?
			     AND (
			       url = ?
			       OR (CASE WHEN instr(url, '#') > 0 THEN substr(url, 1, instr(url, '#') - 1) ELSE url END) = ?
			     )
			   ORDER BY updated_at DESC, id DESC
			   LIMIT 1
			 )
			   AND NOT EXISTS (
			     SELECT 1
			     FROM entries e2
			     WHERE e2.feed_id = ?
			       AND e2.hash = ?
			       AND e2.id <> entries.id
			   )`,
			entry.Hash,
			entry.Title,
			entry.URL,
			entry.Content,
			searchText,
			entry.ThumbnailURL,
			entry.Author,
			publishedAt,
			now,
			entry.FeedID,
			entry.Hash,
			entry.URL,
			normalizedURL,
			entry.FeedID,
			entry.Hash,
		)
		if err != nil {
			return err
		}
		if affected, err := result.RowsAffected(); err == nil && affected > 0 {
			return nil
		}
	}

	_, err := r.db.ExecContext(
		ctx,
		`INSERT INTO entries (id, feed_id, hash, title, url, content, search_text, thumbnail_url, author, published_at, read, created_at, updated_at)
		 VALUES (?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?)
		 ON CONFLICT(feed_id, hash) DO UPDATE SET
		   title = excluded.title,
		   url = excluded.url,
		   content = excluded.content,
		   search_text = excluded.search_text,
		   thumbnail_url = excluded.thumbnail_url,
		   author = excluded.author,
		   published_at = COALESCE(entries.published_at, excluded.published_at),
		   updated_at = excluded.updated_at`,
		id,
		entry.FeedID,
		entry.Hash,
		entry.Title,
		entry.URL,
		entry.Content,
		searchText,
		entry.ThumbnailURL,
		entry.Author,
		publishedAt,
		boolToInt(entry.Read),
		now,
		now,
	)
	return err
}

func (r *entryRepository) ExistsByHash(ctx context.Context, feedID int64, hash string) (bool, error) {
	var count int
	err := r.db.QueryRowContext(
		ctx,
		`SELECT COUNT(*) FROM entries WHERE feed_id = ? AND hash = ?`,
		feedID,
		hash,
	).Scan(&count)
	if err != nil {
		return false, err
	}
	return count > 0, nil
}

func (r *entryRepository) ExistsByLegacyURL(ctx context.Context, feedID int64, rawURL string, hash string) (bool, error) {
	trimmedURL := strings.TrimSpace(rawURL)
	if trimmedURL == "" {
		return false, nil
	}
	normalizedURL := urlutil.StripFragment(trimmedURL)

	var count int
	err := r.db.QueryRowContext(
		ctx,
		`SELECT COUNT(*) FROM entries
		 WHERE feed_id = ?
		   AND hash <> ?
		   AND (
		     url = ?
		     OR (CASE WHEN instr(url, '#') > 0 THEN substr(url, 1, instr(url, '#') - 1) ELSE url END) = ?
		   )`,
		feedID,
		hash,
		trimmedURL,
		normalizedURL,
	).Scan(&count)
	if err != nil {
		return false, err
	}
	return count > 0, nil
}

func (r *entryRepository) UpdateReadableContent(ctx context.Context, id int64, content string) error {
	_, err := r.db.ExecContext(
		ctx,
		`UPDATE entries SET readable_content = ?, updated_at = ? WHERE id = ?`,
		content,
		formatTime(time.Now()),
		id,
	)
	return err
}

func (r *entryRepository) UpdateStarredStatus(ctx context.Context, id int64, starred bool) error {
	starredInt := 0
	if starred {
		starredInt = 1
	}

	_, err := r.db.ExecContext(
		ctx,
		`UPDATE entries SET starred = ?, updated_at = ? WHERE id = ?`,
		starredInt,
		formatTime(time.Now()),
		id,
	)
	return err
}

func (r *entryRepository) GetStarredCount(ctx context.Context) (int, error) {
	var count int
	err := r.db.QueryRowContext(ctx, `SELECT COUNT(*) FROM entries WHERE starred = 1`).Scan(&count)
	return count, err
}

func (r *entryRepository) ClearAllReadableContent(ctx context.Context) (int64, error) {
	result, err := r.db.ExecContext(ctx, `UPDATE entries SET readable_content = NULL, updated_at = ? WHERE readable_content IS NOT NULL`, formatTime(time.Now()))
	if err != nil {
		return 0, err
	}
	return result.RowsAffected()
}

func (r *entryRepository) DeleteUnstarred(ctx context.Context) (int64, error) {
	result, err := r.db.ExecContext(ctx, `DELETE FROM entries WHERE starred = 0`)
	if err != nil {
		return 0, err
	}
	return result.RowsAffected()
}
