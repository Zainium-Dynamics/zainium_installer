package progress

import (
	"encoding/json"
	"fmt"
	"io"
	"strings"
)

// TextWriter turns the JSON progress lines into readable output for a terminal.
type TextWriter struct{ W io.Writer }

func (t *TextWriter) Write(b []byte) (int, error) {
	for _, line := range strings.Split(strings.TrimSpace(string(b)), "\n") {
		var ev Event
		if json.Unmarshal([]byte(line), &ev) != nil {
			continue
		}
		switch {
		case ev.Error != "":
			fmt.Fprintf(t.W, "FAILED: %s\n", ev.Error)
		case ev.Done:
			fmt.Fprintln(t.W, "[100%] Installation complete")
		case ev.Log != "":
			fmt.Fprintf(t.W, "       %s\n", ev.Log)
		default:
			fmt.Fprintf(t.W, "[%3.0f%%] %s\n", ev.Percent, ev.Message)
		}
	}
	return len(b), nil
}
