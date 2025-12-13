package texerror

import "fmt"

var (
	ERR_CODE_INTERNAL         = 1000000 // OMS内部错误, 一般用于各个组件非预期错误
	ERR_CODE_INPOSSIBLE_STATE = 1000001 // 不可能的状态
	ERR_CODE_INVALID_REQUEST  = 1000002 // 请求参数无效

	// Tex定义
	ERR_CODE_TEX_ORDER_NOT_FOUND = 1001001 // Tex没查到订单
)

var (
	ErrInternal         = newInternalErr(int32(ERR_CODE_INTERNAL))
	ErrTexOrderNotFound = newInternalErr(int32(ERR_CODE_TEX_ORDER_NOT_FOUND))
)

type TexError struct {
	Code int32  `json:"code"`
	Msg  string `json:"msg"`
}

var _ error = &TexError{}

func (e *TexError) Error() string {
	return fmt.Sprintf("TexErr{code=%d, msg=%s}", e.Code, e.Msg)
}

// 同一code可能对应多个不同的错误信息
func newInternalErr(code int32) func(msg string) *TexError {
	return func(msg string) *TexError {
		return &TexError{
			Code: code,
			Msg:  msg,
		}
	}
}
