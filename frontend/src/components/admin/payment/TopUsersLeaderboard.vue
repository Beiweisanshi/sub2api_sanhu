<!-- 作者：mkx | 日期：2026-04-21 | 变更：批量清理 Tailwind 暗色变体类名并同步补充暖色主题改造注释 -->
<template>
 <div class="card p-4">
 <h3 class="mb-4 text-sm font-semibold text-gray-900">
 {{ t('payment.admin.topUsers') }}
 </h3>
 <div
 v-if="!users?.length"
 class="flex h-32 items-center justify-center text-sm text-gray-500"
 >
 {{ t('payment.admin.noData') }}
 </div>
 <div v-else class="space-y-2">
 <div
 v-for="(user, idx) in users"
 :key="user.user_id"
 class="flex items-center justify-between rounded-lg px-3 py-2 hover:bg-gray-50"
 >
 <div class="flex items-center gap-3">
 <span
 :class="[
 'flex h-6 w-6 items-center justify-center rounded-full text-xs font-bold',
 rankClass(idx),
 ]"
 >
 {{ idx + 1 }}
 </span>
 <span class="text-sm text-gray-700">{{ user.email }}</span>
 </div>
 <span class="text-sm font-medium text-gray-900">
 ${{ user.amount.toFixed(2) }}
 </span>
 </div>
 </div>
 </div>
</template>

<script setup lang="ts">
import { useI18n } from 'vue-i18n'

const { t } = useI18n()

defineProps<{
 users: { user_id: number; email: string; amount: number }[]
}>()

function rankClass(idx: number): string {
 if (idx === 0) return 'bg-yellow-100 text-yellow-700 '
 if (idx === 1) return 'bg-gray-200 text-gray-600 '
 if (idx === 2) return 'bg-amber-100 text-amber-700 '
 return 'bg-gray-100 text-gray-500 '
}
</script>
