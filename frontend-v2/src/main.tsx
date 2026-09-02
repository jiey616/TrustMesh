import React from 'react'
import ReactDOM from 'react-dom/client'
import '@ant-design/v5-patch-for-react-19'
import dayjs from 'dayjs'
import 'dayjs/locale/zh-cn'
import relativeTime from 'dayjs/plugin/relativeTime'
import { App } from './App'
import { AppErrorBoundary } from './components/AppErrorBoundary'
import './index.css'

dayjs.extend(relativeTime)
dayjs.locale('zh-cn')

ReactDOM.createRoot(document.getElementById('root')!).render(
  <React.StrictMode>
    <AppErrorBoundary>
      <App />
    </AppErrorBoundary>
  </React.StrictMode>,
)
