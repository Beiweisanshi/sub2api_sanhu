/**
 * 作者：mkx
 * 日期：2026-04-21
 * 变更说明：批量清理 Tailwind 暗色变体类名并同步补充暖色主题改造注释
 */
/**
 * Centralized platform color definitions.
 *
 * All components that need platform-specific styling should import from here
 * instead of defining their own color mappings.
 */

export type Platform = 'anthropic' | 'openai' | 'antigravity' | 'gemini'

// ── Badge (bg + text + border, for inline badges with border) ───────
const BADGE: Record<Platform, string> = {
 anthropic: 'bg-orange-500/10 text-orange-600 border-orange-500/30 ',
 openai: 'bg-primary-500/10 text-primary-600 border-primary-500/30 ',
 antigravity: 'bg-gray-500/10 text-gray-600 border-gray-500/30 ',
 gemini: 'bg-primary-500/10 text-primary-600 border-primary-500/30 ',
}
const BADGE_DEFAULT = 'bg-gray-500/10 text-gray-700 border-gray-500/25 '

// ── Light badge (softer bg, no border) ──────────────────────────────
const BADGE_LIGHT: Record<Platform, string> = {
 anthropic: 'bg-orange-500/10 text-orange-600 ',
 openai: 'bg-primary-500/10 text-primary-600 ',
 antigravity: 'bg-gray-500/10 text-gray-600 ',
 gemini: 'bg-primary-500/10 text-primary-600 ',
}

// ── Border ──────────────────────────────────────────────────────────
const BORDER: Record<Platform, string> = {
 anthropic: 'border-orange-500/20 ',
 openai: 'border-primary-500/20 ',
 antigravity: 'border-gray-500/20 ',
 gemini: 'border-primary-500/20 ',
}
const BORDER_DEFAULT = 'border-gray-200 '

// ── Accent bar (gradient) ───────────────────────────────────────────
const ACCENT_BAR: Record<Platform, string> = {
 anthropic: 'bg-gradient-to-r from-orange-400 to-orange-500',
 openai: 'bg-gradient-to-r from-primary-400 to-primary-600',
 antigravity: 'bg-gradient-to-r from-gray-400 to-gray-500',
 gemini: 'bg-gradient-to-r from-primary-400 to-primary-500',
}
const ACCENT_BAR_DEFAULT = 'bg-gradient-to-r from-primary-400 to-primary-500'

// ── Text (price, icon) ─────────────────────────────────────────────
const TEXT: Record<Platform, string> = {
 anthropic: 'text-orange-600 ',
 openai: 'text-primary-600 ',
 antigravity: 'text-gray-600 ',
 gemini: 'text-primary-600 ',
}
const TEXT_DEFAULT = 'text-primary-600 '

// ── Icon (check mark etc.) ──────────────────────────────────────────
const ICON: Record<Platform, string> = {
 anthropic: 'text-orange-500 ',
 openai: 'text-primary-500 ',
 antigravity: 'text-gray-500 ',
 gemini: 'text-primary-500 ',
}
const ICON_DEFAULT = 'text-primary-500 '

// ── Button (solid bg) ───────────────────────────────────────────────
const BUTTON: Record<Platform, string> = {
 anthropic: 'bg-orange-500 text-white hover:bg-orange-600 active:bg-orange-700 ',
 openai: 'bg-primary-600 text-white hover:bg-primary-700 active:bg-primary-800 ',
 antigravity: 'bg-gray-500 text-white hover:bg-gray-600 active:bg-gray-700 ',
 gemini: 'bg-primary-500 text-white hover:bg-primary-600 active:bg-primary-700 ',
}
const BUTTON_DEFAULT = 'bg-primary-500 text-white hover:bg-primary-600 '

// ── Discount badge ──────────────────────────────────────────────────
const DISCOUNT: Record<Platform, string> = {
 anthropic: 'bg-orange-100 text-orange-700 ',
 openai: 'bg-primary-100 text-primary-700 ',
 antigravity: 'bg-gray-100 text-gray-700 ',
 gemini: 'bg-primary-100 text-primary-700 ',
}
const DISCOUNT_DEFAULT = 'bg-red-100 text-red-700 '

// ── Header gradient (subscription confirm) ─────────────────────────
const GRADIENT: Record<Platform, string> = {
 anthropic: 'from-orange-500 to-orange-600',
 openai: 'from-primary-500 to-primary-600',
 antigravity: 'from-gray-500 to-gray-600',
 gemini: 'from-primary-500 to-primary-600',
}
const GRADIENT_DEFAULT = 'from-primary-500 to-primary-600'

// ── Header text (light text on gradient bg) ────────────────────────
const GRADIENT_TEXT: Record<Platform, string> = {
 anthropic: 'text-orange-100',
 openai: 'text-primary-100',
 antigravity: 'text-gray-100',
 gemini: 'text-primary-100',
}
const GRADIENT_TEXT_DEFAULT = 'text-primary-100'

const GRADIENT_SUBTEXT: Record<Platform, string> = {
 anthropic: 'text-orange-200',
 openai: 'text-primary-200',
 antigravity: 'text-gray-200',
 gemini: 'text-primary-200',
}
const GRADIENT_SUBTEXT_DEFAULT = 'text-primary-200'

// ── Public API ──────────────────────────────────────────────────────

function isPlatform(p: string): p is Platform {
 return p === 'anthropic' || p === 'openai' || p === 'antigravity' || p === 'gemini'
}

export function platformBadgeClass(p: string): string {
 return isPlatform(p) ? BADGE[p] : BADGE_DEFAULT
}

export function platformBadgeLightClass(p: string): string {
 return isPlatform(p) ? BADGE_LIGHT[p] : BADGE_DEFAULT
}

export function platformBorderClass(p: string): string {
 return isPlatform(p) ? BORDER[p] : BORDER_DEFAULT
}

export function platformAccentBarClass(p: string): string {
 return isPlatform(p) ? ACCENT_BAR[p] : ACCENT_BAR_DEFAULT
}

export function platformTextClass(p: string): string {
 return isPlatform(p) ? TEXT[p] : TEXT_DEFAULT
}

export function platformIconClass(p: string): string {
 return isPlatform(p) ? ICON[p] : ICON_DEFAULT
}

export function platformButtonClass(p: string): string {
 return isPlatform(p) ? BUTTON[p] : BUTTON_DEFAULT
}

export function platformDiscountClass(p: string): string {
 return isPlatform(p) ? DISCOUNT[p] : DISCOUNT_DEFAULT
}

export function platformGradientClass(p: string): string {
 return isPlatform(p) ? GRADIENT[p] : GRADIENT_DEFAULT
}

export function platformGradientTextClass(p: string): string {
 return isPlatform(p) ? GRADIENT_TEXT[p] : GRADIENT_TEXT_DEFAULT
}

export function platformGradientSubtextClass(p: string): string {
 return isPlatform(p) ? GRADIENT_SUBTEXT[p] : GRADIENT_SUBTEXT_DEFAULT
}

export function platformLabel(p: string): string {
 switch (p) {
 case 'anthropic': return 'Anthropic'
 case 'openai': return 'OpenAI'
 case 'antigravity': return 'Antigravity'
 case 'gemini': return 'Gemini'
 default: return p || 'API'
 }
}
