# API 测试手册使用说明

## 文档位置
- 全量测试用例：`docs/testing/all-api-test-cases-v1.md`

## 覆盖声明
- 数据源：`apps/core/docs/swagger.json`
- 覆盖数量：`108` 个 API 操作（method + path）
- 覆盖范围：OpenAPI v1 当前全部已登记接口

## 测试环境
- 推荐环境：`https://dev.revieu.weijun.online/api/v1`
- 可切本地：`http://127.0.0.1:8080/api/v1`

## 通用前置
1. 准备至少 2 个账号（普通用户 + 商家测试账号）。
2. 先执行登录接口拿 `access_token`。
3. 对需要鉴权的接口，加请求头：`Authorization: Bearer <ACCESS_TOKEN>`。
4. 每次提交结果时，必须记录：`请求参数`、`状态码`、`响应体`、`数据库关键字段变化`（如适用）。

## 分批执行建议（给多人并行）
1. Batch A（基础账号与资料）
- 标签：`auth` `user` `profile` `verification`
- 目标：确认登录、资料、隐私、通知偏好、地址管理链路稳定

2. Batch B（内容与社交）
- 标签：`feed` `content` `review` `follow` `conversation` `notification`
- 目标：确认发布/读取、关注、互动（like/comment）不回归

3. Batch C（商家与门店）
- 标签：`merchant` `store` `category`
- 目标：确认 merchant/store 创建、激活、公开查询与筛选

4. Batch D（交易闭环）
- 标签：`coupon` `order` `voucher` `payment` `package`
- 目标：确认券创建、校验、下单、支付、发券、核销闭环

5. Batch E（平台能力）
- 标签：`admin` `media` `ai`
- 目标：确认管理后台、媒体上传、AI 建议接口可用

## 执行与回传规范
1. 每个接口至少验证一次成功响应（2xx/3xx）和一次失败响应（4xx）
2. 失败响应至少覆盖：参数错误、无权限、资源不存在（按接口定义）
3. 回传格式建议：
- `Case ID`（如 `TC-046`）
- `Result`（PASS/FAIL）
- `HTTP Status`
- `Evidence`（关键响应 JSON）
- `Notes`（异常、性能、数据污染）

## 注意事项
- 全量文档里的 JSON 为 schema 推导示例，字段结构是权威，字段值是示例值。
- 若 swagger 与实际行为不一致，以实际行为为准并提 issue 修正文档/注解。
- 交易类接口测试会产生真实测试数据，执行前请确认清理策略。
