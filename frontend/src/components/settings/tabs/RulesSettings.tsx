import { useState } from "react";
import { useTranslation } from "react-i18next";
import { Switch } from "@/components/ui/switch";
import { RuleEditorDialog } from "@/components/rules/RuleEditorDialog";
import {
  useDeleteRule,
  useRules,
  useRunRule,
  useSetRuleEnabled,
} from "@/hooks/useRules";
import type { ArticleRule } from "@/types/api";

export function RulesSettings() {
  const { t } = useTranslation();
  const { data: rules = [], isLoading, isError } = useRules();
  const setEnabled = useSetRuleEnabled();
  const deleteRule = useDeleteRule();
  const runRule = useRunRule();

  const [editorOpen, setEditorOpen] = useState(false);
  const [editingRule, setEditingRule] = useState<ArticleRule | null>(null);
  const [runningId, setRunningId] = useState<string | null>(null);
  const [message, setMessage] = useState<string | null>(null);
  const [error, setError] = useState<string | null>(null);

  const openAdd = () => {
    setEditingRule(null);
    setEditorOpen(true);
    setMessage(null);
    setError(null);
  };

  const openEdit = (rule: ArticleRule) => {
    setEditingRule(rule);
    setEditorOpen(true);
    setMessage(null);
    setError(null);
  };

  const handleDelete = async (rule: ArticleRule) => {
    if (!window.confirm(t("rules.delete_confirm", { name: rule.name }))) {
      return;
    }
    setError(null);
    try {
      await deleteRule.mutateAsync(rule.id);
    } catch {
      setError(t("rules.delete_failed"));
    }
  };

  const handleRun = async (rule: ArticleRule) => {
    setRunningId(rule.id);
    setError(null);
    setMessage(null);
    try {
      const result = await runRule.mutateAsync(rule.id);
      setMessage(
        t("rules.run_result", {
          matched: result.matched,
          updated: result.updated,
        }),
      );
    } catch {
      setError(t("rules.run_failed"));
    } finally {
      setRunningId(null);
    }
  };

  const conditionSummary = (rule: ArticleRule) => {
    const first = rule.conditions[0];
    if (!first) return "";
    const text = `${t(`rules.fields.${first.field}`)} ${t(
      `rules.operators.${first.operator}`,
    )} ${first.value}`;
    if (rule.conditions.length === 1) return text;
    return `${text} · ${t("rules.condition_count", {
      count: rule.conditions.length,
    })}`;
  };

  return (
    <div className="space-y-4">
      <div className="flex items-center justify-between gap-3">
        <div>
          <h3 className="text-sm font-semibold text-muted-foreground">
            {t("settings.rules")}
          </h3>
          <p className="mt-1 text-xs text-muted-foreground">
            {t("rules.empty_description")}
          </p>
        </div>
        <button
          type="button"
          onClick={openAdd}
          className="rounded-md bg-primary px-3 py-1.5 text-sm font-medium text-primary-foreground transition-colors hover:bg-primary/90"
        >
          + {t("rules.add")}
        </button>
      </div>

      {message && (
        <div className="rounded-md bg-primary/10 px-3 py-2 text-sm text-foreground">
          {message}
        </div>
      )}
      {error && (
        <div className="rounded-md bg-destructive/10 px-3 py-2 text-sm text-destructive">
          {error}
        </div>
      )}

      {isLoading ? (
        <div className="flex h-32 items-center justify-center">
          <div className="size-5 animate-spin rounded-full border-2 border-primary border-t-transparent" />
        </div>
      ) : isError ? (
        <div className="rounded-lg border border-dashed border-border p-8 text-center text-sm text-destructive">
          {t("rules.load_failed")}
        </div>
      ) : rules.length === 0 ? (
        <div className="rounded-lg border border-dashed border-border bg-muted/20 p-8 text-center">
          <p className="text-sm font-medium">{t("rules.empty")}</p>
          <p className="mt-1 text-xs text-muted-foreground">
            {t("rules.empty_description")}
          </p>
        </div>
      ) : (
        <div className="space-y-2">
          {rules.map((rule) => (
            <div
              key={rule.id}
              className="rounded-lg border border-border bg-background p-3"
            >
              <div className="flex flex-wrap items-start justify-between gap-3">
                <div className="min-w-0 flex-1">
                  <div className="flex items-center gap-2">
                    <span className="truncate text-sm font-medium">
                      {rule.name}
                    </span>
                    <span className="shrink-0 rounded-full bg-muted px-2 py-0.5 text-[0.65rem] text-muted-foreground">
                      {rule.enabled ? t("rules.enabled") : t("rules.disabled")}
                    </span>
                  </div>
                  <div className="mt-1 text-xs text-muted-foreground">
                    {t("rules.feed_count", { count: rule.feedIds.length })} ·{" "}
                    {t("rules.condition_count", {
                      count: rule.conditions.length,
                    })}{" "}
                    · {t("rules.action_mark_read")}
                  </div>
                  <div className="mt-1 truncate text-xs text-muted-foreground">
                    {conditionSummary(rule)}
                  </div>
                </div>

                <div className="flex shrink-0 items-center gap-1.5">
                  <Switch
                    checked={rule.enabled}
                    onCheckedChange={(checked) =>
                      setEnabled.mutate({ id: rule.id, enabled: checked })
                    }
                  />
                  <button
                    type="button"
                    onClick={() => handleRun(rule)}
                    disabled={runningId === rule.id}
                    className="rounded-md border border-border px-2 py-1 text-xs font-medium transition-colors hover:bg-item-hover disabled:cursor-not-allowed disabled:opacity-50"
                  >
                    {runningId === rule.id ? t("rules.running") : t("rules.run")}
                  </button>
                  <button
                    type="button"
                    onClick={() => openEdit(rule)}
                    className="rounded-md border border-border px-2 py-1 text-xs font-medium transition-colors hover:bg-item-hover"
                  >
                    {t("rules.edit")}
                  </button>
                  <button
                    type="button"
                    onClick={() => handleDelete(rule)}
                    className="rounded-md border border-border px-2 py-1 text-xs font-medium text-destructive transition-colors hover:bg-destructive/10"
                  >
                    {t("rules.delete")}
                  </button>
                </div>
              </div>
            </div>
          ))}
        </div>
      )}

      {editorOpen && (
        <RuleEditorDialog
          open={editorOpen}
          onOpenChange={setEditorOpen}
          rule={editingRule}
        />
      )}
    </div>
  );
}
