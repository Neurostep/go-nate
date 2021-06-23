package logger

import (
	"go.uber.org/zap"
)

type (
	Props struct {
		Debug bool
		Cmd string
		OutputPaths []string
	}

	Logger struct {
		*zap.SugaredLogger
	}
)

func New(props Props) (*Logger, error) {
	var config zap.Config
	config = zap.NewProductionConfig()
	if props.Debug {
		config = zap.NewDevelopmentConfig()
	}
	config.OutputPaths = props.OutputPaths
	config.DisableStacktrace = true
	config.InitialFields = map[string]interface{}{
		"cmd": props.Cmd,
	}

	log, err := config.Build()
	if err != nil {
		return nil, err
	}

	return &Logger{log.Sugar()}, nil
}

func(l *Logger) Warningf(str string, args ...interface{}) {
	l.SugaredLogger.Warnf(str, args...)
}
