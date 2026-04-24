<template>
 <BaseDialog
 :show="show"
 :title="isCreate ? t('admin.modelPricing.addCustom') : t('admin.modelPricing.edit')"
 width="normal"
 @close="emit('close')"
 >
 <form id="model-pricing-form" class="space-y-4" @submit.prevent="handleSubmit">
 <div v-if="isCreate">
 <label class="input-label">{{ t('admin.modelPricing.columns.model') }} <span class="text-red-500">*</span></label>
 <input v-model.trim="form.model_name" required maxlength="200" class="input" placeholder="gpt-custom-model" />
 </div>
 <div v-else-if="item" class="rounded-lg bg-gray-50 p-3">
 <p class="text-xs text-gray-500">{{ t('admin.modelPricing.columns.model') }}</p>
 <p class="break-all font-medium text-gray-900">{{ item.model_name }}</p>
 </div>

 <div class="grid grid-cols-1 gap-3 sm:grid-cols-2">
 <PriceInput v-model="form.input" :label="t('admin.modelPricing.columns.input')" />
 <PriceInput v-model="form.output" :label="t('admin.modelPricing.columns.output')" />
 <PriceInput v-model="form.cache_read" :label="t('admin.modelPricing.columns.cacheRead')" />
 <PriceInput v-model="form.cache_write" :label="t('admin.modelPricing.columns.cacheWrite')" />
 </div>

 <div>
 <label class="input-label">{{ t('admin.modelPricing.note') }}</label>
 <textarea v-model.trim="form.note" rows="3" maxlength="1000" class="input" />
 </div>
 </form>

 <template #footer>
 <button type="button" class="btn btn-secondary" @click="emit('close')">
 {{ t('common.cancel', 'Cancel') }}
 </button>
 <button type="submit" form="model-pricing-form" class="btn btn-primary" :disabled="saving">
 {{ saving ? t('common.saving', 'Saving...') : t('common.save', 'Save') }}
 </button>
 </template>
 </BaseDialog>
</template>

<script setup lang="ts">
import { computed, reactive, watch, defineComponent, h } from 'vue'
import { useI18n } from 'vue-i18n'
import BaseDialog from '@/components/common/BaseDialog.vue'
import type { ModelPricingItem, ModelPricingPayload } from '@/api/admin/modelPricing'
import { mTokToPerToken, perTokenToMTok } from '@/components/admin/channel/types'

const props = defineProps<{
 show: boolean
 item?: ModelPricingItem | null
 saving?: boolean
}>()

const emit = defineEmits<{
 close: []
 save: [modelName: string, payload: ModelPricingPayload, isCreate: boolean]
}>()

const { t } = useI18n()

const form = reactive({
 model_name: '',
 input: '' as number | string,
 output: '' as number | string,
 cache_read: '' as number | string,
 cache_write: '' as number | string,
 note: ''
})

const isCreate = computed(() => !props.item)

watch(
 () => [props.show, props.item] as const,
 () => {
 if (!props.show) return
 const item = props.item
 form.model_name = item?.model_name || ''
 form.input = perTokenToMTok(item?.effective.input_cost_per_token) ?? ''
 form.output = perTokenToMTok(item?.effective.output_cost_per_token) ?? ''
 form.cache_read = perTokenToMTok(item?.effective.cache_read_input_token_cost) ?? ''
 form.cache_write = perTokenToMTok(item?.effective.cache_creation_input_token_cost) ?? ''
 form.note = item?.note || ''
 },
 { immediate: true }
)


function handleSubmit() {
 const modelName = form.model_name.trim()
 const payload: ModelPricingPayload = {
 input_cost_per_token: mTokToPerToken(form.input),
 output_cost_per_token: mTokToPerToken(form.output),
 cache_read_input_token_cost: mTokToPerToken(form.cache_read),
 cache_creation_input_token_cost: mTokToPerToken(form.cache_write),
 note: form.note.trim()
 }
 emit('save', modelName, payload, isCreate.value)
}

const PriceInput = defineComponent({
 name: 'ModelPricingPriceInput',
 props: {
 modelValue: { type: [Number, String], default: undefined },
 label: { type: String, required: true }
 },
 emits: ['update:modelValue'],
 setup(inputProps, { emit: inputEmit }) {
 return () => h('div', [
 h('label', { class: 'input-label' }, [
 inputProps.label,
 h('span', { class: 'ml-1 font-normal text-gray-400' }, '$/MTok')
 ]),
 h('input', {
 value: inputProps.modelValue ?? '',
 type: 'number',
 min: '0',
 step: 'any',
 class: 'input font-mono',
 placeholder: '0',
 onInput: (event: Event) => inputEmit('update:modelValue', (event.target as HTMLInputElement).value)
 })
 ])
 }
})
</script>
