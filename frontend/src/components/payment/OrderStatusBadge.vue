<!-- 作者：mkx | 日期：2026-04-22 | 变更：方案 A 配色优化 - 订单成功态恢复 emerald 绿色 -->
<template>
 <span
 class="inline-flex items-center rounded-full px-2.5 py-0.5 text-xs font-medium"
 :class="statusClass"
 >
 {{ statusLabel }}
 </span>
</template>

<script setup lang="ts">
import { computed } from 'vue'
import { useI18n } from 'vue-i18n'
import type { OrderStatus } from '@/types/payment'

const props = defineProps<{
 status: OrderStatus
}>()

const { t } = useI18n()

const statusMap: Record<OrderStatus, { key: string; class: string }> = {
 PENDING: { key: 'payment.status.pending', class: 'bg-yellow-100 text-yellow-800 ' },
 PAID: { key: 'payment.status.paid', class: 'bg-emerald-100 text-emerald-800 ' },
 RECHARGING: { key: 'payment.status.recharging', class: 'bg-sky-100 text-sky-800 ' },
 COMPLETED: { key: 'payment.status.completed', class: 'bg-emerald-100 text-emerald-800 ' },
 EXPIRED: { key: 'payment.status.expired', class: 'bg-gray-100 text-gray-800 ' },
 CANCELLED: { key: 'payment.status.cancelled', class: 'bg-gray-100 text-gray-800 ' },
 FAILED: { key: 'payment.status.failed', class: 'bg-red-100 text-red-800 ' },
 REFUND_REQUESTED: { key: 'payment.status.refund_requested', class: 'bg-orange-100 text-orange-800 ' },
 REFUNDING: { key: 'payment.status.refunding', class: 'bg-orange-100 text-orange-800 ' },
 REFUNDED: { key: 'payment.status.refunded', class: 'bg-gray-100 text-gray-800 ' },
 PARTIALLY_REFUNDED: { key: 'payment.status.partially_refunded', class: 'bg-gray-100 text-gray-800 ' },
 REFUND_FAILED: { key: 'payment.status.refund_failed', class: 'bg-red-100 text-red-800 ' },
}

const statusLabel = computed(() => {
 const entry = statusMap[props.status]
 return entry ? t(entry.key) : props.status
})

const statusClass = computed(() => {
 const entry = statusMap[props.status]
 return entry?.class ?? 'bg-gray-100 text-gray-800 '
})
</script>
