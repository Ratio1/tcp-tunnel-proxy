package logging

import (
	"encoding/json"
	"fmt"
	"io"
	"log"
	"os"
	"strings"
	"time"
)

type Field struct {
	Key   string
	Value any
}

type Logger struct {
	component string
	format    string
	out       io.Writer
}

var defaultFormat = "plain"
var defaultWriter io.Writer = os.Stdout

// Setup configures the default logger output/format.
func Setup(format string) {
	if strings.EqualFold(format, "json") {
		defaultFormat = "json"
		log.SetFlags(0)
		log.SetOutput(os.Stdout)
		return
	}
	defaultFormat = "plain"
	log.SetFlags(log.LstdFlags | log.Lmicroseconds)
	log.SetOutput(os.Stdout)
}

// New returns a component-specific logger using the default format/output.
func New(component string) *Logger {
	return &Logger{
		component: component,
		format:    defaultFormat,
		out:       defaultWriter,
	}
}

func (l *Logger) Info(msg string, fields ...Field) {
	l.log("INFO", msg, fields...)
}

func (l *Logger) Error(msg string, fields ...Field) {
	l.log("ERROR", msg, fields...)
}

func (l *Logger) Infof(format string, args ...any) {
	l.log("INFO", fmt.Sprintf(format, args...))
}

func (l *Logger) Errorf(format string, args ...any) {
	l.log("ERROR", fmt.Sprintf(format, args...))
}

func (l *Logger) log(level, msg string, fields ...Field) {
	if l.format == "json" {
		l.writeJSON(level, msg, fields...)
		return
	}
	l.writePlain(level, msg, fields...)
}

func (l *Logger) writePlain(level, msg string, fields ...Field) {
	var sb strings.Builder
	sb.WriteString("[")
	sb.WriteString(level)
	sb.WriteString("]")
	if l.component != "" {
		sb.WriteString("[")
		sb.WriteString(l.component)
		sb.WriteString("]")
	}
	if len(fields) > 0 {
		sb.WriteString(" ")
		for i, f := range fields {
			sb.WriteString(fmt.Sprintf("%s=%v", f.Key, f.Value))
			if i != len(fields)-1 {
				sb.WriteString(" ")
			}
		}
		sb.WriteString(" ")
	}
	sb.WriteString(msg)
	log.Print(sb.String())
}

func (l *Logger) writeJSON(level, msg string, fields ...Field) {
	entry := map[string]any{
		"ts":        time.Now().UTC().Format(time.RFC3339Nano),
		"level":     level,
		"component": l.component,
		"msg":       msg,
	}
	if len(fields) > 0 {
		m := make(map[string]any, len(fields))
		for _, f := range fields {
			m[f.Key] = f.Value
		}
		entry["fields"] = m
	}
	data, err := json.Marshal(entry)
	if err != nil {
		return
	}
	data = append(data, '\n')
	_, _ = l.out.Write(data)
}
