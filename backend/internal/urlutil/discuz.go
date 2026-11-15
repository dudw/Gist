package urlutil

import (
	"net/url"
	"regexp"
	"strings"
)

var (
	// discuzHtmDataPath 匹配 discuz 静态化帖子路径，例如 /htm_data/2611/5/7511015.html
	discuzHtmDataPath = regexp.MustCompile(`^/htm_data/[^/]+/[^/]+/(\d+)\.html$`)
	// discuzRewritePath 匹配 discuz 伪静态帖子路径，例如 /thread-7511015-1-1.html
	discuzRewritePath = regexp.MustCompile(`^/thread-(\d+)-(\d+)-(\d+)\.html$`)
)

// discuzThreadKey 把 discuz 的多种帖子路由归并为同一个身份键。
// 同一个帖子在 RSS 输出中可能交替出现为 read.php?tid=N 与 htm_data/<分类>/<版块>/N.html，
// 链接不同会让哈希不同，进而绕过 (feed_id, hash) 唯一约束。
func discuzThreadKey(host, path, rawQuery string) (string, bool) {
	var threadID, page string

	switch {
	case path == "/read.php":
		values, err := url.ParseQuery(rawQuery)
		if err != nil {
			return "", false
		}
		threadID = strings.TrimSpace(values.Get("tid"))
		page = strings.TrimSpace(values.Get("page"))
	case discuzHtmDataPath.MatchString(path):
		threadID = discuzHtmDataPath.FindStringSubmatch(path)[1]
	case discuzRewritePath.MatchString(path):
		matches := discuzRewritePath.FindStringSubmatch(path)
		threadID = matches[1]
		page = matches[2]
	default:
		return "", false
	}

	if !isDigits(threadID) {
		return "", false
	}

	key := "discuz://" + host + "/thread/" + threadID
	if isDigits(page) && page != "1" {
		key += "?page=" + page
	}
	return key, true
}

func isDigits(value string) bool {
	if value == "" {
		return false
	}
	for _, r := range value {
		if r < '0' || r > '9' {
			return false
		}
	}
	return true
}
