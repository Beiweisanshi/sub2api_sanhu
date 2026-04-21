<!-- 作者：mkx | 日期：2026-04-21 | 变更：批量清理 Tailwind 暗色变体类名并同步补充暖色主题改造注释 -->
<template>
 <BaseDialog :show="show" :title="title" width="narrow" @close="handleCancel">
 <div class="space-y-4">
 <p class="text-sm text-gray-600">{{ message }}</p>
 <slot></slot>
 </div>

 <template #footer>
 <div class="flex justify-end space-x-3">
 <button
 @click="handleCancel"
 type="button"
 class="rounded-md border border-gray-300 bg-white px-4 py-2 text-sm font-medium text-gray-700 hover:bg-gray-50 focus:outline-none focus:ring-2 focus:ring-primary-500 focus:ring-offset-2"
 >
 {{ cancelText }}
 </button>
 <button
 @click="handleConfirm"
 type="button"
 :class="[
 'rounded-md px-4 py-2 text-sm font-medium text-white focus:outline-none focus:ring-2 focus:ring-offset-2 ',
 danger
 ? 'bg-red-600 hover:bg-red-700 focus:ring-red-500'
 : 'bg-primary-600 hover:bg-primary-700 focus:ring-primary-500'
 ]"
 >
 {{ confirmText }}
 </button>
 </div>
 </template>
 </BaseDialog>
</template>

<script setup lang="ts">
import { computed } from 'vue'
import { useI18n } from 'vue-i18n'
import BaseDialog from './BaseDialog.vue'

const { t } = useI18n()

interface Props {
 show: boolean
 title: string
 message: string
 confirmText?: string
 cancelText?: string
 danger?: boolean
}

interface Emits {
 (e: 'confirm'): void
 (e: 'cancel'): void
}

const props = withDefaults(defineProps<Props>(), {
 danger: false
})

const confirmText = computed(() => props.confirmText || t('common.confirm'))
const cancelText = computed(() => props.cancelText || t('common.cancel'))

const emit = defineEmits<Emits>()

const handleConfirm = () => {
 emit('confirm')
}

const handleCancel = () => {
 emit('cancel')
}
</script>
