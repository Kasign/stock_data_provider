package research

import "errors"

// Research 公共错误供 Application Port、Repository Port 和 Adapter 共享。
var (
	ErrNotFound = errors.New("research record not found")
	ErrConflict = errors.New("research record conflict")
	ErrInvalidRepository = errors.New("research repository is not configured")
	ErrCorruptRecord = errors.New("research stored record is corrupt")
)
