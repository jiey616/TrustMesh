import { useState } from 'react'
import { useNavigate } from 'react-router-dom'
import { useQuery, useQueryClient } from '@tanstack/react-query'
import { Button, Dialog, Input, Switch, Toast } from 'antd-mobile'
import { PageScaffold } from '@/components/PageScaffold'
import { listOrganizations } from '@/api/orgs'
import { useAuthStore } from '@/stores/authStore'
import { runWorkspaceCalibration, useWorkspaceStore } from '@/stores/workspaceStore'
import { apiBaseLabel } from '@/lib/apiBaseLabel'
import { POLL_OPTIONS, useSettingsStore, type PollInterval } from '@/stores/settingsStore'

export function MePage() {
  const navigate = useNavigate()
  const queryClient = useQueryClient()
  const user = useAuthStore((s) => s.user)
  const logout = useAuthStore((s) => s.logout)
  const setApiBaseOverride = useAuthStore((s) => s.setApiBaseOverride)

  const activeOrgId = useWorkspaceStore((s) => s.activeOrgId)
  const personalOrgId = useWorkspaceStore((s) => s.personalOrgId)
  const { data: orgs = [] } = useQuery({ queryKey: ['orgs'], queryFn: listOrganizations })

  const showUnreadBadge = useSettingsStore((s) => s.showUnreadBadge)
  const setShowUnreadBadge = useSettingsStore((s) => s.setShowUnreadBadge)
  const pollInterval = useSettingsStore((s) => s.unreadPollInterval)
  const setUnreadPollInterval = useSettingsStore((s) => s.setUnreadPollInterval)

  const [editingBase, setEditingBase] = useState(false)
  const [baseInput, setBaseInput] = useState('')

  function switchOrg(orgId: string | null) {
    const { setPersonalOrgId, setActiveOrgId, setCalibrated } = useWorkspaceStore.getState()
    runWorkspaceCalibration(
      {
        setPersonalOrgId,
        setActiveOrgId,
        clearQueries: () => queryClient.removeQueries(),
        setCalibrated,
      },
      { personalOrgId: personalOrgId ?? orgId, activeOrgId: orgId },
    )
    Toast.show({ content: '已切换空间' })
  }

  async function confirmLogout() {
    const ok = await Dialog.confirm({ content: '确定退出登录？' })
    if (!ok) return
    logout()
    useWorkspaceStore.getState().reset()
    queryClient.removeQueries()
    navigate('/login', { replace: true })
  }

  function saveBase() {
    setApiBaseOverride(baseInput.trim() || null)
    setEditingBase(false)
    Toast.show({ content: '已保存，正在重新加载' })
    setTimeout(() => window.location.reload(), 400)
  }

  return (
    <PageScaffold title="我的">
      <section className="rounded-[var(--tm-radius-card)] border border-[var(--tm-line)] bg-white px-4 py-3">
        <p className="text-[16px] font-medium">{user?.name ?? '未登录'}</p>
        <p className="mt-[2px] text-[13px] text-[var(--tm-text-2)]">{user?.email ?? ''}</p>
      </section>

      <h2 className="mb-2 mt-5 text-[13px] text-[var(--tm-text-2)]">工作区</h2>
      <div className="overflow-hidden rounded-[var(--tm-radius-card)] border border-[var(--tm-line)] bg-white">
        {orgs.map((org) => {
          const active = org.id === (activeOrgId ?? personalOrgId)
          return (
            <button
              key={org.id}
              type="button"
              onClick={() => switchOrg(org.id)}
              className="flex w-full items-center justify-between border-b border-[var(--tm-line)] px-4 py-3 text-left last:border-b-0"
            >
              <span className="text-[15px]">{org.name}</span>
              <span
                className="text-[12px]"
                style={{ color: active ? 'var(--tm-brand)' : 'var(--tm-text-3)' }}
              >
                {active ? '当前' : org.kind === 'personal' ? '个人' : '企业'}
              </span>
            </button>
          )
        })}
        {orgs.length === 0 ? (
          <p className="px-4 py-3 text-[13px] text-[var(--tm-text-3)]">暂无可用空间</p>
        ) : null}
      </div>

      <h2 className="mb-2 mt-5 text-[13px] text-[var(--tm-text-2)]">通知</h2>
      <div className="overflow-hidden rounded-[var(--tm-radius-card)] border border-[var(--tm-line)] bg-white">
        <div className="flex items-center justify-between px-4 py-3">
          <span className="text-[15px]">底部导航显示未读红点</span>
          <Switch checked={showUnreadBadge} onChange={setShowUnreadBadge} />
        </div>
        <div className="border-t border-[var(--tm-line)] px-4 py-3">
          <p className="text-[15px]">未读刷新频率</p>
          <p className="mt-[2px] text-[12px] leading-relaxed text-[var(--tm-text-3)]">
            后端暂无推送通道，这里只控制前台轮询；真正的应用内推送要等 P3（原生壳 + 厂商通道）。
          </p>
          <div className="mt-2 flex flex-wrap gap-2">
            {POLL_OPTIONS.map((opt) => {
              const on = pollInterval === opt.value
              return (
                <button
                  key={opt.value}
                  type="button"
                  onClick={() => setUnreadPollInterval(opt.value as PollInterval)}
                  className="rounded-[8px] border px-3 py-[6px] text-[13px]"
                  style={{
                    color: on ? 'var(--tm-brand)' : 'var(--tm-text-2)',
                    borderColor: on ? 'var(--tm-brand)' : 'var(--tm-line)',
                    background: on ? 'var(--tm-brand-soft)' : '#fff',
                  }}
                >
                  {opt.label}
                </button>
              )
            })}
          </div>
        </div>
      </div>

      <h2 className="mb-2 mt-5 text-[13px] text-[var(--tm-text-2)]">服务地址</h2>
      <div className="overflow-hidden rounded-[var(--tm-radius-card)] border border-[var(--tm-line)] bg-white">
        {editingBase ? (
          <div className="p-3">
            <Input
              placeholder="https://example.com/api/v1"
              value={baseInput}
              onChange={setBaseInput}
            />
            <div className="mt-2 flex gap-2">
              <Button size="small" color="primary" onClick={saveBase}>
                保存
              </Button>
              <Button size="small" fill="none" onClick={() => setEditingBase(false)}>
                取消
              </Button>
            </div>
          </div>
        ) : (
          <button
            type="button"
            onClick={() => {
              setBaseInput(useAuthStore.getState().apiBaseOverride ?? '')
              setEditingBase(true)
            }}
            className="flex w-full items-center justify-between px-4 py-3 text-left"
          >
            <span className="text-[15px]">{apiBaseLabel()}</span>
            <span className="text-[12px] text-[var(--tm-text-3)]">修改</span>
          </button>
        )}
      </div>

      <Button
        block
        fill="none"
        color="danger"
        onClick={confirmLogout}
        style={{ marginTop: 28 }}
      >
        退出登录
      </Button>
    </PageScaffold>
  )
}
