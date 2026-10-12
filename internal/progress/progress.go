// Package progress is the engine → GUI protocol: one JSON object per line.
package progress

import (
	"encoding/json"
	"io"
	"sync"
)

type Event struct {
	Step    string  `json:"step,omitempty"`
	Percent float64 `json:"percent"`
	Message string  `json:"message,omitempty"`
	Log     string  `json:"log,omitempty"`
	Done    bool    `json:"done,omitempty"`
	Error   string  `json:"error,omitempty"`
}

// Step names, in install order; the GUI shows their titles.
const (
	Check     = "check"
	Partition = "partition"
	Format    = "format"
	Mount     = "mount"
	Copy      = "copy"
	Boot      = "boot"
	Configure = "configure"
	Finish    = "finish"
)

var Titles = map[string]string{
	Check:     "Checking the plan",
	Partition: "Partitioning the disk",
	Format:    "Creating file systems",
	Mount:     "Mounting",
	Copy:      "Copying Zainium OS",
	Boot:      "Installing the boot loader",
	Configure: "Setting up your system",
	Finish:    "Finishing",
}

type Reporter struct {
	mu  sync.Mutex
	enc *json.Encoder
	cur Event
}

func NewReporter(w io.Writer) *Reporter { return &Reporter{enc: json.NewEncoder(w)} }

func (r *Reporter) emit(e Event) {
	r.mu.Lock()
	defer r.mu.Unlock()
	_ = r.enc.Encode(e)
}

// Step starts a step at the given overall percent.
func (r *Reporter) Step(step string, percent float64, msg string) {
	r.mu.Lock()
	r.cur = Event{Step: step, Percent: percent, Message: msg}
	e := r.cur
	r.mu.Unlock()
	r.emit(e)
}

// Percent updates the overall percent within the current step.
func (r *Reporter) Percent(percent float64, msg string) {
	r.mu.Lock()
	r.cur.Percent = percent
	if msg != "" {
		r.cur.Message = msg
	}
	e := r.cur
	r.mu.Unlock()
	r.emit(e)
}

func (r *Reporter) Logf(line string) { r.emit(Event{Log: line}) }

func (r *Reporter) Done() { r.emit(Event{Percent: 100, Done: true}) }

func (r *Reporter) Fail(err error) { r.emit(Event{Error: err.Error()}) }
