/**
 * 图片 URL 工具。
 *
 * 图片由浏览器直接加载（不再经后端代理），因此这里只做纯 URL 解析：
 * 把 RSS 内容里的相对地址按文章链接还原成绝对地址。
 */

/** 浏览器直连图片时依次尝试的 Referer 策略 */
export const IMAGE_REFERRER_POLICIES = [
  // 不发送 Referer：多数防盗链只拦截白名单以外的 Referer，空 Referer 往往放行
  "no-referrer",
  // 只发送本站 origin，满足要求 Referer 必须存在但允许跨域的站点
  "origin",
] as const;

/**
 * Convert relative URL to absolute URL based on the article link.
 * Returns null when the URL cannot be resolved safely.
 */
export function toAbsoluteUrl(
  url: string,
  baseUrl?: string,
): string | null {
  if (!url) return null;

  // Inline resources are already loadable as-is
  if (url.startsWith("data:") || url.startsWith("blob:")) {
    return url;
  }

  // Keep absolute URLs untouched to avoid re-encoding them
  if (/^https?:\/\//i.test(url)) {
    return url;
  }

  // Protocol-relative URL
  if (url.startsWith("//")) {
    return `https:${url}`;
  }

  // Relative URL needs a base to resolve against
  if (!baseUrl) return null;

  try {
    const parsed = new URL(url, baseUrl);
    if (parsed.protocol !== "http:" && parsed.protocol !== "https:") {
      return null;
    }
    return parsed.toString();
  } catch {
    return null;
  }
}

/**
 * Resolve every candidate in a srcset attribute against the article link.
 * Source descriptors (e.g. `2x`, `640w`) are preserved.
 */
export function resolveSrcset(srcset: string, baseUrl?: string): string {
  return srcset
    .split(",")
    .map((entry) => {
      const parts = entry.trim().split(/\s+/);
      if (parts[0]) {
        parts[0] = toAbsoluteUrl(parts[0], baseUrl) ?? parts[0];
      }
      return parts.join(" ");
    })
    .join(", ");
}

/**
 * 生成带重试标记的 URL。
 *
 * 只修改 referrerPolicy 不会让浏览器重新请求图片，因此用 fragment 制造一个新 URL
 * 强制重试；fragment 不参与 HTTP 请求，不会影响源站。
 */
export function withRetryFragment(url: string, attempt: number): string {
  // data:/blob: 无法通过 fragment 触发新请求
  if (
    attempt <= 0 ||
    !url ||
    url.startsWith("data:") ||
    url.startsWith("blob:")
  ) {
    return url;
  }
  const base = url.split("#")[0];
  return `${base}#gist-image-retry-${attempt}`;
}
