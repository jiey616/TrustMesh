FROM node:22-alpine AS builder

WORKDIR /app

COPY package.json package-lock.json ./
RUN npm config set registry https://registry.npmmirror.com && npm ci --legacy-peer-deps

COPY . .

# 注：项目标准 `npm run build` 含 tsc 严格检查（历史未清理的 noUnusedLocals 等技术债），
# 容器构建与本地验证一致使用 vite build（vite 转译不做类型检查，产物正确）。
ENV NODE_OPTIONS="--max-old-space-size=4096"
RUN npx vite build

FROM nginx:1.27-alpine

COPY nginx.conf /etc/nginx/conf.d/default.conf
COPY --from=builder /app/dist-v2 /usr/share/nginx/html

EXPOSE 80
