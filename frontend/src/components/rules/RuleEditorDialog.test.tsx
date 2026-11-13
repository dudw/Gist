import { fireEvent, render, screen, waitFor } from "@testing-library/react";
import type { ReactNode } from "react";
import { beforeEach, describe, expect, it, vi } from "vitest";
import { RuleEditorDialog } from "./RuleEditorDialog";
import type { ArticleRule } from "@/types/api";

const { mockCreateRule, mockUpdateRule, mockRunRule, mockFeeds } = vi.hoisted(
  () => ({
    mockCreateRule: vi.fn(),
    mockUpdateRule: vi.fn(),
    mockRunRule: vi.fn(),
    mockFeeds: [
      { id: "feed-1", title: "Feed One", type: "article" },
      { id: "feed-2", title: "Feed Two", type: "article" },
    ],
  }),
);

vi.mock("react-i18next", () => ({
  useTranslation: () => ({
    t: (key: string) => key,
  }),
}));

vi.mock("@/hooks/useFeeds", () => ({
  useFeeds: () => ({ data: mockFeeds }),
}));

vi.mock("@/hooks/useRules", () => ({
  useCreateRule: () => ({ mutateAsync: mockCreateRule, isPending: false }),
  useUpdateRule: () => ({ mutateAsync: mockUpdateRule, isPending: false }),
  useRunRule: () => ({ mutateAsync: mockRunRule, isPending: false }),
}));

vi.mock("@/components/ui/dialog", () => ({
  Dialog: ({ children }: { children: ReactNode }) => <div>{children}</div>,
  DialogContent: ({ children }: { children: ReactNode }) => (
    <div>{children}</div>
  ),
  DialogHeader: ({ children }: { children: ReactNode }) => <div>{children}</div>,
  DialogTitle: ({ children }: { children: ReactNode }) => <div>{children}</div>,
  DialogDescription: ({ children }: { children: ReactNode }) => (
    <div>{children}</div>
  ),
  DialogClose: ({ children }: { children: ReactNode }) => <div>{children}</div>,
}));

vi.mock("@/components/ui/dropdown-menu", () => ({
  DropdownMenu: ({ children }: { children: ReactNode }) => <div>{children}</div>,
  DropdownMenuTrigger: ({ children }: { children: ReactNode }) => (
    <>{children}</>
  ),
  DropdownMenuContent: ({ children }: { children: ReactNode }) => (
    <div>{children}</div>
  ),
  DropdownMenuItem: ({
    children,
    onSelect,
    disabled,
  }: {
    children: ReactNode;
    onSelect?: (event: { preventDefault: () => void }) => void;
    disabled?: boolean;
  }) => (
    <button
      type="button"
      disabled={disabled}
      onClick={() => onSelect?.({ preventDefault: () => undefined })}
    >
      {children}
    </button>
  ),
}));

const existingRule: ArticleRule = {
  id: "rule-1",
  name: "AI rule",
  enabled: true,
  fullWord: false,
  feedIds: ["feed-1"],
  action: { type: "mark_read" },
  conditions: [
    { join: "and", field: "title", operator: "contains", value: "AI" },
  ],
  createdAt: "2026-01-01T00:00:00Z",
  updatedAt: "2026-01-01T00:00:00Z",
};

describe("RuleEditorDialog", () => {
  beforeEach(() => {
    vi.clearAllMocks();
    mockCreateRule.mockResolvedValue({ ...existingRule, id: "new-rule" });
    mockUpdateRule.mockResolvedValue(existingRule);
    mockRunRule.mockResolvedValue({ matched: 2, updated: 2 });
  });

  it("edits and saves an existing rule", async () => {
    const onOpenChange = vi.fn();
    render(
      <RuleEditorDialog
        open
        onOpenChange={onOpenChange}
        rule={existingRule}
      />,
    );

    fireEvent.change(screen.getByDisplayValue("AI rule"), {
      target: { value: "Updated rule" },
    });
    fireEvent.click(screen.getByRole("button", { name: "rules.save" }));

    await waitFor(() => {
      expect(mockUpdateRule).toHaveBeenCalledWith({
        id: "rule-1",
        rule: expect.objectContaining({
          name: "Updated rule",
          feedIds: ["feed-1"],
        }),
      });
    });
    expect(onOpenChange).toHaveBeenCalledWith(false);
  });

  it("validates a rule before creating it", () => {
    render(<RuleEditorDialog open onOpenChange={vi.fn()} />);

    fireEvent.click(screen.getByRole("button", { name: "rules.save" }));

    expect(screen.getByText("rules.name_required")).toBeTruthy();
    expect(mockCreateRule).not.toHaveBeenCalled();
  });

  it("saves a new rule before running it", async () => {
    render(
      <RuleEditorDialog
        open
        onOpenChange={vi.fn()}
        initialFeedIds={["feed-1"]}
      />,
    );

    fireEvent.change(screen.getByPlaceholderText("rules.name_placeholder"), {
      target: { value: "New rule" },
    });
    const textboxes = screen.getAllByRole("textbox");
    fireEvent.change(textboxes[1]!, { target: { value: "AI" } });
    fireEvent.click(screen.getByRole("button", { name: "rules.run" }));

    await waitFor(() => {
      expect(mockCreateRule).toHaveBeenCalled();
      expect(mockRunRule).toHaveBeenCalledWith("new-rule");
    });
    expect(screen.getByText("rules.run_result")).toBeTruthy();
  });
});
