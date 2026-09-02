import { useState } from 'react'
import { useSearchParams, useNavigate } from 'react-router-dom'
import { Row, Col, Card, Tag, Typography, Input, Empty, Skeleton, Button } from 'antd'
import { ShopOutlined, SearchOutlined, ReloadOutlined } from '@ant-design/icons'
import { motion } from 'framer-motion'
import { useMarketDepts, useMarketRoles } from '@/hooks/useMarket'
import { useDebounce } from '@/hooks/useDebounce'
import { PageHeader } from '@/components/shared/PageHeader'
import type { MarketRoleListItem } from '@/types'

const { Text } = Typography

const DEPT_GRADIENTS = [
  'linear-gradient(135deg,#6d5ff5,#6366f1)',
  'linear-gradient(135deg,#3b82f6,#22d3ee)',
  'linear-gradient(135deg,#f43f5e,#f59e0b)',
  'linear-gradient(135deg,#10b981,#22d3ee)',
]

function RoleCard({ role, index }: { role: MarketRoleListItem; index: number }) {
  const navigate = useNavigate()
  const gradient = DEPT_GRADIENTS[index % DEPT_GRADIENTS.length]

  return (
    <motion.div
      initial={{ opacity: 0, y: 12 }}
      animate={{ opacity: 1, y: 0 }}
      transition={{ duration: 0.3, delay: index * 0.03 }}
    >
      <Card
        hoverable
        onClick={() => navigate(`/market/roles/${role.id}`)}
        styles={{ body: { padding: 16 } }}
        style={{ height: '100%', cursor: 'pointer' }}
      >
        <div style={{ display: 'flex', alignItems: 'flex-start', gap: 12, marginBottom: 8 }}>
          <div
            style={{
              width: 40,
              height: 40,
              borderRadius: 12,
              background: gradient,
              display: 'flex',
              alignItems: 'center',
              justifyContent: 'center',
              color: '#fff',
              fontWeight: 700,
              fontSize: 17,
              flexShrink: 0,
              boxShadow: `0 4px 16px ${gradient.includes('a855f7') ? 'rgba(109,95,245,0.35)' : 'rgba(59,130,246,0.35)'}`,
            }}
          >
            {role.name.charAt(0).toUpperCase()}
          </div>
          <div style={{ minWidth: 0, flex: 1 }}>
            <Text strong style={{ fontSize: 15, display: 'block', whiteSpace: 'nowrap', overflow: 'hidden', textOverflow: 'ellipsis' }}>
              {role.name}
            </Text>
            <Tag style={{ marginTop: 4, fontSize: 11 }}>{role.dept_name}</Tag>
          </div>
        </div>
        <Text
          type="secondary"
          style={{
            fontSize: 13,
            display: '-webkit-box',
            WebkitLineClamp: 2,
            WebkitBoxOrient: 'vertical',
            overflow: 'hidden',
          }}
        >
          {role.description || '暂无描述'}
        </Text>
      </Card>
    </motion.div>
  )
}

export function MarketPage() {
  const [searchParams, setSearchParams] = useSearchParams()
  const [inputValue, setInputValue] = useState(searchParams.get('q') ?? '')
  const debouncedQuery = useDebounce(inputValue, 300)

  const activeDept = searchParams.get('dept') ?? ''

  const { data: depts, isLoading: deptsLoading, isError } = useMarketDepts()
  const { data: roles, isLoading: rolesLoading, refetch } = useMarketRoles({
    dept: activeDept || undefined,
    q: debouncedQuery || undefined,
  })

  function handleDeptSelect(deptId: string) {
    setSearchParams((prev) => {
      const next = new URLSearchParams(prev)
      if (deptId) {
        next.set('dept', deptId)
      } else {
        next.delete('dept')
      }
      return next
    })
  }

  function handleSearch(value: string) {
    setInputValue(value)
    setSearchParams((prev) => {
      const next = new URLSearchParams(prev)
      if (value) {
        next.set('q', value)
      } else {
        next.delete('q')
      }
      return next
    })
  }

  const totalCount = roles?.length ?? 0
  const activeDeptName = depts?.find((d) => d.id === activeDept)?.name

  return (
    <div>
      <PageHeader
        title="岗位市场"
        icon={<ShopOutlined />}
        subtitle="预置 AI 数字员工角色模板，下载到本地即可接入团队"
        actions={
          <Input
            prefix={<SearchOutlined style={{ color: '#8b8f9e' }} />}
            placeholder="搜索角色名称或描述..."
            value={inputValue}
            onChange={(e) => handleSearch(e.target.value)}
            style={{ width: 260 }}
            allowClear
          />
        }
      />

      <div style={{ display: 'flex', gap: 16, alignItems: 'flex-start' }}>
        {/* Left: department nav */}
        <div
          style={{
            width: 180,
            flexShrink: 0,
            borderRadius: 12,
            border: '1px solid rgba(255,255,255,0.07)',
            background: 'rgba(255,255,255,0.02)',
            padding: 8,
            maxHeight: 'calc(100vh - 260px)',
            overflow: 'auto',
          }}
        >
          <div
            onClick={() => handleDeptSelect('')}
            style={{
              display: 'flex',
              alignItems: 'center',
              justifyContent: 'space-between',
              padding: '8px 12px',
              borderRadius: 8,
              cursor: 'pointer',
              marginBottom: 2,
              transition: 'all 0.2s ease',
              background: !activeDept ? 'linear-gradient(90deg, rgba(109,95,245,0.18), rgba(99,102,241,0.08))' : 'transparent',
              border: !activeDept ? '1px solid rgba(109,95,245,0.3)' : '1px solid transparent',
            }}
          >
            <span style={{ fontWeight: activeDept ? 400 : 600, color: !activeDept ? '#8b7ff8' : '#c8ccd8' }}>全部</span>
            <Text type="secondary" style={{ fontSize: 12 }}>
              {depts?.reduce((s, d) => s + d.count, 0) ?? 0}
            </Text>
          </div>
          {deptsLoading ? (
            <Skeleton active paragraph={{ rows: 6 }} title={false} />
          ) : (
            (depts || []).map((dept) => (
              <div
                key={dept.id}
                onClick={() => handleDeptSelect(dept.id)}
                style={{
                  display: 'flex',
                  alignItems: 'center',
                  justifyContent: 'space-between',
                  padding: '8px 12px',
                  borderRadius: 8,
                  cursor: 'pointer',
                  marginBottom: 2,
                  transition: 'all 0.2s ease',
                  background: activeDept === dept.id ? 'linear-gradient(90deg, rgba(109,95,245,0.18), rgba(99,102,241,0.08))' : 'transparent',
                  border: activeDept === dept.id ? '1px solid rgba(109,95,245,0.3)' : '1px solid transparent',
                }}
              >
                <span style={{ fontSize: 13, overflow: 'hidden', textOverflow: 'ellipsis', whiteSpace: 'nowrap', color: activeDept === dept.id ? '#8b7ff8' : '#c8ccd8' }}>
                  {dept.name}
                </span>
                <Text type="secondary" style={{ fontSize: 12 }}>{dept.count}</Text>
              </div>
            ))
          )}
        </div>

        {/* Right: role list */}
        <div style={{ flex: 1, minWidth: 0 }}>
          {/* status bar */}
          <div style={{ fontSize: 13, color: '#8b8f9e', marginBottom: 12 }}>
            {activeDeptName ? (
              <span>
                <span style={{ color: '#f4f4f8', fontWeight: 500 }}>{activeDeptName}</span>
                {debouncedQuery && (
                  <>
                    {' · 搜索 "'}
                    <span style={{ color: '#f4f4f8', fontWeight: 500 }}>{debouncedQuery}</span>
                    {'"'}
                  </>
                )}
              </span>
            ) : debouncedQuery ? (
              <span>
                搜索 "<span style={{ color: '#f4f4f8', fontWeight: 500 }}>{debouncedQuery}</span>"
              </span>
            ) : (
              <span>全部角色</span>
            )}
            {!rolesLoading && <span style={{ marginLeft: 8 }}>· {totalCount} 个结果</span>}
          </div>

          {rolesLoading ? (
            <Row gutter={[16, 16]}>
              {Array.from({ length: 6 }).map((_, i) => (
                <Col xs={24} sm={12} lg={8} key={i}>
                  <Card loading />
                </Col>
              ))}
            </Row>
          ) : roles && roles.length > 0 ? (
            <Row gutter={[16, 16]}>
              {roles.map((role, index) => (
                <Col xs={24} sm={12} lg={8} key={role.id}>
                  <RoleCard role={role} index={index} />
                </Col>
              ))}
            </Row>
          ) : (
            <Empty
              description={isError ? '岗位市场暂未开放' : '未找到匹配的角色'}
              style={{ marginTop: 48 }}
            >
              {isError ? (
                <Button icon={<ReloadOutlined />} onClick={() => refetch()}>重新加载</Button>
              ) : (debouncedQuery || activeDept) ? (
                <Button
                  onClick={() => {
                    handleDeptSelect('')
                    handleSearch('')
                  }}
                >
                  清除筛选
                </Button>
              ) : null}
            </Empty>
          )}
        </div>
      </div>
    </div>
  )
}
