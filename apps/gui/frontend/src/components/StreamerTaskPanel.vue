<template>
  <section class="task-panel">
    <div class="task-toolbar">
      <el-input :model-value="searchQuery" placeholder="搜索主播 ID" clearable @update:model-value="value => emit('update:searchQuery', value)" />
      <el-button icon="Refresh" :loading="refreshLoading" @click="emit('refresh')">刷新</el-button>
    </div>

    <div v-if="bulkState.active || bulkState.done > 0" class="queue-banner" :class="{ done: !bulkState.active }">
      <div>
        <strong>{{ bulkState.title }}</strong>
        <span>{{ bulkState.message }}</span>
      </div>
      <div class="queue-progress">{{ bulkState.done }} / {{ bulkState.total }}</div>
    </div>

    <div class="task-list">
      <button
        v-for="streamer in streamers"
        :key="streamer.screen_id"
        class="streamer-row"
        :class="{
          active: selectedId === streamer.screen_id,
          recording: streamer.current_status === 'recording',
          'has-error': streamer.lastError || streamer.consecutiveFailures > 0
        }"
        type="button"
        @click="emit('select', streamer)"
      >
        <el-avatar :size="42" :src="streamer.avatar || 'https://twitcasting.tv/img/user_default.png'" @error="emit('avatar-error', streamer)" />
        <div class="streamer-main">
          <div class="streamer-title-line">
            <span class="streamer-id">{{ streamer.screen_id }}</span>
            <span class="status-pill" :class="streamer.current_status">
              <span class="status-dot"></span>{{ statusText(streamer.current_status) }}
            </span>
          </div>
          <div class="streamer-subtitle">{{ streamer.nickname || '未获取昵称' }}</div>
          <div class="streamer-message">{{ realtimeMessage(streamer) }}</div>
          <div class="streamer-meta">
            <span>更新 {{ shortTime(streamer.lastCheckAt) }}</span>
            <span v-if="streamer.consecutiveFailures > 0" class="danger-meta">异常 {{ streamer.consecutiveFailures }} 次</span>
            <span v-else-if="streamer.lastSuccessAt">成功 {{ shortTime(streamer.lastSuccessAt) }}</span>
          </div>
        </div>
        <div class="row-actions" @click.stop>
          <el-button
            size="small"
            :type="streamer.current_status === 'idle' ? 'success' : 'warning'"
            :loading="isBusy(streamer.screen_id)"
            :disabled="bulkState.active"
            @click="emit('toggle-monitoring', streamer)"
          >
            {{ streamer.current_status === 'idle' ? '开始' : '暂停' }}
          </el-button>
          <el-button size="small" @click="emit('open-details', streamer)">详情</el-button>
        </div>
      </button>
      <div v-if="streamers.length === 0" class="empty-state">暂无匹配主播</div>
    </div>
  </section>
</template>

<script setup>
defineProps({
  streamers: {
    type: Array,
    required: true
  },
  selectedId: {
    type: String,
    default: ''
  },
  searchQuery: {
    type: String,
    default: ''
  },
  refreshLoading: {
    type: Boolean,
    default: false
  },
  bulkState: {
    type: Object,
    required: true
  },
  statusText: {
    type: Function,
    required: true
  },
  realtimeMessage: {
    type: Function,
    required: true
  },
  shortTime: {
    type: Function,
    required: true
  },
  isBusy: {
    type: Function,
    required: true
  }
})

const emit = defineEmits([
  'update:searchQuery',
  'refresh',
  'select',
  'toggle-monitoring',
  'open-details',
  'avatar-error'
])
</script>

<style scoped>
.task-panel {
  border: 1px solid #e5e7eb;
  border-radius: 8px;
  background: #ffffff;
  min-height: 0;
  display: flex;
  flex-direction: column;
  flex: 1;
  min-width: 0;
}

.task-toolbar {
  display: grid;
  grid-template-columns: minmax(0, 1fr) auto;
  gap: 8px;
  padding: 12px;
  border-bottom: 1px solid #eef2f7;
}

.queue-banner {
  display: flex;
  align-items: center;
  justify-content: space-between;
  gap: 12px;
  padding: 8px 12px;
  border-bottom: 1px solid #dbeafe;
  background: #eff6ff;
  color: #1d4ed8;
  font-size: 12px;
}

.queue-banner.done {
  border-bottom-color: #dcfce7;
  background: #f0fdf4;
  color: #047857;
}

.queue-banner strong,
.queue-banner span {
  display: block;
}

.queue-banner span {
  margin-top: 2px;
  color: inherit;
  opacity: 0.82;
}

.queue-progress {
  flex: 0 0 auto;
  border-radius: 999px;
  background: rgba(255, 255, 255, 0.8);
  padding: 4px 8px;
  font-weight: 800;
}

.task-list {
  flex: 1 1 auto;
  min-height: 0;
  overflow-y: auto;
  padding: 8px;
}

.streamer-row {
  width: 100%;
  min-height: 78px;
  display: grid;
  grid-template-columns: 42px minmax(0, 1fr) auto;
  gap: 12px;
  align-items: center;
  border: 1px solid transparent;
  border-radius: 8px;
  background: transparent;
  padding: 10px;
  text-align: left;
  cursor: pointer;
  color: inherit;
}

.streamer-row:hover,
.streamer-row.active {
  background: #f8fafc;
  border-color: #dbe3ed;
}

.streamer-row.recording {
  border-color: #fecaca;
  background: #fff7f7;
}

.streamer-row.has-error {
  border-left-color: #f59e0b;
  border-left-width: 4px;
}

.streamer-main {
  min-width: 0;
}

.streamer-title-line {
  display: flex;
  align-items: center;
  gap: 8px;
  min-width: 0;
}

.streamer-id {
  min-width: 0;
  overflow: hidden;
  text-overflow: ellipsis;
  white-space: nowrap;
  color: #111827;
  font-size: 14px;
  font-weight: 800;
}

.streamer-subtitle {
  margin-top: 3px;
  color: #64748b;
  font-size: 12px;
}

.streamer-message {
  margin-top: 8px;
  color: #475569;
  font-size: 12px;
  line-height: 1.45;
  overflow: hidden;
  text-overflow: ellipsis;
  white-space: nowrap;
}

.streamer-meta {
  display: flex;
  flex-wrap: wrap;
  gap: 8px;
  margin-top: 5px;
  color: #94a3b8;
  font-size: 11px;
}

.streamer-meta .danger-meta {
  color: #b45309;
  font-weight: 700;
}

.row-actions {
  display: flex;
  gap: 6px;
  align-items: center;
  flex: 0 0 auto;
}

.status-pill {
  display: inline-flex;
  align-items: center;
  gap: 5px;
  flex: 0 0 auto;
  height: 24px;
  padding: 0 8px;
  border-radius: 999px;
  font-size: 12px;
  font-weight: 700;
  color: #475569;
  background: #f1f5f9;
  white-space: nowrap;
}

.status-dot {
  width: 7px;
  height: 7px;
  border-radius: 999px;
  background: #94a3b8;
}

.status-pill.recording {
  color: #b91c1c;
  background: #fef2f2;
}

.status-pill.recording .status-dot {
  background: #ef4444;
}

.status-pill.monitoring {
  color: #047857;
  background: #ecfdf5;
}

.status-pill.monitoring .status-dot {
  background: #10b981;
}

.status-pill.error {
  color: #b45309;
  background: #fffbeb;
}

.status-pill.error .status-dot {
  background: #f59e0b;
}

.status-pill.restricted {
  color: #7c3aed;
  background: #f5f3ff;
}

.status-pill.restricted .status-dot {
  background: #8b5cf6;
}

.empty-state {
  color: #64748b;
  padding: 24px;
  text-align: center;
}
</style>
