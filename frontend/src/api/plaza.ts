// mkx: 新增模型广场接口，2026-04-24
import { apiClient } from './client'

export interface PlazaModelPrice {
  input_per_mtok: number
  output_per_mtok: number
  cache_read_per_mtok: number
  input_priority_per_mtok?: number
  output_priority_per_mtok?: number
  cache_read_priority_per_mtok?: number
  output_image_per_image?: number
  output_image_1k_per_image?: number
  output_image_2k_per_image?: number
  output_image_4k_per_image?: number
}

export interface PlazaModel {
  name: string
  mode: string
  original: PlazaModelPrice
  actual: PlazaModelPrice
}

export interface PlazaGroup {
  id: number
  name: string
  description?: string
  platform: string
  default_multiplier: number
  effective_multiplier: number
  has_personal_rate: boolean
  sort_order: number
  supported_scopes: string[]
  models: PlazaModel[]
}

export interface PlazaResponse {
  currency: string
  groups: PlazaGroup[]
}

export async function getPlaza(): Promise<PlazaResponse> {
  const { data } = await apiClient.get<PlazaResponse>('/plaza')
  return data
}

export const plazaAPI = { getPlaza }

export default plazaAPI
