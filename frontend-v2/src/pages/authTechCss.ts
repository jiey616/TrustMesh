/** 登录/注册页共享的科技感样式（CSS-in-JS 常量，随组件注入 <style>） */
export const AUTH_TECH_CSS = `
/* ========== 全局壳（全屏自适应） ========== */
.tm-login-shell {
  position: relative;
  width: 100%;
  height: 100dvh;
  min-height: 520px;
  display: flex;
  align-items: stretch;
  justify-content: center;
  padding: clamp(10px, 2vh, 28px);
  overflow: hidden;
}

/* 背景：深色底 + 网格 + 光斑 + 噪点 */
.tm-login-bg {
  position: absolute;
  inset: 0;
  background:
    radial-gradient(ellipse 80% 60% at 20% -10%, rgba(109, 95, 245, 0.18), transparent 60%),
    radial-gradient(ellipse 60% 50% at 90% 110%, rgba(34, 211, 238, 0.1), transparent 55%),
    var(--canvas);
}
.tm-login-bg-grid {
  position: absolute;
  inset: 0;
  background-image:
    linear-gradient(rgba(255, 255, 255, 0.03) 1px, transparent 1px),
    linear-gradient(90deg, rgba(255, 255, 255, 0.03) 1px, transparent 1px);
  background-size: 56px 56px;
  mask-image: radial-gradient(ellipse 70% 70% at 50% 50%, black 20%, transparent 80%);
}
.tm-login-bg-orb {
  position: absolute;
  border-radius: 50%;
  filter: blur(60px);
  opacity: 0.5;
  animation: tm-float 10s ease-in-out infinite;
}
.tm-login-bg-orb--1 {
  width: 420px; height: 420px;
  left: -120px; top: -120px;
  background: radial-gradient(circle, rgba(109, 95, 245, 0.35), transparent 70%);
}
.tm-login-bg-orb--2 {
  width: 380px; height: 380px;
  right: -100px; bottom: -100px;
  background: radial-gradient(circle, rgba(34, 211, 238, 0.22), transparent 70%);
  animation-delay: -5s;
}
.tm-login-bg-noise {
  position: absolute;
  inset: 0;
  background-image: url("data:image/svg+xml,%3Csvg viewBox='0 0 200 200' xmlns='http://www.w3.org/2000/svg'%3E%3Cfilter id='n'%3E%3CfeTurbulence type='fractalNoise' baseFrequency='0.9' numOctaves='2' stitchTiles='stitch'/%3E%3C/filter%3E%3Crect width='100%25' height='100%25' filter='url(%23n)' opacity='0.05'/%3E%3C/svg%3E");
  pointer-events: none;
}

/* ========== 主卡片（占满视口可用空间） ========== */
.tm-login-card {
  position: relative;
  display: flex;
  width: 100%;
  max-width: 1480px;
  height: 100%;
  min-height: 0;
  border-radius: clamp(12px, 1.6vw, 20px);
  overflow: hidden;
  background: linear-gradient(160deg, rgba(22, 22, 34, 0.85), rgba(12, 12, 20, 0.92));
  backdrop-filter: blur(28px) saturate(150%);
  -webkit-backdrop-filter: blur(28px) saturate(150%);
  border: 1px solid rgba(255, 255, 255, 0.08);
  box-shadow:
    0 30px 80px rgba(0, 0, 0, 0.55),
    0 0 0 1px rgba(109, 95, 245, 0.08),
    0 0 60px rgba(109, 95, 245, 0.06);
}

/* 卡片边缘流光 */
.tm-login-card-border {
  position: absolute;
  inset: 0;
  padding: 1px;
  border-radius: clamp(12px, 1.6vw, 20px);
  background: linear-gradient(135deg, rgba(109, 95, 245, 0.5), transparent 30%, transparent 70%, rgba(34, 211, 238, 0.35));
  -webkit-mask: linear-gradient(#fff 0 0) content-box, linear-gradient(#fff 0 0);
  -webkit-mask-composite: xor;
  mask-composite: exclude;
  pointer-events: none;
}

/* ========== 左侧品牌区 ========== */
.tm-login-left {
  flex: 1 1 55%;
  min-width: 0;
  position: relative;
  display: flex;
  border-right: 1px solid rgba(255, 255, 255, 0.06);
  overflow: hidden;
}

.tm-login-hero {
  position: relative;
  width: 100%;
  display: flex;
  align-items: center;
  justify-content: center;
}

.tm-login-hero-grid {
  position: absolute;
  inset: 0;
  background-image:
    linear-gradient(rgba(109, 95, 245, 0.06) 1px, transparent 1px),
    linear-gradient(90deg, rgba(109, 95, 245, 0.06) 1px, transparent 1px);
  background-size: 40px 40px;
  mask-image: radial-gradient(ellipse 80% 80% at 50% 50%, black 30%, transparent 75%);
}

.tm-login-hero-glow {
  position: absolute;
  border-radius: 50%;
  filter: blur(80px);
}
.tm-login-hero-glow--a {
  width: 500px; height: 500px;
  top: -160px; left: -120px;
  background: radial-gradient(circle, rgba(109, 95, 245, 0.28), transparent 65%);
  animation: tm-float 12s ease-in-out infinite;
}
.tm-login-hero-glow--b {
  width: 420px; height: 420px;
  bottom: -140px; right: -100px;
  background: radial-gradient(circle, rgba(34, 211, 238, 0.18), transparent 65%);
  animation: tm-float 14s ease-in-out infinite reverse;
}

.tm-login-hero-scanline {
  position: absolute;
  left: 0; right: 0;
  height: 2px;
  background: linear-gradient(90deg, transparent, rgba(34, 211, 238, 0.5), rgba(109, 95, 245, 0.5), transparent);
  animation: tm-scan 6s linear infinite;
  opacity: 0.6;
}

.tm-login-hero-content {
  position: relative;
  z-index: 2;
  width: 100%;
  height: 100%;
  min-height: 0;
  padding: clamp(28px, 4.5vh, 56px) clamp(24px, 4vw, 64px);
  display: flex;
  flex-direction: column;
  overflow: hidden;
}

.tm-login-logo-row {
  display: flex;
  align-items: center;
  gap: 14px;
  flex-shrink: 0;
}

.tm-login-hero-title {
  color: var(--text-primary) !important;
  font-size: clamp(30px, 4.4vw, 58px) !important;
  line-height: 1.1 !important;
  letter-spacing: -1.5px !important;
  margin: clamp(18px, 3vh, 40px) 0 clamp(12px, 2vh, 22px) !important;
  background: linear-gradient(120deg, #ffffff 20%, #c4b5fd 55%, #67e8f9 90%);
  -webkit-background-clip: text;
  background-clip: text;
  -webkit-text-fill-color: transparent;
}

.tm-login-hero-sub {
  color: var(--text-tertiary) !important;
  font-size: clamp(13px, 1.1vw, 16px);
  line-height: 1.7;
  display: block;
  margin-bottom: clamp(16px, 3vh, 32px);
}

/* 3D 粒子球 + 脉冲环：flex 中间自适应，占满剩余高度 */
.tm-login-network-wrap {
  position: relative;
  flex: 1 1 auto;
  min-height: 0;
  display: flex;
  align-items: center;
  justify-content: center;
  margin: 0;
}
.tm-login-3d-canvas {
  position: relative;
  z-index: 2;
  width: min(100%, 520px);
  height: 100%;
  max-height: 480px;
  aspect-ratio: 4 / 3.2;
  cursor: grab;
}
.tm-login-3d-canvas:active { cursor: grabbing; }
.tm-login-network-ring {
  position: absolute;
  border-radius: 50%;
  border: 1px solid rgba(109, 95, 245, 0.24);
  animation: tm-ring 4s cubic-bezier(0.2, 0.6, 0.4, 1) infinite;
}
.tm-login-network-ring--1 { width: 260px; height: 260px; }
.tm-login-network-ring--2 { width: 260px; height: 260px; animation-delay: 1.33s; }
.tm-login-network-ring--3 { width: 260px; height: 260px; animation-delay: 2.66s; }

.tm-login-hero-status {
  display: flex;
  align-items: center;
  gap: 10px;
  font-size: 12px;
  color: var(--text-quaternary);
  font-family: var(--font-mono);
  letter-spacing: 0.4px;
  flex-shrink: 0;
  margin-top: 12px;
}
.tm-login-pulse-dot {
  width: 8px; height: 8px;
  border-radius: 50%;
  background: var(--success);
  box-shadow: 0 0 12px rgba(34, 197, 94, 0.8);
  animation: tm-pulse 2s infinite;
}
.tm-login-hero-status-sep {
  width: 3px; height: 3px;
  border-radius: 50%;
  background: var(--text-quaternary);
  opacity: 0.6;
}

/* ========== 右侧表单区（占剩余宽度） ========== */
.tm-login-right {
  flex: 0 1 42%;
  min-width: 380px;
  position: relative;
  display: flex;
  align-items: center;
  justify-content: center;
  padding: clamp(24px, 4vh, 56px) clamp(28px, 4vw, 60px);
  overflow-y: auto;
  overflow-x: hidden;
}

.tm-login-right-glow {
  position: absolute;
  left: 10%;
  right: 10%;
  bottom: 0;
  height: 1px;
  background: linear-gradient(90deg, transparent, rgba(109, 95, 245, 0.45), rgba(34, 211, 238, 0.35), transparent);
}

.tm-login-form-inner { width: 100%; }

.tm-login-form-kicker {
  font-family: var(--font-mono);
  font-size: 11px;
  letter-spacing: 3px;
  color: var(--signal);
  margin-bottom: 12px;
}
.tm-login-form-title {
  color: var(--text-primary) !important;
  font-size: clamp(24px, 2.6vw, 34px) !important;
  letter-spacing: -0.5px !important;
  margin: 0 0 8px !important;
}
.tm-login-form-sub {
  color: var(--text-tertiary) !important;
  font-size: 14px;
  display: block;
  margin-bottom: clamp(20px, 3.5vh, 36px);
}

.tm-login-form .ant-form-item { margin-bottom: 20px; }

/* 输入框：深色内嵌 + 聚焦发光 */
.tm-login-input.ant-input-affix-wrapper,
.tm-login-input.ant-input-password {
  background: rgba(0, 0, 0, 0.28) !important;
  border: 1px solid rgba(255, 255, 255, 0.09) !important;
  border-radius: 10px !important;
  padding: 12px 16px !important;
  transition: border-color 0.2s, box-shadow 0.2s, background 0.2s;
}
.tm-login-input:hover,
.tm-login-input:focus,
.tm-login-input.ant-input-affix-wrapper-focused {
  border-color: rgba(109, 95, 245, 0.55) !important;
  box-shadow: 0 0 0 3px rgba(109, 95, 245, 0.12), 0 0 20px rgba(109, 95, 245, 0.12) !important;
  background: rgba(0, 0, 0, 0.34) !important;
}
.tm-login-input .ant-input {
  background: transparent !important;
  color: var(--text-primary) !important;
  font-size: 14px;
}
.tm-login-input .ant-input::placeholder { color: var(--text-quaternary) !important; }
.tm-login-input .ant-input-prefix {
  color: var(--text-tertiary);
  margin-right: 10px;
  font-size: 15px;
}
.tm-login-input .ant-input-suffix { color: var(--text-tertiary); }

/* 提交按钮：渐变 + 发光 */
.tm-login-submit.ant-btn {
  height: 48px;
  border-radius: 10px;
  border: none;
  font-size: 15px;
  font-weight: 600;
  letter-spacing: 0.5px;
  background: linear-gradient(135deg, #6d5ff5 0%, #8b5cf6 55%, #22d3ee 130%);
  box-shadow: 0 8px 24px rgba(109, 95, 245, 0.35), inset 0 1px 0 rgba(255, 255, 255, 0.18);
  transition: transform 0.15s ease, box-shadow 0.2s ease, filter 0.2s ease;
}
.tm-login-submit.ant-btn:hover {
  transform: translateY(-1px);
  filter: brightness(1.06);
  box-shadow: 0 12px 32px rgba(109, 95, 245, 0.45), inset 0 1px 0 rgba(255, 255, 255, 0.22);
}
.tm-login-submit.ant-btn:active { transform: translateY(0); }

.tm-login-form-foot {
  display: flex;
  align-items: center;
  justify-content: center;
  gap: 6px;
  margin-top: 8px;
}
.tm-login-link {
  color: var(--signal) !important;
  font-size: 13px;
  transition: color 0.15s;
}
.tm-login-link:hover { color: var(--signal-hover) !important; }

/* ========== 动画 ========== */
@keyframes tm-float {
  0%, 100% { transform: translate(0, 0) scale(1); }
  50% { transform: translate(18px, -22px) scale(1.05); }
}
@keyframes tm-scan {
  0% { top: -2px; opacity: 0; }
  8% { opacity: 0.7; }
  92% { opacity: 0.7; }
  100% { top: 100%; opacity: 0; }
}
@keyframes tm-ring {
  0% { transform: scale(0.6); opacity: 0; }
  30% { opacity: 0.7; }
  100% { transform: scale(1.5); opacity: 0; }
}
@keyframes tm-pulse {
  0%, 100% { transform: scale(1); opacity: 1; }
  50% { transform: scale(1.3); opacity: 0.7; }
}

/* ========== 响应式 ========== */
@media (max-width: 1080px) {
  .tm-login-card { max-width: 560px; min-height: 0; }
  .tm-login-left { display: none; }
  .tm-login-right { flex: 1 1 auto; min-width: 0; }
  .tm-login-shell { padding: clamp(8px, 1.5vh, 20px); }
}
@media (max-width: 560px) {
  .tm-login-right { padding: 32px 24px; }
}
@media (max-height: 640px) {
  .tm-login-hero-sub { display: none; }
  .tm-login-hero-title { margin: 12px 0; }
  .tm-login-form-sub { margin-bottom: 16px; }
  .tm-login-hero-status { display: none; }
}
`
