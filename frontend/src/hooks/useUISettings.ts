import { useCallback, useSyncExternalStore } from "react";

interface UISettings {
  feedColWidth: number;
  entryColWidth: number;
  sidebarVisible: boolean;
  articleWidth: number;
  showEntrySummary: boolean;
  hideReadEntries: boolean;
}

const STORAGE_KEY = "gist-ui-settings";

export const ARTICLE_WIDTH_MIN = 45;
// 实际宽度上限由内容区宽度动态计算，此值仅作为测量不可用时的后备上限
export const ARTICLE_WIDTH_MAX = 100;
export const ARTICLE_WIDTH_STEP = 5;

export const defaultUISettings: UISettings = {
  feedColWidth: 256,
  entryColWidth: 356,
  sidebarVisible: true,
  articleWidth: 65,
  showEntrySummary: true,
  hideReadEntries: false,
};

function getStoredSettings(): UISettings {
  if (typeof window === "undefined") return defaultUISettings;
  try {
    const stored = localStorage.getItem(STORAGE_KEY);
    if (stored) {
      return { ...defaultUISettings, ...JSON.parse(stored) };
    }
  } catch {
    // ignore parse errors
  }
  return defaultUISettings;
}

let cachedSettings: UISettings = getStoredSettings();
const listeners = new Set<() => void>();

function emitChange() {
  for (const listener of listeners) {
    listener();
  }
}

export function getUISettings(): UISettings {
  return cachedSettings;
}

export function hasSidebarVisibilitySetting(): boolean {
  if (typeof window === "undefined") return false;
  try {
    const stored = localStorage.getItem(STORAGE_KEY);
    if (stored) {
      const parsed = JSON.parse(stored);
      return "sidebarVisible" in parsed;
    }
  } catch {
    // ignore parse errors
  }
  return false;
}

export function setUISetting<K extends keyof UISettings>(
  key: K,
  value: UISettings[K],
): void {
  cachedSettings = { ...cachedSettings, [key]: value };
  try {
    localStorage.setItem(STORAGE_KEY, JSON.stringify(cachedSettings));
  } catch {
    // ignore storage errors
  }
  emitChange();
}

function subscribe(callback: () => void): () => void {
  listeners.add(callback);
  return () => listeners.delete(callback);
}

function getSnapshot(): UISettings {
  return cachedSettings;
}

export function useUISettings(): UISettings {
  return useSyncExternalStore(subscribe, getSnapshot, () => defaultUISettings);
}

export function useUISettingKey<K extends keyof UISettings>(
  key: K,
): UISettings[K] {
  const settings = useUISettings();
  return settings[key];
}

export function useUISettingActions() {
  const setFeedColWidth = useCallback((width: number) => {
    setUISetting("feedColWidth", width);
  }, []);

  const setEntryColWidth = useCallback((width: number) => {
    setUISetting("entryColWidth", width);
  }, []);

  const setArticleWidth = useCallback((width: number) => {
    // 上限由阅读区实际能容纳的字符宽度决定，这里只限制下限
    const clamped = Math.max(ARTICLE_WIDTH_MIN, Math.round(width));
    setUISetting("articleWidth", clamped);
  }, []);

  const setSidebarVisible = useCallback((visible: boolean) => {
    setUISetting("sidebarVisible", visible);
  }, []);

  const setShowEntrySummary = useCallback((visible: boolean) => {
    setUISetting("showEntrySummary", visible);
  }, []);

  const setHideReadEntries = useCallback((hidden: boolean) => {
    setUISetting("hideReadEntries", hidden);
  }, []);

  const toggleSidebarVisible = useCallback(() => {
    const current = getUISettings().sidebarVisible;
    setUISetting("sidebarVisible", !current);
  }, []);

  const resetToDefaults = useCallback(() => {
    setUISetting("feedColWidth", defaultUISettings.feedColWidth);
    setUISetting("entryColWidth", defaultUISettings.entryColWidth);
    setUISetting("sidebarVisible", defaultUISettings.sidebarVisible);
    setUISetting("articleWidth", defaultUISettings.articleWidth);
    setUISetting("showEntrySummary", defaultUISettings.showEntrySummary);
    setUISetting("hideReadEntries", defaultUISettings.hideReadEntries);
  }, []);

  return {
    setFeedColWidth,
    setEntryColWidth,
    setArticleWidth,
    setShowEntrySummary,
    setHideReadEntries,
    setSidebarVisible,
    toggleSidebarVisible,
    resetToDefaults,
  };
}
