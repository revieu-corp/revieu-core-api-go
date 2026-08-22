# HTTP 回归覆盖增量（#214）

本文档记录 `apps/core/internal/router/http_critical_coverage_test.go` 的请求级回归用例。它是 `all-api-test-cases-v1.md` 的增量，不改变既有 Case ID；每个用例使用 `testutil.SetupTestDB` 创建隔离 SQLite fixture，测试结束后由测试进程销毁数据库。

## 用例目录

| Case ID | 场景 | 关键断言 |
| --- | --- | --- |
| HTTP-001 | 关键受保护路由无 JWT | 所有列出的会话、交易、优惠券、媒体、AI、验证、菜品和 admin 路由在 handler 前返回 `401` 与 JSON `error` 字段 |
| HTTP-002 | 会话参与者成功路径 | 参与者可以列出会话、读取消息、发送消息、修改静音设置；响应结构和数据库消息/设置变化一致 |
| HTTP-003 | 会话非参与者权限隔离 | 非参与者读取、发送消息、修改设置均返回 `403`，且消息数量和参与者设置不变 |
| HTTP-004 | 会话 ID 参数校验 | 非数字会话 ID 返回 `400`，不会进入数据库查询业务分支 |
| HTTP-005 | 商户验证资料校验与持久化 | 缺少文档 URL 返回 `400`；有效资料返回 `201`，验证记录和 merchant `verification_status=pending` 持久化，状态查询返回 `200` |
| HTTP-006 | OAuth callback 缺少授权码 | 缺少 `code` 时不访问 Google，直接返回 `400` 和稳定错误 |
| HTTP-007 | OAuth 未配置启动边界 | 未配置 Google client ID 时登录入口返回 `500`，不生成伪 OAuth URL |

## 已由其他覆盖 PR 负责的范围

以下 #214 acceptance 中的范围不在本增量测试文件中重复实现：

- admin role gate：PR #245
- private visibility：PR #255
- media owner binding：PR #256
- AI quota/rate-limit：PR #260

这些 PR 合并后，仍应把本文件的请求级覆盖作为统一回归入口运行；本分支不复制其实现，避免两套修复产生冲突。

## 执行方式

```bash
cd apps/core
go test ./internal/router -run 'TestCriticalProtectedRoutesRequireJWT|TestConversationHTTP|TestMerchantVerificationHTTP|TestOAuthCallback|TestGoogleLogin'
```

每次测试都断言 HTTP 状态码、JSON 响应结构和关键数据库变化；外部 Google、Gemini、对象存储和真实支付服务不在此 fixture 中调用。
