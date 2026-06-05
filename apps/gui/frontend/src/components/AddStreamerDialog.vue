<template>
  <el-dialog :model-value="modelValue" title="添加主播" width="420px" @update:model-value="$emit('update:modelValue', $event)">
    <el-form :model="form" label-position="top">
      <el-form-item label="TwitCasting ID">
        <el-input v-model="form.id" placeholder="例如: mashiro_mayuki_" />
      </el-form-item>
      <el-form-item label="检查频率 (Cron)">
        <el-input v-model="form.schedule" placeholder="*/1 * * * *" />
      </el-form-item>
      <el-form-item label="单主播画质策略">
        <el-select v-model="form.qualityMode" style="width: 100%">
          <el-option label="跟随全局" value="" />
          <el-option label="稳定中档" value="stable" />
          <el-option label="自动尝试高/中/低" value="auto" />
          <el-option label="保持原始流地址" value="original" />
        </el-select>
      </el-form-item>
      <el-form-item label="单主播封装格式">
        <el-select v-model="form.containerMode" style="width: 100%">
          <el-option label="跟随全局" value="" />
          <el-option label="MKV 稳定默认" value="mkv" />
          <el-option label="TS 兼容模式" value="ts" />
          <el-option label="MP4 后处理转封装" value="mp4" />
        </el-select>
      </el-form-item>
      <el-form-item label="单主播鉴权策略">
        <el-select v-model="form.authMode" style="width: 100%">
          <el-option label="跟随全局" value="" />
          <el-option label="强制 Cookie" value="cookie" />
          <el-option label="禁用 Cookie" value="no_cookie" />
        </el-select>
      </el-form-item>
    </el-form>
    <template #footer>
      <el-button @click="$emit('update:modelValue', false)">取消</el-button>
      <el-button type="primary" @click="submit">确认添加</el-button>
    </template>
  </el-dialog>
</template>

<script setup>
import { reactive } from 'vue'

defineProps({
  modelValue: {
    type: Boolean,
    default: false,
  },
})

const emit = defineEmits(['update:modelValue', 'add'])

const form = reactive({
  id: '',
  schedule: '*/1 * * * *',
  qualityMode: '',
  containerMode: '',
  authMode: '',
})

const submit = () => {
  if (!form.id) return
  emit('add', { ...form })
  form.id = ''
  form.qualityMode = ''
  form.containerMode = ''
  form.authMode = ''
}
</script>
