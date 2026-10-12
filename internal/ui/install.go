package ui

import (
	"bufio"
	"encoding/json"
	"errors"
	"fmt"
	"os"
	"os/exec"
	"strings"
	"sync"

	"gioui.org/font"
	"gioui.org/layout"
	"gioui.org/widget/material"

	"github.com/Zainium-Dynamics/zainium-installer/internal/progress"
)

// Engine is the root-side installer, started through pkexec.
const Engine = "/overlayer/zaisys/bin/zainium-install"

type installer struct {
	mu      sync.Mutex
	step    string
	percent float64
	message string
	log     []string
	done    bool
	err     string
	steps   []string // steps seen, in order
}

func (in *installer) snapshot() installer {
	in.mu.Lock()
	defer in.mu.Unlock()
	return installer{step: in.step, percent: in.percent, message: in.message, log: in.log, done: in.done, err: in.err, steps: in.steps}
}

func engineCommand() *exec.Cmd {
	engine := Engine
	if e := os.Getenv("ZAINIUM_INSTALL_ENGINE"); e != "" {
		engine = e // development: a locally built engine
	}
	if os.Geteuid() == 0 {
		return exec.Command(engine, "--plan", "-")
	}
	return exec.Command("pkexec", engine, "--plan", "-")
}

func (a *App) startInstall() {
	p := a.Plan()
	if err := p.Validate(); err != nil {
		a.inst = &installer{err: err.Error()}
		a.page = pageInstall
		return
	}
	in := &installer{step: progress.Check, message: "Asking for permission to install"}
	a.inst = in
	a.page = pageInstall
	go func() {
		err := a.runEngine(in, p)
		in.mu.Lock()
		if err != nil && in.err == "" {
			in.err = err.Error()
		}
		if !in.done && in.err == "" {
			in.err = "the installer stopped without finishing"
		}
		in.mu.Unlock()
		a.w.Invalidate()
	}()
}

func (a *App) runEngine(in *installer, p any) error {
	cmd := engineCommand()
	stdin, err := cmd.StdinPipe()
	if err != nil {
		return err
	}
	stdout, err := cmd.StdoutPipe()
	if err != nil {
		return err
	}
	var stderr strings.Builder
	cmd.Stderr = &stderr
	if err := cmd.Start(); err != nil {
		return fmt.Errorf("starting the installer: %w", err)
	}
	json.NewEncoder(stdin).Encode(p)
	stdin.Close()

	s := bufio.NewScanner(stdout)
	s.Buffer(make([]byte, 64<<10), 1<<20)
	for s.Scan() {
		var ev progress.Event
		if json.Unmarshal(s.Bytes(), &ev) != nil {
			continue
		}
		in.mu.Lock()
		switch {
		case ev.Error != "":
			in.err = ev.Error
		case ev.Done:
			in.done, in.percent = true, 100
		case ev.Log != "":
			in.log = append(in.log, ev.Log)
		default:
			if ev.Step != "" && ev.Step != in.step || len(in.steps) == 0 {
				in.steps = append(in.steps, ev.Step)
			}
			in.step, in.percent, in.message = ev.Step, ev.Percent, ev.Message
		}
		in.mu.Unlock()
		a.w.Invalidate()
	}
	err = cmd.Wait()
	var ee *exec.ExitError
	if errors.As(err, &ee) && cmd.Args[0] == "pkexec" && (ee.ExitCode() == 126 || ee.ExitCode() == 127) {
		return errors.New("permission to install was not given (authentication cancelled or failed)")
	}
	if err != nil {
		if msg := strings.TrimSpace(stderr.String()); msg != "" {
			return errors.New(msg)
		}
	}
	return err
}

var stepOrder = []string{progress.Check, progress.Partition, progress.Format, progress.Mount, progress.Copy, progress.Boot, progress.Configure, progress.Finish}

func (a *App) layoutInstall(gtx layout.Context) {
	st := a.inst.snapshot()
	if st.done && a.page == pageInstall {
		a.page = pageDone
	}
	if a.restartBtn.Clicked(gtx) {
		exec.Command("systemctl", "reboot").Start()
	}
	if a.quitBtn.Clicked(gtx) {
		os.Exit(0)
	}
	if a.detailsBtn.Clicked(gtx) {
		a.showDetails = !a.showDetails
	}
	if a.doneBtn.Clicked(gtx) && st.err != "" {
		a.page = pageHub // back to fix the plan or retry
		a.inst = nil
		return
	}

	title := "INSTALLING ZAINIUM OS"
	switch {
	case st.done:
		title = "INSTALLATION COMPLETE"
	case st.err != "":
		title = "INSTALLATION FAILED"
	}
	layout.Flex{Axis: layout.Vertical}.Layout(gtx,
		layout.Rigid(func(gtx layout.Context) layout.Dimensions { return a.header(gtx, title) }),
		layout.Flexed(1, func(gtx layout.Context) layout.Dimensions {
			gtx.Constraints.Min = gtx.Constraints.Max
			return layout.Inset{Top: 28, Bottom: 20, Left: 48, Right: 48}.Layout(gtx, func(gtx layout.Context) layout.Dimensions {
				return layout.Flex{Axis: layout.Vertical}.Layout(gtx,
					layout.Rigid(func(gtx layout.Context) layout.Dimensions { return a.installTop(gtx, st) }),
					vspace(20),
					layout.Flexed(1, func(gtx layout.Context) layout.Dimensions {
						if a.showDetails || st.err != "" {
							return a.logView(gtx, st)
						}
						return a.stepList(gtx, st)
					}),
				)
			})
		}),
		layout.Rigid(func(gtx layout.Context) layout.Dimensions { return a.installFooter(gtx, st) }),
	)
}

func (a *App) installTop(gtx layout.Context, st installer) layout.Dimensions {
	return layout.Flex{Axis: layout.Vertical}.Layout(gtx,
		layout.Rigid(func(gtx layout.Context) layout.Dimensions {
			return layout.Flex{Alignment: layout.Middle}.Layout(gtx,
				layout.Rigid(func(gtx layout.Context) layout.Dimensions { return drawImage(gtx, a.logoSmall, 64) }),
				hspace(18),
				layout.Flexed(1, func(gtx layout.Context) layout.Dimensions {
					head, sub, hc := "Installing Zainium OS", st.message, colText
					switch {
					case st.done:
						head, sub = "Zainium OS is installed", "Restart the computer and remove the installation medium. Log in with the account you created."
					case st.err != "":
						head, sub, hc = "The installation could not be completed", st.err, colError
					}
					return layout.Flex{Axis: layout.Vertical}.Layout(gtx,
						layout.Rigid(label(a.th, 21, head, hc, font.SemiBold).Layout),
						vspace(4),
						layout.Rigid(label(a.th, 14, sub, colDim, font.Normal).Layout),
					)
				}),
				layout.Rigid(label(a.th, 26, fmt.Sprintf("%.0f%%", st.percent), colAccent, font.Bold).Layout),
			)
		}),
		vspace(18),
		layout.Rigid(func(gtx layout.Context) layout.Dimensions {
			pb := material.ProgressBar(a.th, float32(st.percent/100))
			pb.Color, pb.TrackColor, pb.Height, pb.Radius = colAccent, colCard, 8, 4
			if st.err != "" {
				pb.Color = colError
			}
			return pb.Layout(gtx)
		}),
	)
}

func (a *App) stepList(gtx layout.Context, st installer) layout.Dimensions {
	cur := -1
	for i, s := range stepOrder {
		if s == st.step {
			cur = i
		}
	}
	var rows []layout.FlexChild
	for i, s := range stepOrder {
		state := "todo"
		switch {
		case st.done || i < cur:
			state = "done"
		case i == cur:
			state = "now"
		}
		rows = append(rows, layout.Rigid(func(gtx layout.Context) layout.Dimensions {
			return layout.Inset{Bottom: 12}.Layout(gtx, func(gtx layout.Context) layout.Dimensions {
				return layout.Flex{Alignment: layout.Middle}.Layout(gtx,
					layout.Rigid(func(gtx layout.Context) layout.Dimensions {
						switch state {
						case "done":
							return icon(gtx, icCheck, colOK, 20)
						case "now":
							return icon(gtx, icWorking, colAccent, 20)
						}
						return icon(gtx, icPending, rgb(0x4a4a4a), 20)
					}),
					hspace(12),
					layout.Rigid(func(gtx layout.Context) layout.Dimensions {
						c, w := colDim, font.Normal
						if state == "now" {
							c, w = colText, font.SemiBold
						}
						return label(a.th, 15, progress.Titles[s], c, w).Layout(gtx)
					}),
				)
			})
		}))
	}
	return layout.Flex{Axis: layout.Vertical}.Layout(gtx, rows...)
}

func (a *App) logView(gtx layout.Context, st installer) layout.Dimensions {
	return border(gtx, colBorder, 8, func(gtx layout.Context) layout.Dimensions {
		gtx.Constraints.Min = gtx.Constraints.Max
		return layout.UniformInset(10).Layout(gtx, func(gtx layout.Context) layout.Dimensions {
			a.logList.ScrollToEnd = true
			return material.List(a.th, &a.logList).Layout(gtx, len(st.log), func(gtx layout.Context, i int) layout.Dimensions {
				l := label(a.th, 12, st.log[i], colDim, font.Normal)
				l.Font.Typeface = "Go Mono"
				return l.Layout(gtx)
			})
		})
	})
}

func (a *App) installFooter(gtx layout.Context, st installer) layout.Dimensions {
	gtx.Constraints.Min.X = gtx.Constraints.Max.X
	return background(gtx, colHeader, 0, func(gtx layout.Context) layout.Dimensions {
		gtx.Constraints.Min.X = gtx.Constraints.Max.X
		return layout.Inset{Top: 14, Bottom: 14, Left: 48, Right: 48}.Layout(gtx, func(gtx layout.Context) layout.Dimensions {
			details := "Show details"
			if a.showDetails {
				details = "Hide details"
			}
			children := []layout.FlexChild{
				layout.Rigid(func(gtx layout.Context) layout.Dimensions {
					if st.err != "" {
						return layout.Dimensions{}
					}
					return button(a.th, gtx, &a.detailsBtn, details, false, true)
				}),
				spacer(),
			}
			switch {
			case st.done:
				children = append(children,
					layout.Rigid(func(gtx layout.Context) layout.Dimensions { return button(a.th, gtx, &a.quitBtn, "Quit", false, true) }),
					hspace(12),
					layout.Rigid(func(gtx layout.Context) layout.Dimensions {
						return button(a.th, gtx, &a.restartBtn, "Restart Now", true, true)
					}))
			case st.err != "":
				children = append(children,
					layout.Rigid(func(gtx layout.Context) layout.Dimensions { return button(a.th, gtx, &a.quitBtn, "Quit", false, true) }),
					hspace(12),
					layout.Rigid(func(gtx layout.Context) layout.Dimensions {
						return button(a.th, gtx, &a.doneBtn, "Back to Summary", true, true)
					}))
			default:
				children = append(children, layout.Rigid(label(a.th, 13, "Please keep the computer on while Zainium OS is being installed.", colDim, font.Normal).Layout))
			}
			return layout.Flex{Alignment: layout.Middle}.Layout(gtx, children...)
		})
	})
}
