package application

import (
	"context"
	"errors"

	"example.com/stock-analysis-flow/internal/quant/manifest"
	"example.com/stock-analysis-flow/internal/quant/result"
)

var (
	errInvalidResultPersisterRequest = errors.New("invalid Quant result creation request")
	ErrResultPersistenceFailed    = errors.New("Quant result persistence failed")
)

// resultPersister 是 QNT-014 Facade 内部使用的低层持久化步骤，不是对 Server/Researchflow 暴露的公开用例。
// 领域计算由调用方完成；本用例负责将 Quant 强类型 Payload、Manifest 和不可变存储连接起来。
// 不接受任意 JSON，也不签发访问授权或 Researchflow Run 关联。
type resultPersister struct {
	repository result.Repository
}

func newResultPersister(repository result.Repository) (*resultPersister, error) {
	if repository == nil {
		return nil, errInvalidResultPersisterRequest
	}
	return &resultPersister{repository: repository}, nil
}

// persist 持久化一个已完成领域计算的 Quant 结果，并返回内容寻址 Manifest。
// Run→QuantResult 关联由 Researchflow 在提交 Stage Output 时建立，不属于本用例职责。
func (usecase *resultPersister) persist(ctx context.Context, input manifest.Input, payload result.Payload) (manifest.Manifest, error) {
	if usecase == nil || usecase.repository == nil || ctx == nil {
		return manifest.Manifest{}, errInvalidResultPersisterRequest
	}
	if err := ctx.Err(); err != nil {
		return manifest.Manifest{}, err
	}
	record, err := result.BuildRecord(input, payload)
	if err != nil {
		return manifest.Manifest{}, err
	}
	if err := ctx.Err(); err != nil {
		return manifest.Manifest{}, err
	}
	if err := usecase.repository.Save(ctx, record); err != nil {
		if ctxErr := ctx.Err(); ctxErr != nil {
			return manifest.Manifest{}, ctxErr
		}
		return manifest.Manifest{}, errors.Join(ErrResultPersistenceFailed, err)
	}
	if err := ctx.Err(); err != nil {
		return manifest.Manifest{}, err
	}
	return record.Manifest, nil
}
