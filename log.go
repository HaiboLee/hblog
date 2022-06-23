package hblog

import (
	"bytes"
	rotate "github.com/lestrrat-go/file-rotatelogs"
	"github.com/mattn/go-colorable"
	"github.com/rs/zerolog"
	"github.com/rs/zerolog/log"
	"github.com/rs/zerolog/pkgerrors"
	"io"
	"os"
	"path"
	"runtime"
	"strconv"
	"sync"
	"time"
)

// 日志模式
type Mode string

const (
	ModeDev   Mode = "dev"
	ModePro   Mode = "pro"
	ModeDebug Mode = "debug"
)

var (
	instance *logger
	initOnce sync.Once
)

// 时间格式化格式
var timeFormat = "2006-01-02T15:04:05.000+0800"

type Application struct {
	Version  string `json:"version,omitempty"`
	Name     string `json:"name,omitempty"`
	Hostname string `json:"hostname,omitempty"`
}

type logger struct {
	isDetailLog bool
	application Application
}

// 创建默认的对象
func NewLogger() *logger {
	initOnce.Do(func() {
		appConfig := map[string]string{"log.mode": "dev", "log.level": "trace"}
		l := logger{}
		err := l.Config(appConfig)
		if nil != err {
			panic("log init fail")
		}
		instance = &l
	})

	return instance
}

func (l logger) Config(config map[string]string) error {
	var writer io.Writer

	isColorable := config["log.colorable"] == "true"

	switch Mode(config["log.mode"]) {
	case ModeDev:
		writer = l.consoleWriter(isColorable)
		break
	case ModePro:
		writer = l.fileWriter(config)
		break
	case ModeDebug:
		writer = zerolog.MultiLevelWriter(l.fileWriter(config), l.consoleWriter(isColorable))
		break
	default:
		writer = zerolog.ConsoleWriter{Out: os.Stdout}
	}

	// 设置日志级别
	level, _ := zerolog.ParseLevel(config["log.level"])
	zerolog.SetGlobalLevel(level)
	// 设置时间经度
	zerolog.TimeFieldFormat = timeFormat
	// 文件名脱敏 存在问题 而且生产打印这个毫无意义
	//zerolog.CallerMarshalFunc = func(file string, line int) string {
	//	return l.washPath(file) + ":" + strconv.Itoa(line)
	//}
	// 使用官方提供的，输出更友好
	zerolog.ErrorStackMarshaler = pkgerrors.MarshalStack
	// 定义Logger
	withStack, err := strconv.ParseBool(config["log.with-stack"])
	if nil != err {
		withStack = false
	}
	if l.isDetailLog {
		if withStack {
			log.Logger = zerolog.New(writer).With().Timestamp().Stack().CallerWithSkipFrameCount(2).Logger().Hook(DetailLogHook{application: l.application})
		} else {
			log.Logger = zerolog.New(writer).With().Timestamp().Logger().Hook(DetailLogHook{application: l.application})
		}
	} else {
		if withStack {
			log.Logger = zerolog.New(writer).With().Timestamp().Stack().CallerWithSkipFrameCount(2).Logger()
		} else {
			log.Logger = zerolog.New(writer).With().Timestamp().Logger()
		}
	}

	//根据配置的参数决定是否增加输出协程号的钩子
	if config["log.gid-display"] == "true" {
		log.Logger = log.Logger.Hook(AddGidHook{})
	}

	log.Info().Msg("log configuration success")
	return nil
}

// 定义控制台打印
func (l logger) consoleWriter(isColorable bool) io.Writer {
	if isColorable {
		return zerolog.ConsoleWriter{Out: colorable.NewColorableStdout(), TimeFormat: timeFormat}
	} else {
		return zerolog.ConsoleWriter{Out: colorable.NewColorableStdout(), NoColor: true, TimeFormat: timeFormat}
	}
}

// 定义文件打印
func (l logger) fileWriter(config map[string]string) io.Writer {
	logPath := path.Join(config["log.path"], "LOG-%Y%m%d.%H.log")
	maxAge, err := strconv.Atoi(config["log.max-age"])
	if err != nil {
		log.Panic().Err(err).Send()
		panic(err)
	}
	rotationTime, err := strconv.Atoi(config["log.rotation-time"])
	if err != nil {
		log.Panic().Err(err).Send()
		panic(err)
	}
	if config["log.with-link-name"] == "false" {
		fileWriter, err := rotate.New(
			logPath,
			rotate.WithMaxAge(time.Duration(maxAge)*time.Hour*24),
			rotate.WithRotationTime(time.Duration(rotationTime)*time.Hour),
		)
		if err != nil {
			log.Panic().Err(err).Send()
			panic(err)
		}
		return fileWriter
	}
	fileWriter, err := rotate.New(
		logPath,
		rotate.WithLinkName("LOG.log"),
		rotate.WithMaxAge(time.Duration(maxAge)*time.Minute),
		rotate.WithRotationTime(time.Duration(rotationTime)*time.Minute),
	)
	if err != nil {
		log.Panic().Err(err).Send()
		panic(err)
	}
	return fileWriter
}

type AddGidHook struct {
}

func (AddGidHook) Run(e *zerolog.Event, level zerolog.Level, msg string) {
	b := make([]byte, 64)
	b = b[:runtime.Stack(b, false)]
	b = bytes.TrimPrefix(b, []byte("goroutine "))
	b = b[:bytes.IndexByte(b, ' ')]
	e.Str("Gid", string(b))
}

type DetailLogHook struct {
	application Application
}

func (d DetailLogHook) Run(e *zerolog.Event, level zerolog.Level, msg string) {
	e.Interface("application", d.application)
}
