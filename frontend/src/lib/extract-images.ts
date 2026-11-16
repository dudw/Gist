import { toAbsoluteUrl } from "./image-url";
import { isSafeUrl } from "./url";

/**
 * Extract image URLs from HTML content.
 * Resolves relative addresses against the article URL and returns unique absolute URLs.
 */
export function extractImagesFromHtml(
  html: string,
  articleUrl?: string,
): string[] {
  if (!html) return [];

  const doc = new DOMParser().parseFromString(html, "text/html");
  const imgs = doc.querySelectorAll("img");

  const urls = new Set<string>();

  for (const img of imgs) {
    // 用 getAttribute 而不是 img.src：DOMParser 会按当前页面地址解析相对路径
    // 依次尝试常见属性，跳过 data: 占位图，取出懒加载的真实地址
    const src = [
      img.getAttribute("src"),
      img.getAttribute("data-src"),
      img.getAttribute("data-lazy-src"),
    ].find((value) => value && !value.startsWith("data:"));
    if (!src) continue;

    const absoluteUrl = toAbsoluteUrl(src, articleUrl);
    if (absoluteUrl && isSafeUrl(absoluteUrl)) {
      urls.add(absoluteUrl);
    }
  }

  return Array.from(urls);
}

/**
 * Get all images for an entry, combining thumbnailUrl and content images.
 * The browser loads these URLs directly, so they must be absolute.
 */
export function getEntryImages(
  thumbnailUrl?: string,
  content?: string,
  articleUrl?: string,
): string[] {
  const images: string[] = [];

  // Add thumbnail first if it exists
  if (thumbnailUrl) {
    images.push(toAbsoluteUrl(thumbnailUrl, articleUrl) ?? thumbnailUrl);
  }

  // Extract images from content
  if (content) {
    for (const img of extractImagesFromHtml(content, articleUrl)) {
      // Avoid duplicates
      if (!images.includes(img)) {
        images.push(img);
      }
    }
  }

  return images;
}
