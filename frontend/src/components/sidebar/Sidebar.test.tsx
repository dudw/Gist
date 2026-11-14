import { describe, it, expect, vi, beforeEach } from "vitest";
import { fireEvent, render, screen } from "@testing-library/react";
import type { ReactNode } from "react";
import { Sidebar } from "./Sidebar";
import type { Feed } from "@/types/api";

const { mockDeleteFeed } = vi.hoisted(() => ({
  mockDeleteFeed: vi.fn(),
}));

const feed: Feed = {
  id: "feed-1",
  title: "Feed A",
  url: "https://example.com/feed.xml",
  type: "article",
  createdAt: "2026-01-01T00:00:00Z",
  updatedAt: "2026-01-01T00:00:00Z",
};

vi.mock("react-i18next", () => ({
  useTranslation: () => ({
    t: (key: string, params?: Record<string, string | number>) =>
      params && typeof params.name !== "undefined"
        ? `${key}:${params.name}`
        : key,
  }),
}));

vi.mock("framer-motion", () => ({
  AnimatePresence: ({ children }: { children: ReactNode }) => <>{children}</>,
  motion: {
    div: ({ children, className }: { children?: ReactNode; className?: string }) => (
      <div className={className}>{children}</div>
    ),
  },
}));

vi.mock("@/hooks/useFeeds", () => ({
  useFeeds: () => ({ data: [feed] }),
  useDeleteFeed: () => ({ mutate: mockDeleteFeed }),
  useUpdateFeed: () => ({ mutate: vi.fn() }),
  useUpdateFeedType: () => ({ mutate: vi.fn() }),
  useRefreshFeed: () => ({ mutate: vi.fn() }),
  useRefreshingFeedIds: () => [],
}));

vi.mock("@/hooks/useFolders", () => ({
  useFolders: () => ({ data: [] }),
  useDeleteFolder: () => ({ mutate: vi.fn() }),
  useUpdateFolderType: () => ({ mutate: vi.fn() }),
}));

vi.mock("@/hooks/useEntries", () => ({
  useUnreadCounts: () => ({ data: { counts: {} } }),
}));

vi.mock("@/hooks/useAuth", () => ({
  useAuth: () => ({ user: { username: "tester" }, logout: vi.fn() }),
}));

vi.mock("@/components/settings", () => ({
  SettingsModal: () => null,
  ProfileModal: () => null,
}));

vi.mock("@/components/settings/tabs/EditFeedDialog", () => ({
  EditFeedDialog: () => null,
}));

vi.mock("./SidebarHeader", () => ({
  SidebarHeader: () => <div />,
}));

vi.mock("./ContentTypeSwitcher", () => ({
  ContentTypeSwitcher: () => <div />,
}));

vi.mock("./FeedItem", () => ({
  FeedItem: ({
    name,
    feedId,
    onDelete,
  }: {
    name: string;
    feedId: string;
    onDelete?: (feedId: string) => void;
  }) => <button onClick={() => onDelete?.(feedId)}>{name}</button>,
}));

vi.mock("@/components/ui/dialog", () => ({
  Dialog: ({ open, children }: { open?: boolean; children: ReactNode }) =>
    open ? <div>{children}</div> : null,
  DialogContent: ({ children }: { children: ReactNode }) => (
    <div>{children}</div>
  ),
  DialogHeader: ({ children }: { children: ReactNode }) => <div>{children}</div>,
  DialogTitle: ({ children }: { children: ReactNode }) => <div>{children}</div>,
  DialogDescription: ({ children }: { children: ReactNode }) => (
    <div>{children}</div>
  ),
}));

function renderSidebar() {
  return render(
    <Sidebar
      selection={{ type: "all" }}
      onSelectFeed={vi.fn()}
      onSelectFolder={vi.fn()}
      onSelectStarred={vi.fn()}
      onSelectAll={vi.fn()}
      contentType="article"
    />,
  );
}

describe("Sidebar 删除订阅", () => {
  beforeEach(() => {
    vi.clearAllMocks();
  });

  it("右键删除时先弹出确认，确认后才删除", () => {
    renderSidebar();

    fireEvent.click(screen.getByRole("button", { name: "Feed A" }));

    expect(screen.getByText("feeds.delete_confirm_title")).not.toBeNull();
    expect(
      screen.getByText("feeds.delete_confirm_message:Feed A"),
    ).not.toBeNull();
    expect(mockDeleteFeed).not.toHaveBeenCalled();

    fireEvent.click(screen.getByRole("button", { name: "actions.delete" }));

    expect(mockDeleteFeed).toHaveBeenCalledWith("feed-1");
    expect(screen.queryByText("feeds.delete_confirm_title")).toBeNull();
  });

  it("取消后不删除订阅", () => {
    renderSidebar();

    fireEvent.click(screen.getByRole("button", { name: "Feed A" }));
    fireEvent.click(screen.getByRole("button", { name: "actions.cancel" }));

    expect(mockDeleteFeed).not.toHaveBeenCalled();
    expect(screen.queryByText("feeds.delete_confirm_title")).toBeNull();
  });
});
