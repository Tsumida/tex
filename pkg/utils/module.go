package utils

import (
	"github.com/tsumida/lunaship/infra"
	"go.uber.org/zap"
)

type ModuleAPI interface {
	ModuleName() string
	Log() *zap.Logger
}

type Module struct {
	logger     *zap.Logger
	moduleName string
}

func NewModule(moduleName string, logger *zap.Logger) *Module {
	if logger == nil {
		logger = infra.GlobalLog().With(
			zap.String("module", moduleName),
		)
	}
	return &Module{
		moduleName: moduleName,
		logger:     logger,
	}
}

func (m *Module) ModuleName() string {
	return m.moduleName
}

func (m *Module) Log() *zap.Logger {
	return m.logger
}

var _ ModuleAPI = (*Module)(nil)
