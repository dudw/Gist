import {
  useQuery,
  useMutation,
  useMutationState,
  useQueryClient,
} from "@tanstack/react-query";
import {
  listFeeds,
  deleteFeed,
  updateFeed,
  updateFeedType,
  refreshFeed,
} from "@/api";
import type { ContentType } from "@/types/api";

export function useFeeds() {
  return useQuery({
    queryKey: ["feeds"],
    queryFn: () => listFeeds(),
  });
}

export function useDeleteFeed() {
  const queryClient = useQueryClient();
  return useMutation({
    mutationFn: (id: string) => deleteFeed(id),
    onSuccess: () => {
      queryClient.invalidateQueries({ queryKey: ["feeds"] });
      queryClient.invalidateQueries({ queryKey: ["unreadCounts"] });
    },
  });
}

export function useUpdateFeed() {
  const queryClient = useQueryClient();
  return useMutation({
    mutationFn: (payload: {
      id: string;
      title: string;
      folderId?: string;
      summaryPromptReminder?: string;
    }) =>
      updateFeed(payload.id, {
        title: payload.title,
        folderId: payload.folderId,
        summaryPromptReminder: payload.summaryPromptReminder,
      }),
    onSuccess: () => {
      queryClient.invalidateQueries({ queryKey: ["feeds"] });
    },
  });
}

export function useUpdateFeedType() {
  const queryClient = useQueryClient();
  return useMutation({
    mutationFn: (payload: { id: string; type: ContentType }) =>
      updateFeedType(payload.id, payload.type),
    onSuccess: () => {
      queryClient.invalidateQueries({ queryKey: ["feeds"] });
    },
  });
}

export function useRefreshFeed() {
  const queryClient = useQueryClient();
  return useMutation({
    mutationKey: ["refresh-feed"],
    mutationFn: (id: string) => refreshFeed(id),
    onSuccess: () => {
      queryClient.invalidateQueries({ queryKey: ["entries"] });
      queryClient.invalidateQueries({ queryKey: ["unreadCounts"] });
      queryClient.invalidateQueries({ queryKey: ["feeds"] });
    },
    onError: () => {
      // Refresh the feed list so the backend error message becomes visible.
      queryClient.invalidateQueries({ queryKey: ["feeds"] });
    },
  });
}

export function useRefreshingFeedIds(): string[] {
  return useMutationState({
    filters: { mutationKey: ["refresh-feed"], status: "pending" },
    select: (mutation) => mutation.state.variables as string,
  });
}
