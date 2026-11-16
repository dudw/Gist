import { describe, it, expect } from "vitest";
import {
  IMAGE_REFERRER_POLICIES,
  resolveSrcset,
  toAbsoluteUrl,
  withRetryFragment,
} from "./image-url";

describe("toAbsoluteUrl", () => {
  it("returns absolute URLs untouched", () => {
    expect(toAbsoluteUrl("https://example.com/image.jpg")).toBe(
      "https://example.com/image.jpg",
    );
  });

  it("converts protocol-relative URLs", () => {
    expect(toAbsoluteUrl("//example.com/image.jpg")).toBe(
      "https://example.com/image.jpg",
    );
  });

  it("keeps data URIs", () => {
    const dataUri = "data:image/png;base64,abc123";
    expect(toAbsoluteUrl(dataUri)).toBe(dataUri);
  });

  it("resolves relative paths against the article URL", () => {
    expect(
      toAbsoluteUrl("image.jpg", "https://example.com/article/post.html"),
    ).toBe("https://example.com/article/image.jpg");
  });

  it("resolves root-relative paths against the article origin", () => {
    expect(toAbsoluteUrl("/images/photo.jpg", "https://example.com/a/")).toBe(
      "https://example.com/images/photo.jpg",
    );
  });

  it("normalizes parent segments", () => {
    expect(toAbsoluteUrl("../img.jpg", "https://example.com/a/b/page")).toBe(
      "https://example.com/a/img.jpg",
    );
  });

  it("returns null for relative URLs without a base", () => {
    expect(toAbsoluteUrl("image.jpg")).toBeNull();
  });

  it("returns null for an invalid base URL", () => {
    expect(toAbsoluteUrl("image.jpg", "not a valid url")).toBeNull();
  });

  it("rejects non-http protocols", () => {
    expect(
      toAbsoluteUrl("javascript:alert(1)", "https://example.com/article"),
    ).toBeNull();
  });
});

describe("resolveSrcset", () => {
  it("resolves every candidate and keeps descriptors", () => {
    expect(
      resolveSrcset(
        "/a.jpg 1x, /b.jpg 2x",
        "https://example.com/article/post.html",
      ),
    ).toBe(
      "https://example.com/a.jpg 1x, https://example.com/b.jpg 2x",
    );
  });

  it("leaves unresolvable candidates as-is", () => {
    expect(resolveSrcset("image.jpg")).toBe("image.jpg");
  });
});

describe("withRetryFragment", () => {
  it("returns the original URL for the first attempt", () => {
    expect(withRetryFragment("https://example.com/a.jpg", 0)).toBe(
      "https://example.com/a.jpg",
    );
  });

  it("adds a retry fragment for later attempts", () => {
    expect(withRetryFragment("https://example.com/a.jpg", 2)).toBe(
      "https://example.com/a.jpg#gist-image-retry-2",
    );
  });

  it("adds a retry fragment to relative URLs", () => {
    expect(withRetryFragment("image.jpg", 1)).toBe(
      "image.jpg#gist-image-retry-1",
    );
  });

  it("does not touch inline resources", () => {
    expect(withRetryFragment("data:image/png;base64,abc", 1)).toBe(
      "data:image/png;base64,abc",
    );
  });
});

describe("IMAGE_REFERRER_POLICIES", () => {
  it("tries no-referrer before origin", () => {
    expect(IMAGE_REFERRER_POLICIES).toEqual(["no-referrer", "origin"]);
  });
});
