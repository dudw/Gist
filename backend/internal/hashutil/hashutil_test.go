package hashutil_test

import (
	"testing"
	"time"

	"gist/backend/internal/hashutil"

	"github.com/stretchr/testify/require"
)

func TestEntryContentHash_StableAcrossWhitespaceDifferences(t *testing.T) {
	first := hashutil.EntryContentHash("标题", nil, "<p>正文  内容</p>\n")
	second := hashutil.EntryContentHash(" 标题 ", nil, "<p>正文 内容</p>")
	require.NotEmpty(t, first)
	require.Equal(t, first, second)
}

func TestEntryContentHash_DistinguishesDifferentContent(t *testing.T) {
	require.NotEqual(
		t,
		hashutil.EntryContentHash("标题", nil, "正文 A"),
		hashutil.EntryContentHash("标题", nil, "正文 B"),
	)
}

func TestEntryContentHash_EmptyContentReturnsEmpty(t *testing.T) {
	require.Empty(t, hashutil.EntryContentHash("只有标题", nil, "   "))
}

func TestEntryContentHash_DistinguishesDifferentPublishedAt(t *testing.T) {
	first := time.Date(2026, 11, 12, 9, 55, 44, 0, time.UTC)
	later := first.Add(48 * time.Hour)

	require.NotEqual(
		t,
		hashutil.EntryContentHash("标题", &first, "正文"),
		hashutil.EntryContentHash("标题", &later, "正文"),
	)
}

func TestEntryContentHash_IgnoresTimeZoneOffset(t *testing.T) {
	utc := time.Date(2026, 11, 12, 9, 55, 44, 0, time.UTC)
	shanghai := utc.In(time.FixedZone("CST", 8*3600))

	require.Equal(
		t,
		hashutil.EntryContentHash("标题", &utc, "正文"),
		hashutil.EntryContentHash("标题", &shanghai, "正文"),
	)
}
