import { useState } from "react";
import { useTranslation } from "react-i18next";
import { cn } from "@/lib/utils";
import {
  Dialog,
  DialogClose,
  DialogContent,
  DialogDescription,
  DialogHeader,
  DialogTitle,
} from "@/components/ui/dialog";
import {
  DropdownMenu,
  DropdownMenuContent,
  DropdownMenuItem,
  DropdownMenuTrigger,
} from "@/components/ui/dropdown-menu";
import { useFeeds } from "@/hooks/useFeeds";
import {
  useCreateRule,
  useRunRule,
  useUpdateRule,
} from "@/hooks/useRules";
import type {
  ArticleRule,
  ArticleRuleInput,
  RuleCondition,
  RuleField,
  RuleJoin,
  RuleOperator,
} from "@/types/api";

const RULE_FIELDS: RuleField[] = [
  "title_or_content",
  "title",
  "content",
  "author",
  "url",
  "url_no_domain",
];

const RULE_OPERATORS: RuleOperator[] = [
  "contains",
  "not_contains",
  "is",
  "is_not",
  "starts_with",
  "ends_with",
  "regex",
  "not_regex",
];

const RULE_JOINS: RuleJoin[] = ["and", "or"];

function newCondition(): RuleCondition {
  return {
    join: "and",
    field: "title_or_content",
    operator: "contains",
    value: "",
  };
}

interface RuleEditorDialogProps {
  open: boolean;
  onOpenChange: (open: boolean) => void;
  rule?: ArticleRule | null;
  initialFeedIds?: string[];
}

export function RuleEditorDialog({
  open,
  onOpenChange,
  rule = null,
  initialFeedIds = [],
}: RuleEditorDialogProps) {
  const { t } = useTranslation();
  const { data: feeds = [] } = useFeeds();
  const createRule = useCreateRule();
  const updateRule = useUpdateRule();
  const runRule = useRunRule();

  const [name, setName] = useState(rule?.name ?? "");
  const enabled = rule?.enabled ?? true;
  const [fullWord, setFullWord] = useState(rule?.fullWord ?? false);
  const [feedIds, setFeedIds] = useState<string[]>(
    rule?.feedIds ?? initialFeedIds,
  );
  const [conditions, setConditions] = useState<RuleCondition[]>(
    rule?.conditions.length ? rule.conditions : [newCondition()],
  );
  const [savedId, setSavedId] = useState<string | null>(rule?.id ?? null);
  const [error, setError] = useState<string | null>(null);
  const [runMessage, setRunMessage] = useState<string | null>(null);

  const isSaving = createRule.isPending || updateRule.isPending;
  const isRunning = runRule.isPending;

  const selectedFeedLabel = (() => {
    if (feedIds.length === 0) return t("rules.select_feeds");
    if (feedIds.length === 1) {
      return feeds.find((feed) => feed.id === feedIds[0])?.title ?? t("rules.feed_count", { count: 1 });
    }
    return t("rules.feed_count", { count: feedIds.length });
  })();

  const toggleFeed = (feedId: string) => {
    setFeedIds((current) =>
      current.includes(feedId)
        ? current.filter((id) => id !== feedId)
        : [...current, feedId],
    );
  };

  const updateCondition = <K extends keyof RuleCondition>(
    index: number,
    key: K,
    value: RuleCondition[K],
  ) => {
    setConditions((current) =>
      current.map((condition, i) =>
        i === index ? { ...condition, [key]: value } : condition,
      ),
    );
  };

  const removeCondition = (index: number) => {
    setConditions((current) => {
      if (current.length <= 1) return current;
      return current.filter((_, i) => i !== index);
    });
  };

  const validate = (): ArticleRuleInput | null => {
    const trimmedName = name.trim();
    if (!trimmedName) {
      setError(t("rules.name_required"));
      return null;
    }
    if (feedIds.length === 0) {
      setError(t("rules.feed_required"));
      return null;
    }
    for (const condition of conditions) {
      if (!condition.value.trim()) {
        setError(t("rules.condition_value_required"));
        return null;
      }
      if (
        condition.operator === "regex" ||
        condition.operator === "not_regex"
      ) {
        try {
          new RegExp(condition.value);
        } catch {
          setError(t("rules.invalid_regex"));
          return null;
        }
      }
    }

    return {
      name: trimmedName,
      enabled,
      fullWord,
      feedIds,
      action: { type: "mark_read" },
      conditions: conditions.map((condition, index) => ({
        ...condition,
        join: index === 0 ? "and" : condition.join,
        value: condition.value.trim(),
      })),
    };
  };

  const save = async (): Promise<ArticleRule | null> => {
    const input = validate();
    if (!input) return null;

    setError(null);
    try {
      const saved = savedId
        ? await updateRule.mutateAsync({ id: savedId, rule: input })
        : await createRule.mutateAsync(input);
      setSavedId(saved.id);
      return saved;
    } catch {
      setError(t("rules.save_failed"));
      return null;
    }
  };

  const handleSave = async () => {
    const saved = await save();
    if (saved) {
      onOpenChange(false);
    }
  };

  const handleRun = async () => {
    const saved = await save();
    if (!saved) return;

    setRunMessage(null);
    try {
      const result = await runRule.mutateAsync(saved.id);
      setRunMessage(
        t("rules.run_result", {
          matched: result.matched,
          updated: result.updated,
        }),
      );
    } catch {
      setError(t("rules.run_failed"));
    }
  };

  return (
    <Dialog open={open} onOpenChange={onOpenChange}>
      <DialogContent className="max-h-[90vh] w-[calc(100%-2rem)] max-w-3xl overflow-y-auto p-0">
        <DialogHeader className="border-b border-border px-6 py-4 text-left">
          <div className="flex items-center justify-between gap-4">
            <DialogTitle>{rule ? t("rules.edit") : t("rules.add")}</DialogTitle>
            <DialogClose asChild>
              <button
                type="button"
                aria-label={t("actions.close")}
                className="flex size-8 items-center justify-center rounded-md text-muted-foreground transition-colors hover:bg-item-hover hover:text-foreground"
              >
                ×
              </button>
            </DialogClose>
          </div>
          <DialogDescription className="sr-only">
            {rule ? t("rules.edit") : t("rules.add")}
          </DialogDescription>
        </DialogHeader>

        <div className="space-y-4 px-6 py-5">
          <div>
            <label className="mb-1.5 block text-sm font-medium">
              {t("rules.name")}
            </label>
            <input
              value={name}
              onChange={(event) => setName(event.target.value)}
              placeholder={t("rules.name_placeholder")}
              className="h-9 w-full rounded-md border border-border bg-background px-3 text-sm focus:border-primary focus:outline-none focus:ring-2 focus:ring-primary/20"
            />
          </div>

          <div className="flex flex-wrap items-center gap-2 rounded-md bg-muted/40 px-3 py-2.5 text-sm">
            <span className="w-10 shrink-0 font-medium">{t("rules.when")}</span>
            <span className="text-muted-foreground">
              {t("rules.when_new_article")}
            </span>
            <DropdownMenu>
              <DropdownMenuTrigger asChild>
                <button
                  type="button"
                  className="ml-auto flex h-8 min-w-[9rem] items-center justify-between gap-2 rounded-md border border-border bg-background px-2.5 text-sm"
                >
                  <span className="truncate">{selectedFeedLabel}</span>
                  <svg
                    className="size-3.5 shrink-0 text-muted-foreground"
                    fill="none"
                    stroke="currentColor"
                    viewBox="0 0 24 24"
                  >
                    <path
                      strokeLinecap="round"
                      strokeLinejoin="round"
                      strokeWidth={2}
                      d="M19 9l-7 7-7-7"
                    />
                  </svg>
                </button>
              </DropdownMenuTrigger>
              <DropdownMenuContent
                align="end"
                className="z-[300] max-h-64 w-64 overflow-y-auto"
              >
                {feeds.length === 0 ? (
                  <DropdownMenuItem disabled>
                    {t("rules.no_feeds")}
                  </DropdownMenuItem>
                ) : (
                  feeds.map((feed) => {
                    const selected = feedIds.includes(feed.id);
                    return (
                      <DropdownMenuItem
                        key={feed.id}
                        onSelect={(event) => {
                          event.preventDefault();
                          toggleFeed(feed.id);
                        }}
                      >
                        <span className="flex size-4 shrink-0 items-center justify-center">
                          {selected && (
                            <svg
                              className="size-3.5"
                              fill="none"
                              stroke="currentColor"
                              viewBox="0 0 24 24"
                            >
                              <path
                                strokeLinecap="round"
                                strokeLinejoin="round"
                                strokeWidth={2.5}
                                d="M5 13l4 4L19 7"
                              />
                            </svg>
                          )}
                        </span>
                        <span className="truncate">{feed.title}</span>
                      </DropdownMenuItem>
                    );
                  })
                )}
              </DropdownMenuContent>
            </DropdownMenu>
          </div>

          <div className="space-y-3 rounded-md bg-muted/40 px-3 py-3">
            <div className="flex items-center gap-2 text-sm">
              <span className="w-10 shrink-0 font-medium">{t("rules.if")}</span>
            </div>

            {conditions.map((condition, index) => (
              <div
                key={index}
                className="flex flex-wrap items-center gap-2 text-sm"
              >
                {index > 0 && (
                  <select
                    value={condition.join}
                    onChange={(event) =>
                      updateCondition(
                        index,
                        "join",
                        event.target.value as RuleJoin,
                      )
                    }
                    className="h-8 w-20 rounded-md border border-border bg-background px-2"
                  >
                    {RULE_JOINS.map((join) => (
                      <option key={join} value={join}>
                        {t(`rules.joins.${join}`)}
                      </option>
                    ))}
                  </select>
                )}
                <select
                  value={condition.field}
                  onChange={(event) =>
                    updateCondition(
                      index,
                      "field",
                      event.target.value as RuleField,
                    )
                  }
                  className="h-8 min-w-[8.5rem] rounded-md border border-border bg-background px-2"
                >
                  {RULE_FIELDS.map((field) => (
                    <option key={field} value={field}>
                      {t(`rules.fields.${field}`)}
                    </option>
                  ))}
                </select>
                <select
                  value={condition.operator}
                  onChange={(event) =>
                    updateCondition(
                      index,
                      "operator",
                      event.target.value as RuleOperator,
                    )
                  }
                  className="h-8 min-w-[8.5rem] rounded-md border border-border bg-background px-2"
                >
                  {RULE_OPERATORS.map((operator) => (
                    <option key={operator} value={operator}>
                      {t(`rules.operators.${operator}`)}
                    </option>
                  ))}
                </select>
                <input
                  value={condition.value}
                  onChange={(event) =>
                    updateCondition(index, "value", event.target.value)
                  }
                  className="h-8 min-w-[10rem] flex-1 rounded-md border border-border bg-background px-2.5"
                />
                <button
                  type="button"
                  onClick={() => removeCondition(index)}
                  disabled={conditions.length <= 1}
                  aria-label={t("rules.remove_condition")}
                  className="flex size-7 shrink-0 items-center justify-center rounded-md text-muted-foreground transition-colors hover:bg-item-hover hover:text-foreground disabled:cursor-not-allowed disabled:opacity-40"
                >
                  ×
                </button>
              </div>
            ))}

            <div className="flex flex-wrap items-center justify-between gap-2">
              <button
                type="button"
                onClick={() => setConditions((current) => [...current, newCondition()])}
                className="text-xs font-medium text-muted-foreground transition-colors hover:text-foreground"
              >
                + {t("rules.add_condition")}
              </button>
              <label className="flex items-center gap-2 text-xs text-muted-foreground">
                <input
                  type="checkbox"
                  checked={fullWord}
                  onChange={(event) => setFullWord(event.target.checked)}
                  className="size-4 accent-primary"
                />
                {t("rules.full_word")}
              </label>
            </div>
          </div>

          <div className="flex flex-wrap items-center gap-2 rounded-md bg-muted/40 px-3 py-2.5 text-sm">
            <span className="w-10 shrink-0 font-medium">{t("rules.then")}</span>
            <select
              defaultValue="mark_read"
              className="h-8 min-w-[10rem] rounded-md border border-border bg-background px-2"
            >
              <option value="mark_read">{t("rules.action_mark_read")}</option>
            </select>
          </div>

          {error && (
            <div className="rounded-md bg-destructive/10 px-3 py-2 text-sm text-destructive">
              {error}
            </div>
          )}
          {runMessage && (
            <div className="rounded-md bg-primary/10 px-3 py-2 text-sm text-foreground">
              {runMessage}
            </div>
          )}
        </div>

        <div className="flex flex-wrap items-center justify-between gap-2 border-t border-border px-6 py-4">
          <button
            type="button"
            onClick={handleRun}
            disabled={isSaving || isRunning}
            className={cn(
              "h-9 rounded-md border border-border px-3 text-sm font-medium transition-colors",
              "hover:bg-item-hover disabled:cursor-not-allowed disabled:opacity-50",
            )}
          >
            {isRunning ? t("rules.running") : t("rules.run")}
          </button>
          <div className="flex items-center gap-2">
            <button
              type="button"
              onClick={() => onOpenChange(false)}
              className="h-9 rounded-md border border-border px-3 text-sm font-medium transition-colors hover:bg-item-hover"
            >
              {t("rules.cancel")}
            </button>
            <button
              type="button"
              onClick={handleSave}
              disabled={isSaving || isRunning}
              className="h-9 rounded-md bg-primary px-3 text-sm font-medium text-primary-foreground transition-colors hover:bg-primary/90 disabled:cursor-not-allowed disabled:opacity-50"
            >
              {isSaving ? t("rules.saving") : t("rules.save")}
            </button>
          </div>
        </div>
      </DialogContent>
    </Dialog>
  );
}
