package hashutil

import (
	"crypto/sha256"
	"encoding/hex"
	"strconv"
	"strings"
	"time"
)

// SHA256Hex returns a trimmed-input SHA-256 hash encoded in hex.
func SHA256Hex(input string) string {
	sum := sha256.Sum256([]byte(strings.TrimSpace(input)))
	return hex.EncodeToString(sum[:])
}

// EntryContentHash 计算文章内容指纹（标题 + 发布时间 + 正文），用于链接不稳定
// 或链接不同但内容完全一致时兜底判重。
// 正文为空时返回空串，避免仅凭标题把不同文章判为重复；
// publishedAt 为 nil 时忽略时间维度（例如订阅源本身不提供发布时间）。
func EntryContentHash(title string, publishedAt *time.Time, content string) string {
	body := collapseWhitespace(content)
	if body == "" {
		return ""
	}

	stamp := ""
	if publishedAt != nil {
		stamp = strconv.FormatInt(publishedAt.UTC().Unix(), 10)
	}
	return SHA256Hex(collapseWhitespace(title) + "\x00" + stamp + "\x00" + body)
}

// collapseWhitespace 压缩连续空白，避免排版差异影响内容指纹。
func collapseWhitespace(value string) string {
	return strings.Join(strings.Fields(value), " ")
}
