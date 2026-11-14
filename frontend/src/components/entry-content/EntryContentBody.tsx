import type { RefCallback } from "react";
import { useCallback, useLayoutEffect, useRef, useState } from "react";
import { useTranslation } from "react-i18next";
import { useCodeHighlight } from "@/hooks/useCodeHighlight";
import { useEntryMeta } from "@/hooks/useEntryMeta";
import {
  ARTICLE_WIDTH_MAX,
  ARTICLE_WIDTH_MIN,
  ARTICLE_WIDTH_STEP,
  useUISettingActions,
  useUISettingKey,
} from "@/hooks/useUISettings";
import { isSafeUrl } from "@/lib/url";
import { ArticleContent } from "@/components/ui/article-content";
import type { ArticleContentBlock } from "@/components/ui/article-content";
import { UserIcon, CalendarIcon, ClockIcon } from "@/components/ui/icons";
import { AiSummaryBox } from "./AiSummaryBox";
import { BackToTopButton } from "./BackToTopButton";
import type { Entry } from "@/types/api";

interface EntryContentBodyProps {
  entry: Entry;
  displayTitle?: string | null;
  scrollRef: RefCallback<HTMLDivElement>;
  scrollNode?: HTMLDivElement | null;
  displayContent: string | null | undefined;
  displayBlocks?: ArticleContentBlock[] | null;
  highlightContent?: string;
  aiSummary?: string | null;
  isLoadingSummary?: boolean;
  summaryError?: string | null;
}

export function EntryContentBody({
  entry,
  displayTitle,
  scrollRef,
  scrollNode,
  displayContent,
  displayBlocks,
  highlightContent,
  aiSummary,
  isLoadingSummary,
  summaryError,
}: EntryContentBodyProps) {
  const { t } = useTranslation();
  const { publishedLong, readingTime } = useEntryMeta(entry);
  const title = displayTitle ?? entry.title ?? t("entry.untitled");
  const contentRef = useRef<HTMLDivElement>(null);
  const articleWidth = useUISettingKey("articleWidth");
  const { setArticleWidth } = useUISettingActions();
  const articleRef = useRef<HTMLElement>(null);
  const widthProbeRef = useRef<HTMLSpanElement>(null);
  // 内容区当前能容纳的最大字符宽度，保证放大到最后正好铺满内容区
  const [maxArticleWidth, setMaxArticleWidth] = useState(ARTICLE_WIDTH_MAX);

  useLayoutEffect(() => {
    const container = articleRef.current?.parentElement;
    const probe = widthProbeRef.current;
    if (!container || !probe) return;

    const updateMaxWidth = () => {
      const charWidth = probe.offsetWidth / 100;
      const containerWidth = container.clientWidth;
      if (!charWidth || !containerWidth) return;

      setMaxArticleWidth(
        Math.max(ARTICLE_WIDTH_MIN, Math.floor(containerWidth / charWidth)),
      );
    };

    updateMaxWidth();

    if (typeof ResizeObserver === "undefined") {
      window.addEventListener("resize", updateMaxWidth);
      return () => window.removeEventListener("resize", updateMaxWidth);
    }

    const observer = new ResizeObserver(updateMaxWidth);
    observer.observe(container);
    return () => observer.disconnect();
  }, []);

  const effectiveArticleWidth = Math.min(articleWidth, maxArticleWidth);
  const isFullWidth = articleWidth >= maxArticleWidth;

  const handleDecreaseWidth = useCallback(() => {
    setArticleWidth(
      Math.max(effectiveArticleWidth - ARTICLE_WIDTH_STEP, ARTICLE_WIDTH_MIN),
    );
  }, [effectiveArticleWidth, setArticleWidth]);

  const handleIncreaseWidth = useCallback(() => {
    setArticleWidth(
      Math.min(effectiveArticleWidth + ARTICLE_WIDTH_STEP, maxArticleWidth),
    );
  }, [effectiveArticleWidth, maxArticleWidth, setArticleWidth]);

  // Apply code highlighting after content renders
  useCodeHighlight(contentRef, highlightContent ?? displayContent ?? "");

  const hasBlocks = !!displayBlocks && displayBlocks.length > 0;
  const hasContent =
    hasBlocks || (!!displayContent && displayContent.trim().length > 0);

  return (
    <div
      ref={scrollRef}
      className="entry-content-viewport flex-1 overflow-y-auto [overflow-anchor:none]"
    >
      <article
        ref={articleRef}
        className="entry-content mx-auto w-full min-w-0 overflow-x-hidden px-4 sm:px-6 pb-20 pt-16"
        style={{
          maxWidth: isFullWidth ? "100%" : `${effectiveArticleWidth}ch`,
        }}
      >
        <span
          ref={widthProbeRef}
          aria-hidden="true"
          className="pointer-events-none invisible absolute h-0"
          style={{ width: "100ch" }}
        />
        <header className="mb-4 space-y-5">
          <h1 className="text-3xl font-bold leading-tight tracking-tight text-foreground wrap-anywhere sm:text-4xl sm:leading-[1.15]">
            {entry.url && isSafeUrl(entry.url) ? (
              <a
                href={entry.url}
                target="_blank"
                rel="noopener noreferrer"
                className="transition-opacity hover:opacity-80"
              >
                {title}
              </a>
            ) : (
              title
            )}
          </h1>

          <div className="flex flex-wrap items-center gap-x-6 gap-y-3 text-sm text-muted-foreground">
            {entry.author && (
              <div className="flex items-center gap-1.5">
                <UserIcon className="size-4 opacity-70" />
                <span>{entry.author}</span>
              </div>
            )}

            {publishedLong && (
              <div className="flex items-center gap-1.5">
                <CalendarIcon className="size-4 opacity-70" />
                <span className="tabular-nums">{publishedLong}</span>
              </div>
            )}

            {readingTime && (
              <div className="flex items-center gap-1.5">
                <ClockIcon className="size-4 opacity-70" />
                <span>{readingTime}</span>
              </div>
            )}

            <div className="ml-auto flex items-center gap-1">
              <button
                type="button"
                onClick={handleDecreaseWidth}
                disabled={effectiveArticleWidth <= ARTICLE_WIDTH_MIN}
                title={t("entry.decrease_width")}
                aria-label={t("entry.decrease_width")}
                className="flex size-7 items-center justify-center rounded-md text-muted-foreground transition-colors hover:bg-item-hover hover:text-foreground disabled:cursor-not-allowed disabled:opacity-40"
              >
                <svg
                  className="size-4"
                  viewBox="0 0 24 24"
                  fill="none"
                  stroke="currentColor"
                  strokeWidth={2}
                  strokeLinecap="round"
                >
                  <path d="M5 12h14" />
                </svg>
              </button>
              <button
                type="button"
                onClick={handleIncreaseWidth}
                disabled={effectiveArticleWidth >= maxArticleWidth}
                title={t("entry.increase_width")}
                aria-label={t("entry.increase_width")}
                className="flex size-7 items-center justify-center rounded-md text-muted-foreground transition-colors hover:bg-item-hover hover:text-foreground disabled:cursor-not-allowed disabled:opacity-40"
              >
                <svg
                  className="size-4"
                  viewBox="0 0 24 24"
                  fill="none"
                  stroke="currentColor"
                  strokeWidth={2}
                  strokeLinecap="round"
                >
                  <path d="M12 5v14M5 12h14" />
                </svg>
              </button>
            </div>
          </div>
          <hr className="border-border/60" />
        </header>

        <AiSummaryBox
          content={aiSummary ?? null}
          isLoading={isLoadingSummary}
          error={summaryError}
        />

        <div
          ref={contentRef}
          className="prose dark:prose-invert max-w-none hyphens-auto text-[1.0625rem] leading-[1.8] break-words prose-img:!max-w-full prose-img:!h-auto prose-img:shadow-[0_4px_20px_rgba(0,0,0,0.08)] prose-video:!max-w-full prose-video:!h-auto prose-video:shadow-[0_4px_20px_rgba(0,0,0,0.08)] prose-figure:!max-w-full prose-a:underline prose-a:decoration-primary/30 prose-a:underline-offset-[3px] prose-a:break-words prose-blockquote:not-italic prose-blockquote:border-l-2 prose-blockquote:border-muted-foreground/30 prose-blockquote:bg-transparent prose-blockquote:text-foreground/85 prose-blockquote:py-1 prose-blockquote:pl-5 prose-blockquote:ml-0 prose-blockquote:rounded-none prose-th:bg-muted/60 prose-code:bg-muted/60 prose-code:px-1.5 prose-code:py-0.5 prose-code:rounded-md prose-code:break-all prose-code:before:content-none prose-code:after:content-none prose-pre:p-0 prose-pre:rounded-lg prose-pre:border prose-pre:border-border prose-pre:overflow-hidden"
        >
          {hasContent ? (
            hasBlocks ? (
              <ArticleContent
                blocks={displayBlocks ?? []}
                articleUrl={entry.url}
              />
            ) : (
              <ArticleContent
                content={displayContent ?? ""}
                articleUrl={entry.url}
              />
            )
          ) : (
            <div className="rounded-lg border border-dashed border-border p-8 text-center text-muted-foreground">
              {t("entry.no_content")}
            </div>
          )}
        </div>
      </article>
      {scrollNode && <BackToTopButton scrollNode={scrollNode} />}
    </div>
  );
}
