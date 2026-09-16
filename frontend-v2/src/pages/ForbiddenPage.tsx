import { Button, Result } from 'antd'
import { useNavigate } from 'react-router-dom'

/**
 * 越权访问的统一落地页（设计文档 §7：越权统一 403 页，非白屏）。
 *
 * 触发场景：当前角色不具备该页面/菜单所需的权限点
 * （身份层不匹配——平台管理员访问业务页——不落这里，直接送回平台首页）。
 */
export function ForbiddenPage() {
  const navigate = useNavigate()

  return (
    <Result
      status="403"
      title="403"
      subTitle="抱歉，你没有访问该页面的权限。如需要请联系企业管理员调整你的角色。"
      extra={
        <Button type="primary" onClick={() => navigate('/dashboard')}>
          返回仪表盘
        </Button>
      }
    />
  )
}