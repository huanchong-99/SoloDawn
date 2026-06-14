import { type ClassValue, clsx } from 'clsx';

export function cn(...inputs: ClassValue[]) {
  return clsx(inputs);
}

function formatBytes(bytes: bigint | null | undefined): string {
  if (bytes === null || bytes === undefined) return '';
  const num = Number(bytes);
  if (num < 1024) return `${num} B`;
  if (num < 1024 * 1024) return `${(num / 1024).toFixed(1)} KB`;
  return `${(num / (1024 * 1024)).toFixed(1)} MB`;
}

export function formatFileSize(bytes: bigint | null | undefined): string {
  return formatBytes(bytes);
}
