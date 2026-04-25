<template>
  <DataTable
    :columns="columns"
    :data="items"
    :loading="loading"
    :row-key="row => row.model_name"
    :sticky-first-column="true"
    :sticky-actions-column="true"
  >
    <template #cell-model_name="{ row }">
      <div class="max-w-xs">
        <p class="truncate font-medium text-gray-900" :title="row.model_name">{{ row.model_name }}</p>
        <p v-if="row.note" class="mt-0.5 truncate text-xs text-gray-400" :title="row.note">{{ row.note }}</p>
      </div>
    </template>

    <template #cell-provider="{ value }">
      <span class="inline-flex rounded bg-gray-100 px-2 py-0.5 text-xs font-medium text-gray-700">
        {{ value || '-' }}
      </span>
    </template>

    <template #cell-source="{ row }">
      <span :class="sourceBadgeClass(row)">
        {{ sourceLabel(row) }}
      </span>
    </template>

    <template #cell-input="{ row }">
      <PriceCell :value="row.effective.input_cost_per_token" />
    </template>
    <template #cell-output="{ row }">
      <PriceCell :value="row.effective.output_cost_per_token" />
    </template>
    <template #cell-cache_read="{ row }">
      <PriceCell :value="row.effective.cache_read_input_token_cost" />
    </template>
    <template #cell-cache_write="{ row }">
      <PriceCell :value="row.effective.cache_creation_input_token_cost" />
    </template>
    <template #cell-fast_multiplier="{ row }">
      <MultiplierCell :value="row.effective.fast_price_multiplier" />
    </template>

    <template #cell-actions="{ row }">
      <div class="flex items-center gap-1">
        <button
          type="button"
          class="flex flex-col items-center gap-0.5 rounded-lg p-1.5 text-gray-500 transition-colors hover:bg-gray-100 hover:text-primary-600"
          @click="emit('edit', row)"
        >
          <Icon name="edit" size="sm" />
          <span class="text-xs">{{ t('admin.modelPricing.edit') }}</span>
        </button>
        <button
          v-if="row.has_override || row.is_custom"
          type="button"
          class="flex flex-col items-center gap-0.5 rounded-lg p-1.5 text-gray-500 transition-colors hover:bg-red-50 hover:text-red-600"
          @click="emit('reset', row)"
        >
          <Icon name="trash" size="sm" />
          <span class="text-xs">{{ row.is_custom ? t('common.delete', 'Delete') : t('admin.modelPricing.reset') }}</span>
        </button>
      </div>
    </template>

    <template #empty>
      <EmptyState :title="t('admin.modelPricing.empty')" />
    </template>
  </DataTable>
</template>

<script setup lang="ts">
import { computed, defineComponent, h } from 'vue'
import { useI18n } from 'vue-i18n'
import DataTable from '@/components/common/DataTable.vue'
import EmptyState from '@/components/common/EmptyState.vue'
import Icon from '@/components/icons/Icon.vue'
import type { Column } from '@/components/common/types'
import type { ModelPricingItem } from '@/api/admin/modelPricing'
import { perTokenToMTok } from '@/components/admin/channel/types'

defineProps<{
  items: ModelPricingItem[]
  loading?: boolean
}>()

const emit = defineEmits<{
  edit: [item: ModelPricingItem]
  reset: [item: ModelPricingItem]
}>()

const { t } = useI18n()

const columns = computed<Column[]>(() => [
  { key: 'model_name', label: t('admin.modelPricing.columns.model'), sortable: true, class: 'min-w-[260px]' },
  { key: 'provider', label: t('admin.modelPricing.columns.provider'), sortable: true },
  { key: 'mode', label: t('admin.modelPricing.columns.mode'), sortable: true },
  { key: 'input', label: t('admin.modelPricing.columns.input'), class: 'text-right' },
  { key: 'output', label: t('admin.modelPricing.columns.output'), class: 'text-right' },
  { key: 'cache_read', label: t('admin.modelPricing.columns.cacheRead'), class: 'text-right' },
  { key: 'cache_write', label: t('admin.modelPricing.columns.cacheWrite'), class: 'text-right' },
  { key: 'fast_multiplier', label: t('admin.modelPricing.columns.fastMultiplier'), class: 'text-right' },
  { key: 'source', label: t('admin.modelPricing.columns.source') },
  { key: 'actions', label: t('common.actions'), class: 'text-right' }
])

const sourceLabel = (item: ModelPricingItem): string => {
  if (item.is_custom) return t('admin.modelPricing.source.custom')
  if (item.has_override) return t('admin.modelPricing.source.overridden')
  return t('admin.modelPricing.source.embedded')
}

const sourceBadgeClass = (item: ModelPricingItem): string => {
  const base = 'inline-flex rounded-full px-2 py-0.5 text-xs font-medium'
  if (item.is_custom) return `${base} bg-purple-100 text-purple-700`
  if (item.has_override) return `${base} bg-amber-100 text-amber-700`
  return `${base} bg-emerald-100 text-emerald-700`
}

const PriceCell = defineComponent({
  name: 'ModelPricingPriceCell',
  props: {
    value: { type: Number, default: null }
  },
  setup(cellProps) {
    return () => {
      const display = perTokenToMTok(cellProps.value)
      return h('span', { class: 'font-mono text-sm text-gray-700' }, display === null ? '-' : display.toString())
    }
  }
})

const MultiplierCell = defineComponent({
  name: 'ModelPricingMultiplierCell',
  props: {
    value: { type: Number, default: null }
  },
  setup(cellProps) {
    return () => h('span', { class: 'font-mono text-sm text-gray-700' }, cellProps.value ? `${cellProps.value}x` : '-')
  }
})
</script>
