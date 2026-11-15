package urlutil

import (
	"net/url"
	"strings"
)

// trackingQueryKeys 是只用于渠道归因、不影响文章指向的查询参数，
// 参与去重哈希会把同一篇文章拆成多条记录，因此需要剔除。
var trackingQueryKeys = map[string]struct{}{
	"fbclid":  {},
	"gclid":   {},
	"yclid":   {},
	"spm":     {},
	"scm":     {},
	"ref_src": {},
}

// NormalizeForHash 把链接归一化为稳定的身份字符串，供文章去重哈希使用。
// 只做不影响内容指向的变换：scheme/host 小写、剔除默认端口与追踪参数、
// 折叠路径中的连续斜杠、去掉结尾斜杠与 fragment、查询参数按键排序。
// discuz 站点的多种帖子路由会归并为同一形式（见 discuzThreadKey）。
func NormalizeForHash(raw string) string {
	trimmed := strings.TrimSpace(raw)
	if trimmed == "" {
		return ""
	}

	parsed, err := url.Parse(trimmed)
	if err != nil || parsed.Host == "" {
		return collapseSlashesKeepingScheme(trimmed)
	}

	scheme := strings.ToLower(parsed.Scheme)
	host := strings.ToLower(parsed.Hostname())
	port := normalizePort(scheme, parsed.Port())
	// 使用转义后的路径：parsed.Path 会把 %2F 解码成 /，导致 /a%2Fb 与 /a/b
	// 被当成同一身份，从而误判为重复文章。
	path := normalizePath(parsed.EscapedPath())
	query := normalizeQuery(parsed.RawQuery)

	if threadKey, ok := discuzThreadKey(host, path, parsed.RawQuery); ok {
		return threadKey
	}

	var builder strings.Builder
	if scheme != "" {
		builder.WriteString(scheme)
		builder.WriteString("://")
	}
	builder.WriteString(host)
	if port != "" {
		builder.WriteString(":")
		builder.WriteString(port)
	}
	builder.WriteString(path)
	if query != "" {
		builder.WriteString("?")
		builder.WriteString(query)
	}
	return builder.String()
}

// normalizePort 去掉与 scheme 默认端口等价的显式端口。
func normalizePort(scheme, port string) string {
	if port == "" {
		return ""
	}
	if (scheme == "http" && port == "80") || (scheme == "https" && port == "443") {
		return ""
	}
	return port
}

// normalizePath 折叠连续斜杠并去掉结尾斜杠，空路径统一为根路径。
func normalizePath(path string) string {
	if path == "" {
		return "/"
	}

	collapsed := collapseSlashes(path)
	if collapsed == "/" {
		return collapsed
	}

	collapsed = strings.TrimRight(collapsed, "/")
	if collapsed == "" {
		return "/"
	}
	return collapsed
}

// normalizeQuery 剔除追踪参数并按参数名排序，使参数顺序不同的链接归一到同一形式。
func normalizeQuery(rawQuery string) string {
	if rawQuery == "" {
		return ""
	}

	values, err := url.ParseQuery(rawQuery)
	if err != nil {
		return rawQuery
	}

	filtered := make(url.Values, len(values))
	for key, items := range values {
		if isTrackingQueryKey(key) {
			continue
		}
		filtered[key] = items
	}
	return filtered.Encode()
}

func isTrackingQueryKey(key string) bool {
	lower := strings.ToLower(key)
	if strings.HasPrefix(lower, "utm_") {
		return true
	}
	_, ok := trackingQueryKeys[lower]
	return ok
}

// collapseSlashes 把连续斜杠替换为单个斜杠。
func collapseSlashes(value string) string {
	if !strings.Contains(value, "//") {
		return value
	}

	var builder strings.Builder
	builder.Grow(len(value))
	previousSlash := false
	for _, r := range value {
		if r == '/' {
			if previousSlash {
				continue
			}
			previousSlash = true
		} else {
			previousSlash = false
		}
		builder.WriteRune(r)
	}
	return builder.String()
}

// collapseSlashesKeepingScheme 在无法解析为 URL 时折叠斜杠，但保留 scheme 分隔符。
func collapseSlashesKeepingScheme(raw string) string {
	if idx := strings.Index(raw, "://"); idx >= 0 {
		return raw[:idx+3] + collapseSlashes(raw[idx+3:])
	}
	return collapseSlashes(raw)
}
