package urlutil_test

import (
	"testing"

	"gist/backend/internal/urlutil"

	"github.com/stretchr/testify/require"
)

func TestNormalizeForHash_StripsFragmentAndDefaultPort(t *testing.T) {
	require.Equal(
		t,
		"https://example.com/post/1?page=2",
		urlutil.NormalizeForHash(" HTTPS://Example.com:443/post/1?page=2#comments "),
	)
	require.Equal(
		t,
		"http://example.com/a",
		urlutil.NormalizeForHash("http://example.com:80/a"),
	)
}

func TestNormalizeForHash_CollapsesDuplicateSlashes(t *testing.T) {
	require.Equal(
		t,
		"https://example.com/a/b/c.html",
		urlutil.NormalizeForHash("https://example.com//a//b//c.html"),
	)
	require.Equal(
		t,
		"https://example.com/a/b",
		urlutil.NormalizeForHash("https://example.com//a///b/"),
	)
}

func TestNormalizeForHash_DropsTrackingParamsAndSortsQuery(t *testing.T) {
	require.Equal(
		t,
		"https://example.com/post?a=1&b=2",
		urlutil.NormalizeForHash("https://example.com/post?b=2&utm_source=rss&a=1&fbclid=xyz"),
	)
}

func TestNormalizeForHash_KeepsMeaningfulQueryOrderIndependent(t *testing.T) {
	first := urlutil.NormalizeForHash("https://example.com/search?q=go&page=2")
	second := urlutil.NormalizeForHash("https://example.com/search?page=2&q=go")
	require.Equal(t, first, second)
}

func TestNormalizeForHash_UnifiesDiscuzThreadRoutes(t *testing.T) {
	expected := "discuz://www.t66y.com/thread/7511015"
	for _, raw := range []string{
		"https://www.t66y.com/read.php?tid=7511015",
		"https://www.t66y.com/read.php?tid=7511015&page=1",
		"https://www.t66y.com//htm_data/2611/5/7511015.html",
		"https://www.t66y.com/htm_data/2611/5/7511015.html",
		"https://www.t66y.com/thread-7511015-1-1.html",
	} {
		require.Equal(t, expected, urlutil.NormalizeForHash(raw), "raw=%s", raw)
	}
}

func TestNormalizeForHash_KeepsDiscuzPageNumber(t *testing.T) {
	require.Equal(
		t,
		"discuz://www.t66y.com/thread/7511015?page=3",
		urlutil.NormalizeForHash("https://www.t66y.com/read.php?tid=7511015&page=3"),
	)
}

func TestNormalizeForHash_EmptyAndRelative(t *testing.T) {
	require.Equal(t, "", urlutil.NormalizeForHash("   "))
	require.Equal(t, "/post/1", urlutil.NormalizeForHash("/post//1"))
}

// 百分号转义的斜杠是路径的一部分，折叠斜杠时不能把它变成真正的路径分隔符。
func TestNormalizeForHash_KeepsEncodedSlash(t *testing.T) {
	encoded := urlutil.NormalizeForHash("https://example.com/a%2Fb")
	plain := urlutil.NormalizeForHash("https://example.com/a/b")
	require.Equal(t, "https://example.com/a%2Fb", encoded)
	require.NotEqual(t, plain, encoded)
}
