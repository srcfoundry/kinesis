package anylogger

import (
	"fmt"
	"log"
	"reflect"
)

type AnyLogger[T any] struct {
	LType       T
	LvlLogger   LevelLogger[T]
	MsgLogger   MsgLogger[T]
	IsLvlLogger bool
}

var (
	anylogger interface{}
	anyType   interface{}
)

type LevelLogger[T any] interface {
	Info(msg string, fields ...T)
	Debug(msg string, fields ...T)
	Error(msg string, fields ...T)
	Warn(msg string, fields ...T)
}

type MsgLogger[T any] interface {
	Msg(string, ...T)
}

func AssignLevelLogger[a any](lvlLogger LevelLogger[a]) {
	anylog := &AnyLogger[a]{LvlLogger: lvlLogger, IsLvlLogger: true}
	anyType = anylog.LType
	anylogger = nil
	anylogger = anylog
}

func AssignMsgLogger[a any](msgLogger MsgLogger[a]) {
	anylogger = nil
	anylogger = &AnyLogger[a]{MsgLogger: msgLogger}
}

func TTY() func(string, ...interface{}) {
	// Check if anylogger is assigned
	if anylogger == nil {
		return nil
	}

	// Use reflection to get the value of lvlLogger from AnyLogger
	lvlLoggerValue := reflect.ValueOf(anylogger).Elem().FieldByName("LvlLogger")
	if !lvlLoggerValue.IsValid() {
		return nil
	}

	// Extract the Info method from lvlLogger
	infoMethod := lvlLoggerValue.MethodByName("Info")
	if !infoMethod.IsValid() {
		return nil
	}

	// Return a reference to the Info method
	//return infoMethod.Interface().(func(string, ...interface{}))

	// switch aType := anyType.(type) {
	// case string:
	// 	df := aType + "dfds"
	// case interface{}:
	// 	return infoMethod.Interface().(func(string, ...aType))
	// }

	return infoMethod.Interface().(func(string, ...any))

	// Create and return a function that calls the Info method
	// return func(msg string, fields ...interface{}) {
	//  // Prepare arguments for the Info method
	//  args := make([]reflect.Value, len(fields)+1)
	//  args[0] = reflect.ValueOf(msg)
	//  for i, field := range fields {
	//      args[i+1] = reflect.ValueOf(field)
	//  }

	//  // Call the Info method with the provided arguments
	//  infoMethod.Call(args)
	// }

	// Return the dynamically created function
	//return infoFunc.Interface()
	// Get the value of anylogger
	// v := reflect.ValueOf(anylogger)

	// ty := v.Elem().NumField()
	// fmt.Println(ty)

	// // Get the LevelLogger interface from the value
	// lvlLogger := v.Elem().FieldByName("lvlLogger")
	// lvlLoggerIf := lvlLogger.Interface()

	// // Get the type of the arguments for the Info function
	// argsType := reflect.TypeOf(anyType)

	// // Get the method Info from the LevelLogger interface
	// method, _ := reflect.TypeOf(lvlLoggerIf).MethodByName("Info")

	// // Create a function with the same type as the Info method
	// infoFunc := reflect.MakeFunc(method.Type, func(args []reflect.Value) []reflect.Value {
	//  // Call the Info method on the LevelLogger interface
	//  return v.MethodByName("Info").Call(args)
	// })

	// fmt.Println(argsType)
	// fmt.Println(infoFunc)

	// loggerValue := reflect.ValueOf(anylogger)
	// fmt.Println(loggerValue)
	// fmt.Println(loggerValue.Type())

	// for i := 0; i < loggerValue.NumMethod(); i++ {
	//  fmt.Println("III", i, loggerValue.Method(i))
	// }

	// lvlLogger := loggerValue.Elem().FieldByName("lvlLogger")
	// lvlLoggerIf := lvlLogger.Addr().Pointer()
	// fmt.Println(lvlLoggerIf)
	//Info(loggerValue.Interface().(LevelLogger[any]), loggerValue.Interface())
	//Info(anylogger, anylogger.(LevelLogger))
}

func Info[L LevelLogger[A], A any](l L, a A) func(string, ...A) {
	fmt.Println(a, l)
	// if anylogger == nil {
	//  anylogger = &AnyLogger{defaultLog, nil, true}
	// }

	// anyLog := anylogger.(AnyLogger[any])
	// fieldType := reflect.TypeOf(anyLog.lType).Elem()

	// if !anyLog.isLvlLogger {
	//  return anyLog.msgLogger.Msg
	// }
	// return (func(string, ...logType))(anyLog.lvlLogger.Info)
	return l.Info
}

// func Debug() func(string, ...interface{}) {
//  if anylogger == nil {
//      anylogger = &AnyLogger{defaultLog, nil, true}
//  }

//  if !anylogger.isLvlLogger {
//      return anylogger.msgLogger.Msg
//  }
//  return anylogger.lvlLogger.Debug
// }

// func Error() func(string, ...interface{}) {
//  if anylogger == nil {
//      anylogger = &AnyLogger{defaultLog, nil, true}
//  }

//  if !anylogger.isLvlLogger {
//      return anylogger.msgLogger.Msg
//  }
//  return anylogger.lvlLogger.Debug
// }

// func Infof() func(string, ...interface{}) {
//  if anylogger == nil {
//      anylogger = &AnyLogger{defaultLog, nil, true}
//  }

//  if !anylogger.isLvlLogger {
//      return anylogger.msgLogger.Msg
//  }
//  return anylogger.lvlLogger.Infof
// }

// func Debugf() func(string, ...interface{}) {
//  if anylogger == nil {
//      anylogger = &AnyLogger{defaultLog, nil, true}
//  }

//  if !anylogger.isLvlLogger {
//      return anylogger.msgLogger.Msg
//  }
//  return anylogger.lvlLogger.Debugf
// }

// func Errorf() func(string, ...interface{}) {
//  if anylogger == nil {
//      anylogger = &AnyLogger{defaultLog, nil, true}
//  }

//  if !anylogger.isLvlLogger {
//      return anylogger.msgLogger.Msg
//  }
//  return anylogger.lvlLogger.Debugf
// }

// type str[T interface{}] struct{}

// func (s str[T]) Debug(str string, data ...T) {

// }

// func yyu() {
//  zapLogger, _ := zap.NewProduction()
//  debugFn = (zapLogger.Debug)
//  debugFn("")
// }

// todo: add doc
type defaultLogger struct {
	MsgLogger[any]
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

func (defaultLogger) Infof(format string, fields ...interface{}) {
	log.Printf("INFO: "+format, fields...)
}
func (defaultLogger) Debugf(format string, fields ...interface{}) {
	log.Printf("DEBUG: "+format, fields...)
}
func (defaultLogger) Errorf(format string, fields ...interface{}) {
	log.Printf("ERROR: "+format, fields...)
}
func (defaultLogger) Warnf(format string, fields ...interface{}) {
	log.Printf("WARN: "+format, fields...)
}
func (defaultLogger) Fatalf(format string, fields ...interface{}) {
	log.Printf("FATAL: "+format, fields...)
}

func (defaultLogger) Msg(msg string) {
	fmt.Println(msg)
}
