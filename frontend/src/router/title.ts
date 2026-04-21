/**
 * 浏览器标签页 title 固定为"芝麻灵码"（品牌统一）。
 * 保留函数签名以兼容现有调用点。
 */
export function resolveDocumentTitle(_routeTitle?: unknown, _siteName?: string, _titleKey?: string): string {
  return '芝麻灵码'
}
