// Package searchtext 负责构建文章搜索用的纯文本。
package searchtext

import (
	"strings"

	"golang.org/x/net/html"

	"gist/backend/pkg/sanitizer"
)

// Build 生成用于全文搜索的纯文本：标题 + 去除 HTML 标签后的正文。
// HTML 标签与属性不参与搜索，避免搜索 "div"、"class" 等误命中。
func Build(title, content *string) string {
	parts := make([]string, 0, 2)

	if title != nil {
		if trimmed := strings.TrimSpace(*title); trimmed != "" {
			parts = append(parts, trimmed)
		}
	}

	if content != nil {
		if text := sanitizer.StripTags(*content); text != "" {
			parts = append(parts, html.UnescapeString(text))
		}
	}

	// 归一化空白，保证 FTS 分词与 LIKE 匹配结果稳定
	return strings.Join(strings.Fields(strings.Join(parts, "\n")), " ")
}
