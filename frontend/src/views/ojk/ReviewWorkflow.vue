<template>
  <!-- 深链入口：/platform/ojk/review/:id 直接全屏打开审核滑窗 -->
  <ReviewDrawer :run-id="runId" :visible="true" @close="router.push('/platform/ojk/rules')" />
</template>

<script setup lang="ts">
import { computed } from 'vue'
import { useRoute, useRouter } from 'vue-router'
import ReviewDrawer from './ReviewDrawer.vue'

const route = useRoute()
const router = useRouter()
const runId = computed(() => route.params.id as string)
</script>
