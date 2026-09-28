# ExpenseDesk 本地差旅报销审批工具

面向小型团队的本地命令行工具：管理差旅报销单明细、分级审批和发票占用，
确保同一张发票不会被重复报销。仅依赖 Go 标准库，不需要任何在线服务。

## 运行方法

需要 Go 1.26+（仅使用标准库）。

```sh
go run ./cmd/expensedesk --state ./data
```

或编译后运行：

```sh
go build -o expensedesk ./cmd/expensedesk
./expensedesk --state ./data
```

`--state` 指定数据目录（必填，不存在会自动创建）。程序从标准输入逐行读取
JSON 请求，每读取一行就在标准输出打印一行 JSON 结果，直到输入结束正常退出。
状态以 JSON 原子写入数据目录下的 `expensedesk-state.json`，重开后完全一致。

## 请求格式

每行一个 JSON 对象，通用字段按 `action` 取用：

| action | 必填字段 | 说明 |
| --- | --- | --- |
| `init` | `threshold` | 初始化二审金额门槛（正整数，单位分），只能调用一次 |
| `create` | `applicant_id`, `manager_id`, `finance_id` | 创建草稿单，返回 `id` |
| `replace_items` | `id`, `items` | 整体替换草稿明细 |
| `submit` | `id` | 提交草稿进入主管审批 |
| `approve` | `id`, `actor_id` | 当前阶段审批人通过 |
| `reject` | `id`, `actor_id`, `reason` | 当前阶段审批人拒绝（原因非空） |
| `get` | `id` | 查询单张单据详情 |
| `list` | 可选 `applicant_id`, `status` | 组合筛选并按单号排序 |
| `status` | 无 | 查询是否已初始化及门槛 |

明细对象 `items[]` 字段：

```json
{"category": "交通", "invoice_no": "INV-001", "desc": "机场打车", "amount_cents": 1500}
```

- `category` 只能是 `交通`、`住宿`、`餐饮`。
- `invoice_no`、`desc` 非空；`amount_cents` 为正整数，金额单位是分，不使用浮点。

成功响应形如 `{"ok":true,"id":"R1"}`；失败响应形如
`{"ok":false,"error":"invoice_conflict"}`，`error` 为稳定的错误码。
详情响应额外包含 `report`：总额 `total_cents`、当前审批人 `current_approver`
（草稿和已结束单据中省略）以及 `history`（成功提交/审批记录）。

## 业务规则

- **初始化**：门槛必须为正整数，设置后永久不变；重复初始化返回 `already_initialized`。
- **人员**：申请人、主管、财务编号必须非空且互不相同，提交后不得修改。
- **明细**：总额由各项 `amount_cents` 相加；单内发票号不得重复；提交时至少一项。
- **草稿**：只有草稿可以替换明细；草稿不占用发票。
- **发票占用**：处于主管待审、财务待审、已批准的单据占用其全部发票。
  提交时若任一发票被其他活动单据占用，整单拒绝（`invoice_conflict`），
  不会只占用其中一部分，单据保持草稿。
- **审批分级**：提交后先由主管审批；总额 `>= threshold` 时主管通过后还需财务通过，
  小于门槛时主管通过即批准。
- **越权处理**：只有当前阶段指定的审批人能操作；越级、重复审批、审批草稿或
  已结束单据分别返回 `wrong_stage_or_actor` 或 `report_already_ended`。
- **拒绝**：任一级拒绝都必须填写原因，单据进入 `rejected` 终态，不可再提交，
  其发票全部释放，可被其他单据使用。
- **已批准**：`approved` 为终态，发票继续占用。
- **原子性**：任何非法请求都不会改变单据、发票占用和历史。
- **列表**：支持按申请人和状态组合筛选，按单号（`R1`, `R2`, ...）升序返回。

### 错误码

`bad_threshold`、`already_initialized`、`not_initialized`、`empty_person_id`、
`people_must_be_distinct`、`bad_category`、`empty_invoice_no`、
`empty_description`、`bad_amount`、`duplicate_invoice_in_report`、`empty_items`、
`report_not_found`、`not_a_draft`、`report_not_submittable`、`invoice_conflict`、
`wrong_stage_or_actor`、`report_already_ended`、`empty_reject_reason`、
`unknown_action`、`invalid_json`。

## 示例会话

```sh
$ go run ./cmd/expensedesk --state ./data <<'EOF'
{"action":"init","threshold":1000}
{"action":"create","applicant_id":"alice","manager_id":"bob","finance_id":"carol"}
{"action":"replace_items","id":"R1","items":[{"category":"交通","invoice_no":"INV-1","desc":"taxi","amount_cents":1500}]}
{"action":"submit","id":"R1"}
{"action":"approve","id":"R1","actor_id":"bob"}
{"action":"get","id":"R1"}
EOF
```

## 测试

```sh
go test ./...
```

测试覆盖：门槛边界（等于门槛需要二审、低于门槛主管即批）、越级/重复/终态审批、
草稿与明细校验、跨单发票整单冲突、拒绝后发票复用与已批准持续占用、重开后的
状态/历史/占用一致性，以及列表筛选排序与 JSON 协议错误码。
