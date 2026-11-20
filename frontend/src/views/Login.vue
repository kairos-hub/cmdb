<template>
  <div class="login-wrap">
    <div class="glow glow-1"></div>
    <div class="glow glow-2"></div>

    <div class="login-card">
      <div class="brand">
        <span class="brand-badge">C</span>
        <div class="brand-text">
          <b>天与CMDB</b>
          <i>配置管理平台</i>
        </div>
      </div>

      <div class="welcome">欢迎登录</div>

      <el-form ref="formRef" :model="form" :rules="rules" @submit.prevent="onSubmit">
        <el-form-item prop="username">
          <el-input v-model="form.username" placeholder="用户名" size="large" :prefix-icon="User" />
        </el-form-item>
        <el-form-item prop="password">
          <el-input v-model="form.password" type="password" placeholder="密码" size="large"
            :prefix-icon="Lock" show-password @keyup.enter="onSubmit" />
        </el-form-item>
        <el-form-item>
          <el-button type="primary" size="large" class="btn" :loading="loading" @click="onSubmit">
            登 录
          </el-button>
        </el-form-item>
      </el-form>

      <div class="foot">天与CMDB</div>
    </div>
  </div>
</template>

<script setup>
import { ref } from 'vue'
import { useRouter, useRoute } from 'vue-router'
import { ElMessage } from 'element-plus'
import { User, Lock } from '@element-plus/icons-vue'
import { useUserStore } from '@/store/user'

const router = useRouter()
const route = useRoute()
const userStore = useUserStore()

const formRef = ref()
const loading = ref(false)
const form = ref({ username: '', password: '' })
const rules = {
  username: [{ required: true, message: '请输入用户名', trigger: 'blur' }],
  password: [{ required: true, message: '请输入密码', trigger: 'blur' }],
}

function onSubmit() {
  formRef.value.validate(async (valid) => {
    if (!valid) return
    loading.value = true
    try {
      await userStore.login(form.value.username, form.value.password)
      ElMessage.success('登录成功')
      router.push(route.query.redirect || '/')
    } catch {
      // 错误提示已由 axios 响应拦截器统一处理
    } finally {
      loading.value = false
    }
  })
}
</script>

<style scoped>
.login-wrap {
  height: 100%;
  display: flex;
  align-items: center;
  justify-content: center;
  position: relative;
  overflow: hidden;
  background:
    radial-gradient(900px 500px at 18% 20%, rgba(47, 102, 240, 0.35), transparent 60%),
    radial-gradient(800px 520px at 85% 82%, rgba(37, 84, 204, 0.3), transparent 60%),
    linear-gradient(135deg, #141c2b 0%, #1b2435 55%, #223052 100%);
}
.glow {
  position: absolute;
  border-radius: 50%;
  filter: blur(80px);
  pointer-events: none;
}
.glow-1 {
  width: 420px;
  height: 420px;
  top: -120px;
  left: -80px;
  background: rgba(79, 123, 245, 0.35);
}
.glow-2 {
  width: 480px;
  height: 480px;
  right: -120px;
  bottom: -140px;
  background: rgba(47, 102, 240, 0.28);
}

.login-card {
  position: relative;
  z-index: 1;
  width: 400px;
  padding: 36px 36px 22px;
  background: rgba(255, 255, 255, 0.97);
  border: 1px solid rgba(255, 255, 255, 0.5);
  border-radius: 18px;
  box-shadow: 0 24px 70px rgba(10, 18, 34, 0.45);
  backdrop-filter: blur(6px);
}
.brand {
  display: flex;
  align-items: center;
  justify-content: center;
  gap: 12px;
  margin-bottom: 26px;
}
.brand-badge {
  display: inline-flex;
  align-items: center;
  justify-content: center;
  width: 46px;
  height: 46px;
  border-radius: 12px;
  background: linear-gradient(135deg, var(--brand), #4f7bf5);
  box-shadow: 0 6px 16px rgba(47, 102, 240, 0.35);
  color: #fff;
  font-size: 24px;
  font-weight: 700;
  flex-shrink: 0;
}
.brand-text { display: flex; flex-direction: column; align-items: center; line-height: 1.25; }
.brand-text b { font-size: 19px; font-weight: 700; color: var(--text-primary); letter-spacing: 0.2px; }
.brand-text i { font-size: 12.5px; font-style: normal; color: var(--text-muted); }

.welcome {
  font-size: 22px;
  font-weight: 700;
  color: var(--text-primary);
  letter-spacing: -0.3px;
  text-align: center;
  margin-bottom: 22px;
}
.btn {
  width: 100%;
  letter-spacing: 4px;
  font-weight: 600;
}
.foot {
  margin-top: 8px;
  text-align: center;
  font-size: 12px;
  color: var(--text-muted);
}
</style>
