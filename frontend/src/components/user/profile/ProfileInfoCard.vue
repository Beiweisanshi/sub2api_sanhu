<!-- 作者：mkx | 日期：2026-04-21 | 变更：批量清理 Tailwind 暗色变体类名并同步补充暖色主题改造注释 -->
<template>
 <div class="card overflow-hidden">
 <div
 class="border-b border-gray-100 bg-gradient-to-r from-primary-500/10 to-primary-600/5 px-6 py-5"
 >
 <div class="flex items-center gap-4">
 <!-- Avatar -->
 <div
 class="flex h-16 w-16 items-center justify-center rounded-2xl bg-gradient-to-br from-primary-500 to-primary-600 text-2xl font-bold text-white shadow-lg shadow-primary-500/20"
 >
 {{ user?.email?.charAt(0).toUpperCase() || 'U' }}
 </div>
 <div class="min-w-0 flex-1">
 <h2 class="truncate text-lg font-semibold text-gray-900">
 {{ user?.email }}
 </h2>
 <div class="mt-1 flex items-center gap-2">
 <span :class="['badge', user?.role === 'admin' ? 'badge-primary' : 'badge-gray']">
 {{ user?.role === 'admin' ? t('profile.administrator') : t('profile.user') }}
 </span>
 <span
 :class="['badge', user?.status === 'active' ? 'badge-success' : 'badge-danger']"
 >
 {{ user?.status }}
 </span>
 </div>
 </div>
 </div>
 </div>
 <div class="px-6 py-4">
 <div class="space-y-3">
 <div class="flex items-center gap-3 text-sm text-gray-600">
 <Icon name="mail" size="sm" class="text-gray-400" />
 <span class="truncate">{{ user?.email }}</span>
 </div>
 <div
 v-if="user?.username"
 class="flex items-center gap-3 text-sm text-gray-600"
 >
 <Icon name="user" size="sm" class="text-gray-400" />
 <span class="truncate">{{ user.username }}</span>
 </div>
 </div>
 </div>
 </div>
</template>

<script setup lang="ts">
import { useI18n } from 'vue-i18n'
import Icon from '@/components/icons/Icon.vue'
import type { User } from '@/types'

defineProps<{
 user: User | null
}>()

const { t } = useI18n()
</script>
