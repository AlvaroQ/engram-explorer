/**
 * Helpers to export an array of plain objects as JSON or CSV and trigger a
 * browser download.
 */

export function downloadJson(filename: string, data: unknown): void {
  const blob = new Blob([JSON.stringify(data, null, 2)], { type: 'application/json' });
  triggerDownload(blob, filename);
}

export function downloadCsv<T>(
  filename: string,
  rows: T[],
  columns?: ReadonlyArray<keyof T & string>,
): void {
  if (rows.length === 0) {
    triggerDownload(new Blob([''], { type: 'text/csv' }), filename);
    return;
  }
  const cols: ReadonlyArray<keyof T & string> =
    columns ?? (Object.keys(rows[0] as Record<string, unknown>) as Array<keyof T & string>);
  const header = cols.map((c) => csvCell(String(c))).join(',');
  const body = rows
    .map((row) =>
      cols
        .map((c) => csvCell(formatValue((row as Record<string, unknown>)[c as string])))
        .join(','),
    )
    .join('\n');
  const csv = `${header}\n${body}\n`;
  triggerDownload(new Blob([csv], { type: 'text/csv' }), filename);
}

function csvCell(input: string): string {
  if (/[",\n\r]/.test(input)) {
    return `"${input.replace(/"/g, '""')}"`;
  }
  return input;
}

function formatValue(v: unknown): string {
  if (v === null || v === undefined) return '';
  if (typeof v === 'string') return v;
  if (typeof v === 'number' || typeof v === 'boolean') return String(v);
  return JSON.stringify(v);
}

function triggerDownload(blob: Blob, filename: string): void {
  const url = URL.createObjectURL(blob);
  const a = document.createElement('a');
  a.href = url;
  a.download = filename;
  document.body.appendChild(a);
  a.click();
  document.body.removeChild(a);
  URL.revokeObjectURL(url);
}
