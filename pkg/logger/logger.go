// Package logger provides package-aware logging with explicit per-package levels.
package logger

import (
	"fmt"
	"strings"

	nested "github.com/antonfisher/nested-logrus-formatter"
	"github.com/sirupsen/logrus"
)

var base = logrus.New()

// Register every package that logs here. The default is only a safety net.
var packageLogLevels = map[string]logrus.Level{
	"main":             logrus.TraceLevel,
	"printer-monitor":  logrus.TraceLevel,
	"printer-registry": logrus.TraceLevel,
	"bambu-mqtt":       logrus.TraceLevel,
	"telegram":         logrus.TraceLevel,
	"default":          logrus.TraceLevel,
}

type CustomLogger struct {
	pkg    string
	level  logrus.Level
	logger *logrus.Entry
}

func init() {
	base.SetLevel(logrus.DebugLevel)
	base.SetFormatter(&nested.Formatter{
		HideKeys:        true,
		TrimMessages:    true,
		CallerFirst:     true,
		TimestampFormat: "15:04:05.000",
	})
}

// Configure sets the application-wide maximum level. Package levels still
// apply, so both the global and package filters must permit an entry.
func Configure(level string) error {
	parsed, err := logrus.ParseLevel(strings.ToLower(strings.TrimSpace(level)))
	if err != nil {
		return fmt.Errorf("invalid log level %q: %w", level, err)
	}
	base.SetLevel(parsed)
	return nil
}

func WithPackage(pkg string) *CustomLogger {
	level, exists := packageLogLevels[pkg]
	if !exists {
		level = packageLogLevels["default"]
	}
	return &CustomLogger{pkg: pkg, level: level, logger: base.WithField("package", pkg)}
}

func (l *CustomLogger) enabled(level logrus.Level) bool { return level <= l.level }

func (l *CustomLogger) Debugf(format string, args ...any) {
	if l.enabled(logrus.DebugLevel) {
		l.logger.Debugf(format, args...)
	}
}
func (l *CustomLogger) Debugln(args ...any) {
	if l.enabled(logrus.DebugLevel) {
		l.logger.Debugln(args...)
	}
}
func (l *CustomLogger) Debug(args ...any) {
	if l.enabled(logrus.DebugLevel) {
		l.logger.Debug(args...)
	}
}
func (l *CustomLogger) Infof(format string, args ...any) {
	if l.enabled(logrus.InfoLevel) {
		l.logger.Infof(format, args...)
	}
}
func (l *CustomLogger) Info(args ...any) {
	if l.enabled(logrus.InfoLevel) {
		l.logger.Info(args...)
	}
}
func (l *CustomLogger) Warningf(format string, args ...any) {
	if l.enabled(logrus.WarnLevel) {
		l.logger.Warningf(format, args...)
	}
}
func (l *CustomLogger) Warn(args ...any) {
	if l.enabled(logrus.WarnLevel) {
		l.logger.Warn(args...)
	}
}
func (l *CustomLogger) Warnf(format string, args ...any) {
	if l.enabled(logrus.WarnLevel) {
		l.logger.Warnf(format, args...)
	}
}
func (l *CustomLogger) Errorf(format string, args ...any) {
	if l.enabled(logrus.ErrorLevel) {
		l.logger.Errorf(format, args...)
	}
}
func (l *CustomLogger) Error(args ...any) {
	if l.enabled(logrus.ErrorLevel) {
		l.logger.Error(args...)
	}
}
func (l *CustomLogger) Errorln(args ...any) {
	if l.enabled(logrus.ErrorLevel) {
		l.logger.Errorln(args...)
	}
}
func (l *CustomLogger) Fatalf(format string, args ...any) {
	if l.enabled(logrus.FatalLevel) {
		l.logger.Fatalf(format, args...)
	}
}
func (l *CustomLogger) Fatal(args ...any) {
	if l.enabled(logrus.FatalLevel) {
		l.logger.Fatal(args...)
	}
}
func (l *CustomLogger) Panicf(format string, args ...any) {
	if l.enabled(logrus.PanicLevel) {
		l.logger.Panicf(format, args...)
	}
}
func (l *CustomLogger) Panic(args ...any) {
	if l.enabled(logrus.PanicLevel) {
		l.logger.Panic(args...)
	}
}
