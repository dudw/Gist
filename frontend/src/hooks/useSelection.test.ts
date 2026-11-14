import { act, renderHook } from "@testing-library/react";
import { afterEach, beforeEach, describe, expect, it, vi } from "vitest";
import { useSelection } from "./useSelection";

const routeState = {
  location: "/feed/1",
  search: "type=article",
};

const { mockNavigate } = vi.hoisted(() => ({
  mockNavigate: vi.fn(),
}));

vi.mock("wouter", () => ({
  useLocation: () => [routeState.location, mockNavigate],
  useSearch: () => routeState.search,
}));

const storage = new Map<string, string>();

describe("useSelection search navigation", () => {
  beforeEach(() => {
    routeState.location = "/feed/1";
    routeState.search = "type=article";
    mockNavigate.mockClear();
    storage.clear();

    vi.stubGlobal("sessionStorage", {
      getItem: (key: string) => storage.get(key) ?? null,
      setItem: (key: string, value: string) => {
        storage.set(key, value);
      },
    });
  });

  afterEach(() => {
    vi.unstubAllGlobals();
  });

  it("从普通视图进入搜索时记录来源视图并使用 push", () => {
    const { result } = renderHook(() => useSelection());

    act(() => {
      result.current.selectSearch("并发");
    });

    expect(mockNavigate).toHaveBeenCalledWith(
      "/search?q=%E5%B9%B6%E5%8F%91&type=article",
      undefined,
    );

    act(() => {
      result.current.exitSearch();
    });

    expect(mockNavigate).toHaveBeenLastCalledWith("/feed/1?type=article", {
      replace: true,
    });
  });

  it("搜索视图内修改关键词时替换当前历史记录", () => {
    routeState.location = "/search";
    routeState.search = "q=%E5%B9%B6%E5%8F%91&type=article";

    const { result } = renderHook(() => useSelection());

    act(() => {
      result.current.selectSearch("goroutine");
    });

    expect(mockNavigate).toHaveBeenCalledWith(
      "/search?q=goroutine&type=article",
      { replace: true },
    );
  });

  it("没有历史视图时退出搜索回到全部文章", () => {
    routeState.location = "/search";
    routeState.search = "q=goroutine&type=article";

    const { result } = renderHook(() => useSelection());

    act(() => {
      result.current.exitSearch();
    });

    expect(mockNavigate).toHaveBeenCalledWith("/all?type=article", {
      replace: true,
    });
  });
});
