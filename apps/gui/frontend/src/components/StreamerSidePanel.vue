<template>
  <aside class="detail-panel">
    <template v-if="streamer">
      <div class="detail-head">
        <el-avatar :size="52" :src="streamer.avatar || 'https://twitcasting.tv/img/user_default.png'" />
        <div>
          <h2>{{ streamer.nickname || streamer.screen_id }}</h2>
          <p>{{ streamer.screen_id }}</p>
        </div>
        <span class="status-pill large" :class="streamer.current_status">
          <span class="status-dot"></span>{{ statusText(streamer.current_status) }}
        </span>
      </div>

      <div class="detail-stats">
        <div v-for="item in statItems(streamer)" :key="item.key">
          <span>{{ item.label }}</span>
          <strong>{{ item.value }}</strong>
        </div>
      </div>

      <div class="detail-section policy-section">
        <div class="section-label">单主播策略</div>
        <div class="policy-control">
          <span>画质</span>
          <el-select :model-value="streamer.quality_mode || ''" size="small" @change="value => emit('update-quality', streamer, value)">
            <el-option v-for="item in qualityModeOptions" :key="item.value" :label="item.label" :value="item.value" />
          </el-select>
        </div>
        <div class="policy-control">
          <span>鉴权</span>
          <el-select :model-value="streamer.auth_mode || ''" size="small" @change="value => emit('update-auth', streamer, value)">
            <el-option v-for="item in authModeOptions" :key="item.value" :label="item.label" :value="item.value" />
          </el-select>
        </div>
        <div class="policy-control">
          <span>TG 推送</span>
          <el-switch
            :model-value="!!streamer.telegram_enabled"
            size="small"
            inline-prompt
            active-text="开"
            inactive-text="关"
            @change="value => emit('update-telegram', streamer, value)"
          />
        </div>
      </div>

      <div class="detail-section">
        <div class="section-label">当前状态</div>
        <p>{{ realtimeMessage(streamer) }}</p>
      </div>

      <div class="detail-section">
        <div class="section-label">诊断</div>
        <p>{{ diagnosticsMessage(streamer) }}</p>
      </div>

      <div class="detail-actions">
        <el-button size="small" @click="emit('open-link', streamer.screen_id)">打开主页</el-button>
        <el-button size="small" type="primary" plain @click="emit('show-history', streamer)">历史录播</el-button>
        <el-button size="small" type="danger" plain @click="emit('remove', streamer.screen_id)">删除主播</el-button>
        <el-button
          size="small"
          :type="streamer.current_status === 'idle' ? 'success' : 'warning'"
          :loading="isBusy(streamer.screen_id)"
          :disabled="bulkActive"
          @click="emit('toggle-monitoring', streamer)"
        >
          {{ streamer.current_status === 'idle' ? '开始监听' : '暂停监听' }}
        </el-button>
      </div>

      <div class="recent-log">
        <div class="section-label">最近日志</div>
        <div v-if="!streamer.recentLogs || streamer.recentLogs.length === 0" class="log-empty">暂无日志</div>
        <div v-for="(line, index) in streamer.recentLogs.slice(0, 6)" :key="index" class="log-line">{{ line }}</div>
      </div>
    </template>
    <div v-else class="detail-empty">
      <h2>选择一个主播</h2>
      <p>在左侧任务列表中选择主播后，这里会显示录制状态、诊断和最近日志。</p>
    </div>
  </aside>
</template>

<script setup>
defineProps({
  streamer: {
    type: Object,
    default: null
  },
  statusText: {
    type: Function,
    required: true
  },
  statItems: {
    type: Function,
    required: true
  },
  realtimeMessage: {
    type: Function,
    required: true
  },
  diagnosticsMessage: {
    type: Function,
    required: true
  },
  authModeOptions: {
    type: Array,
    required: true
  },
  qualityModeOptions: {
    type: Array,
    required: true
  },
  isBusy: {
    type: Function,
    required: true
  },
  bulkActive: {
    type: Boolean,
    default: false
  }
})

const emit = defineEmits([
  'open-link',
  'show-history',
  'remove',
  'toggle-monitoring',
  'update-auth',
  'update-quality',
  'update-telegram'
])
</script>

<style scoped>
.detail-panel {
  width: 360px;
  box-sizing: border-box;
  padding: 14px;
  display: flex;
  flex-direction: column;
  overflow: auto;
  min-width: 0;
  border: 1px solid #e5e7eb;
  border-radius: 8px;
  background: #ffffff;
}

.detail-head {
  display: flex;
  align-items: center;
  gap: 12px;
  min-width: 0;
}

.detail-head > div {
  flex: 1 1 auto;
  min-width: 0;
}

.detail-head h2 {
  margin: 0;
  color: #111827;
  font-size: 16px;
  line-height: 1.25;
  overflow: hidden;
  text-overflow: ellipsis;
  white-space: nowrap;
}

.detail-head p {
  margin: 3px 0 0;
  color: #64748b;
  font-size: 12px;
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

.status-pill.large {
  height: 28px;
  margin-left: auto;
}

.status-dot {
  width: 8px;
  height: 8px;
  border-radius: 50%;
  background: #94a3b8;
}

.status-pill.recording {
  color: #dc2626;
  background: #fff1f2;
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

.detail-stats {
  display: grid;
  grid-template-columns: repeat(3, minmax(0, 1fr));
  gap: 8px;
  margin-top: 14px;
}

.detail-stats div {
  border: 1px solid #edf2f7;
  border-radius: 8px;
  padding: 8px;
  min-width: 0;
}

.detail-stats span,
.section-label {
  color: #64748b;
  font-size: 12px;
  font-weight: 700;
}

.detail-stats strong {
  display: block;
  margin-top: 5px;
  color: #111827;
  font-size: 13px;
  overflow-wrap: anywhere;
}

.detail-section {
  margin-top: 14px;
}

.policy-section {
  border-top: 1px solid #eef2f7;
  padding-top: 12px;
}

.policy-control {
  display: grid;
  grid-template-columns: 46px minmax(0, 1fr);
  align-items: center;
  gap: 8px;
  margin-top: 8px;
}

.policy-control > span {
  color: #64748b;
  font-size: 12px;
  font-weight: 700;
}

.detail-section p {
  margin: 7px 0 0;
  color: #334155;
  font-size: 13px;
  line-height: 1.55;
  overflow-wrap: anywhere;
}

.detail-actions {
  display: grid;
  grid-template-columns: repeat(2, minmax(0, 1fr));
  gap: 8px;
  margin-top: 14px;
}

.detail-actions :deep(.el-button) {
  width: 100%;
  margin-left: 0;
}

.recent-log {
  margin-top: 16px;
  border-top: 1px solid #eef2f7;
  padding-top: 12px;
}

.log-line,
.log-empty {
  margin-top: 6px;
  color: #475569;
  font-family: Consolas, Monaco, monospace;
  font-size: 12px;
  line-height: 1.45;
  overflow-wrap: anywhere;
}

.detail-empty {
  color: #64748b;
  padding: 24px;
  text-align: center;
}

@media (max-width: 1100px) {
  .detail-panel {
    width: auto;
    max-height: 360px;
    min-height: 320px;
  }

  .status-pill.large {
    margin-left: 0;
  }
}
</style>
