<template>
 <AppLayout>
 <TablePageLayout>
 <template #filters>
 <div class="flex flex-col gap-4 lg:flex-row lg:items-start lg:justify-between">
 <div>
 <h1 class="text-2xl font-semibold text-gray-900">{{ t('admin.modelPricing.title') }}</h1>
 <p class="mt-1 text-sm text-gray-500">{{ t('admin.modelPricing.description') }}</p>
 </div>

 <div class="flex w-full flex-col gap-3 sm:flex-row lg:w-auto">
 <div class="relative w-full sm:w-72">
 <Icon name="search" size="md" class="absolute left-3 top-1/2 -translate-y-1/2 text-gray-400" />
 <input v-model.trim="search" type="text" class="input pl-10" :placeholder="t('admin.modelPricing.search')" />
 </div>
 <Select v-model="provider" :options="providerOptions" class="w-full sm:w-48" />
 <button class="btn btn-secondary" :disabled="loading" @click="loadItems">
 <Icon name="refresh" size="md" :class="loading ? 'animate-spin' : ''" />
 </button>
 <button class="btn btn-primary whitespace-nowrap" @click="openCreate">
 <Icon name="plus" size="md" class="mr-2" />
 {{ t('admin.modelPricing.addCustom') }}
 </button>
 </div>
 </div>
 </template>

 <template #table>
 <ModelPricingTable
 :items="pagedItems"
 :loading="loading"
 @edit="openEdit"
 @reset="handleReset"
 />
 </template>

 <template #pagination>
 <Pagination
 v-if="filteredItems.length > pageSize"
 :page="page"
 :total="filteredItems.length"
 :page-size="pageSize"
 @update:page="page = $event"
 @update:pageSize="handlePageSizeChange"
 />
 </template>
 </TablePageLayout>

 <ModelPricingEditModal
 :show="showModal"
 :item="editingItem"
 :saving="saving"
 @close="closeModal"
 @save="handleSave"
 />
 </AppLayout>
</template>

<script setup lang="ts">
import { computed, onMounted, ref, watch } from 'vue'
import { useI18n } from 'vue-i18n'
import { useAppStore } from '@/stores/app'
import { extractApiErrorMessage } from '@/utils/apiError'
import AppLayout from '@/components/layout/AppLayout.vue'
import TablePageLayout from '@/components/layout/TablePageLayout.vue'
import Select from '@/components/common/Select.vue'
import Pagination from '@/components/common/Pagination.vue'
import Icon from '@/components/icons/Icon.vue'
import ModelPricingTable from '@/components/admin/modelPricing/ModelPricingTable.vue'
import ModelPricingEditModal from '@/components/admin/modelPricing/ModelPricingEditModal.vue'
import modelPricingAPI, { type ModelPricingItem, type ModelPricingPayload } from '@/api/admin/modelPricing'

const { t } = useI18n()
const appStore = useAppStore()
const pageSize = ref(100)
const page = ref(1)
const search = ref('')
const provider = ref<string | null>('')
const items = ref<ModelPricingItem[]>([])
const loading = ref(false)
const saving = ref(false)
const showModal = ref(false)
const editingItem = ref<ModelPricingItem | null>(null)

const providerOptions = computed(() => {
 const providers = Array.from(new Set(items.value.map(item => item.provider).filter(Boolean))).sort()
 return [
 { value: '', label: t('admin.modelPricing.provider') },
 ...providers.map(value => ({ value, label: value }))
 ]
})

const filteredItems = computed(() => {
 const keyword = search.value.toLowerCase()
 return items.value.filter(item => {
 const matchSearch = !keyword || item.model_name.toLowerCase().includes(keyword)
 const matchProvider = !provider.value || item.provider === provider.value
 return matchSearch && matchProvider
 })
})

const pagedItems = computed(() => {
 const start = (page.value - 1) * pageSize.value
 return filteredItems.value.slice(start, start + pageSize.value)
})

watch([search, provider], () => {
 page.value = 1
})

onMounted(loadItems)

async function loadItems() {
 loading.value = true
 try {
 items.value = await modelPricingAPI.list()
 } catch (error) {
 appStore.showError(extractApiErrorMessage(error, t('admin.modelPricing.loadError')))
 } finally {
 loading.value = false
 }
}

function openCreate() {
 editingItem.value = null
 showModal.value = true
}

function openEdit(item: ModelPricingItem) {
 editingItem.value = item
 showModal.value = true
}

function closeModal() {
 showModal.value = false
 editingItem.value = null
}

async function handleSave(modelName: string, payload: ModelPricingPayload, isCreate: boolean) {
 if (!modelName) return
 saving.value = true
 try {
 if (isCreate) {
 await modelPricingAPI.create({ model_name: modelName, ...payload })
 } else {
 await modelPricingAPI.upsert(modelName, payload)
 }
 closeModal()
 await loadItems()
 } catch (error) {
 appStore.showError(extractApiErrorMessage(error, t('admin.modelPricing.saveError')))
 } finally {
 saving.value = false
 }
}

async function handleReset(item: ModelPricingItem) {
 const key = item.is_custom ? 'admin.modelPricing.deleteCustomConfirm' : 'admin.modelPricing.resetConfirm'
 const confirmed = window.confirm(t(key, { name: item.model_name }))
 if (!confirmed) return
 try {
 await modelPricingAPI.remove(item.model_name)
 await loadItems()
 } catch (error) {
 appStore.showError(extractApiErrorMessage(error, t('admin.modelPricing.deleteError')))
 }
}

function handlePageSizeChange(nextPageSize: number) {
 pageSize.value = nextPageSize
 page.value = 1
}
</script>
