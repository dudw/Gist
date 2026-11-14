import { fireEvent, render, screen } from "@testing-library/react";
import { beforeEach, describe, expect, it, vi } from "vitest";
import { EntryContentBody } from "./EntryContentBody";
import type { Entry } from "@/types/api";

const { mockSetArticleWidth, mockArticleWidth } = vi.hoisted(() => ({
  mockSetArticleWidth: vi.fn(),
  mockArticleWidth: { value: 65 },
}));

vi.mock("@/hooks/useUISettings", () => ({
  ARTICLE_WIDTH_MIN: 45,
  ARTICLE_WIDTH_MAX: 100,
  ARTICLE_WIDTH_STEP: 5,
  useUISettingKey: () => mockArticleWidth.value,
  useUISettingActions: () => ({ setArticleWidth: mockSetArticleWidth }),
}));

vi.mock("react-i18next", () => ({
  useTranslation: () => ({
    t: (key: string) => key,
  }),
}));

vi.mock("@/hooks/useCodeHighlight", () => ({
  useCodeHighlight: vi.fn(),
}));

vi.mock("@/hooks/useEntryMeta", () => ({
  useEntryMeta: () => ({
    publishedLong: null,
    readingTime: null,
  }),
}));

vi.mock("@/components/ui/article-content", () => ({
  ArticleContent: ({ content }: { content?: string }) => <div>{content}</div>,
}));

const entry: Entry = {
  id: "entry-1",
  feedId: "feed-1",
  title: "重复下载:\nhttps://haloshell.halocloudnet.com/download",
  url: "https://haloshell.halocloudnet.com/download",
  content: "<p>修复了部分错误</p>",
  read: false,
  starred: false,
  createdAt: "2026-05-10T00:00:00.000Z",
  updatedAt: "2026-05-10T00:00:00.000Z",
};

describe("EntryContentBody", () => {
  beforeEach(() => {
    mockArticleWidth.value = 65;
    mockSetArticleWidth.mockClear();
  });

  it("allows long URL titles to wrap instead of clipping horizontally", () => {
    render(
      <EntryContentBody
        entry={entry}
        scrollRef={vi.fn()}
        displayContent={entry.content}
      />,
    );

    const titleLink = screen.getByRole("link", { name: /haloshell/ });
    const title = titleLink.closest("h1");

    expect(title?.className).toContain("wrap-anywhere");
    expect(titleLink.className).not.toContain("wrap-anywhere");
  });

  it("applies the saved article width", () => {
    mockArticleWidth.value = 80;
    const { container } = render(
      <EntryContentBody
        entry={entry}
        scrollRef={vi.fn()}
        displayContent={entry.content}
      />,
    );

    expect(container.querySelector("article")?.style.maxWidth).toBe("80ch");
  });

  it("fills the content area when the width reaches the maximum", () => {
    mockArticleWidth.value = 100;
    const { container } = render(
      <EntryContentBody
        entry={entry}
        scrollRef={vi.fn()}
        displayContent={entry.content}
      />,
    );

    expect(container.querySelector("article")?.style.maxWidth).toBe("100%");
  });

  it("uses the content area width as the dynamic maximum", () => {
    const spanOffsetWidth = Object.getOwnPropertyDescriptor(
      HTMLSpanElement.prototype,
      "offsetWidth",
    );
    const divClientWidth = Object.getOwnPropertyDescriptor(
      HTMLDivElement.prototype,
      "clientWidth",
    );

    try {
      Object.defineProperty(HTMLSpanElement.prototype, "offsetWidth", {
        configurable: true,
        get: () => 800,
      });
      Object.defineProperty(HTMLDivElement.prototype, "clientWidth", {
        configurable: true,
        get() {
          return this.classList?.contains("entry-content-viewport")
            ? 1200
            : 0;
        },
      });

      mockArticleWidth.value = 95;
      const { container, unmount } = render(
        <EntryContentBody
          entry={entry}
          scrollRef={vi.fn()}
          displayContent={entry.content}
        />,
      );

      // 95ch 只占内容区的一部分，仍按 5ch 逐级调整
      expect(container.querySelector("article")?.style.maxWidth).toBe("95ch");
      fireEvent.click(
        screen.getByRole("button", { name: "entry.increase_width" }),
      );
      expect(mockSetArticleWidth).toHaveBeenCalledWith(100);

      fireEvent.click(
        screen.getByRole("button", { name: "entry.decrease_width" }),
      );
      expect(mockSetArticleWidth).toHaveBeenCalledWith(90);

      unmount();
      mockArticleWidth.value = 150;
      const fullWidthRender = render(
        <EntryContentBody
          entry={entry}
          scrollRef={vi.fn()}
          displayContent={entry.content}
        />,
      );

      expect(
        fullWidthRender.container.querySelector("article")?.style.maxWidth,
      ).toBe("100%");
    } finally {
      if (spanOffsetWidth) {
        Object.defineProperty(
          HTMLSpanElement.prototype,
          "offsetWidth",
          spanOffsetWidth,
        );
      } else {
        delete (HTMLSpanElement.prototype as { offsetWidth?: number })
          .offsetWidth;
      }
      if (divClientWidth) {
        Object.defineProperty(
          HTMLDivElement.prototype,
          "clientWidth",
          divClientWidth,
        );
      } else {
        delete (HTMLDivElement.prototype as { clientWidth?: number })
          .clientWidth;
      }
    }
  });

  it("increases and decreases the article width", () => {
    mockArticleWidth.value = 80;
    render(
      <EntryContentBody
        entry={entry}
        scrollRef={vi.fn()}
        displayContent={entry.content}
      />,
    );

    fireEvent.click(
      screen.getByRole("button", { name: "entry.increase_width" }),
    );
    expect(mockSetArticleWidth).toHaveBeenCalledWith(85);

    fireEvent.click(
      screen.getByRole("button", { name: "entry.decrease_width" }),
    );
    expect(mockSetArticleWidth).toHaveBeenCalledWith(75);
  });

  it("disables width controls at the limits", () => {
    mockArticleWidth.value = 45;
    const { unmount } = render(
      <EntryContentBody
        entry={entry}
        scrollRef={vi.fn()}
        displayContent={entry.content}
      />,
    );

    expect(
      (
        screen.getByRole("button", {
          name: "entry.decrease_width",
        }) as HTMLButtonElement
      ).disabled,
    ).toBe(true);
    expect(
      (
        screen.getByRole("button", {
          name: "entry.increase_width",
        }) as HTMLButtonElement
      ).disabled,
    ).toBe(false);

    unmount();
    mockArticleWidth.value = 100;
    render(
      <EntryContentBody
        entry={entry}
        scrollRef={vi.fn()}
        displayContent={entry.content}
      />,
    );

    expect(
      (
        screen.getByRole("button", {
          name: "entry.increase_width",
        }) as HTMLButtonElement
      ).disabled,
    ).toBe(true);
    expect(
      (
        screen.getByRole("button", {
          name: "entry.decrease_width",
        }) as HTMLButtonElement
      ).disabled,
    ).toBe(false);
  });
});
