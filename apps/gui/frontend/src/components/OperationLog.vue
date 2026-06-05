<template>
  <div class="ops-panel">
    <div class="ops-header">
      <span>运行日志</span>
      <div class="ops-tools">
        <button
          v-for="item in filters"
          :key="item.value"
          class="filter-btn"
          :class="{ active: activeFilter === item.value }"
          type="button"
          @click="activeFilter = item.value"
        >
          {{ item.label }}
        </button>
        <el-button link type="primary" @click="$emit('clear')">清空</el-button>
      </div>
    </div>
    <div class="ops-body">
      <div v-if="filteredLogs.length === 0" class="ops-empty">暂无日志</div>
      <div v-for="(line, idx) in filteredLogs" :key="idx" class="ops-line" :class="levelOf(line)">
        {{ line }}
      </div>
    </div>
  </div>
</template>

<script setup>
import { computed, ref } from 'vue'

const props = defineProps({
  logs: {
    type: Array,
    default: () => [],
  },
})

defineEmits(['clear'])

const activeFilter = ref('all')
const filters = [
  { value: 'all', label: '全部' },
  { value: 'error', label: '错误' },
  { value: 'recording', label: '录制' },
  { value: 'system', label: '系统' },
]

const levelOf = (line) => {
  const lower = String(line || '').toLowerCase()
  if (lower.includes('失败') || lower.includes('错误') || lower.includes('error') || lower.includes('failed')) return 'error'
  if (lower.includes('录制') || lower.includes('recording') || lower.includes('开播') || lower.includes('写入')) return 'recording'
  return 'system'
}

const filteredLogs = computed(() => {
  if (activeFilter.value === 'all') return props.logs
  return props.logs.filter(line => levelOf(line) === activeFilter.value)
})
</script>

<style scoped>
.ops-panel {
  flex: 0 0 auto;
  margin-top: 0;
  border: 1px solid #e5e7eb;
  border-radius: 8px;
  background: #ffffff;
}

.ops-header {
  display: flex;
  align-items: center;
  justify-content: space-between;
  gap: 10px;
  padding: 7px 12px;
  border-bottom: 1px solid #eef2f7;
  color: #334155;
  font-size: 12px;
  font-weight: 700;
}

.ops-tools {
  display: flex;
  align-items: center;
  gap: 6px;
}

.filter-btn {
  height: 24px;
  padding: 0 8px;
  border: 1px solid transparent;
  border-radius: 6px;
  background: transparent;
  color: #64748b;
  cursor: pointer;
  font-size: 12px;
}

.filter-btn.active {
  border-color: #bfdbfe;
  background: #eff6ff;
  color: #2563eb;
  font-weight: 700;
}

.ops-body {
  height: 84px;
  overflow-y: auto;
  padding: 7px 12px;
  color: #475569;
  font-family: Consolas, 'Courier New', monospace;
  font-size: 12px;
  line-height: 1.5;
}

.ops-line {
  white-space: pre-wrap;
  word-break: break-word;
}

.ops-line.error {
  color: #b91c1c;
}

.ops-line.recording {
  color: #047857;
}

.ops-empty {
  color: #94a3b8;
}
</style>
