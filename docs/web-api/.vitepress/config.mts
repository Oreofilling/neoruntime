import { defineConfig } from 'vitepress'

// Base path for GitHub Pages project sites (<user>.github.io/<repo>).
// If you deploy to a custom domain or user-site, change this to '/'.
const base = '/neoruntime/'
const specVersion = 'v1.0.2'

const guideEn = [
  { text: 'Introduction', link: '/introduction' },
  { text: 'Quickstart', link: '/quickstart' },
  { text: 'Authentication', link: '/authentication' },
  { text: 'Errors & Status Codes', link: '/errors' },
  { text: 'API Reference', link: '/api-reference' },
  { text: 'Keeping Docs in Sync', link: '/update-mechanism' },
]

const guideZh = [
  { text: '简介', link: '/zh/introduction' },
  { text: '快速开始', link: '/zh/quickstart' },
  { text: '认证', link: '/zh/authentication' },
  { text: '错误与状态码', link: '/zh/errors' },
  { text: 'API 参考', link: '/zh/api-reference' },
  { text: '文档同步机制', link: '/zh/update-mechanism' },
]

export default defineConfig({
  base,
  lastUpdated: true,
  srcExclude: ['**/README.md'], // repo-level README, not a docs page
  sitemap: {
    hostname: 'https://camthink-ai.github.io/neoruntime/',
  },
  head: [
    ['meta', { name: 'theme-color', content: '#0053b3' }],
  ],
  locales: {
    // English is the default locale at the site root.
    root: {
      label: 'English',
      lang: 'en',
      title: 'NeoRuntime Web API',
      description: 'Bilingual reference for the NeoRuntime platform Web API — REST, WebSocket and SSE for edge AI devices.',
      themeConfig: {
        nav: [
          ...guideEn,
          { text: `Spec ${specVersion}`, items: [
            { text: 'swagger.yaml (source)', link: 'https://github.com/camthink-ai/neoruntime/blob/main/docs/api/swagger.yaml' },
            { text: 'swagger.json (site)', link: '/swagger.json' },
          ] },
        ],
        sidebar: guideEn.map(({ text, link }) => ({ text, link })),
        outline: { label: 'On this page' },
        docFooter: { prev: 'Previous', next: 'Next' },
        lastUpdated: { text: 'Last updated' },
        returnToTopLabel: 'Back to top',
        sidebarMenuLabel: 'Menu',
        darkModeSwitchLabel: 'Appearance',
        lightModeSwitchTitle: 'Switch to light theme',
        darkModeSwitchTitle: 'Switch to dark theme',
      },
    },
    zh: {
      label: '简体中文',
      lang: 'zh-CN',
      title: 'NeoRuntime Web API',
      description: 'NeoRuntime 平台 Web API 双语参考文档 —— 面向边缘 AI 设备的 REST、WebSocket 与 SSE 接口。',
      themeConfig: {
        nav: [
          ...guideZh,
          { text: `Spec ${specVersion}`, items: [
            { text: 'swagger.yaml（源文件）', link: 'https://github.com/camthink-ai/neoruntime/blob/main/docs/api/swagger.yaml' },
            { text: 'swagger.json（本站）', link: '/swagger.zh.json' },
          ] },
        ],
        sidebar: guideZh.map(({ text, link }) => ({ text, link })),
        outline: { label: '本页目录' },
        docFooter: { prev: '上一篇', next: '下一篇' },
        lastUpdated: { text: '最后更新' },
        returnToTopLabel: '回到顶部',
        sidebarMenuLabel: '菜单',
        darkModeSwitchLabel: '外观',
        lightModeSwitchTitle: '切换到浅色模式',
        darkModeSwitchTitle: '切换到深色模式',
      },
    },
  },
  themeConfig: {
    search: {
      provider: 'local',
      options: {
        translations: {
          button: { buttonText: 'Search / 搜索', buttonAriaLabel: 'Search docs' },
          modal: {
            noResultsText: 'No results',
            resetButtonTitle: 'Clear query',
            displayDetails: 'Display detailed list',
            footer: { selectText: 'Select', navigateText: 'Navigate', closeText: 'Close' },
          },
        },
      },
    },
    socialLinks: [
      { icon: 'github', link: 'https://github.com/camthink-ai/neoruntime' },
    ],
  },
})
