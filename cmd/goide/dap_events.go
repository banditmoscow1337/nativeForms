package goide

import (
	"encoding/json"
	"fmt"
	"sort"
	"strings"
)

func (d *debugSession) receive(data json.RawMessage) {
	if d.app == nil || d.app.debug != d {
		return
	}
	var envelope struct {
		Type       string          `json:"type"`
		Event      string          `json:"event"`
		Command    string          `json:"command"`
		RequestSeq int             `json:"request_seq"`
		Success    bool            `json:"success"`
		Message    string          `json:"message"`
		Body       json.RawMessage `json:"body"`
	}
	if json.Unmarshal(data, &envelope) != nil {
		return
	}
	switch envelope.Type {
	case "response":
		callback := d.pending[envelope.RequestSeq]
		delete(d.pending, envelope.RequestSeq)
		if callback != nil {
			if !envelope.Success {
				detail := envelope.Message
				var errBody struct {
					Error struct {
						Format string `json:"format"`
					} `json:"error"`
				}
				if len(envelope.Body) > 0 && json.Unmarshal(envelope.Body, &errBody) == nil && errBody.Error.Format != "" {
					detail = errBody.Error.Format
				}
				callback(nil, fmt.Errorf("%s: %s", envelope.Command, detail))
			} else {
				callback(envelope.Body, nil)
			}
		}
	case "event":
		switch envelope.Event {
		case "initialized":
			d.initializedReceived = true
			if d.launchSent || d.programDir == "" {
				d.configureAndStart()
			}
		case "progressStart", "progressUpdate":
			var body struct {
				Title   string `json:"title"`
				Message string `json:"message"`
			}
			if json.Unmarshal(envelope.Body, &body) == nil {
				txt := strings.TrimSpace(body.Title + " " + body.Message)
				if txt != "" {
					d.app.setTaskBusy(true, "Delve: "+txt)
					d.app.appendLogf("debug", "[Delve] %s\n", txt)
				}
			}
		case "progressEnd":
			d.app.setTaskBusy(false, "")
		case "stopped":
			d.app.setTaskBusy(false, "")
			var body struct {
				Reason   string `json:"reason"`
				ThreadID int    `json:"threadId"`
			}
			if json.Unmarshal(envelope.Body, &body) == nil {
				reason := nonEmptyStr(body.Reason, "breakpoint")
				d.app.setStatus("Отладка остановлена: " + reason + " (F10 — шаг, F11 — внутрь, F5 — продолжить)")
				d.app.updateDebugBadge("Debug: paused ("+reason+")", true)
				d.app.appendLogf("debug", "[Delve] Остановка (%s, поток #%d)\n", reason, body.ThreadID)
				d.fetchStack(body.ThreadID)
			}
		case "continued":
			d.scopeGeneration++
			d.threadID = 0
			d.app.clearAllDebugLines()
			d.app.stack.rows = nil
			d.app.variables.rows = nil
			if d.app.stackView != nil {
				d.app.stackView.ModelChanged()
			}
			if d.app.variablesView != nil {
				d.app.variablesView.ModelChanged()
			}
			d.app.updateDebugBadge("Debug: running", true)
		case "output":
			var body struct {
				Category string `json:"category"`
				Output   string `json:"output"`
			}
			if json.Unmarshal(envelope.Body, &body) == nil && body.Output != "" {
				d.app.output.AppendText(body.Output)
				if d.app.debugOutput != nil {
					d.app.debugOutput.AppendText(body.Output)
				}
			}
		case "terminated", "exited":
			d.app.setTaskBusy(false, "")
			d.app.clearAllDebugLines()
			d.app.appendLogf("debug", "[Delve] Сессия отладки завершена.\n")
			d.app.setStatus("Отладка завершена")
			d.app.updateDebugBadge("Debug: idle", false)
		}
	}
}

func (d *debugSession) setBreakpoints(path string) {
	d.setBreakpointsWithCallback(path, nil)
}

func (d *debugSession) setBreakpointsWithCallback(path string, done func()) {
	lineSet := make(map[int]bool)
	for line := range d.app.breakpoints[path] {
		lineSet[line+1] = true
	}
	if d.autoEntryBPPath == path && d.autoEntryBPLine >= 0 {
		lineSet[d.autoEntryBPLine+1] = true
	}
	lines := make([]int, 0, len(lineSet))
	for line := range lineSet {
		lines = append(lines, line)
	}
	sort.Ints(lines)
	points := make([]map[string]int, 0, len(lines))
	for _, line := range lines {
		points = append(points, map[string]int{"line": line})
	}
	d.request("setBreakpoints", map[string]any{
		"source":      map[string]any{"path": path, "name": fileName(path)},
		"breakpoints": points,
	}, func(_ json.RawMessage, err error) {
		if d.app != nil && d.app.debug == d && err != nil {
			d.app.setStatus("Breakpoint: " + err.Error())
		}
		if done != nil {
			done()
		}
	})
}

func (d *debugSession) control(command string) {
	if d.threadID == 0 {
		d.app.setStatus("Нет остановленного потока")
		return
	}
	// If we are resuming from the automatic entry breakpoint at func main(),
	// clear the temporary entry breakpoint first so the program can continue freely.
	if command == "continue" && d.autoEntryBPPath != "" && d.autoEntryBPLine >= 0 {
		autoPath := d.autoEntryBPPath
		d.autoEntryBPPath = ""
		d.autoEntryBPLine = -1
		d.setBreakpointsWithCallback(autoPath, func() {
			if d.app != nil && d.app.debug == d {
				d.request(command, map[string]any{"threadId": d.threadID}, func(_ json.RawMessage, err error) {
					if d.app != nil && d.app.debug == d && err != nil {
						d.app.setStatus("dlv: " + err.Error())
					}
				})
			}
		})
		return
	}
	d.request(command, map[string]any{"threadId": d.threadID}, func(_ json.RawMessage, err error) {
		if d.app.debug == d && err != nil {
			d.app.setStatus("dlv: " + err.Error())
		}
	})
}

func (d *debugSession) evaluate(expr string) {
	expr = strings.TrimSpace(expr)
	if expr == "" {
		return
	}
	if d.app.debugOutput != nil {
		d.app.debugOutput.AppendText("dlv> " + expr + "\n")
	}
	args := map[string]any{
		"expression": expr,
		"context":    "repl",
	}
	if d.activeFrameID > 0 {
		args["frameId"] = d.activeFrameID
	}
	d.request("evaluate", args, func(result json.RawMessage, err error) {
		if d.app.debug != d {
			return
		}
		if err != nil {
			msg := fmt.Sprintf("Ошибка вычисления: %v\n", err)
			d.app.output.AppendText(msg)
			if d.app.debugOutput != nil {
				d.app.debugOutput.AppendText(msg)
			}
			return
		}
		var body struct {
			Result string `json:"result"`
			Type   string `json:"type"`
		}
		if json.Unmarshal(result, &body) == nil {
			line := fmt.Sprintf("=> %s (%s)\n", body.Result, nonEmptyStr(body.Type, "value"))
			d.app.output.AppendText(line)
			if d.app.debugOutput != nil {
				d.app.debugOutput.AppendText(line)
			}
		}
	})
}

func (d *debugSession) fetchStack(thread int) {
	if thread == 0 {
		d.request("threads", map[string]any{}, func(result json.RawMessage, err error) {
			if err != nil || d.app.debug != d {
				return
			}
			var body struct {
				Threads []struct {
					ID int `json:"id"`
				} `json:"threads"`
			}
			if json.Unmarshal(result, &body) == nil && len(body.Threads) > 0 {
				d.fetchStack(body.Threads[0].ID)
			}
		})
		return
	}
	d.threadID = thread
	d.request("stackTrace", map[string]any{"threadId": thread, "startFrame": 0, "levels": 40}, func(result json.RawMessage, err error) {
		if err != nil || d.app.debug != d {
			return
		}
		var body struct {
			Frames []struct {
				ID     int    `json:"id"`
				Name   string `json:"name"`
				Line   int    `json:"line"`
				Source struct {
					Path string `json:"path"`
				} `json:"source"`
			} `json:"stackFrames"`
		}
		if json.Unmarshal(result, &body) != nil {
			return
		}
		d.app.stack.rows = nil
		for _, f := range body.Frames {
			d.app.stack.rows = append(d.app.stack.rows, stackRow{
				id:   f.ID,
				name: f.Name,
				path: f.Source.Path,
				line: f.Line,
			})
		}
		if d.app.stackView != nil {
			d.app.stackView.ModelChanged()
		}
		if len(d.app.stack.rows) > 0 {
			chosen := d.app.stack.rows[0]
			for _, row := range d.app.stack.rows {
				if row.path != "" && within(d.app.project.root, row.path) {
					chosen = row
					break
				}
			}
			d.activeFrameID = chosen.id
			if d.app.stackView != nil {
				d.app.stackView.SelectedID = fmt.Sprint(chosen.id)
				d.app.stackView.UIState().InvalidatePaint()
			}
			d.app.showFrame(chosen)
			d.fetchScopes(chosen.id)
		}
	})
}

func (d *debugSession) fetchScopes(frameID int) {
	d.activeFrameID = frameID
	d.scopeGeneration++
	generation := d.scopeGeneration
	d.app.variables.rows = nil
	if d.app.variablesView != nil {
		d.app.variablesView.ModelChanged()
	}
	d.request("scopes", map[string]any{"frameId": frameID}, func(result json.RawMessage, err error) {
		if err != nil || d.app.debug != d || d.scopeGeneration != generation {
			return
		}
		var body struct {
			Scopes []struct {
				Name               string `json:"name"`
				VariablesReference int    `json:"variablesReference"`
			} `json:"scopes"`
		}
		if json.Unmarshal(result, &body) != nil {
			return
		}
		for _, scope := range body.Scopes {
			if scope.VariablesReference == 0 {
				continue
			}
			name := scope.Name
			d.request("variables", map[string]any{"variablesReference": scope.VariablesReference}, func(result json.RawMessage, err error) {
				if err != nil || d.app.debug != d || d.scopeGeneration != generation {
					return
				}
				var vars struct {
					Variables []struct {
						Name  string `json:"name"`
						Value string `json:"value"`
						Type  string `json:"type"`
					} `json:"variables"`
				}
				if json.Unmarshal(result, &vars) != nil {
					return
				}
				for _, v := range vars.Variables {
					d.app.variables.rows = append(d.app.variables.rows, variableRow{
						id:    fmt.Sprint(len(d.app.variables.rows)),
						name:  name + " · " + v.Name,
						value: v.Value,
						kind:  v.Type,
					})
				}
				if d.app.variablesView != nil {
					d.app.variablesView.ModelChanged()
				}
			})
		}
	})
}
