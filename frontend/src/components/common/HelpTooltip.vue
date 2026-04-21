<!-- 作者：mkx | 日期：2026-04-21 | 变更：批量清理 Tailwind 暗色变体类名并同步补充暖色主题改造注释 -->
<script setup lang="ts">
import { ref, useTemplateRef, nextTick } from 'vue'

defineProps<{
 content?: string
}>()

const show = ref(false)
const triggerRef = useTemplateRef<HTMLElement>('trigger')
const tooltipStyle = ref({ top: '0px', left: '0px' })

function onEnter() {
 show.value = true
 nextTick(updatePosition)
}

function onLeave() {
 show.value = false
}

function updatePosition() {
 const el = triggerRef.value
 if (!el) return
 const rect = el.getBoundingClientRect()
 tooltipStyle.value = {
 top: `${rect.top + window.scrollY}px`,
 left: `${rect.left + rect.width / 2 + window.scrollX}px`,
 }
}
</script>

<template>
 <div
 ref="trigger"
 class="group relative ml-1 inline-flex items-center align-middle"
 @mouseenter="onEnter"
 @mouseleave="onLeave"
 >
 <!-- Trigger Icon -->
 <slot name="trigger">
 <svg
 class="h-4 w-4 cursor-help text-gray-400 transition-colors hover:text-primary-600"
 fill="none"
 viewBox="0 0 24 24"
 stroke="currentColor"
 stroke-width="2"
 >
 <path
 stroke-linecap="round"
 stroke-linejoin="round"
 d="M13 16h-1v-4h-1m1-4h.01M21 12a9 9 0 11-18 0 9 9 0 0118 0z"
 />
 </svg>
 </slot>

 <!-- Teleport to body to escape modal overflow clipping -->
 <Teleport to="body">
 <div
 v-show="show"
 class="fixed z-[99999] w-64 -translate-x-1/2 -translate-y-full rounded-lg bg-gray-900 p-3 text-xs leading-relaxed text-white shadow-xl ring-1 ring-white/10"
 :style="{ top: `calc(${tooltipStyle.top} - 8px)`, left: tooltipStyle.left }"
 >
 <slot>{{ content }}</slot>
 <div class="absolute -bottom-1 left-1/2 h-2 w-2 -translate-x-1/2 rotate-45 bg-gray-900"></div>
 </div>
 </Teleport>
 </div>
</template>
