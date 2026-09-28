# ExpenseDesk 本地差旅报销审批工具

面向小型团队的本地差旅报销审批命令行工具。费用明细、分级审批与发票占用相互核对，
防止同一张发票被多张单据重复报销。仅使用 Go 标准库，不依赖任何在线服务。

## 运行方法

```bash
go run ./cmd/expensedesk --state ./data
```

或编译后运行：

```bash
go build -o expensedesk ./cmd/expensedesk
./expensedesk --state ./data      # Windows: expensedesk.exe --state .\data
```

- `--state` 必填，指定数据目录；不存在会自动创建，全部状态保存在其中的 `state.json`。
- 程序从标准输入**逐行读取 JSON 请求**，对每行向标准输出返回**一行 JSON 结果**。
- 正常退出（输入 EOF）后重新运行会自动恢复全部单据、审批历史与发票占用。

### 交互示例

```
输入: {"op":"init","threshold":10000}
输出: {"ok":true,"message":"初始化成功","status":"ready"}

输入: {"op":"create","applicant":"alice","manager":"bob","finance":"carol",
       "items":[{"category":"交通","invoice":"INV001","desc":"高铁票","amount":9900}]}
输出: {"ok":true,"id":"S000001","status":"draft","total":9900,"message":"草稿已创建"}
```

Windows PowerShell 下批量喂入示例：

```powershell
Get-Content requests.jsonl | .\expensedesk.exe --state .\data
```

## 请求格式

公共字段：`op`（操作名）、`id`（单号）、`reviewer`、`reason`、`applicant`、`status`。
所有金额均为**正整数分**，全程不使用浮点。

| op | 必填字段 | 说明 |
| --- | --- | --- |
| `init` | `threshold` | 设置二审金额门槛（正整数分），仅首次启动时可设置，之后不可修改 |
| `create` | `applicant` `manager` `finance`，可选 `items` | 创建草稿，返回唯一单号 `S000001`… |
| `replace_items` | `id` `items` | 整组替换草稿明细，重新计算总额 |
| `submit` | `id` | 提交草稿，进入主管审批 |
| `approve` | `id` `reviewer` | 当前审批人通过 |
| `reject` | `id` `reviewer` `reason` | 当前审批人拒绝，`reason` 必填 |
| `get` | `id` | 返回详情：明细、总额、当前审批人、历史 |
| `list` | 可选 `applicant` `status` | 组合筛选，按单号升序 |

明细 `items` 元素：

```json
{"category": "交通", "invoice": "INV001", "desc": "高铁票", "amount": 9900}
```

- `category` 仅允许 `交通`、`住宿`、`餐饮`。
- `invoice` 发票号非空；`desc` 说明非空；`amount` 为正整数分。

统一返回：成功 `{"ok":true,...}`；失败 `{"ok":false,"error":"错误码","message":"中文说明"}`。
非法请求返回可区分的错误码，且**不改变**任何单据、发票占用与操作历史。

## 业务规则

### 人员与金额

- 申请人、主管、财务三方编号非空且两两不同；提交后不得修改。
- 总额由明细金额相加得到（int64 分），不由客户端传入。

### 单据状态

`draft`（草稿）→ `pending_manager`（待主管）→ `pending_finance`（待财务，仅高额单）
→ `approved`（已批准）；任一级拒绝进入终态 `rejected`（已拒绝）。

- 只有草稿允许 `replace_items`；提交时至少有一项费用。
- 提交后先等主管审批。总额 **>= 门槛**时，主管通过后还需财务通过；
  总额严格小于门槛时主管通过即批准（门槛边界本身需要二审）。
- 仅当前阶段指定的审批人能决定：越级、越权、重复审批、对已结束单据继续审批均拒绝。
- 拒绝必须填写原因；已拒绝单据不可再提交或修改。

### 发票占用

- 同一张单据内发票号不可重复。
- 草稿**不占用**发票。
- 提交时对全部明细做原子占用检查：只要有一张发票已被其他
  `pending_manager` / `pending_finance` / `approved` 单据占用，**整单拒绝**，
  不会只占用其中一部分，单据保持草稿。
- 拒绝时释放该单全部发票，发票号可被新单据复用。
- 已批准单据继续占用发票。

### 历史与详情

每次成功的提交与审批都追加一条不可变历史：`submit`、`manager_approve`、
`finance_approve`、`manager_reject`、`finance_reject`，含操作人与纳秒时间戳，
拒绝记录包含原因。`get` 返回总额与当前审批人；草稿和已结束单据没有当前审批人。

## 错误码一览

`not_initialized` `already_initialized` `invalid_threshold` `invalid_people`
`invalid_category` `invalid_invoice` `invalid_desc` `invalid_amount`
`duplicate_invoice_in_sheet` `empty_items` `sheet_not_found` `not_editable`
`not_submittable` `invoice_already_occupied` `invalid_reviewer`
`reviewer_forbidden` `reason_required` `not_in_review` `already_finished`
`invalid_status` `unknown_op` `invalid_json` `persist_failed`

## 测试

```bash
go test ./...
```

覆盖：门槛边界（低于/等于/超过）、越级与重复审批、跨单发票冲突整单拒绝、
拒绝后发票复用且不可再提交、草稿不占用、明细替换限制、列表组合筛选与排序、
以及退出重开后的状态/占用/单号一致性。
