/**
 * 作者：mkx
 * 日期：2026-04-21
 * 变更说明：提取 Claude 暖色主题图表共用色板与 tooltip 配置，统一前端数据可视化观感。
 */

export const CLAUDE_CHART_TEXT = '#504B44'
export const CLAUDE_CHART_GRID = '#E8E5DF'
export const CLAUDE_CHART_TITLE = '#2D2A26'
export const CLAUDE_CHART_TOOLTIP_BODY = '#6B665E'

export const CLAUDE_CHART_SERIES = {
  primary: '#D96C4A',
  ember: '#C1573A',
  wood: '#9C422C',
  clay: '#C58A5F',
  sand: '#D6A487',
  muted: '#8C8578',
  smoke: '#504B44',
  cream: '#F1CFAE',
  amber: '#B77744',
  brick: '#A3472B',
} as const

export const CLAUDE_CHART_PALETTE = [
  CLAUDE_CHART_SERIES.primary,
  CLAUDE_CHART_SERIES.ember,
  CLAUDE_CHART_SERIES.clay,
  CLAUDE_CHART_SERIES.wood,
  CLAUDE_CHART_SERIES.sand,
  CLAUDE_CHART_SERIES.muted,
  CLAUDE_CHART_SERIES.smoke,
  CLAUDE_CHART_SERIES.cream,
  '#B97955',
  '#7A5A42',
  '#CDA58E',
  '#A9836A',
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
