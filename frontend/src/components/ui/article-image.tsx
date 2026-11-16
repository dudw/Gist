import {
  memo,
  useState,
  useCallback,
  useContext,
  createContext,
  useMemo,
} from "react";
import { useTranslation } from "react-i18next";
import { cn } from "@/lib/utils";
import {
  IMAGE_REFERRER_POLICIES,
  resolveSrcset,
  toAbsoluteUrl,
  withRetryFragment,
} from "@/lib/image-url";
import { ImagePreviewContext } from "./article-content";

// Context for passing article URL to resolve relative URLs and set Referer
// eslint-disable-next-line react-refresh/only-export-components
export const ArticleLinkContext = createContext<string | undefined>(undefined);

// Global cache to track loaded images - prevents flash when component remounts
const loadedImagesCache = new Set<string>();

/**
 * 把 width/height 归一化为像素数：非数字（如 "100%"）返回 undefined，避免 NaN 尺寸
 */
function toPixelSize(value?: string | number): number | undefined {
  const size = typeof value === "number" ? value : Number(value);
  return Number.isFinite(size) && size > 0 ? size : undefined;
}

interface ArticleImageProps {
  src?: string;
  alt?: string;
  width?: string | number;
  height?: string | number;
  srcSet?: string;
  sizes?: string;
  className?: string;
}

/**
 * Article image component with lazy loading and error handling.
 *
 * 图片由浏览器直接加载：先按文章链接把相对地址绝对化，再依次尝试多种
 * Referer 策略（见 IMAGE_REFERRER_POLICIES），全部失败才显示占位。
 * Uses memo to prevent re-rendering when parent content changes.
 */
export const ArticleImage = memo(function ArticleImage({
  src,
  alt = "",
  width,
  height,
  srcSet,
  sizes,
  className,
  ...props
}: ArticleImageProps & React.ImgHTMLAttributes<HTMLImageElement>) {
  const { t } = useTranslation();
  const articleUrl = useContext(ArticleLinkContext);
  const imagePreviewContext = useContext(ImagePreviewContext);

  // 浏览器直连地址：相对路径按文章链接还原
  const imageUrl = useMemo(
    () => (src ? (toAbsoluteUrl(src, articleUrl) ?? src) : ""),
    [src, articleUrl],
  );
  const imageSrcset = useMemo(
    () => (srcSet ? resolveSrcset(srcSet, articleUrl) : undefined),
    [srcSet, articleUrl],
  );

  // 已失败次数：每次失败换一种 Referer 策略重试，耗尽后显示占位
  const [attempt, setAttempt] = useState(0);
  const isError = attempt >= IMAGE_REFERRER_POLICIES.length;
  const referrerPolicy =
    IMAGE_REFERRER_POLICIES[
      Math.min(attempt, IMAGE_REFERRER_POLICIES.length - 1)
    ];
  const activeSrc = useMemo(
    () => withRetryFragment(imageUrl, attempt),
    [imageUrl, attempt],
  );

  // Initialize isLoaded based on cache - prevents flash on remount
  const [isLoaded, setIsLoaded] = useState(() =>
    loadedImagesCache.has(imageUrl),
  );

  const handleLoad = useCallback(() => {
    loadedImagesCache.add(imageUrl);
    setIsLoaded(true);
  }, [imageUrl]);

  const handleError = useCallback(() => {
    setIsLoaded(false);
    const canRetry =
      !imageUrl.startsWith("data:") && !imageUrl.startsWith("blob:");
    // 不可重试的资源直接进入错误态，避免停留在无法重新请求的地址上
    setAttempt((prev) => (canRetry ? prev + 1 : IMAGE_REFERRER_POLICIES.length));
  }, [imageUrl]);

  const handleClick = useCallback(
    (e: React.MouseEvent) => {
      if (imageUrl && imagePreviewContext) {
        e.preventDefault();
        e.stopPropagation();
        imagePreviewContext.openPreview(imageUrl);
      }
    },
    [imageUrl, imagePreviewContext],
  );

  if (!src) return null;

  if (isError) {
    return (
      <span
        className={cn(
          "flex min-h-24 w-full max-w-full flex-col items-center justify-center gap-1.5 rounded-lg border border-dashed border-border bg-muted/40 p-4 text-xs text-muted-foreground",
          className,
        )}
        style={{
          width: toPixelSize(width),
          height: toPixelSize(height),
        }}
      >
        <svg
          className="size-5 opacity-40"
          fill="none"
          stroke="currentColor"
          viewBox="0 0 24 24"
        >
          <path
            strokeLinecap="round"
            strokeLinejoin="round"
            strokeWidth={1.5}
            d="M4 16l4.586-4.586a2 2 0 012.828 0L16 16m-2-2l1.586-1.586a2 2 0 012.828 0L20 14m-6-6h.01M6 20h12a2 2 0 002-2V6a2 2 0 00-2-2H6a2 2 0 00-2 2v12a2 2 0 002 2z"
          />
        </svg>
        <span className="break-all text-center">
          {alt || t("entry.image_load_failed")}
        </span>
        {imageUrl && (
          <a
            href={imageUrl}
            target="_blank"
            rel="noopener noreferrer"
            className="underline underline-offset-2 hover:text-foreground"
            onClick={(e) => e.stopPropagation()}
          >
            {t("entry.open_original_image")}
          </a>
        )}
      </span>
    );
  }

  // Only show skeleton placeholder if we have dimensions
  const showSkeleton = !isLoaded && width && height;

  return (
    <img
      src={activeSrc}
      sizes={sizes}
      alt={alt}
      width={width}
      height={height}
      loading="lazy"
      decoding="async"
      onLoad={handleLoad}
      onError={handleError}
      onClick={handleClick}
      className={cn(
        "max-w-full h-auto rounded transition-opacity duration-200",
        showSkeleton && "bg-muted/30",
        isLoaded ? "opacity-100" : "opacity-70",
        imagePreviewContext && "cursor-zoom-in",
        className,
      )}
      {...props}
      srcSet={attempt === 0 ? imageSrcset : undefined}
      referrerPolicy={referrerPolicy}
    />
  );
});
