import { apiClient } from '../client'

export interface PriceFields {
  input_cost_per_token: number | null
  output_cost_per_token: number | null
  cache_read_input_token_cost: number | null
  cache_creation_input_token_cost: number | null
  fast_price_multiplier: number | null
}

export interface ModelPricingItem {
  model_name: string
  provider: string
  mode: string
  is_custom: boolean
  has_override: boolean
  effective: PriceFields
  base: PriceFields | null
  override: PriceFields | null
  note: string
  updated_at?: string
}

export interface ModelPricingPayload extends PriceFields {
  note?: string
}

export interface CreateModelPricingPayload extends ModelPricingPayload {
  model_name: string
}

export async function list(): Promise<ModelPricingItem[]> {
  const { data } = await apiClient.get<ModelPricingItem[]>('/admin/model-pricing')
  return data
}

export async function upsert(modelName: string, payload: ModelPricingPayload): Promise<void> {
  await apiClient.put(`/admin/model-pricing/${encodeURIComponent(modelName).replace(/%2F/g, '/')}`, payload)
}

export async function create(payload: CreateModelPricingPayload): Promise<void> {
  await apiClient.post('/admin/model-pricing', payload)
}

export async function remove(modelName: string): Promise<void> {
  await apiClient.delete(`/admin/model-pricing/${encodeURIComponent(modelName).replace(/%2F/g, '/')}`)
}

const modelPricingAPI = { list, upsert, create, remove }
export default modelPricingAPI
