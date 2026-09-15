// A local preview only. No network request or original statement is included.
export const RESULT_SHARE_LIMIT = 12 * 1024;
const sensitive = /password|passwd|secret|token|api.?key|authorization|cookie|credential|private.?key/i;
const clean = (value: unknown, depth = 0, columns?: string[]): unknown => {
  if (depth > 3) return '[omitted]';
  if (typeof value === 'string') return value.length > 256 ? value.slice(0, 256) + '…' : value;
  if (value === null || typeof value !== 'object') return value;
  if (Array.isArray(value)) return value.slice(0, 10).map(item => clean(item, depth + 1));
  const allEntries = Object.entries(value as Record<string, unknown>);
  const entries = allEntries.filter(([key]) => !columns || columns.includes(key)).slice(0, 10);
  const secretPair = allEntries.some(([key, item]) => ['name','key'].includes(key) && typeof item === 'string' && sensitive.test(item));
  return Object.fromEntries(entries.map(([key, item]) => [key.slice(0,128), sensitive.test(key) || (secretPair && key === 'value') ? '[redacted]' : clean(item, depth + 1)]));
};

export function resultSharePreview(result: API.WebDataExecuteResult, protocol: string, selection?: {rows:number[];columns:string[]}) {
  const source = result.rows || [];
  const columns = selection?.columns ?? [...new Set(source.flatMap(row => Object.keys(row)))].slice(0,10);
  const indices = selection ? [...new Set(selection.rows)].filter(i=>Number.isInteger(i)&&i>=0&&i<source.length).slice(0,20) : source.slice(0,20).map((_,i)=>i);
  const rows = indices.map(i => clean(source[i],0,columns));
  const render = () => JSON.stringify({
    protocol,
    source: 'User-selected result snapshot; may differ from current navigation',
    rows_in_browser: result.rows?.length || 0,
    upstream_truncated: Boolean(result.truncated),
    // Every preview is a bounded sample, never evidence of full database contents.
    sample_only: true,
    rows,
  }, null, 2);
  let text = render();
  while (new TextEncoder().encode(text).length > RESULT_SHARE_LIMIT && rows.length) {
    rows.pop(); text = render();
  }
  return text;
}
