import { useCallback, useMemo } from "react";
import { useLocation, useSearch } from "wouter";
import { parseRoute, buildPath } from "@/lib/router";
import type { ContentType } from "@/types/api";

export type SelectionType =
  | { type: "all" }
  | { type: "feed"; feedId: string }
  | { type: "folder"; folderId: string }
  | { type: "starred" }
  | { type: "search"; query: string };

interface NavigateOptions {
  replace?: boolean;
}

const LAST_VIEW_STORAGE_KEY = "gist:last-view";

function rememberLastView(path: string): void {
  try {
    sessionStorage.setItem(LAST_VIEW_STORAGE_KEY, path);
  } catch {
    // ignore storage errors
  }
}

function getLastView(): string | null {
  try {
    return sessionStorage.getItem(LAST_VIEW_STORAGE_KEY);
  } catch {
    return null;
  }
}

interface UseSelectionReturn {
  selection: SelectionType;
  selectAll: (contentType?: ContentType, options?: NavigateOptions) => void;
  selectFeed: (feedId: string, options?: NavigateOptions) => void;
  selectFolder: (folderId: string, options?: NavigateOptions) => void;
  selectStarred: (options?: NavigateOptions) => void;
  selectSearch: (query: string, options?: NavigateOptions) => void;
  exitSearch: (options?: NavigateOptions) => void;
  selectedEntryId: string | null;
  selectEntry: (entryId: string | null, options?: NavigateOptions) => void;
  unreadOnly: boolean;
  toggleUnreadOnly: () => void;
  contentType: ContentType;
  setContentType: (contentType: ContentType) => void;
}

export function useSelection(): UseSelectionReturn {
  const [location, navigate] = useLocation();
  const search = useSearch();

  const routeState = useMemo(
    () => parseRoute(location, search),
    [location, search],
  );

  const selectAll = useCallback(
    (contentType?: ContentType, options?: NavigateOptions) => {
      navigate(
        buildPath(
          { type: "all" },
          null,
          routeState.unreadOnly,
          contentType ?? routeState.contentType,
        ),
        options,
      );
    },
    [navigate, routeState.unreadOnly, routeState.contentType],
  );

  const selectFeed = useCallback(
    (feedId: string, options?: NavigateOptions) => {
      navigate(
        buildPath(
          { type: "feed", feedId },
          null,
          routeState.unreadOnly,
          routeState.contentType,
        ),
        options,
      );
    },
    [navigate, routeState.unreadOnly, routeState.contentType],
  );

  const selectFolder = useCallback(
    (folderId: string, options?: NavigateOptions) => {
      navigate(
        buildPath(
          { type: "folder", folderId },
          null,
          routeState.unreadOnly,
          routeState.contentType,
        ),
        options,
      );
    },
    [navigate, routeState.unreadOnly, routeState.contentType],
  );

  const selectStarred = useCallback(
    (options?: NavigateOptions) => {
      navigate(
        buildPath(
          { type: "starred" },
          null,
          routeState.unreadOnly,
          routeState.contentType,
        ),
        options,
      );
    },
    [navigate, routeState.unreadOnly, routeState.contentType],
  );

  const selectSearch = useCallback(
    (query: string, options?: NavigateOptions) => {
      const trimmed = query.trim();
      if (!trimmed) return;

      const isSearchView = routeState.selection.type === "search";

      // 记录进入搜索前的视图，退出搜索时恢复
      if (!isSearchView) {
        rememberLastView(
          buildPath(
            routeState.selection,
            null,
            routeState.unreadOnly,
            routeState.contentType,
          ),
        );
      }

      navigate(
        buildPath(
          { type: "search", query: trimmed },
          null,
          false,
          routeState.contentType,
        ),
        // 搜索中输入变化时替换当前历史记录，避免返回键逐字回退
        isSearchView ? { replace: true, ...options } : options,
      );
    },
    [
      navigate,
      routeState.selection,
      routeState.unreadOnly,
      routeState.contentType,
    ],
  );

  const exitSearch = useCallback(
    (options?: NavigateOptions) => {
      const fallback = buildPath(
        { type: "all" },
        null,
        false,
        routeState.contentType,
      );
      navigate(getLastView() ?? fallback, { replace: true, ...options });
    },
    [navigate, routeState.contentType],
  );

  const selectEntry = useCallback(
    (entryId: string | null, options?: NavigateOptions) => {
      navigate(
        buildPath(
          routeState.selection,
          entryId,
          routeState.unreadOnly,
          routeState.contentType,
        ),
        options,
      );
    },
    [
      navigate,
      routeState.selection,
      routeState.unreadOnly,
      routeState.contentType,
    ],
  );

  const toggleUnreadOnly = useCallback(() => {
    navigate(
      buildPath(
        routeState.selection,
        routeState.entryId,
        !routeState.unreadOnly,
        routeState.contentType,
      ),
      { replace: true },
    );
  }, [
    navigate,
    routeState.selection,
    routeState.entryId,
    routeState.unreadOnly,
    routeState.contentType,
  ]);

  const setContentType = useCallback(
    (contentType: ContentType) => {
      navigate(
        buildPath(
          routeState.selection,
          routeState.entryId,
          routeState.unreadOnly,
          contentType,
        ),
      );
    },
    [navigate, routeState.selection, routeState.entryId, routeState.unreadOnly],
  );

  return {
    selection: routeState.selection,
    selectAll,
    selectFeed,
    selectFolder,
    selectStarred,
    selectSearch,
    exitSearch,
    selectedEntryId: routeState.entryId,
    selectEntry,
    unreadOnly: routeState.unreadOnly,
    toggleUnreadOnly,
    contentType: routeState.contentType,
    setContentType,
  };
}

export function selectionToParams(
  selection: SelectionType,
  contentType?: ContentType,
): {
  feedId?: string;
  folderId?: string;
  starredOnly?: boolean;
  contentType?: ContentType;
  q?: string;
} {
  const base: {
    feedId?: string;
    folderId?: string;
    starredOnly?: boolean;
    contentType?: ContentType;
    q?: string;
  } = {};

  // Only include contentType for 'all' selection since feed/folder already filter by their own type
  if (selection.type === "all" && contentType) {
    base.contentType = contentType;
  }

  switch (selection.type) {
    case "all":
      return base;
    case "feed":
      return { ...base, feedId: selection.feedId };
    case "folder":
      return { ...base, folderId: selection.folderId };
    case "starred":
      return { ...base, starredOnly: true };
    case "search":
      return { ...base, contentType, q: selection.query };
  }
}
