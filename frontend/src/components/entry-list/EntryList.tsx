import {
  useEffect,
  useLayoutEffect,
  useRef,
  useMemo,
  useCallback,
  useState,
  type ReactNode,
} from "react";
import { useTranslation } from "react-i18next";
import {
  useEntriesInfinite,
  useMarkManyAsRead,
  useUnreadCounts,
} from "@/hooks/useEntries";
import { useFeeds } from "@/hooks/useFeeds";
import { useFolders } from "@/hooks/useFolders";
import { useAISettings } from "@/hooks/useAISettings";
import { useGeneralSettings } from "@/hooks/useGeneralSettings";
import { useUISettingKey } from "@/hooks/useUISettings";
import { useSwipeGesture } from "@/hooks/useSwipeGesture";
import { selectionToParams, type SelectionType } from "@/hooks/useSelection";
import { flattenUniqueEntries } from "@/lib/entry-pagination";
import { stripHtml } from "@/lib/html-utils";
import { EntryListItem } from "./EntryListItem";
import { EntryListHeader } from "./EntryListHeader";
import { needsTranslation as needsTranslationAsync } from "@/lib/language-detect-async";
import {
  translateArticlesBatch,
  cancelAllBatchTranslations,
} from "@/services/translation-service";
import { translationActions } from "@/stores/translation-store";
import { selectionScrollKey, entryListScrollPositions } from "./scroll-key";
import { useScrollToTop } from "@/hooks/useScrollToTop";
import { MobileDocumentHeader } from "@/components/layout/MobileDocumentHeader";
import { RuleEditorDialog } from "@/components/rules/RuleEditorDialog";
import { cn } from "@/lib/utils";
import { useScrollMarkRead } from "./useScrollMarkRead";
import { useEntryListScrollSurface } from "./scroll-surface";
import type { Entry, Feed, Folder, ContentType } from "@/types/api";

interface EntryListProps {
  selection: SelectionType;
  selectedEntryId: string | null;
  onSelectEntry: (entryId: string) => void;
  onMarkAllRead: () => void;
  unreadOnly: boolean;
  onToggleUnreadOnly: () => void;
  contentType: ContentType;
  isMobile?: boolean;
  onMenuClick?: () => void;
  isTablet?: boolean;
  onToggleSidebar?: () => void;
  sidebarVisible?: boolean;
  isActive?: boolean;
  onSearch?: (query: string) => void;
  onExitSearch?: () => void;
}

const KEYBOARD_NAVIGATION_IGNORE_SELECTOR = [
  "input",
  "textarea",
  "select",
  "video",
  "audio",
  '[contenteditable="true"]',
  '[role="dialog"]',
  '[role="menu"]',
  '[role="listbox"]',
  '[role="slider"]',
  '[role="tablist"]',
].join(", ");

function shouldIgnoreKeyboardNavigation(event: KeyboardEvent): boolean {
  if (event.defaultPrevented || event.isComposing) return true;
  if (event.altKey || event.ctrlKey || event.metaKey || event.shiftKey) {
    return true;
  }

  const target = event.target;
  if (
    target instanceof Element &&
    target.closest(KEYBOARD_NAVIGATION_IGNORE_SELECTOR)
  ) {
    return true;
  }

  // Media overlays lock the page body; menus and the mobile sidebar are portaled.
  if (document.body.style.position === "fixed") return true;
  if (document.querySelector('[data-slot="sheet-content"]')) return true;
  if (
    document.querySelector('[role="dialog"], [role="menu"], [role="listbox"]')
  ) {
    return true;
  }

  return false;
}

export function EntryList({
  selection,
  selectedEntryId,
  onSelectEntry,
  onMarkAllRead,
  unreadOnly,
  onToggleUnreadOnly,
  contentType,
  isMobile,
  onMenuClick,
  isTablet,
  onToggleSidebar,
  sidebarVisible,
  isActive = true,
  onSearch,
  onExitSearch,
}: EntryListProps) {
  "use no memo";

  const { t } = useTranslation();
  const params = selectionToParams(selection, contentType);
  const isSearch = selection.type === "search";
  const searchQuery = selection.type === "search" ? selection.query : "";
  const containerRef = useRef<HTMLDivElement>(null);
  const headerRef = useRef<HTMLDivElement>(null);
  const listWrapperRef = useRef<HTMLDivElement>(null);
  const [ruleEditorOpen, setRuleEditorOpen] = useState(false);
  const [ruleEditorFeedIds, setRuleEditorFeedIds] = useState<string[]>([]);
  const usesDocumentScroll = Boolean(isMobile);
  const scrollSurface = useEntryListScrollSurface({
    documentScroll: usesDocumentScroll,
    containerRef,
    headerRef,
  });
  const scrollToTop = useCallback(() => {
    scrollSurface.scrollTo(0, "smooth");
  }, [scrollSurface]);

  useScrollToTop(scrollToTop, "entrylist", isActive);

  const handleMenuClick = useCallback(() => {
    entryListScrollPositions.set(
      selectionScrollKey(selection, contentType),
      scrollSurface.getScrollTop(),
    );
    onMenuClick?.();
  }, [contentType, onMenuClick, scrollSurface, selection]);

  const { data: feeds = [] } = useFeeds();
  const { data: folders = [] } = useFolders();
  const { data: aiSettings } = useAISettings();
  const { data: generalSettings } = useGeneralSettings();
  const { data: unreadCounts } = useUnreadCounts();
  const { mutate: markManyAsRead } = useMarkManyAsRead();
  const showEntrySummary = useUISettingKey("showEntrySummary");
  const hideReadEntries = useUISettingKey("hideReadEntries");

  // Save/restore scroll position per selection+contentType
  const scrollKey = selectionScrollKey(selection, contentType);
  const viewKey = `${scrollKey}\u0000${unreadOnly}`;
  const [showAllView, setShowAllView] = useState<{
    key: string;
    value: boolean;
  }>({ key: "", value: false });
  const showAllOverride = showAllView.key === viewKey && showAllView.value;
  // 搜索视图默认展示全部命中，不受全局「不显示已读」影响
  const unreadMode =
    (unreadOnly || (hideReadEntries && !isSearch)) && !showAllOverride;
  const setShowAllOverride = useCallback(
    (value: boolean) => setShowAllView({ key: viewKey, value }),
    [viewKey],
  );

  const { data, fetchNextPage, hasNextPage, isFetchingNextPage, isLoading } =
    useEntriesInfinite({
      ...params,
      unreadOnly: unreadMode,
      ...(searchQuery ? { q: searchQuery } : {}),
    });

  // Swipe gesture: Right swipe opens sidebar (only on mobile)
  useSwipeGesture(listWrapperRef, {
    onSwipeRight: handleMenuClick,
    enabledDirections: ["right"],
    threshold: 100,
    preventScroll: true,
    startFrom: { left: 32 },
    enabled: Boolean(isMobile && onMenuClick),
  });

  // Track translated entries to avoid re-translating
  const translatedEntries = useRef(new Set<string>());
  const pendingTranslation = useRef(new Map<string, Entry>());
  const pendingDetection = useRef(new Set<string>());
  const debounceTimer = useRef<ReturnType<typeof setTimeout> | null>(null);
  const translationSession = useRef(0);

  const autoTranslate = aiSettings?.autoTranslate ?? false;
  const targetLanguage = aiSettings?.summaryLanguage ?? "zh-CN";
  const markReadOnScroll = generalSettings?.markReadOnScroll ?? false;

  // Save/restore scroll position per selection+contentType

  // Restore scroll position on same-mount key change (e.g., article -> notification)
  // and remount (e.g., returning from picture mode).
  useLayoutEffect(() => {
    if (!isActive) return;

    const saved = entryListScrollPositions.get(scrollKey);
    scrollSurface.scrollTo(saved ?? 0);
    return () => {
      entryListScrollPositions.set(scrollKey, scrollSurface.getScrollTop());
    };
  }, [isActive, scrollKey, scrollSurface]);

  const maybeFetchNextPage = useCallback(() => {
    if (!isActive || !hasNextPage || isFetchingNextPage) return;

    if (scrollSurface.getDistanceToBottom() <= 600) {
      fetchNextPage();
    }
  }, [fetchNextPage, hasNextPage, isActive, isFetchingNextPage, scrollSurface]);

  const handleSelectEntry = useCallback(
    (entryId: string) => {
      entryListScrollPositions.set(scrollKey, scrollSurface.getScrollTop());
      onSelectEntry(entryId);
    },
    [onSelectEntry, scrollKey, scrollSurface],
  );

  useEffect(() => {
    if (!isActive) return;

    const handleScroll = () => {
      entryListScrollPositions.set(scrollKey, scrollSurface.getScrollTop());
      maybeFetchNextPage();
    };

    return scrollSurface.subscribe(handleScroll);
  }, [isActive, maybeFetchNextPage, scrollKey, scrollSurface]);

  // Cancel pending translations and reset state when list changes
  useEffect(() => {
    // Cancel any in-flight batch translations
    cancelAllBatchTranslations();
    // Clear translation tracking for new list
    translationSession.current += 1;
    translatedEntries.current.clear();
    pendingTranslation.current.clear();
    pendingDetection.current.clear();
    if (debounceTimer.current) {
      clearTimeout(debounceTimer.current);
      debounceTimer.current = null;
    }
  }, [selection, contentType]);

  useEffect(() => {
    const pendingDetectionEntries = pendingDetection.current;

    return () => {
      translationSession.current += 1;
      pendingDetectionEntries.clear();
      if (debounceTimer.current) {
        clearTimeout(debounceTimer.current);
        debounceTimer.current = null;
      }
    };
  }, []);

  const feedsMap = useMemo(() => {
    const map = new Map<string, Feed>();
    for (const feed of feeds) {
      map.set(feed.id, feed);
    }
    return map;
  }, [feeds]);

  const foldersMap = useMemo(() => {
    const map = new Map<string, Folder>();
    for (const folder of folders) {
      map.set(folder.id, folder);
    }
    return map;
  }, [folders]);

  const entries = useMemo(() => flattenUniqueEntries(data?.pages), [data]);

  const unreadEntryIds = useMemo(
    () => entries.filter((entry) => !entry.read).map((entry) => entry.id),
    [entries],
  );

  const handleMarkAreaRead = useCallback(() => {
    if (unreadEntryIds.length === 0) return;
    // 未读模式下不立即刷新列表，已读条目保留到切换订阅源后重新请求时才隐藏
    markManyAsRead({
      ids: unreadEntryIds,
      read: true,
      ...(unreadMode ? { skipInvalidate: true } : {}),
    });
  }, [markManyAsRead, unreadEntryIds, unreadMode]);

  const handleToggleUnreadMode = useCallback(() => {
    if (unreadMode) {
      setShowAllOverride(true);
      return;
    }
    if (showAllOverride) {
      setShowAllOverride(false);
      return;
    }
    onToggleUnreadOnly();
  }, [onToggleUnreadOnly, setShowAllOverride, showAllOverride, unreadMode]);

  const handleMarkRangeRead = useCallback(
    (entryId: string, direction: "above" | "below") => {
      const currentIndex = entries.findIndex((entry) => entry.id === entryId);
      if (currentIndex === -1) return;

      const range =
        direction === "above"
          ? entries.slice(0, currentIndex)
          : entries.slice(currentIndex + 1);
      const ids = range
        .filter((entry) => !entry.read)
        .map((entry) => entry.id);
      if (ids.length === 0) return;

      markManyAsRead({
        ids,
        read: true,
        ...(unreadMode ? { skipInvalidate: true } : {}),
      });
    },
    [entries, markManyAsRead, unreadMode],
  );

  const handleAddRule = useCallback((feedId: string) => {
    setRuleEditorFeedIds([feedId]);
    setRuleEditorOpen(true);
  }, []);

  const { endPaddingHeight: scrollReadEndPaddingHeight } = useScrollMarkRead({
    surface: scrollSurface,
    contentRootRef: containerRef,
    entries,
    enabled: markReadOnScroll && isActive,
    unreadOnly: unreadMode,
    hasNextPage: Boolean(hasNextPage),
    resetKey: `${scrollKey}\u0000${unreadMode}\u0000${markReadOnScroll}`,
  });

  useEffect(() => {
    maybeFetchNextPage();
  }, [entries.length, maybeFetchNextPage]);

  useEffect(() => {
    if (!isActive || entries.length === 0) return;

    const handleKeyDown = (event: KeyboardEvent) => {
      if (event.key !== "ArrowUp" && event.key !== "ArrowDown") return;
      if (shouldIgnoreKeyboardNavigation(event)) return;

      const currentIndex = selectedEntryId
        ? entries.findIndex((entry) => entry.id === selectedEntryId)
        : -1;
      const nextIndex =
        event.key === "ArrowDown"
          ? Math.min(currentIndex + 1, entries.length - 1)
          : Math.max(currentIndex - 1, 0);

      if (nextIndex === currentIndex) return;

      const nextEntry = entries[nextIndex];
      if (!nextEntry) return;

      event.preventDefault();
      handleSelectEntry(nextEntry.id);

      const node = containerRef.current?.querySelector<HTMLElement>(
        `[data-entry-id="${nextEntry.id}"]`,
      );
      node?.scrollIntoView?.({ block: "nearest" });
    };

    window.addEventListener("keydown", handleKeyDown);
    return () => window.removeEventListener("keydown", handleKeyDown);
  }, [entries, handleSelectEntry, isActive, selectedEntryId]);

  // Function to trigger batch translation for pending entries
  const triggerBatchTranslation = useCallback(() => {
    if (pendingTranslation.current.size === 0) return;

    const articlesToTranslate = Array.from(pendingTranslation.current.values())
      .filter((entry) => !translatedEntries.current.has(entry.id))
      .map((entry) => ({
        id: entry.id,
        title: entry.title || "",
        summary: entry.content ? stripHtml(entry.content).slice(0, 200) : null,
      }));

    // Mark as translated to prevent re-translating
    for (const article of articlesToTranslate) {
      translatedEntries.current.add(article.id);
    }

    pendingTranslation.current.clear();

    if (articlesToTranslate.length > 0) {
      translateArticlesBatch(articlesToTranslate, targetLanguage).finally(
        () => {
          // Remove entries that didn't actually get translated (cancelled, partial failure, etc.)
          for (const article of articlesToTranslate) {
            const cached = translationActions.get(article.id, targetLanguage);
            if (!cached?.title && !cached?.summary) {
              translatedEntries.current.delete(article.id);
            }
          }
        },
      );
    }
  }, [targetLanguage]);

  const queueEntryForTranslation = useCallback(
    (entry: Entry) => {
      pendingTranslation.current.set(entry.id, entry);

      if (debounceTimer.current) {
        clearTimeout(debounceTimer.current);
      }
      debounceTimer.current = setTimeout(triggerBatchTranslation, 500);
    },
    [triggerBatchTranslation],
  );

  // Schedule entry for translation when visible
  const scheduleTranslation = useCallback(
    (entry: Entry) => {
      if (!autoTranslate) return;
      if (translatedEntries.current.has(entry.id)) {
        // Verify against store: if marked but no actual translation, allow retry
        const cached = translationActions.get(entry.id, targetLanguage);
        if (cached?.title || cached?.summary) return;
        translatedEntries.current.delete(entry.id);
      }
      // Skip if user manually disabled translation for this article
      if (translationActions.isDisabled(entry.id)) return;

      if (
        pendingTranslation.current.has(entry.id) ||
        pendingDetection.current.has(entry.id)
      ) {
        return;
      }

      const summary = entry.content
        ? stripHtml(entry.content).slice(0, 200)
        : null;
      const session = translationSession.current;
      pendingDetection.current.add(entry.id);

      void needsTranslationAsync(entry.title || "", summary, targetLanguage)
        .then((shouldTranslate) => {
          if (translationSession.current !== session) return;

          if (!shouldTranslate) {
            translatedEntries.current.add(entry.id);
            return;
          }

          queueEntryForTranslation(entry);
        })
        .catch(() => {
          if (translationSession.current !== session) return;
          queueEntryForTranslation(entry);
        })
        .finally(() => {
          if (translationSession.current === session) {
            pendingDetection.current.delete(entry.id);
          }
        });
    },
    [autoTranslate, targetLanguage, queueEntryForTranslation],
  );

  // Trigger translation for real visible items and selected entry
  useEffect(() => {
    if (!autoTranslate || !isActive) return;

    const node = containerRef.current;
    if (!node || typeof IntersectionObserver === "undefined") {
      for (const entry of entries.slice(0, 20)) {
        scheduleTranslation(entry);
      }
      return;
    }

    const observer = new IntersectionObserver(
      (items) => {
        for (const item of items) {
          if (!item.isIntersecting) continue;

          const index = Number((item.target as HTMLElement).dataset.index);
          const entry = entries[index];
          if (entry) {
            scheduleTranslation(entry);
          }
        }
      },
      {
        root: scrollSurface.getIntersectionRoot(),
        rootMargin: "200px 0px",
      },
    );

    for (const item of node.querySelectorAll<HTMLElement>("[data-index]")) {
      observer.observe(item);
    }

    if (selectedEntryId) {
      const selectedEntry = entries.find((e) => e.id === selectedEntryId);
      if (selectedEntry) {
        scheduleTranslation(selectedEntry);
      }
    }

    return () => {
      observer.disconnect();
    };
  }, [
    entries,
    autoTranslate,
    isActive,
    scheduleTranslation,
    scrollSurface,
    selectedEntryId,
  ]);

  const title = useMemo(() => {
    switch (selection.type) {
      case "all":
        switch (contentType) {
          case "picture":
            return t("entry_list.all_pictures");
          case "notification":
            return t("entry_list.all_notifications");
          default:
            return t("entry_list.all_articles");
        }
      case "feed":
        return feedsMap.get(selection.feedId)?.title || t("entry_list.feed");
      case "folder":
        return (
          foldersMap.get(selection.folderId)?.name || t("entry_list.folder")
        );
      case "starred":
        return t("entry_list.starred");
      case "search":
        return searchQuery
          ? t("search.results_title", { query: searchQuery })
          : t("search.title");
    }
  }, [selection, contentType, feedsMap, foldersMap, t, searchQuery]);

  // Calculate unread count from API data (not from loaded entries)
  const unreadCount = useMemo(() => {
    if (!unreadCounts) return 0;
    const counts = unreadCounts.counts;
    switch (selection.type) {
      case "all":
        // Sum all feeds' unread counts, filtered by contentType
        return feeds
          .filter((f) => f.type === contentType)
          .reduce((sum, f) => sum + (counts[f.id] ?? 0), 0);
      case "feed":
        return counts[selection.feedId] ?? 0;
      case "folder":
        // Sum unread counts for feeds in this folder with matching contentType
        return feeds
          .filter(
            (f) => f.folderId === selection.folderId && f.type === contentType,
          )
          .reduce((sum, f) => sum + (counts[f.id] ?? 0), 0);
      case "starred":
        return 0; // Starred view doesn't show unread count
      case "search":
        return 0; // Search results don't show unread count
    }
  }, [unreadCounts, selection, feeds, contentType]);

  return (
    <div
      ref={listWrapperRef}
      className={cn(
        usesDocumentScroll
          ? "entry-list-document min-h-[var(--app-dvh)]"
          : "flex h-full flex-col",
      )}
    >
      <MobileDocumentHeader
        enabled={usesDocumentScroll}
        headerRef={headerRef}
        testId="entry-list-header"
      >
        <EntryListHeader
          key={isSearch ? "search" : "browse"}
          title={title}
          unreadCount={unreadCount}
          unreadOnly={unreadMode}
          onToggleUnreadOnly={handleToggleUnreadMode}
          onMarkAllRead={onMarkAllRead}
          scrollToTopScope="entrylist"
          isMobile={isMobile}
          onMenuClick={handleMenuClick}
          isTablet={isTablet}
          onToggleSidebar={onToggleSidebar}
          sidebarVisible={sidebarVisible}
          searchQuery={searchQuery}
          onSearch={onSearch}
          onExitSearch={onExitSearch}
          hideMarkAllRead={isSearch}
        />
      </MobileDocumentHeader>

      <div
        className={cn(
          usesDocumentScroll
            ? "min-h-[calc(var(--app-dvh)-3.5rem)]"
            : "relative min-h-0 flex-1 overflow-hidden",
        )}
      >
        <div
          ref={containerRef}
          data-testid="entry-list-viewport"
          className={cn(
            "w-full overflow-x-hidden rounded-[inherit] [overflow-anchor:none]",
            usesDocumentScroll
              ? "overflow-y-visible"
              : "h-full overflow-y-auto overscroll-y-contain",
          )}
        >
          {isLoading ? (
            <EntryListSkeleton />
          ) : entries.length === 0 ? (
            <EntryListEmpty
              unreadMode={unreadMode}
              isSearch={isSearch}
              footer={
                <EntryListFooter
                  unreadCount={0}
                  unreadMode={unreadMode}
                  onMarkAreaRead={handleMarkAreaRead}
                  onToggleViewAll={handleToggleUnreadMode}
                />
              }
            />
          ) : (
            <div className="w-full">
              {entries.map((entry, index) => (
                <EntryListItem
                  key={entry.id}
                  data-index={index}
                  data-entry-id={entry.id}
                  entry={entry}
                  feed={feedsMap.get(entry.feedId)}
                  isSelected={entry.id === selectedEntryId}
                  onClick={() => handleSelectEntry(entry.id)}
                  onMarkAboveRead={(entryId) =>
                    handleMarkRangeRead(entryId, "above")
                  }
                  onMarkBelowRead={(entryId) =>
                    handleMarkRangeRead(entryId, "below")
                  }
                  onAddRule={handleAddRule}
                  showSummary={showEntrySummary}
                  autoTranslate={autoTranslate}
                  targetLanguage={targetLanguage}
                  searchQuery={isSearch ? searchQuery : undefined}
                />
              ))}
              <EntryListFooter
                unreadCount={unreadEntryIds.length}
                unreadMode={unreadMode}
                onMarkAreaRead={handleMarkAreaRead}
                onToggleViewAll={handleToggleUnreadMode}
              />
              {scrollReadEndPaddingHeight > 0 && (
                <div
                  aria-hidden="true"
                  style={{ height: scrollReadEndPaddingHeight }}
                />
              )}
            </div>
          )}

          {isFetchingNextPage && <LoadingMore />}
        </div>
      </div>

      {ruleEditorOpen && (
        <RuleEditorDialog
          open={ruleEditorOpen}
          onOpenChange={setRuleEditorOpen}
          initialFeedIds={ruleEditorFeedIds}
        />
      )}
    </div>
  );
}

function EntryListSkeleton() {
  return (
    <div className="space-y-px">
      {Array.from({ length: 5 }, (_, i) => (
        <div key={i} className="px-4 py-3 animate-pulse">
          {/* Line 1: icon + feed name + time */}
          <div className="flex items-center gap-1.5">
            <div className="size-4 rounded bg-muted" />
            <div className="h-3 w-24 rounded bg-muted" />
            <div className="h-3 w-12 rounded bg-muted" />
          </div>
          {/* Line 2: title */}
          <div className="mt-1 h-4 w-3/4 rounded bg-muted" />
          {/* Line 3: summary */}
          <div className="mt-1 h-3 w-full rounded bg-muted" />
          <div className="mt-1 h-3 w-2/3 rounded bg-muted" />
        </div>
      ))}
    </div>
  );
}

function EntryListEmpty({
  unreadMode,
  isSearch,
  footer,
}: {
  unreadMode: boolean;
  isSearch?: boolean;
  footer: ReactNode;
}) {
  const { t } = useTranslation();
  return (
    <div className="flex min-h-[60vh] flex-col items-center justify-center px-4 text-center text-sm text-muted-foreground">
      <div>
        {unreadMode
          ? t("entry_list.no_unread")
          : isSearch
            ? t("search.no_results")
            : t("entry_list.no_articles")}
      </div>
      {footer}
    </div>
  );
}

function EntryListFooter({
  unreadCount,
  unreadMode,
  onMarkAreaRead,
  onToggleViewAll,
}: {
  unreadCount: number;
  unreadMode: boolean;
  onMarkAreaRead: () => void;
  onToggleViewAll: () => void;
}) {
  const { t } = useTranslation();
  return (
    <div className="flex flex-col items-center gap-2 px-4 py-6">
      <button
        type="button"
        onClick={onMarkAreaRead}
        disabled={unreadCount === 0}
        className="h-9 w-full max-w-[13rem] rounded-md bg-primary px-4 text-sm font-medium text-primary-foreground transition-colors hover:bg-primary/90 disabled:cursor-not-allowed disabled:opacity-40"
      >
        {t("entry_list.mark_area_read")}
      </button>
      <button
        type="button"
        onClick={onToggleViewAll}
        className="h-9 w-full max-w-[13rem] rounded-md border border-border bg-background px-4 text-sm font-medium transition-colors hover:bg-item-hover"
      >
        {unreadMode
          ? t("entry_list.view_all")
          : t("entry_list.show_unread_only")}
      </button>
    </div>
  );
}

function LoadingMore() {
  return (
    <div className="flex items-center justify-center py-4">
      <svg
        className="size-5 animate-spin text-muted-foreground"
        fill="none"
        viewBox="0 0 24 24"
      >
        <circle
          className="opacity-25"
          cx="12"
          cy="12"
          r="10"
          stroke="currentColor"
          strokeWidth="4"
        />
        <path
          className="opacity-75"
          fill="currentColor"
          d="M4 12a8 8 0 018-8V0C5.373 0 0 5.373 0 12h4zm2 5.291A7.962 7.962 0 014 12H0c0 3.042 1.135 5.824 3 7.938l3-2.647z"
        />
      </svg>
    </div>
  );
}
