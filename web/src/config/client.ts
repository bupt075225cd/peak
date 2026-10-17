// 应用客户端标识（多应用共用用户体系）。
//
// 每个独立域名应用在用户中心（user-service 应用注册表）注册唯一
// client_id：登录时经 X-Client-Id 头声明发起方应用，签发的 JWT 携带
// aud = client_id，网关按路由前缀校验应用匹配——应用间令牌不通用，
// 登录状态互不共享。
//
// 部署形态：
// - 统一前端（当前单 SPA）：默认 app-peak，网关映射同时允许各应用受众；
// - 按域名拆分后：错题本前端设 VITE_APP_CLIENT_ID=app-mistake，
//   古诗文前端设 VITE_APP_CLIENT_ID=app-poetry，网关映射随之收紧。
export const CLIENT_ID = import.meta.env.VITE_APP_CLIENT_ID || 'app-peak'
