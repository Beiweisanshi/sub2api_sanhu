<!-- mkx: 新增模型广场用户页面，2026-04-24 -->
<template>
  <AppLayout>
    <div class="space-y-6">
      <div class="card p-5">
        <div class="flex flex-col gap-4 lg:flex-row lg:items-center lg:justify-between">
          <div>
            <h1 class="text-2xl font-semibold text-gray-900">{{ t('plaza.title') }}</h1>
            <p class="mt-1 text-sm text-gray-500">{{ t('plaza.description') }}</p>
          </div>
          <div class="flex flex-col gap-3 sm:flex-row sm:items-center">
            <SearchInput
              v-model="query"
              :placeholder="t('plaza.searchPlaceholder')"
              class="w-full sm:w-72"
            />
            <Select
              v-model="platform"
              class="w-full sm:w-48"
              :options="platformOptions"
            />
          </div>
        </div>
      </div>

      <div v-if="loading" class="card flex items-center justify-center gap-3 p-10 text-gray-500">
        <LoadingSpinner size="md" />
        <span>{{ t('plaza.loading') }}</span>
      </div>

      <EmptyState
        v-else-if="visibleGroups.length === 0"
        :title="t('plaza.emptyState')"
        :description="t('plaza.description')"
      />

      <div v-else class="space-y-5">
        <section v-for="group in visibleGroups" :key="group.id" class="card overflow-hidden">
          <div class="border-b border-gray-100 p-5">
            <div class="flex flex-wrap items-center gap-2">
              <h2 class="text-lg font-semibold text-gray-900">{{ group.name }}</h2>
              <span class="rounded-full bg-gray-100 px-2.5 py-1 text-xs font-medium text-gray-600">
                {{ group.platform }}
              </span>
              <span class="rounded-full bg-primary-50 px-2.5 py-1 text-xs font-semibold text-primary-700">
                {{ t('plaza.multiplier') }} ×{{ formatMultiplier(group.effective_multiplier) }}
              </span>
              <span
                v-if="group.has_personal_rate"
                class="rounded-full bg-amber-50 px-2.5 py-1 text-xs font-semibold text-amber-700"
              >
                {{ t('plaza.personalRate') }}
              </span>
            </div>
            <p v-if="group.description" class="mt-2 text-sm text-gray-500">
              {{ group.description }}
            </p>
          </div>

          <div class="overflow-x-auto">
            <table class="min-w-full divide-y divide-gray-100 text-sm">
              <thead class="bg-gray-50 text-xs uppercase tracking-wide text-gray-500">
                <tr>
                  <th class="px-5 py-3 text-left font-semibold">Model</th>
                  <th class="px-5 py-3 text-right font-semibold">{{ t('plaza.inputPrice') }}</th>
                  <th class="px-5 py-3 text-right font-semibold">{{ t('plaza.outputPrice') }}</th>
                  <th class="px-5 py-3 text-right font-semibold">{{ t('plaza.cacheReadPrice') }}</th>
                  <th v-if="hasPriority(group)" class="px-5 py-3 text-right font-semibold">
                    {{ t('plaza.priorityInputPrice') }}
                  </th>
                  <th v-if="hasImage(group)" class="px-5 py-3 text-right font-semibold">
                    {{ t('plaza.imagePrice') }}
                  </th>
                </tr>
              </thead>
              <tbody class="divide-y divide-gray-100 bg-white">
                <tr v-for="model in group.models" :key="model.name" class="hover:bg-gray-50/70">
                  <td class="whitespace-nowrap px-5 py-3 font-medium text-gray-900">
                    {{ model.name }}
                  </td>
                  <td class="px-5 py-3 text-right tabular-nums">
                    <PriceCell :original="model.original.input_per_mtok" :actual="model.actual.input_per_mtok" :multiplier="group.effective_multiplier" />
                  </td>
                  <td class="px-5 py-3 text-right tabular-nums">
                    <PriceCell :original="model.original.output_per_mtok" :actual="model.actual.output_per_mtok" :multiplier="group.effective_multiplier" />
                  </td>
                  <td class="px-5 py-3 text-right tabular-nums">
                    <PriceCell :original="model.original.cache_read_per_mtok" :actual="model.actual.cache_read_per_mtok" :multiplier="group.effective_multiplier" />
                  </td>
                  <td v-if="hasPriority(group)" class="px-5 py-3 text-right tabular-nums">
                    <PriceCell :original="model.original.input_priority_per_mtok || 0" :actual="model.actual.input_priority_per_mtok || 0" :multiplier="group.effective_multiplier" />
                  </td>
                  <td v-if="hasImage(group)" class="px-5 py-3 text-right tabular-nums">
                    <PriceCell :original="model.original.output_image_per_image || 0" :actual="model.actual.output_image_per_image || 0" :multiplier="group.effective_multiplier" />
                  </td>
                </tr>
              </tbody>
            </table>
          </div>
        </section>
      </div>
    </div>
  </AppLayout>
</template>

<script setup lang="ts">
import { computed, defineComponent, h, onMounted, ref } from 'vue'
import { useI18n } from 'vue-i18n'
import plazaAPI, { type PlazaGroup, type PlazaModel } from '@/api/plaza'
import AppLayout from '@/components/layout/AppLayout.vue'
import EmptyState from '@/components/common/EmptyState.vue'
import LoadingSpinner from '@/components/common/LoadingSpinner.vue'
import SearchInput from '@/components/common/SearchInput.vue'
import Select from '@/components/common/Select.vue'

const { t } = useI18n()

const loading = ref(false)
const groups = ref<PlazaGroup[]>([])
const query = ref('')
const platform = ref('all')

const platformOptions = computed(() => [
  { value: 'all', label: t('plaza.platformAll') },
  { value: 'anthropic', label: 'Anthropic' },
  { value: 'openai', label: 'OpenAI' },
  { value: 'gemini', label: 'Gemini' },
  { value: 'antigravity', label: 'Antigravity' }
])

const visibleGroups = computed(() => {
  const keyword = query.value.trim().toLowerCase()
  return groups.value
    .filter(group => platform.value === 'all' || group.platform === platform.value)
    .map(group => ({
      ...group,
      models: keyword
        ? group.models.filter(model => model.name.toLowerCase().includes(keyword))
        : group.models
    }))
    .filter(group => group.models.length > 0)
})

const loadPlaza = async () => {
  loading.value = true
  try {
    const data = await plazaAPI.getPlaza()
    groups.value = data.groups || []
  } finally {
    loading.value = false
  }
}

const formatUSD = (value: number): string => {
  if (!Number.isFinite(value) || value === 0) return '$0'
  if (value >= 1) return `$${value.toFixed(2)}`
  if (value < 0.01) {
    return `$${value.toFixed(6).replace(/0+$/, '').replace(/\.$/, '')}`
  }
  return `$${value.toFixed(4).replace(/0+$/, '').replace(/\.$/, '')}`
}

const formatMultiplier = (value: number): string => {
  return Number.isInteger(value) ? String(value) : value.toFixed(4).replace(/0+$/, '').replace(/\.$/, '')
}

const hasPriority = (group: PlazaGroup): boolean => {
  return group.models.some(model => (model.original.input_priority_per_mtok || 0) > 0)
}

const hasImage = (group: PlazaGroup): boolean => {
  return group.models.some(model => isImageModel(model))
}

const isImageModel = (model: PlazaModel): boolean => {
  return model.mode.includes('image') || (model.original.output_image_per_image || 0) > 0
}

const PriceCell = defineComponent({
  props: {
    original: { type: Number, required: true },
    actual: { type: Number, required: true },
    multiplier: { type: Number, required: true }
  },
  setup(props) {
    return () => {
      if (props.multiplier === 1) {
        return h('span', { class: 'text-gray-700' }, formatUSD(props.original))
      }
      return h('span', { class: 'inline-flex items-center justify-end gap-2' }, [
        h('s', { class: 'text-gray-400' }, formatUSD(props.original)),
        h('b', { class: 'text-gray-900' }, formatUSD(props.actual))
      ])
    }
  }
})

onMounted(loadPlaza)
</script>
