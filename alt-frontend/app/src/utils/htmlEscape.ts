/**
 * HTML エスケープユーティリティ
 * XSS攻撃防止のためのHTML特殊文字エスケープ処理
 */

/**
 * HTMLの特殊文字をエスケープして安全な文字列に変換
 * @param text エスケープする文字列
 * @returns エスケープされた安全な文字列
 */
export function escapeHtml(text: string | null | undefined): string {
  if (!text) return '';
  
  return text
    .replace(/&/g, '&amp;')    // & を最初に処理（二重エスケープ防止）
    .replace(/</g, '&lt;')     // < をエスケープ
    .replace(/>/g, '&gt;')     // > をエスケープ
    .replace(/"/g, '&quot;')   // " をエスケープ
    .replace(/'/g, '&#x27;')   // ' をエスケープ
    .replace(/=/g, '&#x3D;');  // = をエスケープ（XSS攻撃防止）
}

/**
 * 表示用にクエリ文字列を安全にエスケープ
 * @param query 検索クエリ文字列
 * @returns 表示用にエスケープされた文字列
 */
export function escapeForDisplay(query: string): string {
  return escapeHtml(query);
}