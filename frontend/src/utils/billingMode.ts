/**
 * 作者：mkx
 * 日期：2026-04-21
 * 变更说明：批量清理 Tailwind 暗色变体类名并同步补充暖色主题改造注释
 */
export const BILLING_MODE_TOKEN = 'token'
export const BILLING_MODE_PER_REQUEST = 'per_request'
export const BILLING_MODE_IMAGE = 'image'

export function getBillingModeLabel(mode: string | null | undefined, t: (key: string) => string): string {
 switch (mode) {
 case BILLING_MODE_PER_REQUEST: return t('admin.usage.billingModePerRequest')
 case BILLING_MODE_IMAGE: return t('admin.usage.billingModeImage')
 default: return t('admin.usage.billingModeToken')
 }
}

export function getBillingModeBadgeClass(mode: string | null | undefined): string {
 switch (mode) {
 case BILLING_MODE_PER_REQUEST: return 'bg-gray-100 text-gray-700 '
 case BILLING_MODE_IMAGE: return 'bg-primary-100 text-primary-700 '
 default: return 'bg-primary-100 text-primary-700 '
 }
}
