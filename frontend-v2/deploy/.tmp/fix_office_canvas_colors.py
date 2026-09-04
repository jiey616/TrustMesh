# 修复 AI Office 文字不可见回归：
# Canvas 2D 的 ctx.fillStyle 不解析 CSS var()，传 'var(--x)' 会回退成上一次颜色
# （即文字块背景色），导致字与背景同色 → 完全隐形。
# office 是独立视觉体系，Canvas 文字必须用真实颜色值。
import io

FILES = {
    'D:/AIWorkspace/TrustMesh/frontend-v2/src/components/office/AgentSprite.tsx': [
        # 状态胶囊文字：亮色背景用深字；dimmed(离线灰底)用浅字
        ("color: 'var(--text-inverse)',",
         "color: dimmed ? 'rgba(255,255,255,0.7)' : 'rgba(10,10,18,0.92)',"),
        # 气泡文字：question(亮琥珀底)深字，普通(深底)浅字
        ("? 'var(--text-inverse)' : 'var(--text-primary)',",
         "? 'rgba(10,10,18,0.92)' : '#f4f4f8',"),
        # 剩余全局 token：名牌/普通气泡文字浅色
        ("'var(--text-primary)'", "'#f4f4f8'"),
        ("'var(--text-tertiary)'", "'rgba(255,255,255,0.55)'"),
        ("'var(--line-strong)'", "'rgba(255,255,255,0.12)'"),
        ("'var(--surface-raised)'", "'#2a2f4a'"),
        ("'var(--warning)'", "'#f59e0b'"),
        ("'var(--error)'", "'#fca5a5'"),
        ("'var(--success)'", "'#bbf7d0'"),
    ],
    'D:/AIWorkspace/TrustMesh/frontend-v2/src/components/office/MeetingTable.tsx': [
        ("color: active ? 'var(--signal-hover)' : 'var(--text-tertiary)',",
         "color: active ? '#8b7ff8' : 'rgba(255,255,255,0.48)',"),
        ("borderColor: active ? 'rgba(109,95,245,0.65)' : 'var(--line-strong)',",
         "borderColor: active ? 'rgba(109,95,245,0.65)' : 'rgba(255,255,255,0.12)',"),
    ],
}

for path, repls in FILES.items():
    with open(path, 'rb') as f:
        raw = f.read()
    text = raw.decode('utf-8').replace('\r\n', '\n')
    for old, new in repls:
        if old not in text:
            print(f'  [WARN] not found in {path}: {old!r}')
            continue
        cnt = text.count(old)
        text = text.replace(old, new)
        print(f'  replaced {cnt}x in {path.split("/")[-1]}: {old!r} -> {new!r}')
    with open(path, 'wb') as f:
        f.write(text.replace('\n', '\r\n').encode('utf-8'))
    print(f'  written {path}')
print('DONE')
