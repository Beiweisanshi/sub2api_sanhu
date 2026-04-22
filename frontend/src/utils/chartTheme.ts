/**
 * 作者：mkx
 * 日期：2026-04-22
 * 变更说明：图表色板恢复语义色（蓝/绿/琥珀/红/紫/粉/青/橙/靛/青柠/天蓝/紫罗兰），
 *          保留轴/网格/提示框的暖色系包装以配合浅色主题。
 */

export const CLAUDE_CHART_TEXT = '#374151'
export const CLAUDE_CHART_GRID = '#E5E7EB'
export const CLAUDE_CHART_TITLE = '#111827'
export const CLAUDE_CHART_TOOLTIP_BODY = '#4B5563'

// 语义色板（Tailwind 500 级 hex），chart 系列使用时按语义挑选。
export const CLAUDE_CHART_SERIES = {
  blue: '#3b82f6',
  green: '#10b981',
  amber: '#f59e0b',
  red: '#ef4444',
  violet: '#8b5cf6',
  pink: '#ec4899',
  teal: '#14b8a6',
  orange: '#f97316',
  indigo: '#6366f1',
  lime: '#84cc16',
  cyan: '#06b6d4',
  purple: '#a855f7',
  gray: '#9ca3af',
} as const

export const CLAUDE_CHART_PALETTE = [
  CLAUDE_CHART_SERIES.blue,
  CLAUDE_CHART_SERIES.green,
  CLAUDE_CHART_SERIES.amber,
  CLAUDE_CHART_SERIES.red,
  CLAUDE_CHART_SERIES.violet,
  CLAUDE_CHART_SERIES.pink,
  CLAUDE_CHART_SERIES.teal,
  CLAUDE_CHART_SERIES.orange,
  CLAUDE_CHART_SERIES.indigo,
  CLAUDE_CHART_SERIES.lime,
  CLAUDE_CHART_SERIES.cyan,
  CLAUDE_CHART_SERIES.purple,
] as const

export const CLAUDE_CHART_TOOLTIP = {
  backgroundColor: '#FFFFFF',
  titleColor: CLAUDE_CHART_TITLE,
  bodyColor: CLAUDE_CHART_TOOLTIP_BODY,
  borderColor: CLAUDE_CHART_GRID,
  borderWidth: 1,
  padding: 10,
  displayColors: true,
} as const

export const withAlpha = (hexColor: string, alphaHex: string) => `${hexColor}${alphaHex}`
