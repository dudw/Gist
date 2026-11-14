import { act, cleanup, fireEvent, render, screen } from "@testing-library/react";
import { afterEach, beforeEach, describe, expect, it, vi } from "vitest";
import { EntryListHeader } from "./EntryListHeader";

vi.mock("react-i18next", () => ({
  useTranslation: () => ({ t: (key: string) => key }),
}));

const { mockDispatchScrollToTop } = vi.hoisted(() => ({
  mockDispatchScrollToTop: vi.fn(),
}));

vi.mock("@/hooks/useScrollToTop", () => ({
  dispatchScrollToTop: mockDispatchScrollToTop,
}));

const defaultProps = {
  title: "全部文章",
  unreadCount: 0,
  unreadOnly: false,
  onToggleUnreadOnly: vi.fn(),
  onMarkAllRead: vi.fn(),
  onSearch: vi.fn(),
  onExitSearch: vi.fn(),
};

describe("EntryListHeader search", () => {
  beforeEach(() => {
    vi.useFakeTimers();
    vi.clearAllMocks();
  });

  afterEach(() => {
    cleanup();
    vi.useRealTimers();
  });

  it("点击搜索按钮展开输入框", () => {
    render(<EntryListHeader {...defaultProps} />);

    fireEvent.click(screen.getByRole("button", { name: "search.open" }));

    expect(screen.getByPlaceholderText("search.placeholder")).toBeTruthy();
  });

  it("输入关键词后防抖触发搜索", () => {
    render(<EntryListHeader {...defaultProps} />);

    fireEvent.click(screen.getByRole("button", { name: "search.open" }));
    const input = screen.getByPlaceholderText("search.placeholder");
    fireEvent.change(input, { target: { value: "并发" } });

    act(() => {
      vi.advanceTimersByTime(299);
    });
    expect(defaultProps.onSearch).not.toHaveBeenCalled();

    act(() => {
      vi.advanceTimersByTime(1);
    });
    expect(defaultProps.onSearch).toHaveBeenCalledWith("并发");
  });

  it("回车立即提交搜索", () => {
    render(<EntryListHeader {...defaultProps} />);

    fireEvent.click(screen.getByRole("button", { name: "search.open" }));
    const input = screen.getByPlaceholderText("search.placeholder");
    fireEvent.change(input, { target: { value: "  goroutine  " } });
    fireEvent.keyDown(input, { key: "Enter" });

    expect(defaultProps.onSearch).toHaveBeenCalledWith("goroutine");
  });

  it("按 Esc 退出搜索", () => {
    render(<EntryListHeader {...defaultProps} searchQuery="并发" />);

    fireEvent.keyDown(screen.getByPlaceholderText("search.placeholder"), {
      key: "Escape",
    });

    expect(defaultProps.onExitSearch).toHaveBeenCalled();
  });

  it("Ctrl/Cmd+K 打开搜索框", () => {
    render(<EntryListHeader {...defaultProps} />);

    fireEvent.keyDown(window, { key: "k", ctrlKey: true });

    expect(screen.getByPlaceholderText("search.placeholder")).toBeTruthy();
  });

  it("搜索模式下隐藏全部标为已读按钮", () => {
    render(
      <EntryListHeader
        {...defaultProps}
        searchQuery="并发"
        hideMarkAllRead
      />,
    );

    expect(screen.queryByTitle("entry.mark_all_read")).toBeNull();
  });

  it("未提供搜索回调时不显示搜索入口", () => {
    render(<EntryListHeader {...defaultProps} onSearch={undefined} />);

    expect(screen.queryByRole("button", { name: "search.open" })).toBeNull();
  });
});
