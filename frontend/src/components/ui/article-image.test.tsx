/**
 * ArticleImage 测试
 *
 * 图片由浏览器直接加载，按文章链接解析绝对地址，并依次尝试 Referer 策略：
 * 1. no-referrer（空 Referer，兼容多数防盗链）
 * 2. origin（只发送本站 origin）
 * 全部失败后展示错误占位与「打开原图」入口，避免静默渲染一个小灰块
 */
import { describe, it, expect, vi } from "vitest";
import { render, screen, fireEvent } from "@testing-library/react";
import { ArticleImage, ArticleLinkContext } from "./article-image";

vi.mock("react-i18next", () => ({
  useTranslation: () => ({ t: (key: string) => key }),
}));

const ARTICLE_URL = "https://www.example.com/torrent/PPV1";
const IMAGE_SRC = "https://www.example.com/movies/PPV1.jpg";

function renderImage(src = IMAGE_SRC) {
  return render(
    <ArticleLinkContext.Provider value={ARTICLE_URL}>
      <ArticleImage src={src} alt="封面" />
    </ArticleLinkContext.Provider>,
  );
}

describe("ArticleImage", () => {
  it("默认直连原图并优先使用 no-referrer", () => {
    renderImage();

    const img = screen.getByRole("img");
    expect(img.getAttribute("src")).toBe(IMAGE_SRC);
    expect(img.getAttribute("referrerpolicy")).toBe("no-referrer");
  });

  it("把相对地址按文章链接解析为绝对地址", () => {
    renderImage("/movies/PPV1.jpg");

    expect(screen.getByRole("img").getAttribute("src")).toBe(IMAGE_SRC);
  });

  it("加载失败后换 origin 策略并强制重新请求", () => {
    renderImage();

    fireEvent.error(screen.getByRole("img"));

    const img = screen.getByRole("img");
    expect(img.getAttribute("src")).toBe(`${IMAGE_SRC}#gist-image-retry-1`);
    expect(img.getAttribute("referrerpolicy")).toBe("origin");
  });

  it("所有策略失败后展示错误占位与「打开原图」入口", () => {
    renderImage();

    fireEvent.error(screen.getByRole("img"));
    fireEvent.error(screen.getByRole("img"));

    expect(screen.queryByRole("img")).toBeNull();
    expect(screen.getByText("封面")).toBeTruthy();

    const link = screen.getByRole("link", {
      name: "entry.open_original_image",
    });
    expect(link.getAttribute("href")).toBe(IMAGE_SRC);
    expect(link.getAttribute("target")).toBe("_blank");
    expect(link.getAttribute("rel")).toBe("noopener noreferrer");
  });

  it("data URI 无法重试时直接展示错误占位", () => {
    renderImage("data:image/gif;base64,R0lGODlhAQABAAAAACw=");

    fireEvent.error(screen.getByRole("img"));

    expect(screen.queryByRole("img")).toBeNull();
    expect(screen.getByText("entry.open_original_image")).toBeTruthy();
  });

  it("无 alt 时错误占位展示默认提示文案", () => {
    const { container } = render(
      <ArticleLinkContext.Provider value={ARTICLE_URL}>
        <ArticleImage src={IMAGE_SRC} />
      </ArticleLinkContext.Provider>,
    );

    // alt 为空时 img 的 role 是 presentation，只能按标签查询
    for (let i = 0; i < 2; i += 1) {
      fireEvent.error(container.querySelector("img")!);
    }

    expect(screen.getByText("entry.image_load_failed")).toBeTruthy();
  });
});
