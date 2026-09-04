// Copies the Redoc standalone UMD bundle from node_modules into public/ so
// the static full-page reference (public/api-reference/index.html) can load
// it as a self-hosted asset with a stable relative path. Runs as part of
// `pnpm build` (after pnpm install).
import { copyFileSync, existsSync } from 'node:fs'
import { dirname, join } from 'node:path'
import { fileURLToPath } from 'node:url'

const here = dirname(fileURLToPath(import.meta.url))
const src = join(here, '..', 'node_modules', 'redoc', 'bundles', 'redoc.standalone.js')
const dst = join(here, '..', 'public', 'redoc.standalone.js')

if (!existsSync(src)) {
  console.error(`redoc standalone bundle not found at ${src} — run pnpm install first`)
  process.exit(1)
}
copyFileSync(src, dst)
console.log(`copied redoc.standalone.js (${(existsSync(dst) ? 'ok' : 'FAILED')}) -> public/`)
