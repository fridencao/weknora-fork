<template>
  <div class="chat-avatar" :class="`chat-avatar--${variant}`" aria-hidden="true">
    <img v-if="src" class="chat-avatar__img" :src="src" :alt="name" referrerpolicy="no-referrer" />
    <span v-else-if="letter" class="chat-avatar__letter">{{ letter }}</span>
    <t-icon v-else :name="variant === 'bot' ? 'robot' : 'user'" />
  </div>
</template>

<script setup lang="ts">
import { computed } from 'vue';

const props = withDefaults(defineProps<{
  /** user = 当前登录用户（无图回落首字母），bot = AI 助手 */
  variant?: 'user' | 'bot';
  /** 头像图片 URL（用户资料里的 avatar，可为空） */
  src?: string;
  /** 回落首字母取自该名称（username / email） */
  name?: string;
}>(), {
  variant: 'bot',
  src: '',
  name: '',
});

const letter = computed(() => (props.name || '').trim().charAt(0).toUpperCase());
</script>

<style lang="less" scoped>
.chat-avatar {
  flex-shrink: 0;
  display: inline-flex;
  align-items: center;
  justify-content: center;
  width: 32px;
  height: 32px;
  border-radius: var(--td-radius-circle);
  overflow: hidden;
  font-size: var(--app-text-xl);
  user-select: none;
}

.chat-avatar__img {
  width: 100%;
  height: 100%;
  object-fit: cover;
}

.chat-avatar__letter {
  font-size: var(--app-text-md);
  font-weight: 600;
}

.chat-avatar--user {
  background: var(--td-bg-color-component);
  color: var(--td-text-color-secondary);
}

.chat-avatar--bot {
  background: var(--td-brand-color-1);
  color: var(--td-brand-color);
}
</style>
