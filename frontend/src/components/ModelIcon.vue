<template>
  <svg
    v-if="brand"
    class="model-icon-svg"
    :style="{ width: px, height: px }"
    viewBox="0 0 24 24"
    :fill="colored ? '#' + brand.hex : 'currentColor'"
    role="img"
    :aria-label="brand.title"
  >
    <path :d="brand.path" />
  </svg>
  <svg
    v-else-if="custom"
    class="model-icon-svg"
    :style="{ width: px, height: px, color: colored ? custom.color : 'currentColor' }"
    :viewBox="custom.viewBox"
    role="img"
    :aria-label="props.objId"
    v-html="custom.body"
  ></svg>
  <el-icon v-else :size="size === 'inherit' ? undefined : size">
    <component :is="fallback || 'Document'" />
  </el-icon>
</template>

<script setup>
import { computed } from 'vue'
import {
  siMysql, siRedis, siApachekafka, siElasticsearch, siKubernetes,
  siNginx, siMinio, siGrafana, siApache, siPrometheus,
} from 'simple-icons'

// obj_id → 官方品牌 logo（用于标识对应产品）；未命中则回退到 Element Plus 图标
const BRAND = {
  mysql: siMysql,
  redis: siRedis,
  kafka: siApachekafka,
  elastic: siElasticsearch,
  kubernetes: siKubernetes,
  nginx: siNginx,
  minio: siMinio,
  grafana: siGrafana,
  prometheus: siPrometheus,
  apache: siApache,
}

// obj_id → 自绘矢量图标（非品牌 logo，概念型）。currentColor 跟随 color。
const CUSTOM = {
  // WAF / 防火墙：红色砖墙（交错砌筑）
  waf: {
    viewBox: '0 0 48 48',
    color: '#DC2626',
    body: `
      <g fill="currentColor">
        <rect x="5" y="9" width="11.5" height="6" rx="1.4"/>
        <rect x="18.1" y="9" width="11.5" height="6" rx="1.4"/>
        <rect x="31.2" y="9" width="11.8" height="6" rx="1.4"/>
        <rect x="5" y="16.6" width="5" height="6" rx="1.4"/>
        <rect x="11.6" y="16.6" width="11.5" height="6" rx="1.4"/>
        <rect x="24.7" y="16.6" width="11.5" height="6" rx="1.4"/>
        <rect x="37.8" y="16.6" width="5.2" height="6" rx="1.4"/>
        <rect x="5" y="24.2" width="11.5" height="6" rx="1.4"/>
        <rect x="18.1" y="24.2" width="11.5" height="6" rx="1.4"/>
        <rect x="31.2" y="24.2" width="11.8" height="6" rx="1.4"/>
        <rect x="5" y="31.8" width="5" height="6" rx="1.4"/>
        <rect x="11.6" y="31.8" width="11.5" height="6" rx="1.4"/>
        <rect x="24.7" y="31.8" width="11.5" height="6" rx="1.4"/>
        <rect x="37.8" y="31.8" width="5.2" height="6" rx="1.4"/>
      </g>`,
  },
}

const props = defineProps({
  objId: { type: String, default: '' },
  fallback: { type: String, default: '' }, // EP 图标名（无品牌 logo 时使用）
  colored: { type: Boolean, default: true }, // true=品牌官方色，false=跟随 currentColor
  size: { type: [Number, String], default: 'inherit' },
})

const brand = computed(() => BRAND[props.objId] || null)
const custom = computed(() => (brand.value ? null : CUSTOM[props.objId] || null))
const px = computed(() => (props.size === 'inherit' ? '1em' : props.size + 'px'))
</script>

<style scoped>
.model-icon-svg {
  display: inline-block;
  vertical-align: -0.15em;
}
</style>
