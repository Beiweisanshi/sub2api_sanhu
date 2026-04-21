<!-- 作者：mkx | 日期：2026-04-21 | 变更：将认证布局调整为 Claude 暖色氛围与玻璃卡片风格 -->
<template>
  <div
    class="relative flex min-h-screen items-center justify-center overflow-hidden bg-claude-bg p-4 text-claude-text"
  >
    <div class="pointer-events-none absolute inset-0 bg-mesh-gradient opacity-90"></div>
    <div class="pointer-events-none absolute inset-0 hero-grid-light opacity-70"></div>
    <div class="ambient-blob blob-1"></div>
    <div class="ambient-blob blob-2"></div>

    <div class="relative z-10 w-full max-w-md">
      <div class="mb-8 text-center reveal active">
        <template v-if="settingsLoaded">
          <div class="mx-auto mb-4 flex h-20 w-20 items-center justify-center">
            <img :src="siteLogo || '/logo.png'" alt="芝麻 ZHIMA" class="h-full w-full object-contain" />
          </div>
          <h1 class="mb-2 text-4xl font-bold leading-none text-claude-text text-breathe">
            <span class="font-serif align-baseline">芝麻</span>
            <span class="ml-2 font-sans align-baseline text-[0.85em] text-primary-500">ZHIMA</span>
          </h1>
          <p class="mt-2 text-sm text-claude-muted">
            {{ siteSubtitle }}
          </p>
        </template>
      </div>

      <div class="glass-card claude-card-hover rounded-2xl p-8 shadow-glass">
        <slot />
      </div>

      <div class="mt-6 text-center text-sm text-claude-muted">
        <slot name="footer" />
      </div>

      <div class="mt-8 text-center text-xs text-gray-500">
        &copy; {{ currentYear }} 芝麻 ZHIMA. All rights reserved.
      </div>
    </div>
  </div>
</template>

<script setup lang="ts">
import { computed, onMounted } from 'vue'
import { useAppStore } from '@/stores'
import { sanitizeUrl } from '@/utils/url'

const appStore = useAppStore()

const siteLogo = computed(() => sanitizeUrl(appStore.siteLogo || '', { allowRelative: true, allowDataUrl: true }))
const siteSubtitle = computed(() => appStore.cachedPublicSettings?.site_subtitle || 'Subscription to API Conversion Platform')
const settingsLoaded = computed(() => appStore.publicSettingsLoaded)

const currentYear = computed(() => new Date().getFullYear())

onMounted(() => {
  appStore.fetchPublicSettings()
})
</script>

<style scoped>
.text-gradient {
  @apply bg-gradient-to-r from-primary-600 to-primary-500 bg-clip-text text-transparent;
}
</style>
