<template>
  <BaseDialog
    :show="show"
    :title="t('admin.channelMonitor.runResultTitle')"
    width="normal"
    @close="$emit('close')"
  >
    <div class="space-y-2">
      <div
        v-for="r in results"
        :key="r.model"
        class="flex items-start justify-between gap-3 rounded-lg border border-gray-200 px-3 py-2 text-sm dark:border-dark-600"
      >
        <div class="min-w-0 flex-1">
          <span class="block truncate font-medium text-gray-900 dark:text-white">{{ r.model }}</span>
          <span
            v-if="r.message"
            class="mt-1 block max-h-28 max-w-full overflow-y-auto break-words text-xs leading-relaxed text-gray-500 dark:text-gray-400"
          >
            {{ r.message }}
          </span>
        </div>
        <div class="flex flex-none items-center gap-2">
          <span
            class="inline-flex items-center rounded-full px-2 py-0.5 text-[11px]"
            :class="statusBadgeClass(r.status)"
          >
            {{ statusLabel(r.status) }}
          </span>
          <span class="text-xs text-gray-500 dark:text-gray-400">{{ formatLatency(r.latency_ms) }} ms</span>
        </div>
      </div>
    </div>
    <template #footer>
      <div class="flex justify-end">
        <button @click="$emit('close')" class="btn btn-primary">
          {{ t('common.close') }}
        </button>
      </div>
    </template>
  </BaseDialog>
</template>

<script setup lang="ts">
import { useI18n } from 'vue-i18n'
import type { CheckResult } from '@/api/admin/channelMonitor'
import BaseDialog from '@/components/common/BaseDialog.vue'
import { useChannelMonitorFormat } from '@/composables/useChannelMonitorFormat'

defineProps<{
  show: boolean
  results: CheckResult[]
}>()

defineEmits<{
  (e: 'close'): void
}>()

const { t } = useI18n()
const { statusLabel, statusBadgeClass, formatLatency } = useChannelMonitorFormat()
</script>
