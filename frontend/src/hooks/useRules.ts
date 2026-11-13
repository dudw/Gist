import { useMutation, useQuery, useQueryClient } from "@tanstack/react-query";
import {
  createRule,
  deleteRule,
  listRules,
  runRule,
  setRuleEnabled,
  updateRule,
} from "@/api";
import type { ArticleRuleInput } from "@/types/api";

export function useRules() {
  return useQuery({
    queryKey: ["rules"],
    queryFn: listRules,
  });
}

export function useCreateRule() {
  const queryClient = useQueryClient();
  return useMutation({
    mutationFn: (rule: ArticleRuleInput) => createRule(rule),
    onSuccess: () => {
      queryClient.invalidateQueries({ queryKey: ["rules"] });
    },
  });
}

export function useUpdateRule() {
  const queryClient = useQueryClient();
  return useMutation({
    mutationFn: ({ id, rule }: { id: string; rule: ArticleRuleInput }) =>
      updateRule(id, rule),
    onSuccess: () => {
      queryClient.invalidateQueries({ queryKey: ["rules"] });
    },
  });
}

export function useDeleteRule() {
  const queryClient = useQueryClient();
  return useMutation({
    mutationFn: (id: string) => deleteRule(id),
    onSuccess: () => {
      queryClient.invalidateQueries({ queryKey: ["rules"] });
    },
  });
}

export function useSetRuleEnabled() {
  const queryClient = useQueryClient();
  return useMutation({
    mutationFn: ({ id, enabled }: { id: string; enabled: boolean }) =>
      setRuleEnabled(id, enabled),
    onSuccess: () => {
      queryClient.invalidateQueries({ queryKey: ["rules"] });
    },
  });
}

export function useRunRule() {
  const queryClient = useQueryClient();
  return useMutation({
    mutationFn: (id: string) => runRule(id),
    onSuccess: () => {
      queryClient.invalidateQueries({ queryKey: ["rules"] });
      queryClient.invalidateQueries({ queryKey: ["entries"] });
      queryClient.invalidateQueries({ queryKey: ["unreadCounts"] });
      queryClient.invalidateQueries({ queryKey: ["starredCount"] });
    },
  });
}
