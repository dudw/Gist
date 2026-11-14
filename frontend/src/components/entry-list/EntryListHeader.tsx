import {
  useCallback,
  useEffect,
  useRef,
  useState,
  type CompositionEvent as ReactCompositionEvent,
  type KeyboardEvent as ReactKeyboardEvent,
} from "react";
import { useTranslation } from "react-i18next";
import {
  CircleOutlineIcon,
  CircleFilledIcon,
  CheckCircleIcon,
  CloseIcon,
  MenuIcon,
  SearchIcon,
} from "@/components/ui/icons";
import { dispatchScrollToTop } from "@/hooks/useScrollToTop";

interface EntryListHeaderProps {
  title: string;
  unreadCount: number;
  unreadOnly: boolean;
  onToggleUnreadOnly: () => void;
  onMarkAllRead: () => void;
  scrollToTopScope?: string;
  isMobile?: boolean;
  onMenuClick?: () => void;
  isTablet?: boolean;
  onToggleSidebar?: () => void;
  sidebarVisible?: boolean;
  searchQuery?: string;
  onSearch?: (query: string) => void;
  onExitSearch?: () => void;
  hideMarkAllRead?: boolean;
}

const SEARCH_DEBOUNCE_MS = 300;

export function EntryListHeader({
  title,
  unreadCount,
  unreadOnly,
  onToggleUnreadOnly,
  onMarkAllRead,
  scrollToTopScope,
  isMobile,
  onMenuClick,
  isTablet,
  onToggleSidebar,
  sidebarVisible,
  searchQuery,
  onSearch,
  onExitSearch,
  hideMarkAllRead,
}: EntryListHeaderProps) {
  const { t } = useTranslation();
  const searchEnabled = Boolean(onSearch);
  const [searchOpen, setSearchOpen] = useState(false);
  // 路由中存在关键词（含直接访问、前进后退）时保持搜索框展开
  const searchVisible = searchEnabled && (searchOpen || Boolean(searchQuery));
  const searchInputRef = useRef<HTMLInputElement>(null);
  const debounceTimer = useRef<ReturnType<typeof setTimeout> | null>(null);
  const composingRef = useRef(false);

  const clearDebounce = useCallback(() => {
    if (debounceTimer.current) {
      clearTimeout(debounceTimer.current);
      debounceTimer.current = null;
    }
  }, []);

  useEffect(() => clearDebounce, [clearDebounce]);

  // 外部导航改变关键词时同步输入框（输入框聚焦时以用户输入为准）
  useEffect(() => {
    const input = searchInputRef.current;
    if (!input || document.activeElement === input) return;
    input.value = searchQuery ?? "";
  }, [searchQuery, searchVisible]);

  useEffect(() => {
    if (searchVisible) {
      searchInputRef.current?.focus();
    }
  }, [searchVisible]);

  // Ctrl/Cmd+K 打开搜索
  useEffect(() => {
    if (!searchEnabled) return;

    const handleKeyDown = (event: KeyboardEvent) => {
      if (!(event.metaKey || event.ctrlKey)) return;
      if (event.key.toLowerCase() !== "k") return;
      event.preventDefault();
      setSearchOpen(true);
    };

    window.addEventListener("keydown", handleKeyDown);
    return () => window.removeEventListener("keydown", handleKeyDown);
  }, [searchEnabled]);

  const closeSearch = useCallback(() => {
    clearDebounce();
    setSearchOpen(false);
    if (searchQuery) {
      onExitSearch?.();
    }
  }, [clearDebounce, onExitSearch, searchQuery]);

  const submitSearch = useCallback(
    (value: string) => {
      clearDebounce();
      const trimmed = value.trim();
      if (!trimmed) {
        closeSearch();
        return;
      }
      if (trimmed !== (searchQuery ?? "")) {
        onSearch?.(trimmed);
      }
    },
    [clearDebounce, closeSearch, onSearch, searchQuery],
  );

  const handleSearchChange = useCallback(
    (value: string) => {
      if (composingRef.current) return;

      clearDebounce();
      debounceTimer.current = setTimeout(() => {
        const trimmed = value.trim();
        if (!trimmed) {
          setSearchOpen(false);
          if (searchQuery) {
            onExitSearch?.();
          }
          return;
        }
        if (trimmed !== (searchQuery ?? "")) {
          onSearch?.(trimmed);
        }
      }, SEARCH_DEBOUNCE_MS);
    },
    [clearDebounce, onExitSearch, onSearch, searchQuery],
  );

  const handleCompositionStart = useCallback(() => {
    composingRef.current = true;
  }, []);

  const handleCompositionEnd = useCallback(
    (event: ReactCompositionEvent<HTMLInputElement>) => {
      composingRef.current = false;
      handleSearchChange(event.currentTarget.value);
    },
    [handleSearchChange],
  );

  const handleSearchKeyDown = useCallback(
    (event: ReactKeyboardEvent<HTMLInputElement>) => {
      if (event.key === "Enter") {
        event.preventDefault();
        submitSearch(event.currentTarget.value);
        return;
      }
      if (event.key === "Escape") {
        event.preventDefault();
        closeSearch();
      }
    },
    [closeSearch, submitSearch],
  );

  if (searchVisible) {
    return (
      <div className="flex h-14 shrink-0 items-center gap-1 px-2">
        {isMobile && onMenuClick && (
          <button
            type="button"
            onClick={onMenuClick}
            className="flex size-11 shrink-0 items-center justify-center rounded-md transition-colors hover:bg-item-hover"
          >
            <MenuIcon className="size-5" />
          </button>
        )}

        <div className="relative min-w-0 flex-1">
          <SearchIcon className="pointer-events-none absolute left-2.5 top-1/2 size-4 -translate-y-1/2 text-muted-foreground" />
          <input
            ref={searchInputRef}
            type="text"
            inputMode="search"
            maxLength={100}
            defaultValue={searchQuery ?? ""}
            onChange={(event) => handleSearchChange(event.target.value)}
            onCompositionStart={handleCompositionStart}
            onCompositionEnd={handleCompositionEnd}
            onKeyDown={handleSearchKeyDown}
            placeholder={t("search.placeholder")}
            aria-label={t("search.placeholder")}
            className="h-9 w-full rounded-md border border-border bg-background pl-8 pr-8 text-sm outline-none transition-colors focus:border-primary/50"
          />
          <button
            type="button"
            onClick={closeSearch}
            aria-label={t("search.clear")}
            className="absolute right-2 top-1/2 flex size-5 -translate-y-1/2 items-center justify-center rounded text-muted-foreground transition-colors hover:text-foreground"
          >
            <CloseIcon className="size-3.5" />
          </button>
        </div>

        <button
          type="button"
          onClick={closeSearch}
          className="h-9 shrink-0 rounded-md px-2.5 text-sm text-muted-foreground transition-colors hover:bg-item-hover hover:text-foreground"
        >
          {t("search.cancel")}
        </button>
      </div>
    );
  }

  return (
    <div className="flex h-14 items-center justify-between gap-4 px-4 shrink-0">
      <div className="flex min-w-0 flex-1 items-center gap-2">
        {isMobile && onMenuClick && (
          <button
            type="button"
            onClick={onMenuClick}
            className="flex size-11 shrink-0 items-center justify-center rounded-md transition-colors hover:bg-item-hover -ml-1.5"
          >
            <MenuIcon className="size-5" />
          </button>
        )}
        {isTablet && onToggleSidebar && (
          <button
            type="button"
            onClick={onToggleSidebar}
            title={
              sidebarVisible
                ? t("actions.hide_sidebar")
                : t("actions.show_sidebar")
            }
            className="flex size-11 shrink-0 items-center justify-center rounded-md transition-all duration-200 ease-[var(--ease-ios)] hover:bg-item-hover active:scale-95 -ml-1.5"
          >
            <MenuIcon className="size-5" />
          </button>
        )}
        <h2
          className="truncate text-lg font-bold cursor-pointer active:opacity-70 transition-opacity"
          onClick={() => dispatchScrollToTop(scrollToTopScope)}
        >
          {title}
        </h2>
        {unreadCount > 0 && (
          <span className="shrink-0 text-xs text-muted-foreground">
            {t("entry.unread_count", { count: unreadCount })}
          </span>
        )}
      </div>

      <div className="flex items-center">
        {searchEnabled && (
          <button
            type="button"
            onClick={() => setSearchOpen(true)}
            title={t("search.open")}
            aria-label={t("search.open")}
            className="flex size-8 items-center justify-center rounded-md transition-colors hover:bg-item-hover"
          >
            <SearchIcon className="size-5" />
          </button>
        )}
        <button
          type="button"
          onClick={onToggleUnreadOnly}
          title={unreadOnly ? t("entry.show_all") : t("entry.show_unread_only")}
          className="flex size-8 items-center justify-center rounded-md transition-colors hover:bg-item-hover"
        >
          {unreadOnly ? (
            <CircleFilledIcon className="size-5" />
          ) : (
            <CircleOutlineIcon className="size-5" />
          )}
        </button>
        {!hideMarkAllRead && (
          <button
            type="button"
            onClick={onMarkAllRead}
            title={t("entry.mark_all_read")}
            className="flex size-8 items-center justify-center rounded-md transition-colors hover:bg-item-hover"
          >
            <CheckCircleIcon className="size-4" />
          </button>
        )}
      </div>
    </div>
  );
}
