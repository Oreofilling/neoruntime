import { defineConfig } from 'vitepress'
import projectSidebar from './generated/project-sidebar.json'

// Base path for GitHub Pages project sites (<user>.github.io/<repo>).
// If you deploy to a custom domain or user-site, change this to '/'.
const base = '/neoruntime/'
const specVersion = 'v1.0.2'

const guideEn = [
  { text: 'Introduction', link: '/introduction' },
  { text: 'Quickstart', link: '/quickstart' },
  { text: 'Authentication', link: '/authentication' },
  { text: 'Errors & Status Codes', link: '/errors' },
  { text: 'Keeping Docs in Sync', link: '/update-mechanism' },
]

const guideZh = [
  { text: '简介', link: '/zh/introduction' },
  { text: '快速开始', link: '/zh/quickstart' },
  { text: '认证', link: '/zh/authentication' },
  { text: '错误与状态码', link: '/zh/errors' },
  { text: '文档同步机制', link: '/zh/update-mechanism' },
]

const apiReferenceEn = { text: 'API Reference (full page)', link: '/api-reference/' }
const apiReferenceZh = { text: 'API 参考（全屏页）', link: '/api-reference/zh/' }

type SidebarGroup = { text: string; collapsed?: boolean; items?: { text: string; link: string }[]; link?: string }
const wikiEn = projectSidebar.en as SidebarGroup[]
const wikiZh = projectSidebar.zh as SidebarGroup[]

export default defineConfig({
  base,
  lastUpdated: true,
  srcExclude: ['**/README.md'], // repo-level README, not a docs page
  // Wiki pages are synced from docs/** at build time; links that point at
  // repository-only files are rewritten to GitHub URLs by
  // scripts/sync_project_docs.py. Anything left pointing at non-site paths
  // (e.g. upstream tool docs referenced by absolute path) is tolerated
  // rather than failing the build.
  ignoreDeadLinks: [/^\/docs\//],
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
      title: 'NeoRuntime Docs',
      description: 'NeoRuntime documentation — project wiki plus the bilingual Web API reference for the edge AI computing platform.',
      themeConfig: {
        nav: [
          { text: 'Web API', items: [...guideEn, apiReferenceEn] },
          { text: 'Project Wiki', items: wikiEn.flatMap((s) =>
            'link' in s ? [s] : (s.items ?? []).slice(0, 2).map((i) => ({ text: `${s.text} / ${i.text}`, link: i.link })),
          ).slice(0, 8) },
          { text: 'API Reference', link: '/api-reference/' },
          { text: `Spec ${specVersion}`, items: [
            { text: 'swagger.yaml (source)', link: 'https://github.com/camthink-ai/neoruntime/blob/main/docs/api/swagger.yaml' },
            { text: 'swagger.json (site)', link: '/swagger.json' },
          ] },
        ],
        sidebar: [
          { text: 'NeoRuntime', items: [{ text: 'Home', link: '/' }] },
          { text: 'Web API', collapsed: false, items: [...guideEn, apiReferenceEn] },
          ...wikiEn,
        ],
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
      title: 'NeoRuntime 文档',
      description: 'NeoRuntime 文档 —— 边缘 AI 计算平台的项目 wiki 与双语 Web API 参考。',
      themeConfig: {
        nav: [
          { text: 'Web API', items: [...guideZh, apiReferenceZh] },
          { text: '项目文档', items: wikiZh.flatMap((s) =>
            'link' in s ? [s] : (s.items ?? []).slice(0, 2).map((i) => ({ text: `${s.text} · ${i.text}`, link: i.link })),
          ).slice(0, 8) },
          { text: 'API 参考', link: '/api-reference/zh/' },
          { text: `Spec ${specVersion}`, items: [
            { text: 'swagger.yaml（源文件）', link: 'https://github.com/camthink-ai/neoruntime/blob/main/docs/api/swagger.yaml' },
            { text: 'swagger.json（本站）', link: '/swagger.zh.json' },
          ] },
        ],
        sidebar: [
          { text: 'NeoRuntime', items: [{ text: '首页', link: '/zh/' }] },
          { text: 'Web API', collapsed: false, items: [...guideZh, apiReferenceZh] },
          ...wikiZh,
        ],
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
