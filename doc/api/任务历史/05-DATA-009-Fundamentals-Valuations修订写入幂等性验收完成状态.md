# DATA-009 Fundamentals/Valuations 修订写入幂等性验收完成记录

更新时间：2026-11-18  
维护分支：`feat/data`  
任务状态：DONE
代码变更提交：`d57f3ff9decc94ac69609064242ee08ba17c16f1`
验收 CI：Go Test Suite Run `44307383999`

## 1. 任务目标

在真实 MariaDB 集成环境中验证 Fundamentals 与 Valuations 修订存储的重复写入语义：

1. 首次保存同一内容生成一条修订记录。
2. 仅改变 FetchedAt 等采集/观察时间后重复保存，不新增内容修订。
3. 指标内容或有效来源版本发生变化时，仍新增修订记录。
4. 历史 PIT 读取行为保持现状，不修改其可见性规则。

## 2. 已有能力（不得重复验收）

- `fundamentalsRevisionHash`、`valuationRevisionHash` 已排除采集/观察时间字段。
- `shouldRecordRevision` 已覆盖相同哈希不记录、内容变化记录及 A→B→A 的状态判定。
- `mysql_versions_test.go` 已覆盖 Fundamentals/Valuations 哈希对采集时间的忽略，以及内容变化会改变哈希。
- `mysql_integration_test.go` 已覆盖 Fundamentals/Valuations 不同来源修订的 as_of 读取、版本选择及连接重启后读回。
- `httpapi/fundamentals_mariadb_integration_test.go` 已覆盖 Fundamentals HTTP→Service→MariaDB→PIT 的端到端路径。

这些测试证明了各自的测试范围，但当前数据库集成测试尚未直接断言 Fundamentals/Valuations 对相同内容重复保存不会增加修订行。

## 3. 本任务唯一执行范围

在现有 `TestMariaDBRevisionPersistenceAndPointInTimeQueries` 中补充数据库级重复保存断言：

- Fundamentals：首次保存后，仅更新 FetchedAt 再保存；确认对应来源的修订行数仍为 1。
- Valuations：首次保存后，仅更新快照及条目 FetchedAt 再保存；确认相同请求指纹、来源的修订行数仍为 1。
- 原有 V1→V2 内容变更仍需生成第二条修订，已有历史 as_of 断言继续保留。
- 不新增 API、模型字段、迁移版本或运行时行为。
- 不修改 `internal/server/**`、Platform 或其他模块。

## 4. 验收标准

| 验收项 | 通过标准 |
|---|---|
| Fundamentals 重复保存 | 同一内容仅改变 FetchedAt，MariaDB 修订行数保持 1 |
| Valuations 重复保存 | 同一内容仅改变快照/条目 FetchedAt，MariaDB 修订行数保持 1 |
| 内容变化 | V1→V2 后修订行数为 2，原有 as_of 读回仍正确 |
| 集成环境未配置 | 明确标记 not_executed，不得报告通过 |
| CI | 当前代码 SHA 的 Market Data tests、Race、Vet、gofmt 与 MariaDB 集成任务按实际运行结果记录 |

## 5. 当前执行状态

- [x] 已核对 `feat/data` 最新 HEAD：`4abde045a161466db8022f42bb971324294091e5`。
- [x] 已审查 Service、Repository、HTTP 和现有 PIT 集成测试，确认本任务是数据库级重复写入断言缺口。
- [x] 已在 `internal/marketdata/repository/mysql_integration_test.go` 补充 Fundamentals/Valuations 重复保存的 MariaDB 行数断言：仅 FetchedAt 变化时保持 1 条修订；V1→V2 内容变化后为 2 条修订。
- [x] MariaDB 集成：Run `44307383999` / Job `134812355546`（代码 SHA `d57f3ff9decc94ac69609064242ee08ba17c16f1`）通过；Job 步骤明确执行 `Run MariaDB migration and PIT persistence integration`，以及 Fundamentals HTTP→MariaDB PIT 端到端测试。新加入的重复保存断言包含在该 Job 中并通过。
- [x] 同一代码 SHA `d57f3ff9decc94ac69609064242ee08ba17c16f1` 的 Market Data gofmt 与 Market Data tests 已通过。
- [x] Market Data Race detector：通过。
- [x] Market Data vet：通过。
- [x] Go Test Suite Run `44307383999`：整体 success；Market Data、Research、Researchflow、Notification 等本次工作流 Job 均完成，News 与 Compose/Docker Job 按 workflow 条件 skipped。
- [x] MariaDB 集成 Job `134812355546`：通过，包含新加的 Fundamentals/Valuations 重复保存断言及既有 PIT/HTTP 集成覆盖。
- [x] Live Provider Probe Run `44307384028`：workflow success，但多个专项探测 skipped；不作为 Provider 真实数据验收通过证据。
- [x] 完成任务归档。

## 6. 被阻塞任务

DATA-003 Fundamentals PIT 仍因真实 Provider 首次披露时点证据不足而保持 IN_PROGRESS。其代码、UTC 和 MariaDB PIT 机制已有验收，不在本任务重复执行。完整阶段记录见 [03-DATA-003](./任务历史/03-DATA-003-Fundamentals-PIT完整性阶段记录.md)。

DATA-004～DATA-007 的既有证据与剩余缺口见模块总规划和 [Data 模块卡点](../整体任务规划/Data模块卡点.md)。


## 7. 完成结论

DATA-009 已按限定范围完成。数据库级证据证明：Fundamentals 和 Valuations 仅改变 FetchedAt（Valuations 同时改变条目 FetchedAt）重复保存时均不增加修订行；内容 V1→V2 变化时均新增第二条修订。原有 PIT 历史读取断言保持通过。未修改运行时实现、API、模型字段或 Schema。

验证代码 SHA：`d57f3ff9decc94ac69609064242ee08ba17c16f1`。Go Test Suite Run `44307383999` success；Market Data gofmt/tests/race/vet 和 MariaDB 集成 Job 均通过。
