<!-- 作者：mkx | 日期：2026-04-21 | 变更：批量清理 Tailwind 暗色变体类名并同步补充暖色主题改造注释 -->
<template>
 <div>
 <label class="mb-2 block text-sm font-medium text-gray-700">
 {{ t('payment.paymentMethod') }}
 </label>
 <div class="grid grid-cols-2 gap-3 sm:flex">
 <button
 v-for="method in sortedMethods"
 :key="method.type"
 type="button"
 :disabled="!method.available"
 :class="[
 'relative flex h-[60px] flex-col items-center justify-center rounded-lg border px-3 transition-all sm:flex-1',
 !method.available
 ? 'cursor-not-allowed border-gray-200 bg-gray-50 opacity-50 '
 : selected === method.type
 ? methodSelectedClass(method.type)
 : 'border-gray-300 bg-white text-gray-700 hover:border-gray-400 ',
 ]"
 @click="method.available && emit('select', method.type)"
 >
 <span class="flex items-center gap-2">
 <img :src="methodIcon(method.type)" :alt="t(`payment.methods.${method.type}`)" class="h-7 w-7" />
 <span class="flex flex-col items-start leading-none">
 <span class="text-base font-semibold">{{ t(`payment.methods.${method.type}`) }}</span>
 <span
 v-if="method.fee_rate > 0"
 class="text-[10px] tracking-wide text-gray-500"
 >
 {{ t('payment.fee') }} {{ method.fee_rate }}%
 </span>
 </span>
 </span>
 </button>
 </div>
 </div>
</template>

<script setup lang="ts">
import { computed } from 'vue'
import { useI18n } from 'vue-i18n'
import { METHOD_ORDER } from './providerConfig'
import alipayIcon from '@/assets/icons/alipay.svg'
import wxpayIcon from '@/assets/icons/wxpay.svg'
import stripeIcon from '@/assets/icons/stripe.svg'

export interface PaymentMethodOption {
 type: string
 fee_rate: number
 available: boolean
}

const props = defineProps<{
 methods: PaymentMethodOption[]
 selected: string
}>()

const emit = defineEmits<{
 select: [type: string]
}>()

const { t } = useI18n()

const METHOD_ICONS: Record<string, string> = {
 alipay: alipayIcon,
 wxpay: wxpayIcon,
 stripe: stripeIcon,
}

const sortedMethods = computed(() => {
 const order: readonly string[] = METHOD_ORDER
 return [...props.methods].sort((a, b) => {
 const ai = order.indexOf(a.type)
 const bi = order.indexOf(b.type)
 return (ai === -1 ? 999 : ai) - (bi === -1 ? 999 : bi)
 })
})

function methodIcon(type: string): string {
 if (type.includes('alipay')) return METHOD_ICONS.alipay
 if (type.includes('wxpay')) return METHOD_ICONS.wxpay
 return METHOD_ICONS[type] || alipayIcon
}

function methodSelectedClass(type: string): string {
 if (type.includes('alipay')) return 'border-[#02A9F1] bg-primary-50 text-gray-900 shadow-sm '
 if (type.includes('wxpay')) return 'border-[#09BB07] bg-primary-50 text-gray-900 shadow-sm '
 if (type === 'stripe') return 'border-[#676BE5] bg-gray-50 text-gray-900 shadow-sm '
 return 'border-primary-500 bg-primary-50 text-gray-900 shadow-sm '
}
</script>
