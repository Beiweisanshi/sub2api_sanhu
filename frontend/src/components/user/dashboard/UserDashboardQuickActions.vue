<!-- 作者：mkx | 日期：2026-04-21 | 变更：批量清理 Tailwind 暗色变体类名并同步补充暖色主题改造注释 -->
<template>
 <div class="card">
 <div class="border-b border-gray-100 px-6 py-4">
 <h2 class="text-lg font-semibold text-gray-900">{{ t('dashboard.quickActions') }}</h2>
 </div>
 <div class="space-y-3 p-4">
 <button @click="router.push('/keys')" class="group flex w-full items-center gap-4 rounded-xl bg-gray-50 p-4 text-left transition-all duration-200 hover:bg-gray-100">
 <div class="flex h-12 w-12 flex-shrink-0 items-center justify-center rounded-xl bg-primary-100 transition-transform group-hover:scale-105">
 <Icon name="key" size="lg" class="text-primary-600" />
 </div>
 <div class="min-w-0 flex-1">
 <p class="text-sm font-medium text-gray-900">{{ t('dashboard.createApiKey') }}</p>
 <p class="text-xs text-gray-500">{{ t('dashboard.generateNewKey') }}</p>
 </div>
 <Icon
 name="chevronRight"
 size="md"
 class="text-gray-400 transition-colors group-hover:text-primary-500"
 />
 </button>

 <button @click="router.push('/usage')" class="group flex w-full items-center gap-4 rounded-xl bg-gray-50 p-4 text-left transition-all duration-200 hover:bg-gray-100">
 <div class="flex h-12 w-12 flex-shrink-0 items-center justify-center rounded-xl bg-primary-100 transition-transform group-hover:scale-105">
 <Icon name="chart" size="lg" class="text-primary-600" />
 </div>
 <div class="min-w-0 flex-1">
 <p class="text-sm font-medium text-gray-900">{{ t('dashboard.viewUsage') }}</p>
 <p class="text-xs text-gray-500">{{ t('dashboard.checkDetailedLogs') }}</p>
 </div>
 <Icon
 name="chevronRight"
 size="md"
 class="text-gray-400 transition-colors group-hover:text-primary-500"
 />
 </button>

 <button @click="router.push('/redeem')" class="group flex w-full items-center gap-4 rounded-xl bg-gray-50 p-4 text-left transition-all duration-200 hover:bg-gray-100">
 <div class="flex h-12 w-12 flex-shrink-0 items-center justify-center rounded-xl bg-amber-100 transition-transform group-hover:scale-105">
 <Icon name="gift" size="lg" class="text-amber-600" />
 </div>
 <div class="min-w-0 flex-1">
 <p class="text-sm font-medium text-gray-900">{{ t('dashboard.redeemCode') }}</p>
 <p class="text-xs text-gray-500">{{ t('dashboard.addBalanceWithCode') }}</p>
 </div>
 <Icon
 name="chevronRight"
 size="md"
 class="text-gray-400 transition-colors group-hover:text-amber-500"
 />
 </button>
 </div>
 </div>
</template>

<script setup lang="ts">
import { useRouter } from 'vue-router'
import { useI18n } from 'vue-i18n'
import Icon from '@/components/icons/Icon.vue'
const router = useRouter()
const { t } = useI18n()
</script>
