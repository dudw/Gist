import { forwardRef, useState, useMemo } from "react";
import { useTranslation } from "react-i18next";
import { cn } from "@/lib/utils";
import { formatRelativeTime } from "@/lib/date-utils";
import { stripHtml } from "@/lib/html-utils";
import { useTranslationStore } from "@/stores/translation-store";
import { FeedIcon } from "@/components/ui/feed-icon";
import {
  DropdownMenu,
  DropdownMenuContent,
  DropdownMenuItem,
  DropdownMenuSeparator,
  DropdownMenuTrigger,
} from "@/components/ui/dropdown-menu";
import type { Entry, Feed } from "@/types/api";

const URL_PATTERN = /\bhttps?:\/\/\S+/i;

interface EntryListItemProps {
  entry: Entry;
  feed?: Feed;
  isSelected: boolean;
  onClick: () => void;
  onMarkAboveRead?: (entryId: string) => void;
  onMarkBelowRead?: (entryId: string) => void;
  onAddRule?: (feedId: string) => void;
  showSummary?: boolean;
  autoTranslate?: boolean;
  targetLanguage?: string;
  style?: React.CSSProperties;
  "data-index"?: number;
  "data-entry-id"?: string;
}

export const EntryListItem = forwardRef<HTMLDivElement, EntryListItemProps>(
  function EntryListItem(
    {
      entry,
      feed,
      isSelected,
      onClick,
      onMarkAboveRead,
      onMarkBelowRead,
      onAddRule,
      showSummary = true,
      autoTranslate,
      targetLanguage,
      style,
      "data-index": dataIndex,
      "data-entry-id": dataEntryId,
    },
    ref,
  ) {
    const { t } = useTranslation();
    const publishedAt = entry.publishedAt
      ? formatRelativeTime(entry.publishedAt, t)
      : null;
    const [iconError, setIconError] = useState(false);
    const showIcon = feed?.iconPath && !iconError;
    const fallbackTitle = t("entry.untitled");
    const fallbackFeedName = t("entry.unknown_feed");

    const translation = useTranslationStore((state) =>
      autoTranslate && targetLanguage
        ? state.getTranslation(entry.id, targetLanguage)
        : undefined,
    );

    const strippedContent = useMemo(
      () => (entry.content ? stripHtml(entry.content).slice(0, 150) : null),
      [entry.content],
    );

    const displayTitle = translation?.title ?? entry.title;
    const displaySummary = showSummary
      ? (translation?.summary ?? strippedContent)
      : null;
    const displayFeedName = feed?.title || fallbackFeedName;
    const titleContainsUrl = URL_PATTERN.test(displayTitle ?? "");
    const summaryContainsUrl = URL_PATTERN.test(displaySummary ?? "");

    return (
      <div
        ref={ref}
        className={cn(
          "relative w-full min-w-0 overflow-hidden px-4 py-3 cursor-pointer transition-colors",
          "hover:bg-item-hover",
          isSelected && "bg-item-active",
          !entry.read && !isSelected && "bg-accent/5",
        )}
        style={style}
        data-index={dataIndex}
        data-entry-id={dataEntryId}
        onClick={onClick}
      >
        {/* Line 1: icon + feed name + time */}
        <div className="flex min-w-0 items-center gap-1.5 overflow-hidden text-xs text-muted-foreground">
          {showIcon ? (
            <img
              src={`/icons/${feed.iconPath}`}
              alt=""
              className="size-4 shrink-0 rounded object-contain"
              onError={() => setIconError(true)}
            />
          ) : (
            <FeedIcon className="size-4 shrink-0 text-muted-foreground/50" />
          )}
          <span className="block min-w-0 truncate">{displayFeedName}</span>
          {publishedAt && (
            <>
              <span className="shrink-0 text-muted-foreground/50">·</span>
              <span className="shrink-0 whitespace-nowrap">{publishedAt}</span>
            </>
          )}
        </div>

        {/* Line 2: title */}
        <div
          className={cn(
            "mt-1 text-sm wrap-anywhere",
            titleContainsUrl ? "line-clamp-3" : "line-clamp-2",
            !entry.read ? "font-semibold" : "font-medium text-muted-foreground",
            !displaySummary && "pr-10",
          )}
        >
          {displayTitle || fallbackTitle}
        </div>

        {/* Line 3: summary */}
        {displaySummary && (
          <div
            className={cn(
              "mt-1 pr-10 text-xs text-muted-foreground wrap-anywhere",
              summaryContainsUrl ? "line-clamp-3" : "line-clamp-2",
            )}
          >
            {displaySummary}
          </div>
        )}

        <DropdownMenu>
          <DropdownMenuTrigger asChild>
            <button
              type="button"
              aria-label={t("entry.more_actions")}
              title={t("entry.more_actions")}
              onClick={(event) => event.stopPropagation()}
              onPointerDown={(event) => event.stopPropagation()}
              className="absolute bottom-1.5 right-2.5 flex size-6 items-center justify-center rounded-md text-muted-foreground/70 transition-colors hover:bg-item-hover hover:text-foreground data-[state=open]:bg-item-hover data-[state=open]:text-foreground"
            >
              <svg
                className="size-4"
                viewBox="0 0 24 24"
                fill="currentColor"
                aria-hidden="true"
              >
                <circle cx="5" cy="12" r="1.6" />
                <circle cx="12" cy="12" r="1.6" />
                <circle cx="19" cy="12" r="1.6" />
              </svg>
            </button>
          </DropdownMenuTrigger>
          <DropdownMenuContent align="end" className="min-w-[11rem]">
            <DropdownMenuItem onSelect={() => onAddRule?.(entry.feedId)}>
              {t("entry.add_rule")}
            </DropdownMenuItem>
            <DropdownMenuSeparator />
            <DropdownMenuItem
              onSelect={() => onMarkAboveRead?.(entry.id)}
            >
              {t("entry.mark_above_read")}
            </DropdownMenuItem>
            <DropdownMenuItem
              onSelect={() => onMarkBelowRead?.(entry.id)}
            >
              {t("entry.mark_below_read")}
            </DropdownMenuItem>
          </DropdownMenuContent>
        </DropdownMenu>
      </div>
    );
  },
);
