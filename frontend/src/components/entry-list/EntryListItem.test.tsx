import { render, screen } from "@testing-library/react";
import { afterEach, beforeEach, describe, expect, it, vi } from "vitest";
import { EntryListItem } from "./EntryListItem";
import type { Entry, Feed } from "@/types/api";

vi.mock("react-i18next", () => ({
  useTranslation: () => ({
    t: (key: string, options?: Record<string, unknown>) => {
      if (key === "add_feed.hours_ago") return `${options?.count} hours ago`;
      if (key === "entry.untitled") return "Untitled";
      if (key === "entry.unknown_feed") return "Unknown feed";
      return key;
    },
  }),
}));

vi.mock("@/stores/translation-store", () => ({
  useTranslationStore: (
    selector: (state: { getTranslation: () => undefined }) => unknown,
  ) => selector({ getTranslation: () => undefined }),
}));

const entry: Entry = {
  id: "entry-1",
  feedId: "feed-1",
  title: "Entry title",
  content: "<p>Entry summary</p>",
  read: false,
  starred: false,
  publishedAt: "2024-01-01T09:00:00.000Z",
  createdAt: "2024-01-01T09:00:00.000Z",
  updatedAt: "2024-01-01T09:00:00.000Z",
};

const feed: Feed = {
  id: "feed-1",
  title: "GitHub File - TechnitiumSoftware/DnsServer",
  url: "https://example.com/feed.xml",
  siteUrl: "https://example.com",
  type: "article",
  createdAt: "2024-01-01T00:00:00.000Z",
  updatedAt: "2024-01-01T00:00:00.000Z",
};

describe("EntryListItem", () => {
  beforeEach(() => {
    vi.useFakeTimers();
    vi.setSystemTime(new Date("2024-01-01T12:00:00.000Z"));
  });

  afterEach(() => {
    vi.useRealTimers();
  });

  it("keeps published time outside the truncated feed title", () => {
    render(
      <EntryListItem
        entry={entry}
        feed={feed}
        isSelected={false}
        onClick={vi.fn()}
      />,
    );

    const feedTitle = screen.getByText(feed.title);
    const publishedAt = screen.getByText("3 hours ago");

    expect(feedTitle.className).toContain("truncate");
    expect(feedTitle.className).not.toContain("flex-1");
    expect(publishedAt.className).toContain("shrink-0");
    expect(publishedAt.className).toContain("whitespace-nowrap");
  });

  it("allows URL previews to wrap instead of clipping horizontally", () => {
    render(
      <EntryListItem
        entry={{
          ...entry,
          title: "重复下载:\nhttps://haloshell.halocloudnet.com/download",
          content:
            "<p>HaloCloud 通知频道（链接：https://haloshell.halocloudnet.com/download）</p>",
        }}
        feed={feed}
        isSelected={false}
        onClick={vi.fn()}
      />,
    );

    const title = screen.getByText(/重复下载/);
    const summary = screen.getByText(/HaloCloud 通知频道/);

    expect(title.className).toContain("wrap-anywhere");
    expect(title.className).toContain("line-clamp-3");
    expect(summary.className).toContain("wrap-anywhere");
    expect(summary.className).toContain("line-clamp-3");
  });

  it("hides the summary when showSummary is false", () => {
    render(
      <EntryListItem
        entry={entry}
        feed={feed}
        isSelected={false}
        onClick={vi.fn()}
        showSummary={false}
      />,
    );

    expect(screen.queryByText("Entry summary")).toBeNull();
  });

  it("搜索时高亮标题中的命中词", () => {
    const { container } = render(
      <EntryListItem
        entry={{ ...entry, title: "Rust 并发编程", content: "<p>正文内容</p>" }}
        feed={feed}
        isSelected={false}
        onClick={vi.fn()}
        searchQuery="并发"
      />,
    );

    const marks = Array.from(container.querySelectorAll("mark"));
    expect(marks.some((mark) => mark.textContent === "并发")).toBe(true);
  });

  it("搜索时展示正文命中片段并高亮", () => {
    const { container } = render(
      <EntryListItem
        entry={{
          ...entry,
          title: "无关标题",
          content: `<p>${"前置内容".repeat(20)}特殊关键词${"后续内容".repeat(20)}</p>`,
        }}
        feed={feed}
        isSelected={false}
        onClick={vi.fn()}
        searchQuery="特殊关键词"
      />,
    );

    const summary = container.querySelector(".line-clamp-2.pr-10");
    expect(summary?.textContent).toContain("特殊关键词");
    expect(summary?.textContent).toContain("…");

    const marks = Array.from(container.querySelectorAll("mark"));
    expect(marks.some((mark) => mark.textContent === "特殊关键词")).toBe(true);
  });

  it("搜索词未命中时不渲染高亮", () => {
    const { container } = render(
      <EntryListItem
        entry={entry}
        feed={feed}
        isSelected={false}
        onClick={vi.fn()}
        searchQuery="不存在"
      />,
    );

    expect(container.querySelectorAll("mark")).toHaveLength(0);
  });
});
