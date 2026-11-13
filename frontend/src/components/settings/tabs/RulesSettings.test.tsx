import { fireEvent, render, screen, waitFor } from "@testing-library/react";
import type { ReactNode } from "react";
import { beforeEach, describe, expect, it, vi } from "vitest";
import { RulesSettings } from "./RulesSettings";
import type { ArticleRule } from "@/types/api";

const {
  mockRules,
  mockSetEnabled,
  mockDeleteRule,
  mockRunRule,
  mockRulesQuery,
} = vi.hoisted(() => ({
  mockRules: [
    {
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
    },
  ] as ArticleRule[],
  mockSetEnabled: vi.fn(),
  mockDeleteRule: vi.fn(),
  mockRunRule: vi.fn(),
  mockRulesQuery: {
    data: [] as ArticleRule[],
    isLoading: false,
    isError: false,
  },
}));

vi.mock("react-i18next", () => ({
  useTranslation: () => ({ t: (key: string) => key }),
}));

vi.mock("@/hooks/useRules", () => ({
  useRules: () => mockRulesQuery,
  useSetRuleEnabled: () => ({ mutate: mockSetEnabled }),
  useDeleteRule: () => ({ mutateAsync: mockDeleteRule }),
  useRunRule: () => ({ mutateAsync: mockRunRule }),
}));

vi.mock("@/components/ui/switch", () => ({
  Switch: ({
    checked,
    onCheckedChange,
  }: {
    checked: boolean;
    onCheckedChange: (checked: boolean) => void;
  }) => (
    <button
      type="button"
      role="switch"
      aria-checked={checked}
      onClick={() => onCheckedChange(!checked)}
    />
  ),
}));

vi.mock("@/components/rules/RuleEditorDialog", () => ({
  RuleEditorDialog: ({
    open,
    rule,
  }: {
    open: boolean;
    rule?: ArticleRule | null;
  }) =>
    open ? (
      <div data-testid="rule-editor">{rule?.id ?? "new"}</div>
    ) : null,
}));

vi.mock("@/components/ui/dialog", () => ({
  Dialog: ({ children }: { children: ReactNode }) => <div>{children}</div>,
}));

describe("RulesSettings", () => {
  beforeEach(() => {
    vi.clearAllMocks();
    mockRulesQuery.data = mockRules;
    mockRulesQuery.isLoading = false;
    mockRulesQuery.isError = false;
    mockRunRule.mockResolvedValue({ matched: 1, updated: 1 });
    mockDeleteRule.mockResolvedValue(undefined);
  });

  it("lists rules and opens the editor", () => {
    render(<RulesSettings />);

    expect(screen.getByText("AI rule")).toBeTruthy();
    expect(screen.getByText(/rules.feed_count/)).toBeTruthy();

    fireEvent.click(screen.getByRole("button", { name: /rules.add/ }));
    expect(screen.getByTestId("rule-editor").textContent).toBe("new");
  });

  it("toggles and runs a rule", async () => {
    render(<RulesSettings />);

    fireEvent.click(screen.getByRole("switch"));
    expect(mockSetEnabled).toHaveBeenCalledWith({
      id: "rule-1",
      enabled: false,
    });

    fireEvent.click(screen.getByRole("button", { name: "rules.run" }));
    await waitFor(() => {
      expect(mockRunRule).toHaveBeenCalledWith("rule-1");
    });
  });
});
