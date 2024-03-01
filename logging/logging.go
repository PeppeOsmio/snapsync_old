package logging

import (
	"fmt"
	"runtime"

	"github.com/sirupsen/logrus"
)

// ContextHook is a Logrus hook designed to add file and line number information to log entries.
type ContextHook struct{}

func (hook ContextHook) Levels() []logrus.Level {
	return logrus.AllLevels
}

func (hook ContextHook) Fire(entry *logrus.Entry) error {
	if pc, file, line, ok := runtime.Caller(5); ok {
		funcName := runtime.FuncForPC(pc).Name()
		entry.Data["file"] = fmt.Sprintf("%s:%d", file, line)
		entry.Data["func"] = funcName
	}
	return nil
}

func SetupLogging() {
	// logrus.AddHook(ContextHook{})
}
