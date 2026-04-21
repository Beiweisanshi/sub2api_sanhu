<!-- 作者：mkx | 日期：2026-04-21 | 变更：批量清理 Tailwind 暗色变体类名并同步补充暖色主题改造注释 -->
<template>
 <button
 type="button"
 @click="toggle"
 class="relative inline-flex h-6 w-11 flex-shrink-0 cursor-pointer rounded-full border-2 border-transparent transition-colors duration-200 ease-in-out focus:outline-none focus:ring-2 focus:ring-primary-500 focus:ring-offset-2"
 :class="[modelValue ? 'bg-primary-600' : 'bg-gray-200 ']"
 role="switch"
 :aria-checked="modelValue"
 >
 <span
 class="pointer-events-none inline-block h-5 w-5 transform rounded-full bg-white shadow ring-0 transition duration-200 ease-in-out"
 :class="[modelValue ? 'translate-x-5' : 'translate-x-0']"
 />
 </button>
</template>

<script setup lang="ts">
const props = defineProps<{
 modelValue: boolean
}>()

const emit = defineEmits<{
 (e: 'update:modelValue', value: boolean): void
}>()

function toggle() {
 emit('update:modelValue', !props.modelValue)
}
</script>
