import { createApp } from 'vue'
import App from './App.vue'
import './style.css';
import { installElementPlus } from './element-plus'

const app = createApp(App)

installElementPlus(app)
app.mount('#app')
