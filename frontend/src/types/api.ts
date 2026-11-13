export type ContentType = "article" | "picture" | "notification";

export interface Folder {
  id: string;
  name: string;
  parentId?: string;
  type: ContentType;
  createdAt: string;
  updatedAt: string;
}

export interface Feed {
  id: string;
  folderId?: string;
  title: string;
  url: string;
  siteUrl?: string;
  description?: string;
  summaryPromptReminder?: string;
  iconPath?: string;
  type: ContentType;
  etag?: string;
  lastModified?: string;
  errorMessage?: string;
  createdAt: string;
  updatedAt: string;
}

export interface FeedPreview {
  url: string;
  title: string;
  description?: string;
  siteUrl?: string;
  imageUrl?: string;
  itemCount?: number;
  lastUpdated?: string;
}

export interface Entry {
  id: string;
  feedId: string;
  title?: string;
  url?: string;
  content?: string;
  readableContent?: string;
  thumbnailUrl?: string;
  author?: string;
  publishedAt?: string;
  read: boolean;
  starred: boolean;
  createdAt: string;
  updatedAt: string;
}

export interface EntryListResponse {
  entries: Entry[];
  hasMore: boolean;
}

export interface EntryListParams {
  feedId?: string;
  folderId?: string;
  contentType?: ContentType;
  unreadOnly?: boolean;
  starredOnly?: boolean;
  hasThumbnail?: boolean;
  limit?: number;
  offset?: number;
}

export interface UnreadCountsResponse {
  counts: Record<string, number>;
}

export interface StarredCountResponse {
  count: number;
}

export interface MarkAllReadParams {
  feedId?: string;
  folderId?: string;
  contentType?: ContentType;
}

export interface ApiErrorResponse {
  error: string;
}

export interface ImportResult {
  foldersCreated: number;
  foldersSkipped: number;
  feedsCreated: number;
  feedsSkipped: number;
}

export interface ImportTask {
  id?: string;
  status: "idle" | "running" | "done" | "error" | "cancelled";
  total: number;
  current: number;
  feed?: string;
  result?: ImportResult;
  error?: string;
  createdAt?: string;
}

export type RuleField =
  | "title_or_content"
  | "title"
  | "content"
  | "author"
  | "url"
  | "url_no_domain";

export type RuleOperator =
  | "contains"
  | "not_contains"
  | "is"
  | "is_not"
  | "starts_with"
  | "ends_with"
  | "regex"
  | "not_regex";

export type RuleJoin = "and" | "or";

export interface RuleCondition {
  join: RuleJoin;
  field: RuleField;
  operator: RuleOperator;
  value: string;
}

export interface ArticleRule {
  id: string;
  name: string;
  enabled: boolean;
  fullWord: boolean;
  feedIds: string[];
  action: { type: "mark_read" };
  conditions: RuleCondition[];
  createdAt: string;
  updatedAt: string;
}

export interface ArticleRuleInput {
  name: string;
  enabled: boolean;
  fullWord: boolean;
  feedIds: string[];
  action: { type: "mark_read" };
  conditions: RuleCondition[];
}

export interface RuleRunResult {
  matched: number;
  updated: number;
}
