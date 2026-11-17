package application

import "errors"

// ErrPersistenceOutcomeUnknown 表示数据库提交调用返回错误，事务最终是否提交无法仅凭该错误确定。
// 调用方必须重新读取持久化状态并执行对账，不得将其直接解释为事务回滚或触发外部副作用重试。
var ErrPersistenceOutcomeUnknown = errors.New("notification: persistence outcome unknown")
