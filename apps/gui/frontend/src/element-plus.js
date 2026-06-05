import {
  ElAvatar,
  ElButton,
  ElCheckbox,
  ElDialog,
  ElEmpty,
  ElForm,
  ElFormItem,
  ElIcon,
  ElInput,
  ElInputNumber,
  ElOption,
  ElSelect,
  ElTabPane,
  ElTable,
  ElTableColumn,
  ElTabs,
  ElTag,
  ElTooltip,
} from 'element-plus'
import { CopyDocument, Delete, Folder, FolderOpened, Plus, Refresh, VideoPause, VideoPlay } from '@element-plus/icons-vue'

import 'element-plus/theme-chalk/base.css'
import 'element-plus/theme-chalk/el-avatar.css'
import 'element-plus/theme-chalk/el-button.css'
import 'element-plus/theme-chalk/el-checkbox.css'
import 'element-plus/theme-chalk/el-dialog.css'
import 'element-plus/theme-chalk/el-empty.css'
import 'element-plus/theme-chalk/el-form.css'
import 'element-plus/theme-chalk/el-icon.css'
import 'element-plus/theme-chalk/el-input.css'
import 'element-plus/theme-chalk/el-input-number.css'
import 'element-plus/theme-chalk/el-option.css'
import 'element-plus/theme-chalk/el-popper.css'
import 'element-plus/theme-chalk/el-select.css'
import 'element-plus/theme-chalk/el-tabs.css'
import 'element-plus/theme-chalk/el-table.css'
import 'element-plus/theme-chalk/el-tag.css'
import 'element-plus/theme-chalk/el-tooltip.css'
import 'element-plus/theme-chalk/el-message.css'
import 'element-plus/theme-chalk/el-message-box.css'
import 'element-plus/theme-chalk/el-overlay.css'

const components = [
  ElAvatar,
  ElButton,
  ElCheckbox,
  ElDialog,
  ElEmpty,
  ElForm,
  ElFormItem,
  ElIcon,
  ElInput,
  ElInputNumber,
  ElOption,
  ElSelect,
  ElTabPane,
  ElTable,
  ElTableColumn,
  ElTabs,
  ElTag,
  ElTooltip,
]

const icons = {
  CopyDocument,
  Delete,
  Folder,
  FolderOpened,
  Plus,
  Refresh,
  VideoPause,
  VideoPlay,
}

export function installElementPlus(app) {
  components.forEach((component) => app.use(component))
  Object.entries(icons).forEach(([name, component]) => app.component(name, component))
}
