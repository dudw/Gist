import { describe, expect, it } from "vitest";
import {
  buildSearchSnippet,
  getSearchTerms,
  splitByTerms,
} from "./search-highlight";

describe("search-highlight", () => {
  describe("getSearchTerms", () => {
    it("按空白拆分、忽略大小写去重并按长度排序", () => {
      expect(getSearchTerms("  Rust   rust 并发 ")).toEqual(["Rust", "并发"]);
    });

    it("空查询返回空数组", () => {
      expect(getSearchTerms("   ")).toEqual([]);
    });
  });

  describe("splitByTerms", () => {
    it("按关键词切分并标记命中", () => {
      expect(splitByTerms("Hello World", "world")).toEqual([
        { text: "Hello ", match: false },
        { text: "World", match: true },
      ]);
    });

    it("支持中文和多关键词", () => {
      const segments = splitByTerms(
        "Go 并发编程与 channel 实践",
        "并发 channel",
      );

      expect(segments.filter((segment) => segment.match).map((s) => s.text)).toEqual(
        ["并发", "channel"],
      );
    });

    it("没有关键词时返回整段文本", () => {
      expect(splitByTerms("abc", "  ")).toEqual([
        { text: "abc", match: false },
      ]);
    });
  });

  describe("buildSearchSnippet", () => {
    it("截取命中词附近并添加省略号", () => {
      const text = `${"前".repeat(200)}关键词${"后".repeat(200)}`;

      const snippet = buildSearchSnippet(text, "关键词");

      expect(snippet).toContain("关键词");
      expect(snippet?.startsWith("…")).toBe(true);
      expect(snippet?.endsWith("…")).toBe(true);
      expect(snippet?.length).toBeLessThanOrEqual(162);
    });

    it("未命中返回 null", () => {
      expect(buildSearchSnippet("没有相关内容", "关键词")).toBeNull();
    });

    it("空文本或空查询返回 null", () => {
      expect(buildSearchSnippet("", "x")).toBeNull();
      expect(buildSearchSnippet("abc", "")).toBeNull();
    });
  });
});
