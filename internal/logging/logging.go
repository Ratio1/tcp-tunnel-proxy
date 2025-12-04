package logging

import (
	"encoding/json"
	"io"
	"log"
	"os"
	"strings"
	"time"
)

// Setup configures the global logger based on the provided format ("plain" or "json").
func Setup(format string) {
	if strings.EqualFold(format, "json") {
		log.SetFlags(0)
		log.SetOutput(&jsonWriter{out: os.Stdout})
		return
	}

	log.SetFlags(log.LstdFlags | log.Lmicroseconds)
	log.SetOutput(os.Stdout)
}

type jsonWriter struct {
	out io.Writer
}

func (w *jsonWriter) Write(p []byte) (int, error) {
	entry := map[string]string{
		"ts":  time.Now().UTC().Format(time.RFC3339Nano),
		"msg": strings.TrimRight(string(p), "\n"),
	}
	data, err := json.Marshal(entry)
	if err != nil {
		return 0, err
	}
	data = append(data, '\n')
	if _, err := w.out.Write(data); err != nil {
		return 0, err
	}
	// Satisfy log.Logger expectations: report bytes "consumed" from original message.
	return len(p), nil
}
