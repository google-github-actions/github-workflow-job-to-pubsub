package logging

import (
	"encoding/json"
	"fmt"
	"io"
	"os"
	"time"
)

type Severity string

const (
	SeverityDebug Severity = "DEBUG"
	SeverityInfo  Severity = "INFO"
	SeverityWarn  Severity = "WARNING"
	SeverityError Severity = "ERROR"
	SeverityFatal Severity = "EMERGENCY"
)

type Logger struct {
	stdout io.Writer
	stderr io.Writer
}

func NewLogger(outw, errw io.Writer) *Logger {
	return &Logger{stdout: outw, stderr: errw}
}

func (l *Logger) Debug(msg string, fields ...interface{}) {
	l.log(l.stdout, msg, SeverityDebug, fields...)
}

func (l *Logger) Info(msg string, fields ...interface{}) {
	l.log(l.stdout, msg, SeverityInfo, fields...)
}

func (l *Logger) Warn(msg string, fields ...interface{}) {
	l.log(l.stdout, msg, SeverityWarn, fields...)
}

func (l *Logger) Error(msg string, fields ...interface{}) {
	l.log(l.stderr, msg, SeverityError, fields...)
}

func (l *Logger) Fatal(msg string, fields ...interface{}) {
	l.log(l.stderr, msg, SeverityFatal, fields...)
	os.Exit(1)
}

func (l *Logger) log(w io.Writer, msg string, sev Severity, fields ...interface{}) {
	if len(fields)%2 != 0 {
		panic("number of fields must be even")
	}

	data := make(map[string]interface{}, len(fields)/2)
	for i := 0; i < len(fields); i += 2 {
		key, ok := fields[i].(string)
		if !ok {
			panic(fmt.Errorf("field %d is not a string (%T, %q)", i, fields[i], fields[i]))
		}

		switch typ := fields[i+1].(type) {
		case error:
			data[key] = typ.Error()
		default:
			data[key] = typ
		}
	}

	jsonPayload, err := json.Marshal(&LogEntry{
		Time:     timePtr(time.Now().UTC()),
		Severity: sev,
		Message:  msg,
		Data:     data,
	})
	if err != nil {
		panic(fmt.Errorf("failed to marshal log entry: %w", err))
	}

	fmt.Fprintln(w, string(jsonPayload))
}

type LogEntry struct {
	Time     *time.Time
	Severity Severity
	Message  string
	Data     map[string]interface{}
}

func (l *LogEntry) MarshalJSON() ([]byte, error) {
	d := make(map[string]interface{}, 8)

	if l.Time != nil {
		d["time"] = l.Time.Format(time.RFC3339)
	}

	d["severity"] = string(l.Severity)
	d["message"] = l.Message

	for k, v := range l.Data {
		d[k] = v
	}

	return json.Marshal(d)
}

func timePtr(t time.Time) *time.Time {
	if t.IsZero() {
		return nil
	}
	return &t
}
