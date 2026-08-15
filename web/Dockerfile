# 构建阶段：Vue3 + Vite 构建静态资源
FROM node:20-alpine AS builder

WORKDIR /app

# 先拷贝依赖清单，充分利用 npm 缓存
COPY package.json package-lock.json ./
RUN npm ci --registry=https://registry.npmmirror.com

# 拷贝源码并构建
COPY . .
RUN npm run build

# 运行阶段：Nginx 托管静态资源并反代 /api
FROM nginx:1.27-alpine

COPY --from=builder /app/dist /usr/share/nginx/html
COPY nginx.conf /etc/nginx/conf.d/default.conf

EXPOSE 80

CMD ["nginx", "-g", "daemon off;"]
