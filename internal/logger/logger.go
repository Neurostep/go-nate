package logger

import (
	"fmt"
	"go.uber.org/zap"
)

func New(cmd, path string) (*zap.SugaredLogger, error) {
	config := zap.NewProductionConfig()
	config.OutputPaths = []string{fmt.Sprintf("%s/%s.log", path, cmd)}
	config.DisableStacktrace = true
	config.InitialFields = map[string]interface{}{
		"cmd": cmd,
	}

	log, err := config.Build()
	if err != nil {
		return nil, err
	}

	return log.Sugar(), nil
}
