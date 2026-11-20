<template>
  <router-view />
</template>

<script setup>
import { onMounted } from 'vue'
import { useUserStore } from '@/store/user'

const userStore = useUserStore()

// 启动时若存在令牌，刷新当前用户信息；失效则清除登录态
onMounted(() => {
  if (userStore.token) {
    userStore.fetchMe().catch(() => userStore.logout())
  }
})
</script>

<style>
html,
body,
#app {
  height: 100%;
  margin: 0;
  font-family: -apple-system, BlinkMacSystemFont, 'Segoe UI', Roboto, 'Helvetica Neue',
    Arial, 'PingFang SC', 'Microsoft YaHei', sans-serif;
}
</style>
