# 前端镜像：构建静态文件后由 nginx 托管，并把 /api 代理到后端。
#   docker build -t scaffold-frontend scaffold-vue
FROM node:22-alpine AS build
WORKDIR /src
COPY package.json package-lock.json ./
RUN npm ci --no-audit --no-fund
COPY . .
RUN npm run build

FROM nginx:1.27-alpine
# 后端地址可用环境变量覆盖（compose 中默认服务名 backend）
ENV BACKEND_UPSTREAM=backend:8080
COPY nginx.conf.template /etc/nginx/templates/default.conf.template
COPY --from=build /src/dist /usr/share/nginx/html
EXPOSE 80
