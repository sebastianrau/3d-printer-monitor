package printermonitor

import (
	"context"
	"fmt"
	"strconv"
	"strings"
	"sync"
	"time"

	"github.com/sebastianrau/3d-printer-monitor/pkg/config"
	"github.com/sebastianrau/3d-printer-monitor/pkg/logger"
	"github.com/sebastianrau/3d-printer-monitor/pkg/messenger"
)

var log = logger.WithPackage("printer-monitor")

type Event struct {
	Key, Label   string
	Progress     int
	Layer, Total *int
	Flag         string
}
type statusEvent struct {
	key                 string
	status              messenger.PrintStatus
	immediate, terminal bool
}
type Monitor struct {
	Config       config.Printer
	Printer      Printer
	Messenger    messenger.Messenger
	mu           sync.Mutex
	printState   map[string]any
	stateMu      sync.Mutex
	state        map[string]any
	initialized  bool
	events       chan Event
	statusEvents chan statusEvent
	progressKey  string
	cancel       context.CancelFunc
	wg           sync.WaitGroup
}

func NewMonitor(c config.Printer, p Printer, m messenger.Messenger) (*Monitor, error) {
	if p == nil {
		return nil, fmt.Errorf("printer implementation is required")
	}
	return &Monitor{Config: c, Printer: p, Messenger: m, printState: map[string]any{}, state: map[string]any{}, events: make(chan Event, c.EventQueueSize), statusEvents: make(chan statusEvent, c.EventQueueSize)}, nil
}
func (r *Monitor) Name() string       { return r.Config.DisplayName() }
func (r *Monitor) Identifier() string { return r.Config.Identifier() }

func (r *Monitor) Run(parent context.Context) error {
	ctx, cancel := context.WithCancel(parent)
	r.cancel = cancel
	r.wg.Add(1)
	go r.worker(ctx)
	err := r.Printer.Start(ctx, func(report map[string]any) {
		r.mu.Lock()
		defer r.mu.Unlock()
		r.printState = cloneMap(report)
		if err := r.Evaluate(report); err != nil {
			log.Errorf("[%s] state evaluation failed: %v", r.Name(), err)
		}
	})
	if err != nil {
		cancel()
		r.wg.Wait()
		return err
	}
	return nil
}

func (r *Monitor) Stop() {
	if r.cancel != nil {
		r.cancel()
	}
	r.Printer.Stop()
	r.wg.Wait()
}

func (r *Monitor) Evaluate(p map[string]any) error {
	gstate := strings.ToUpper(asString(p["gcode_state"]))
	progress := asInt(p["mc_percent"])
	layer := asInt(p["layer_num"])
	total := asInt(p["total_layer_num"])
	task := firstString(p, "task_id", "subtask_id", "gcode_file", "subtask_name")
	if gstate == "" && progress == nil && layer == nil {
		return nil
	}
	if !r.initialized {
		baseline := map[string]any{
			"task_id":          task,
			"last_progress":    value(progress),
			"last_gcode_state": gstate,
			"started_sent":     oneOf(gstate, "PREPARE", "RUNNING", "PAUSE"),
			"layer1_sent":      layer != nil && *layer >= 2,
			"progress50_sent":  progress != nil && *progress >= 50,
			"finished_sent":    gstate == "FINISH" || progress != nil && *progress >= 99,
			"pause_sent":       gstate == "PAUSE",
			"failed_sent":      gstate == "FAILED",
		}
		r.updateState(baseline)
		r.initialized = true
		if oneOf(gstate, "PREPARE", "RUNNING", "PAUSE") {
			r.progressKey = r.Identifier() + ":" + task
			r.enqueueStatus(statusEvent{
				key: r.progressKey,
				status: messenger.PrintStatus{
					Printer: r.Name(), Job: firstString(p, "subtask_name", "gcode_file", "task_id", "subtask_id"), State: gstate, Stage: firstString(p, "stage_name", "stage"),
					Progress: progress, Layer: layer, TotalLayers: total,
					RemainingMinutes:  asInt(firstValue(p, "mc_remaining_time", "remaining_time")),
					NozzleTemperature: asFloat(firstValue(p, "nozzle_temper", "nozzle_temperature")),
					BedTemperature:    asFloat(firstValue(p, "bed_temper", "bed_temperature")),
				},
				immediate: true,
			})
		}
		return nil
	}
	persisted := r.currentState()
	previousTask := asString(persisted["task_id"])
	previousProgress := asInt(persisted["last_progress"])
	hadHistory := persisted["last_gcode_state"] != nil || persisted["task_id"] != nil || persisted["last_progress"] != nil
	newJob := (task != "" && previousTask != "" && task != previousTask) || (task != "" && previousTask == "" && oneOf(gstate, "RUNNING", "PREPARE", "PAUSE")) || (progress != nil && *progress <= 2 && intValue(persisted["last_progress"]) > 10 && oneOf(gstate, "RUNNING", "PREPARE"))
	if newJob {
		r.updateState(map[string]any{"task_id": task, "started_sent": false, "layer1_sent": false, "progress50_sent": false, "finished_sent": false, "pause_sent": false, "failed_sent": false, "last_progress": value(progress), "last_gcode_state": ""})
		persisted = r.currentState()
		r.progressKey = r.Identifier() + ":" + task
	}
	if task != "" && asString(persisted["task_id"]) != task {
		r.updateState(map[string]any{"task_id": task})
	}
	if progress != nil {
		r.updateState(map[string]any{"last_progress": *progress})
	}
	previousState := strings.ToUpper(asString(persisted["last_gcode_state"]))
	if r.progressKey != "" {
		statusState := gstate
		statusProgress, statusLayer, statusTotal := progress, layer, total
		statusRemaining := asInt(firstValue(p, "mc_remaining_time", "remaining_time"))
		if newJob {
			statusState = "STARTED"
			// Bambu can announce the new task before replacing the previous
			// task's final progress, layer, and remaining-time values.
			zero := 0
			statusProgress, statusLayer, statusTotal, statusRemaining = &zero, nil, nil, nil
		} else if gstate == "FAILED" {
			if code := asString(firstValue(p, "print_error", "error_code")); code != "" && code != "0" {
				statusState = "ERROR"
			} else {
				statusState = "ABORTED"
			}
		}
		terminal := oneOf(statusState, "FINISH", "FAILED", "ABORTED", "ERROR")
		r.enqueueStatus(statusEvent{
			key: r.progressKey,
			status: messenger.PrintStatus{
				Printer: r.Name(), Job: firstString(p, "subtask_name", "gcode_file", "task_id", "subtask_id"), State: statusState, Stage: firstString(p, "stage_name", "stage"),
				Progress: statusProgress, Layer: statusLayer, TotalLayers: statusTotal,
				RemainingMinutes:  statusRemaining,
				NozzleTemperature: asFloat(firstValue(p, "nozzle_temper", "nozzle_temperature")),
				BedTemperature:    asFloat(firstValue(p, "bed_temper", "bed_temperature")),
			},
			immediate: newJob || gstate != previousState,
			terminal:  terminal,
		})
		if terminal {
			r.progressKey = ""
		}
	}
	if previousState == "" && oneOf(gstate, "IDLE", "FINISH", "FAILED", "PAUSE") {
		v := map[string]any{"last_gcode_state": gstate}
		if gstate == "FINISH" {
			v["finished_sent"] = true
		}
		if gstate == "FAILED" {
			v["failed_sent"] = true
		}
		if gstate == "PAUSE" {
			v["pause_sent"] = true
		}
		r.updateState(v)
		return nil
	}
	if gstate != "" && gstate != previousState {
		r.updateState(map[string]any{"last_gcode_state": gstate})
	}
	persisted = r.currentState()
	if newJob && hadHistory && r.Config.Notify("started") && oneOf(gstate, "PREPARE", "RUNNING") && !asBool(persisted["started_sent"]) {
		return r.fire(Event{"started", "Druck gestartet", value(progress), layer, total, "started_sent"})
	}
	if r.Config.Notify("finished") && progress != nil && *progress >= 99 && previousProgress != nil && *previousProgress < 99 && !asBool(persisted["finished_sent"]) {
		return r.fire(Event{"finished", "99 % erreicht", *progress, layer, total, "finished_sent"})
	}
	if r.Config.Notify("layer1") && layer != nil && *layer >= 2 && !asBool(persisted["layer1_sent"]) && oneOf(gstate, "RUNNING", "PAUSE") {
		return r.fire(Event{"layer1", "Layer 1 fertig", value(progress), layer, total, "layer1_sent"})
	}
	if r.Config.Notify("progress50") && progress != nil && *progress >= 50 && !asBool(persisted["progress50_sent"]) && !asBool(persisted["finished_sent"]) && oneOf(gstate, "RUNNING", "PAUSE") {
		return r.fire(Event{"progress50", "50 % erreicht", *progress, layer, total, "progress50_sent"})
	}
	if gstate == "RUNNING" && previousState == "PAUSE" {
		r.updateState(map[string]any{"pause_sent": false})
		persisted = r.currentState()
	}
	if r.Config.Notify("pause") && gstate == "PAUSE" && previousState != "PAUSE" && !asBool(persisted["pause_sent"]) {
		return r.fire(Event{"pause", "Druck pausiert", value(progress), layer, total, "pause_sent"})
	}
	if r.Config.Notify("failed") && gstate == "FAILED" && !asBool(persisted["failed_sent"]) {
		return r.fire(Event{"failed", "Druck abgebrochen/fehlgeschlagen", value(progress), layer, total, "failed_sent"})
	}
	return nil
}

func (r *Monitor) fire(e Event) error {
	r.updateState(map[string]any{e.Flag: true})
	select {
	case r.events <- e:
		return nil
	default:
		r.updateState(map[string]any{e.Flag: false})
		return fmt.Errorf("event queue full, dropping %s", e.Label)
	}
}
func (r *Monitor) RequestManualSnapshot() bool {
	r.mu.Lock()
	progress := value(asInt(r.printState["mc_percent"]))
	layer := asInt(r.printState["layer_num"])
	total := asInt(r.printState["total_layer_num"])
	r.mu.Unlock()
	select {
	case r.events <- Event{"manual", "Manueller Snapshot", progress, layer, total, ""}:
		return true
	default:
		return false
	}
}
func (r *Monitor) worker(ctx context.Context) {
	defer r.wg.Done()
	for {
		select {
		case <-ctx.Done():
			return
		case e := <-r.events:
			r.deliver(ctx, e)
		case e := <-r.statusEvents:
			r.deliverStatus(ctx, e)
		}
	}
}

func (r *Monitor) deliverStatus(ctx context.Context, event statusEvent) {
	progress, ok := r.Messenger.(messenger.ProgressMessenger)
	if !ok {
		return
	}
	for attempt := 1; attempt <= r.Config.DeliveryAttempts; attempt++ {
		if err := progress.PublishPrintStatus(ctx, event.key, event.status, event.immediate, event.terminal); err == nil {
			return
		} else {
			log.Errorf("[%s] progress status delivery failed (attempt %d/%d): %v", r.Name(), attempt, r.Config.DeliveryAttempts, err)
		}
		if attempt < r.Config.DeliveryAttempts {
			delay := time.Duration(r.Config.DeliveryRetrySeconds*float64(time.Second)) * time.Duration(1<<(attempt-1))
			select {
			case <-ctx.Done():
				return
			case <-time.After(delay):
			}
		}
	}
}

func (r *Monitor) enqueueStatus(event statusEvent) {
	if _, ok := r.Messenger.(messenger.ProgressMessenger); !ok {
		return
	}
	select {
	case r.statusEvents <- event:
	default:
		log.Warnf("[%s] progress update queue full", r.Name())
	}
}
func (r *Monitor) deliver(ctx context.Context, e Event) {
	safe := strings.Map(func(c rune) rune {
		if c >= 'a' && c <= 'z' || c >= 'A' && c <= 'Z' || c >= '0' && c <= '9' || c == '-' || c == '_' {
			return c
		}
		return '_'
	}, r.Name())
	filename := safe + "-" + time.Now().Format("20060102-150405") + "-" + e.Key + ".jpg"
	var err error
	for attempt := 1; attempt <= r.Config.DeliveryAttempts; attempt++ {
		var image []byte
		if image, err = r.Printer.CaptureSnapshot(ctx); err == nil {
			err = r.Messenger.SendImage(ctx, image, filename, r.Name(), e.Label, e.Progress)
		}
		if err == nil {
			log.Infof("[%s] notification sent: %s", r.Name(), e.Label)
			return
		}
		log.Errorf("[%s] milestone delivery failed: %s (attempt %d/%d): %v", r.Name(), e.Label, attempt, r.Config.DeliveryAttempts, err)
		if attempt < r.Config.DeliveryAttempts {
			select {
			case <-ctx.Done():
				return
			case <-time.After(time.Duration(r.Config.DeliveryRetrySeconds*float64(time.Second)) * time.Duration(1<<(attempt-1))):
			}
		}
	}
	if e.Flag != "" {
		r.updateState(map[string]any{e.Flag: false})
	}
}

func asInt(v any) *int {
	switch n := v.(type) {
	case int:
		return &n
	case float64:
		x := int(n)
		return &x
	case string:
		x, e := strconv.Atoi(n)
		if e == nil {
			return &x
		}
	}
	return nil
}
func value(v *int) int {
	if v == nil {
		return 0
	}
	return *v
}
func intValue(v any) int { return value(asInt(v)) }
func asString(v any) string {
	if v == nil {
		return ""
	}
	return fmt.Sprint(v)
}
func asBool(v any) bool { x, _ := v.(bool); return x }
func oneOf(s string, v ...string) bool {
	for _, x := range v {
		if s == x {
			return true
		}
	}
	return false
}
func firstString(m map[string]any, ks ...string) string {
	for _, k := range ks {
		if s := asString(m[k]); s != "" {
			return s
		}
	}
	return ""
}
func firstValue(m map[string]any, keys ...string) any {
	for _, key := range keys {
		if m[key] != nil {
			return m[key]
		}
	}
	return nil
}
func asFloat(v any) *float64 {
	switch n := v.(type) {
	case float64:
		return &n
	case float32:
		value := float64(n)
		return &value
	case int:
		value := float64(n)
		return &value
	case string:
		value, err := strconv.ParseFloat(n, 64)
		if err == nil {
			return &value
		}
	}
	return nil
}
func cloneMap(in map[string]any) map[string]any {
	out := make(map[string]any, len(in))
	for k, v := range in {
		out[k] = v
	}
	return out
}

func (r *Monitor) currentState() map[string]any {
	r.stateMu.Lock()
	defer r.stateMu.Unlock()
	return cloneMap(r.state)
}

func (r *Monitor) updateState(values map[string]any) {
	r.stateMu.Lock()
	defer r.stateMu.Unlock()
	for key, value := range values {
		r.state[key] = value
	}
}
