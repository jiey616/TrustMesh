import { StrictMode } from 'react'
import { createRoot } from 'react-dom/client'
import { unstableSetRender } from 'antd-mobile'
import { App } from './App'
import './styles/index.css'

// 🔴 antd-mobile v5 默认只兼容 React 16~18：React 19 调整了 react-dom 的导出方式，
// 组件库内部的 ReactDOM.render 会变成 undefined（表现为 Toast/Popup 直接报错）。
// 官方兼容垫片（≥5.40）在此注册渲染器；该接口会在下个 major 移除 ⇒ **集中改这一处**。
unstableSetRender((node, container) => {
  const el = container as HTMLElement & { _reactRoot?: ReturnType<typeof createRoot> }
  el._reactRoot ||= createRoot(container)
  const root = el._reactRoot
  root.render(node)
  return async () => {
    await new Promise((resolve) => setTimeout(resolve, 0))
    root.unmount()
  }
})

const container = document.getElementById('root')
if (!container) throw new Error('#root not found')

createRoot(container).render(
  <StrictMode>
    <App />
  </StrictMode>,
)
