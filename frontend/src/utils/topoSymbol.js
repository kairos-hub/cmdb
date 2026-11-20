// 拓扑节点图标与配色：关联拓扑(RelationPanel) 与 业务拓扑树视图(TopologyView) 共用，
// 保证两处节点外观一致（淡色底 + 彩色图标；异常红描边 + 右上角红「!」角标）。
import { createApp, h } from 'vue'
import * as EPIcons from '@element-plus/icons-vue'
import {
  siMysql, siRedis, siApachekafka, siElasticsearch, siKubernetes,
  siNginx, siMinio, siPrometheus, siGrafana, siApache,
} from 'simple-icons'

// 有官方 logo 的模型 → 品牌矢量图标
const BRAND = {
  mysql: siMysql, redis: siRedis, kafka: siApachekafka, elastic: siElasticsearch,
  kubernetes: siKubernetes, nginx: siNginx, minio: siMinio,
  prometheus: siPrometheus, grafana: siGrafana, apache: siApache,
}

export const DANGER = '#e34948'
const NODE_BG = '#e8f0fe' // 淡蓝底（回退）
const NODE_ACCENT = '#2f66f0' // 非品牌图标主色（默认）
const CAT_PALETTE = ['#2f66f0', '#16a34a', '#d97706', '#7c3aed', '#db2777', '#0891b2', '#e34948', '#0ea5e9']

// 按 key(模型 obj_id / 名称) 稳定取一个分类色，同一模型在各处配色一致。
export function catColor(key) {
  let h = 0
  for (const c of String(key || '')) h = (h * 31 + c.charCodeAt(0)) >>> 0
  return CAT_PALETTE[h % CAT_PALETTE.length]
}

// 非品牌模型的几何图形图标（原创简单矢量；fg=图形色，bg=底色用于镂空）
function glyphFor(objId, fg, bg) {
  switch (objId) {
    case 'host': // 服务器：两条机架 + 指示灯
      return `<rect x="12" y="14" width="22" height="7.5" rx="2" fill="${fg}"/><rect x="12" y="24.5" width="22" height="7.5" rx="2" fill="${fg}"/><circle cx="16.5" cy="17.75" r="1.6" fill="${bg}"/><circle cx="16.5" cy="28.25" r="1.6" fill="${bg}"/>`
    case 'biz': // 业务：楼宇 + 窗户 + 门
      return `<rect x="14" y="12" width="18" height="22" rx="1.5" fill="${fg}"/><g fill="${bg}"><rect x="17" y="15" width="3" height="3"/><rect x="22" y="15" width="3" height="3"/><rect x="27" y="15" width="3" height="3"/><rect x="17" y="20" width="3" height="3"/><rect x="27" y="20" width="3" height="3"/><rect x="21" y="27" width="4" height="7"/></g>`
    case 'set': // 集群：2×2 网格
      return `<g fill="${fg}"><rect x="13" y="13" width="9" height="9" rx="1.5"/><rect x="24" y="13" width="9" height="9" rx="1.5"/><rect x="13" y="24" width="9" height="9" rx="1.5"/><rect x="24" y="24" width="9" height="9" rx="1.5"/></g>`
    case 'module': // 模块：方块
      return `<rect x="13" y="13" width="20" height="20" rx="3" fill="none" stroke="${fg}" stroke-width="2.4"/><rect x="19" y="19" width="8" height="8" rx="1.5" fill="${fg}"/>`
    default: // 通用：文档卡片
      return `<rect x="14" y="12" width="18" height="22" rx="2.5" fill="${fg}"/><g fill="${bg}"><rect x="17.5" y="17" width="11" height="2.3" rx="1"/><rect x="17.5" y="21.5" width="11" height="2.3" rx="1"/><rect x="17.5" y="26" width="7" height="2.3" rx="1"/></g>`
  }
}

// 取 Element Plus 图标的内部 SVG（与模型管理一致的那套图标），运行时渲染一次并缓存
const epCache = {}
function epInner(iconName) {
  if (!iconName) return null
  if (epCache[iconName] !== undefined) return epCache[iconName]
  const comp = EPIcons[iconName]
  if (!comp) {
    epCache[iconName] = null
    return null
  }
  try {
    const el = document.createElement('div')
    const app = createApp({ render: () => h(comp) })
    app.mount(el)
    const svg = el.querySelector('svg')
    const inner = svg ? svg.innerHTML : '' // 保留 currentColor，渲染时再按模型色替换
    app.unmount()
    epCache[iconName] = inner || null
    return epCache[iconName]
  } catch (e) {
    epCache[iconName] = null
    return null
  }
}

// 生成 ECharts 节点图标（image:// 内联 SVG）：统一「淡色底 + 彩色图标」。
// 品牌有 logo 用品牌 logo；否则用模型配置的 EP 图标；再兜底几何图形。
// color：按模型着色的主色（底色取其淡色调），不传则默认淡蓝。
// abnormal=true：红色描边 + 右上角红色警示点（感叹号），用于异常状态实例。
export function nodeSymbol(objId, iconName, color, abnormal) {
  const size = 46
  const accent = color || NODE_ACCENT
  const glyphBg = '#ffffff' // 几何图形镂空底色（白，保证在彩色淡底上可见）
  let bgRect
  if (abnormal) {
    bgRect = `<rect x="1.4" y="1.4" width="${size - 2.8}" height="${size - 2.8}" rx="11" fill="#fdecec" stroke="${DANGER}" stroke-width="2.4"/>`
  } else if (color) {
    bgRect = `<rect width="${size}" height="${size}" rx="12" fill="${accent}" fill-opacity="0.13" stroke="${accent}" stroke-opacity="0.5"/>`
  } else {
    bgRect = `<rect width="${size}" height="${size}" rx="12" fill="${NODE_BG}" stroke="#d6e2fb"/>`
  }
  const brand = BRAND[objId]
  let icon
  if (brand) {
    icon = `<g transform="translate(11,11)"><path d="${brand.path}" fill="#${brand.hex}"/></g>`
  } else {
    const ep = epInner(iconName)
    if (ep) {
      icon = `<g transform="translate(10,10) scale(0.0254)">${ep.replace(/currentColor/g, accent)}</g>`
    } else {
      icon = glyphFor(objId, accent, glyphBg)
    }
  }
  // 异常警示点（右上角），内含感叹号
  const badge = abnormal
    ? `<circle cx="${size - 7}" cy="7" r="6.5" fill="${DANGER}" stroke="#fff" stroke-width="1.4"/><rect x="${size - 7.9}" y="3.3" width="1.8" height="4.4" rx="0.9" fill="#fff"/><circle cx="${size - 7}" cy="9.7" r="1.05" fill="#fff"/>`
    : ''
  const svg = `<svg xmlns="http://www.w3.org/2000/svg" width="${size}" height="${size}" viewBox="0 0 ${size} ${size}">${bgRect}${icon}${badge}</svg>`
  return 'image://data:image/svg+xml,' + encodeURIComponent(svg)
}
