export interface HighlightSegment {
  text: string;
  match: boolean;
}

const SNIPPET_CONTEXT_BEFORE = 40;
const SNIPPET_MAX_LENGTH = 160;

function escapeRegExp(value: string): string {
  return value.replace(/[.*+?^${}()|[\]\\]/g, "\\$&");
}

function buildTermsPattern(query: string, flags: string): RegExp | null {
  const terms = getSearchTerms(query);
  if (terms.length === 0) return null;
  return new RegExp(terms.map(escapeRegExp).join("|"), flags);
}

/**
 * 将搜索关键词按空白拆分成去重后的词条，长词优先以避免被短词截断。
 */
export function getSearchTerms(query: string): string[] {
  const seen = new Set<string>();
  const terms: string[] = [];

  for (const raw of query.trim().split(/\s+/)) {
    const term = raw.trim();
    if (!term) continue;

    const key = term.toLocaleLowerCase();
    if (seen.has(key)) continue;

    seen.add(key);
    terms.push(term);
  }

  return terms.sort((a, b) => b.length - a.length);
}

/**
 * 按搜索词把文本切分为命中/非命中片段，用于安全地渲染高亮（不使用 HTML 注入）。
 */
export function splitByTerms(text: string, query: string): HighlightSegment[] {
  if (!text) return [];

  const pattern = buildTermsPattern(query, "gi");
  if (!pattern) return [{ text, match: false }];

  const segments: HighlightSegment[] = [];
  let lastIndex = 0;

  for (const match of text.matchAll(pattern)) {
    const index = match.index ?? 0;
    if (index > lastIndex) {
      segments.push({ text: text.slice(lastIndex, index), match: false });
    }
    segments.push({ text: match[0], match: true });
    lastIndex = index + match[0].length;
  }

  if (lastIndex < text.length) {
    segments.push({ text: text.slice(lastIndex), match: false });
  }

  return segments;
}

/**
 * 截取包含首个命中词的正文片段，命中位置不在开头时补齐上下文并添加省略号。
 * 未命中或文本为空时返回 null。
 */
export function buildSearchSnippet(
  text: string | null | undefined,
  query: string,
  maxLength = SNIPPET_MAX_LENGTH,
): string | null {
  const source = text?.trim();
  if (!source) return null;

  // 用正则查找首个命中位置，避免 toLowerCase 改变字符长度导致索引错位
  const pattern = buildTermsPattern(query, "i");
  if (!pattern) return null;

  const match = pattern.exec(source);
  if (!match) return null;
  const matchIndex = match.index;

  let start = Math.max(0, matchIndex - SNIPPET_CONTEXT_BEFORE);
  const end = Math.min(source.length, start + maxLength);

  // 接近文本末尾时向前补齐，避免片段过短
  if (end - start < maxLength) {
    start = Math.max(0, end - maxLength);
  }

  let snippet = source.slice(start, end).trim();
  if (start > 0) snippet = `…${snippet}`;
  if (end < source.length) snippet = `${snippet}…`;

  return snippet;
}
