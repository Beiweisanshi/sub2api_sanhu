<!-- 作者：mkx | 日期：2026-04-21 | 变更：批量清理 Tailwind 暗色变体类名并同步补充暖色主题改造注释 -->
<script setup lang="ts">
defineProps<{
 colorClass: string
 tooltip?: string
 current: string | number
 max: string | number
 suffix?: string
}>()
</script>

<template>
 <span
 :class="[
 'inline-flex items-center gap-1 rounded-md px-1.5 py-px text-[10px] font-medium leading-tight',
 colorClass
 ]"
 :title="tooltip"
 >
 <slot />
 <span class="font-mono">{{ current }}</span>
 <span class="text-gray-400">/</span>
 <span class="font-mono">{{ max }}</span>
 <span v-if="suffix" class="text-[9px] opacity-60">{{ suffix }}</span>
 </span>
</template>
