package anylogger

import (
	"fmt"
	"log"
)

type AnyLogger struct {
	lvlLogger   LevelLogger
	msgLogger   MsgLogger
	isLvlLogger bool
}

var (
	anylogger *AnyLogger
)

// func Info(msg string, fields ...interface{}) {
// 	if anylogger.isLvlLogger {
// 		anylogger.lvlLogger.Info(msg, fields...)
// 		return
// 	}
// 	anylogger.msgLogger.Info().Msg(fmt.Sprintln(msg, fields))
// }

// func Debug(msg string, fields ...interface{}) {
// 	if anylogger.isLvlLogger {
// 		anylogger.lvlLogger.Debug(msg, fields...)
// 		return
// 	}
// 	anylogger.msgLogger.Debug().Msg(fmt.Sprintln(msg, fields))
// }

// func Error(msg string, fields ...interface{}) {
// 	if anylogger.isLvlLogger {
// 		anylogger.lvlLogger.Error(msg, fields...)
// 		return
// 	}
// 	anylogger.msgLogger.Error().Msg(fmt.Sprintln(msg, fields))
// }

// func Warn(msg string, fields ...interface{}) {
// 	if anylogger.isLvlLogger {
// 		anylogger.lvlLogger.Warn(msg, fields...)
// 		return
// 	}
// 	anylogger.msgLogger.Warn().Msg(fmt.Sprintln(msg, fields))
// }

type LevelLogger interface {
	Info(string, ...interface{})
	Debug(string, ...interface{})
	Error(string, ...interface{})
	Warn(string, ...interface{})
}

type MsgLogger interface {
	// Info() MsgLogger
	// Debug() MsgLogger
	// Error() MsgLogger
	// Warn() MsgLogger
	Msg(string, ...interface{})
}

func AssignLevelLogger(lvlLogger LevelLogger) {
	anylogger = nil
	anylogger = &AnyLogger{lvlLogger: lvlLogger}
}

func AssignMsgLogger(msgLogger MsgLogger) {
	anylogger = nil
	anylogger = &AnyLogger{msgLogger: msgLogger}
}

func GetLogger() *AnyLogger {
	if anylogger == nil {
		anylogger = &AnyLogger{defaultLog, nil, true}
	}
	return anylogger
}

// todo: add doc
type defaultLogger struct {
	MsgLogger
}

var defaultLog = defaultLogger{}

func (defaultLogger) Info(msg string, fields ...interface{}) {
	log.Println("INFO:", msg, fields)
}
func (defaultLogger) Debug(msg string, fields ...interface{}) {
	log.Println("DEBUG:", msg, fields)
}
func (defaultLogger) Error(msg string, fields ...interface{}) {
	log.Println("ERROR:", msg, fields)
}
func (defaultLogger) Warn(msg string, fields ...interface{}) {
	log.Println("WARN:", msg, fields)
}
func (defaultLogger) Fatal(msg string, fields ...interface{}) {
	log.Fatal("FATAL:", msg, fields)
}

func (defaultLogger) Msg(msg string) {
	fmt.Println(msg)
}
