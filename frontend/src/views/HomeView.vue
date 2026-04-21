<!-- 作者：mkx | 日期：2026-04-21 | 变更：将首页重塑为 Claude 暖色风格并补充滚动显现动画 -->
<template>
 <!-- Custom Home Content: Full Page Mode -->
 <div v-if="homeContent" class="min-h-screen">
 <!-- iframe mode -->
 <iframe
 v-if="isHomeContentUrl"
 :src="homeContent.trim()"
 class="h-screen w-full border-0"
 allowfullscreen
 ></iframe>
 <!-- HTML mode - SECURITY: homeContent is admin-only setting, XSS risk is acceptable -->
 <div v-else v-html="homeContent"></div>
 </div>

 <!-- Default Home Page -->
 <div
 v-else
 ref="homeViewRef"
 class="relative flex min-h-screen flex-col overflow-hidden bg-claude-bg text-claude-text"
 >
 <!-- Background Decorations -->
 <div class="pointer-events-none absolute inset-0 overflow-hidden">
 <div class="absolute inset-0 bg-mesh-gradient opacity-90"></div>
 <div class="hero-grid-light opacity-70"></div>
 <div class="ambient-blob blob-1"></div>
 <div class="ambient-blob blob-2"></div>
 </div>

 <!-- Header -->
 <header class="relative z-20 px-6 py-4">
 <nav class="mx-auto flex max-w-6xl items-center justify-between">
 <!-- Logo -->
 <div class="flex items-center">
 <div class="h-10 w-10 overflow-hidden rounded-xl shadow-md">
 <img :src="siteLogo || '/logo.png'" alt="Logo" class="h-full w-full object-contain" />
 </div>
 </div>

 <!-- Nav Actions -->
 <div class="flex items-center gap-3">
 <!-- Language Switcher -->
 <LocaleSwitcher />

 <!-- Doc Link -->
 <a
 v-if="docUrl"
 :href="docUrl"
 target="_blank"
 rel="noopener noreferrer"
 class="rounded-lg p-2 text-gray-500 transition-colors hover:bg-gray-100 hover:text-claude-text"
 :title="t('home.viewDocs')"
 >
 <Icon name="book" size="md" />
 </a>

 <!-- Login / Dashboard Button -->
 <router-link
 v-if="isAuthenticated"
 :to="dashboardPath"
 class="inline-flex items-center gap-1.5 rounded-full bg-claude-text py-1 pl-1 pr-2.5 transition-colors hover:bg-gray-800"
 >
 <span
 class="flex h-5 w-5 items-center justify-center rounded-full bg-gradient-to-br from-primary-400 to-primary-600 text-[10px] font-semibold text-white"
 >
 {{ userInitial }}
 </span>
 <span class="text-xs font-medium text-white">{{ t('home.dashboard') }}</span>
 <svg
 class="h-3 w-3 text-gray-400"
 fill="none"
 viewBox="0 0 24 24"
 stroke="currentColor"
 stroke-width="2"
 >
 <path
 stroke-linecap="round"
 stroke-linejoin="round"
 d="M4.5 19.5l15-15m0 0H8.25m11.25 0v11.25"
 />
 </svg>
 </router-link>
 <router-link
 v-else
 to="/login"
 class="inline-flex items-center rounded-full bg-claude-text px-3 py-1 text-xs font-medium text-white transition-colors hover:bg-gray-800"
 >
 {{ t('home.login') }}
 </router-link>
 </div>
 </nav>
 </header>

 <!-- Main Content -->
 <main class="relative z-10 flex-1 px-6 py-16">
 <div class="mx-auto max-w-6xl">
 <!-- Hero Section - Left/Right Layout -->
 <div class="mb-12 flex flex-col items-center justify-between gap-12 lg:flex-row lg:gap-16">
 <!-- Left: Text Content -->
 <div class="reveal active flex-1 text-center lg:text-left">
 <h1
 class="mb-4 font-serif text-4xl font-bold text-claude-text text-breathe md:text-5xl lg:text-6xl"
 >
 {{ siteName }}
 </h1>
 <p class="mb-8 text-lg text-claude-muted md:text-xl">
 {{ siteSubtitle }}
 </p>

 <!-- CTA Button -->
 <div>
 <router-link
 :to="isAuthenticated ? dashboardPath : '/login'"
 class="btn btn-primary px-8 py-3 text-base shadow-lg shadow-primary-500/30"
 >
 {{ isAuthenticated ? t('home.goToDashboard') : t('home.getStarted') }}
 <Icon name="arrowRight" size="md" class="ml-2" :stroke-width="2" />
 </router-link>
 </div>
 </div>

 <!-- Right: Terminal Animation -->
 <div class="reveal delay-200 flex flex-1 justify-center lg:justify-end">
 <div class="terminal-container">
 <div class="terminal-window">
 <!-- Window header -->
 <div class="terminal-header">
 <div class="terminal-buttons">
 <span class="btn-close"></span>
 <span class="btn-minimize"></span>
 <span class="btn-maximize"></span>
 </div>
 <span class="terminal-title">terminal</span>
 </div>
 <!-- Terminal content -->
 <div class="terminal-body">
 <div class="code-line line-1">
 <span class="code-prompt">$</span>
 <span class="code-cmd">curl</span>
 <span class="code-flag">-X POST</span>
 <span class="code-url">/v1/messages</span>
 </div>
 <div class="code-line line-2">
 <span class="code-comment"># Routing to upstream...</span>
 </div>
 <div class="code-line line-3">
 <span class="code-success">200 OK</span>
 <span class="code-response">{"content": "Hello!" }</span>
 </div>
 <div class="code-line line-4">
 <span class="code-prompt">$</span>
 <span class="cursor"></span>
 </div>
 </div>
 </div>
 </div>
 </div>
 </div>

 <!-- Feature Tags - Centered -->
 <div class="mb-12 flex flex-wrap items-center justify-center gap-4 md:gap-6">
 <div
 class="reveal delay-100 inline-flex items-center gap-2.5 rounded-full border border-claude-border bg-white/80 px-5 py-2.5 shadow-sm backdrop-blur-sm claude-card-hover"
 >
 <Icon name="swap" size="sm" class="text-primary-500" />
 <span class="text-sm font-medium text-gray-700">{{
 t('home.tags.subscriptionToApi')
 }}</span>
 </div>
 <div
 class="reveal delay-200 inline-flex items-center gap-2.5 rounded-full border border-claude-border bg-white/80 px-5 py-2.5 shadow-sm backdrop-blur-sm claude-card-hover"
 >
 <Icon name="shield" size="sm" class="text-primary-500" />
 <span class="text-sm font-medium text-gray-700">{{
 t('home.tags.stickySession')
 }}</span>
 </div>
 <div
 class="reveal delay-300 inline-flex items-center gap-2.5 rounded-full border border-claude-border bg-white/80 px-5 py-2.5 shadow-sm backdrop-blur-sm claude-card-hover"
 >
 <Icon name="chart" size="sm" class="text-primary-500" />
 <span class="text-sm font-medium text-gray-700">{{
 t('home.tags.realtimeBilling')
 }}</span>
 </div>
 </div>

 <!-- Features Grid -->
 <div class="mb-12 grid gap-6 md:grid-cols-3">
 <!-- Feature 1: Unified Gateway -->
 <div
 class="group reveal delay-100 rounded-2xl border border-claude-border bg-white/75 p-6 backdrop-blur-sm claude-card-hover"
 >
 <div
 class="mb-4 flex h-12 w-12 items-center justify-center rounded-xl bg-gradient-to-br from-primary-500 to-primary-600 shadow-lg shadow-primary-500/30 transition-transform group-hover:scale-110"
 >
 <Icon name="server" size="lg" class="text-white" />
 </div>
 <h3 class="mb-2 text-lg font-semibold text-gray-900">
 {{ t('home.features.unifiedGateway') }}
 </h3>
 <p class="text-sm leading-relaxed text-gray-600">
 {{ t('home.features.unifiedGatewayDesc') }}
 </p>
 </div>

 <!-- Feature 2: Account Pool -->
 <div
 class="group reveal delay-200 rounded-2xl border border-claude-border bg-white/75 p-6 backdrop-blur-sm claude-card-hover"
 >
 <div
 class="mb-4 flex h-12 w-12 items-center justify-center rounded-xl bg-gradient-to-br from-primary-500 to-primary-600 shadow-lg shadow-primary-500/30 transition-transform group-hover:scale-110"
 >
 <svg
 class="h-6 w-6 text-white"
 fill="none"
 viewBox="0 0 24 24"
 stroke="currentColor"
 stroke-width="1.5"
 >
 <path
 stroke-linecap="round"
 stroke-linejoin="round"
 d="M18 18.72a9.094 9.094 0 003.741-.479 3 3 0 00-4.682-2.72m.94 3.198l.001.031c0 .225-.012.447-.037.666A11.944 11.944 0 0112 21c-2.17 0-4.207-.576-5.963-1.584A6.062 6.062 0 016 18.719m12 0a5.971 5.971 0 00-.941-3.197m0 0A5.995 5.995 0 0012 12.75a5.995 5.995 0 00-5.058 2.772m0 0a3 3 0 00-4.681 2.72 8.986 8.986 0 003.74.477m.94-3.197a5.971 5.971 0 00-.94 3.197M15 6.75a3 3 0 11-6 0 3 3 0 016 0zm6 3a2.25 2.25 0 11-4.5 0 2.25 2.25 0 014.5 0zm-13.5 0a2.25 2.25 0 11-4.5 0 2.25 2.25 0 014.5 0z"
 />
 </svg>
 </div>
 <h3 class="mb-2 text-lg font-semibold text-gray-900">
 {{ t('home.features.multiAccount') }}
 </h3>
 <p class="text-sm leading-relaxed text-gray-600">
 {{ t('home.features.multiAccountDesc') }}
 </p>
 </div>

 <!-- Feature 3: Billing & Quota -->
 <div
 class="group reveal delay-300 rounded-2xl border border-claude-border bg-white/75 p-6 backdrop-blur-sm claude-card-hover"
 >
 <div
 class="mb-4 flex h-12 w-12 items-center justify-center rounded-xl bg-gradient-to-br from-gray-500 to-gray-600 shadow-lg shadow-gray-500/30 transition-transform group-hover:scale-110"
 >
 <svg
 class="h-6 w-6 text-white"
 fill="none"
 viewBox="0 0 24 24"
 stroke="currentColor"
 stroke-width="1.5"
 >
 <path
 stroke-linecap="round"
 stroke-linejoin="round"
 d="M2.25 18.75a60.07 60.07 0 0115.797 2.101c.727.198 1.453-.342 1.453-1.096V18.75M3.75 4.5v.75A.75.75 0 013 6h-.75m0 0v-.375c0-.621.504-1.125 1.125-1.125H20.25M2.25 6v9m18-10.5v.75c0 .414.336.75.75.75h.75m-1.5-1.5h.375c.621 0 1.125.504 1.125 1.125v9.75c0 .621-.504 1.125-1.125 1.125h-.375m1.5-1.5H21a.75.75 0 00-.75.75v.75m0 0H3.75m0 0h-.375a1.125 1.125 0 01-1.125-1.125V15m1.5 1.5v-.75A.75.75 0 003 15h-.75M15 10.5a3 3 0 11-6 0 3 3 0 016 0zm3 0h.008v.008H18V10.5zm-12 0h.008v.008H6V10.5z"
 />
 </svg>
 </div>
 <h3 class="mb-2 text-lg font-semibold text-gray-900">
 {{ t('home.features.balanceQuota') }}
 </h3>
 <p class="text-sm leading-relaxed text-gray-600">
 {{ t('home.features.balanceQuotaDesc') }}
 </p>
 </div>
 </div>

 <!-- Supported Providers -->
 <div class="mb-8 text-center reveal delay-200">
 <h2 class="mb-3 font-serif text-2xl font-bold text-claude-text">
 {{ t('home.providers.title') }}
 </h2>
 <p class="text-sm text-claude-muted">
 {{ t('home.providers.description') }}
 </p>
 </div>

 <div class="mb-16 flex flex-wrap items-center justify-center gap-4">
 <!-- Claude - Supported -->
 <div
 class="reveal delay-100 flex items-center gap-2 rounded-xl border border-primary-200 bg-white/75 px-5 py-3 ring-1 ring-primary-500/15 backdrop-blur-sm claude-card-hover"
 >
 <div
 class="flex h-8 w-8 items-center justify-center rounded-lg bg-gradient-to-br from-orange-400 to-orange-500"
 >
 <span class="text-xs font-bold text-white">C</span>
 </div>
 <span class="text-sm font-medium text-gray-700">{{ t('home.providers.claude') }}</span>
 <span
 class="rounded bg-primary-100 px-1.5 py-0.5 text-[10px] font-medium text-primary-600"
 >{{ t('home.providers.supported') }}</span
 >
 </div>
 <!-- GPT - Supported -->
 <div
 class="reveal delay-200 flex items-center gap-2 rounded-xl border border-primary-200 bg-white/75 px-5 py-3 ring-1 ring-primary-500/15 backdrop-blur-sm claude-card-hover"
 >
 <div
 class="flex h-8 w-8 items-center justify-center rounded-lg bg-gradient-to-br from-primary-500 to-primary-600"
 >
 <span class="text-xs font-bold text-white">G</span>
 </div>
 <span class="text-sm font-medium text-gray-700">GPT</span>
 <span
 class="rounded bg-primary-100 px-1.5 py-0.5 text-[10px] font-medium text-primary-600"
 >{{ t('home.providers.supported') }}</span
 >
 </div>
 <!-- Gemini - Supported -->
 <div
 class="reveal delay-300 flex items-center gap-2 rounded-xl border border-primary-200 bg-white/75 px-5 py-3 ring-1 ring-primary-500/15 backdrop-blur-sm claude-card-hover"
 >
 <div
 class="flex h-8 w-8 items-center justify-center rounded-lg bg-gradient-to-br from-primary-500 to-primary-600"
 >
 <span class="text-xs font-bold text-white">G</span>
 </div>
 <span class="text-sm font-medium text-gray-700">{{ t('home.providers.gemini') }}</span>
 <span
 class="rounded bg-primary-100 px-1.5 py-0.5 text-[10px] font-medium text-primary-600"
 >{{ t('home.providers.supported') }}</span
 >
 </div>
 <!-- Antigravity - Supported -->
 <div
 class="reveal delay-400 flex items-center gap-2 rounded-xl border border-primary-200 bg-white/75 px-5 py-3 ring-1 ring-primary-500/15 backdrop-blur-sm claude-card-hover"
 >
 <div
 class="flex h-8 w-8 items-center justify-center rounded-lg bg-gradient-to-br from-orange-500 to-primary-600"
 >
 <span class="text-xs font-bold text-white">A</span>
 </div>
 <span class="text-sm font-medium text-gray-700">{{ t('home.providers.antigravity') }}</span>
 <span
 class="rounded bg-primary-100 px-1.5 py-0.5 text-[10px] font-medium text-primary-600"
 >{{ t('home.providers.supported') }}</span
 >
 </div>
 <!-- More - Coming Soon -->
 <div
 class="reveal delay-500 flex items-center gap-2 rounded-xl border border-claude-border bg-white/60 px-5 py-3 opacity-80 backdrop-blur-sm claude-card-hover"
 >
 <div
 class="flex h-8 w-8 items-center justify-center rounded-lg bg-gradient-to-br from-gray-500 to-gray-600"
 >
 <span class="text-xs font-bold text-white">+</span>
 </div>
 <span class="text-sm font-medium text-gray-700">{{ t('home.providers.more') }}</span>
 <span
 class="rounded bg-gray-100 px-1.5 py-0.5 text-[10px] font-medium text-gray-500"
 >{{ t('home.providers.soon') }}</span
 >
 </div>
 </div>
 </div>
 </main>

 <!-- Footer -->
 <footer class="relative z-10 border-t border-claude-border px-6 py-8">
 <div
 class="mx-auto flex max-w-6xl flex-col items-center justify-center gap-4 text-center sm:flex-row sm:text-left"
 >
 <p class="text-sm text-gray-500">
 &copy; {{ currentYear }} {{ siteName }}. {{ t('home.footer.allRightsReserved') }}
 </p>
 <div class="flex items-center gap-4">
 <a
 v-if="docUrl"
 :href="docUrl"
 target="_blank"
 rel="noopener noreferrer"
 class="text-sm text-gray-500 transition-colors hover:text-claude-text"
 >
 {{ t('home.docs') }}
 </a>
 <a
 :href="githubUrl"
 target="_blank"
 rel="noopener noreferrer"
 class="text-sm text-gray-500 transition-colors hover:text-claude-text"
 >
 GitHub
 </a>
 </div>
 </div>
 </footer>
 </div>
</template>

<script setup lang="ts">
import { ref, computed, nextTick, onMounted, onUnmounted } from 'vue'
import { useI18n } from 'vue-i18n'
import { useAuthStore, useAppStore } from '@/stores'
import LocaleSwitcher from '@/components/common/LocaleSwitcher.vue'
import Icon from '@/components/icons/Icon.vue'

const { t } = useI18n()

const authStore = useAuthStore()
const appStore = useAppStore()

// Site settings - directly from appStore (already initialized from injected config)
const siteName = computed(() => appStore.cachedPublicSettings?.site_name || appStore.siteName || 'Sub2API')
const siteLogo = computed(() => appStore.cachedPublicSettings?.site_logo || appStore.siteLogo || '')
const siteSubtitle = computed(() => appStore.cachedPublicSettings?.site_subtitle || 'AI API Gateway Platform')
const docUrl = computed(() => appStore.cachedPublicSettings?.doc_url || appStore.docUrl || '')
const homeContent = computed(() => appStore.cachedPublicSettings?.home_content || '')

// Check if homeContent is a URL (for iframe display)
const isHomeContentUrl = computed(() => {
 const content = homeContent.value.trim()
 return content.startsWith('http://') || content.startsWith('https://')
})

const homeViewRef = ref<HTMLElement | null>(null)
let revealObserver: IntersectionObserver | null = null

// GitHub URL
const githubUrl = 'https://github.com/Wei-Shaw/sub2api'

// Auth state
const isAuthenticated = computed(() => authStore.isAuthenticated)
const isAdmin = computed(() => authStore.isAdmin)
const dashboardPath = computed(() => isAdmin.value ? '/admin/dashboard' : '/dashboard')
const userInitial = computed(() => {
 const user = authStore.user
 if (!user || !user.email) return ''
 return user.email.charAt(0).toUpperCase()
})

// Current year for footer
const currentYear = computed(() => new Date().getFullYear())

function activateRevealFallback() {
 const revealElements = homeViewRef.value?.querySelectorAll<HTMLElement>('.reveal') ?? []
 revealElements.forEach((element) => {
 window.setTimeout(() => {
 element.classList.add('active')
 }, 80)
 })
}

function setupRevealAnimations() {
 const revealElements = homeViewRef.value?.querySelectorAll<HTMLElement>('.reveal') ?? []
 if (!revealElements.length) return

 if (!('IntersectionObserver' in window)) {
 activateRevealFallback()
 return
 }

 revealObserver?.disconnect()
 revealObserver = new IntersectionObserver(
 (entries) => {
 entries.forEach((entry) => {
 if (entry.isIntersecting) {
 entry.target.classList.add('active')
 revealObserver?.unobserve(entry.target)
 }
 })
 },
 { threshold: 0.16, rootMargin: '0px 0px -10% 0px' }
 )

 revealElements.forEach((element, index) => {
 if (index < 2) {
 element.classList.add('active')
 return
 }
 revealObserver?.observe(element)
 })

 window.setTimeout(() => {
 revealElements.forEach((element) => element.classList.add('active'))
 }, 500)
}

onMounted(() => {
 // Check auth state
 authStore.checkAuth()

 // Ensure public settings are loaded (will use cache if already loaded from injected config)
 if (!appStore.publicSettingsLoaded) {
 appStore.fetchPublicSettings()
 }

 nextTick(() => {
 setupRevealAnimations()
 })
})

onUnmounted(() => {
 revealObserver?.disconnect()
})
</script>

<style scoped>
/* Terminal Container */
.terminal-container {
 position: relative;
 display: inline-block;
}

/* Terminal Window */
.terminal-window {
 width: 420px;
 background: linear-gradient(145deg, #2d2a26 0%, #1e1e1e 100%);
 border-radius: 14px;
 box-shadow:
 0 25px 50px -12px rgba(0, 0, 0, 0.4),
 0 0 0 1px rgba(255, 255, 255, 0.08),
 inset 0 1px 0 rgba(255, 255, 255, 0.1);
 overflow: hidden;
 transform: perspective(1000px) rotateX(2deg) rotateY(-2deg);
 transition: transform 0.3s ease;
}

.terminal-window:hover {
 transform: perspective(1000px) rotateX(0deg) rotateY(0deg) translateY(-4px);
}

/* Terminal Header */
.terminal-header {
 display: flex;
 align-items: center;
 padding: 12px 16px;
 background: rgba(45, 42, 38, 0.88);
 border-bottom: 1px solid rgba(255, 255, 255, 0.05);
}

.terminal-buttons {
 display: flex;
 gap: 8px;
}

.terminal-buttons span {
 width: 12px;
 height: 12px;
 border-radius: 50%;
}

.btn-close {
 background: #ef4444;
}
.btn-minimize {
 background: #eab308;
}
.btn-maximize {
 background: #22c55e;
}

.terminal-title {
 flex: 1;
 text-align: center;
 font-size: 12px;
 font-family: 'JetBrains Mono', ui-monospace, monospace;
 color: #b5afa2;
 margin-right: 52px;
}

/* Terminal Body */
.terminal-body {
 padding: 20px 24px;
 font-family: 'JetBrains Mono', ui-monospace, monospace;
 font-size: 14px;
 line-height: 2;
}

.code-line {
 display: flex;
 align-items: center;
 gap: 8px;
 flex-wrap: wrap;
 opacity: 0;
 animation: line-appear 0.5s ease forwards;
}

.line-1 {
 animation-delay: 0.3s;
}
.line-2 {
 animation-delay: 1s;
}
.line-3 {
 animation-delay: 1.8s;
}
.line-4 {
 animation-delay: 2.5s;
}

@keyframes line-appear {
 from {
 opacity: 0;
 transform: translateY(5px);
 }
 to {
 opacity: 1;
 transform: translateY(0);
 }
}

.code-prompt {
 color: #22c55e;
 font-weight: bold;
}
.code-cmd {
 color: #f4d8cf;
}
.code-flag {
 color: #d9b9ac;
}
.code-url {
 color: #d96c4a;
}
.code-comment {
 color: #8c8578;
 font-style: italic;
}
.code-success {
 color: #22c55e;
 background: rgba(34, 197, 94, 0.15);
 padding: 2px 8px;
 border-radius: 4px;
 font-weight: 600;
}
.code-response {
 color: #f4c98b;
}

/* Blinking Cursor */
.cursor {
 display: inline-block;
 width: 8px;
 height: 16px;
 background: #22c55e;
 animation: blink 1s step-end infinite;
}

@keyframes blink {
 0%,
 50% {
 opacity: 1;
 }
 51%,
 100% {
 opacity: 0;
 }
}

@media (max-width: 640px) {
 .terminal-window {
 width: min(100%, 360px);
 }
}
</style>
