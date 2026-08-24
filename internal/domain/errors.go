package domain

import "errors"

var (
	ErrNotFound       = errors.New("记录不存在")
	ErrConflict       = errors.New("记录已被其他请求更新")
	ErrValidation     = errors.New("输入校验失败")
	ErrInvalidState   = errors.New("当前状态不允许该操作")
	ErrIdempotencyKey = errors.New("幂等键已用于不同请求")
)

// Problem 保留可展示的字段级错误，同时支持 errors.Is 分类。
type Problem struct {
	Kind    error
	Message string
	Fields  map[string]string
}

func (p *Problem) Error() string { return p.Message }

func (p *Problem) Unwrap() error { return p.Kind }

func Validation(message string, fields map[string]string) error {
	return &Problem{Kind: ErrValidation, Message: message, Fields: fields}
}

func Conflict(message string) error {
	return &Problem{Kind: ErrConflict, Message: message}
}

func InvalidState(message string) error {
	return &Problem{Kind: ErrInvalidState, Message: message}
}
