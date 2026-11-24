import fs from 'node:fs'
import path from 'node:path'

/**
 * 本地 SVG sprite 构建插件。
 *
 * 替代原第三方依赖 vite-auto-import-svg（该包已于 npm 官方源下架，仅剩占位版本 0.0.1-security）。
 * 行为对齐：把 src/plugin 与 src/assets/icons 下的 .svg 合成为 sprite，
 * 以 <symbol id="文件名"> 的形式内联进 index.html，组件通过
 * <use xlink:href="#id"> 引用（见 src/components/svgIcon/svgIcon.vue）。
 *
 * 设计要点：
 * - 仅构建期运行，不向页面注入任何脚本，也不发起网络请求；
 * - 去除 width/height，缺省补 viewBox，保证 <symbol> 随容器缩放；
 * - plugin 目录下的图标按目录名加前缀，避免与通用图标重名；
 * - 使用函数式 transformIndexHtml，兼容 rolldown-vite。
 */

const SVG_OPEN_TAG = /<svg([^>]*)>/
const WB_ATTR = /(width|height)="([^"]*)"/g
const VIEW_BOX = /viewBox="[^"]*"/
const LINE_BREAK = /[\r\n]/g

function extractInner(raw) {
  const openEnd = raw.search(SVG_OPEN_TAG) + raw.match(SVG_OPEN_TAG)[0].length
  const closeStart = raw.lastIndexOf('</svg>')
  return closeStart > openEnd ? raw.slice(openEnd, closeStart) : ''
}

function walkSvg(dir, prefix, out) {
  let entries
  try {
    entries = fs.readdirSync(dir, { withFileTypes: true })
  } catch {
    // 目录不存在时静默跳过，避免精简 checkout 中断构建
    return
  }

  for (const entry of entries) {
    if (entry.isDirectory()) {
      walkSvg(path.join(dir, entry.name), prefix, out)
      continue
    }
    if (!entry.name.toLowerCase().endsWith('.svg')) continue

    const raw = fs.readFileSync(path.join(dir, entry.name), 'utf-8').replace(LINE_BREAK, '')
    const openTag = raw.match(SVG_OPEN_TAG)
    if (!openTag) continue

    let width = 0
    let height = 0
    const attrs = openTag[1].replace(WB_ATTR, (_, name, value) => {
      if (name === 'width') width = value
      if (name === 'height') height = value
      return ''
    })
    const viewBox = VIEW_BOX.test(openTag[1]) ? '' : ` viewBox="0 0 ${width} ${height}"`
    const id = `${prefix}${entry.name.slice(0, -4)}`

    out.push(`<symbol id="${id}"${attrs}${viewBox}>${extractInner(raw)}</symbol>`)
  }
}

export function svgBuilder(dirs) {
  const list = Array.isArray(dirs) ? dirs : [dirs]

  return {
    name: 'jxc-svg-sprite',
    transformIndexHtml(html) {
      const symbols = []
      for (const dir of list) {
        const parts = String(dir).split('/')
        const isPluginDir =
          String(dir).includes('/src/plugin/') || String(dir).startsWith('./src/plugin')
        const prefix = isPluginDir && parts[3] ? `${parts[3]}-` : ''
        walkSvg(dir, prefix, symbols)
      }
      if (symbols.length === 0) return html

      const sprite =
        '<svg xmlns="http://www.w3.org/2000/svg" xmlns:xlink="http://www.w3.org/1999/xlink" ' +
        'style="position:absolute;width:0;height:0;overflow:hidden" aria-hidden="true">' +
        symbols.join('') +
        '</svg>'

      return html.includes('</body>')
        ? html.replace('</body>', `${sprite}</body>`)
        : `${html}${sprite}`
    }
  }
}
