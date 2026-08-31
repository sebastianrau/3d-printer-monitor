package printermonitor

import (
	"context"
	"testing"
	"time"

	"github.com/sebastianrau/3d-printer-monitor/pkg/config"
	"github.com/sebastianrau/3d-printer-monitor/pkg/messenger"
)

type fakePrinter struct{}

func (fakePrinter) Start(context.Context, ReportHandler) error      { return nil }
func (fakePrinter) Stop()                                           {}
func (fakePrinter) CaptureSnapshot(context.Context) ([]byte, error) { return []byte("jpeg"), nil }
func (fakePrinter) Diagnose(context.Context, time.Duration) ([]byte, error) {
	return []byte("jpeg"), nil
}

type noopMessenger struct{}

func (noopMessenger) SendImage(context.Context, []byte, string, string, string, int) error {
	return nil
}
func (noopMessenger) SendText(context.Context, string, string) error { return nil }

type progressMessenger struct{ noopMessenger }

func (progressMessenger) PublishPrintStatus(context.Context, string, messenger.PrintStatus, bool, bool) error {
	return nil
}

type blockingSnapshotPrinter struct{ fakePrinter }

func (blockingSnapshotPrinter) CaptureSnapshot(ctx context.Context) ([]byte, error) {
	<-ctx.Done()
	return nil, ctx.Err()
}

type notifyingProgressMessenger struct {
	noopMessenger
	delivered chan struct{}
}

func (m notifyingProgressMessenger) PublishPrintStatus(context.Context, string, messenger.PrintStatus, bool, bool) error {
	m.delivered <- struct{}{}
	return nil
}
func testMonitor(t *testing.T) *Monitor {
	t.Helper()
	p := config.Printer{Name: "Test", ID: "test", Type: "test", EventQueueSize: 10, DeliveryAttempts: 1, Notifications: map[string]bool{}}
	r, e := NewMonitor(p, fakePrinter{}, noopMessenger{})
	if e != nil {
		t.Fatal(e)
	}
	return r
}
func TestFirstActiveObservationIsBaseline(t *testing.T) {
	r := testMonitor(t)
	if e := r.Evaluate(map[string]any{"gcode_state": "RUNNING", "task_id": "one", "mc_percent": 60, "layer_num": 10}); e != nil {
		t.Fatal(e)
	}
	select {
	case e := <-r.events:
		t.Fatalf("unexpected event: %s", e.Key)
	default:
	}
}

func TestActiveBaselineRestartsProgressTracking(t *testing.T) {
	printer := config.Printer{Name: "P1S", ID: "p1s", EventQueueSize: 10, DeliveryAttempts: 1, Notifications: map[string]bool{}}
	monitor, err := NewMonitor(printer, fakePrinter{}, progressMessenger{})
	if err != nil {
		t.Fatal(err)
	}
	if err := monitor.Evaluate(map[string]any{"gcode_state": "RUNNING", "task_id": "job", "subtask_name": "Part", "mc_percent": 42}); err != nil {
		t.Fatal(err)
	}
	status := <-monitor.statusEvents
	if !status.immediate || status.status.Progress == nil || *status.status.Progress != 42 || status.status.State != "RUNNING" {
		t.Fatalf("restart status = %#v", status)
	}
}

func TestPartialActiveBaselineWaitsForJobIdentity(t *testing.T) {
	printer := config.Printer{Name: "P1S", ID: "p1s", EventQueueSize: 10, DeliveryAttempts: 1, Notifications: map[string]bool{}}
	monitor, err := NewMonitor(printer, fakePrinter{}, progressMessenger{})
	if err != nil {
		t.Fatal(err)
	}
	if err := monitor.Evaluate(map[string]any{"gcode_state": "RUNNING", "mc_percent": 0}); err != nil {
		t.Fatal(err)
	}
	select {
	case status := <-monitor.statusEvents:
		t.Fatalf("status created before task identity was known: %#v", status)
	default:
	}
	if err := monitor.Evaluate(map[string]any{"gcode_state": "RUNNING", "task_id": "job", "subtask_name": "Part", "mc_percent": 1}); err != nil {
		t.Fatal(err)
	}
	status := <-monitor.statusEvents
	if status.key != "p1s:job" || !status.immediate || status.status.Job != "Part" {
		t.Fatalf("first complete status = %#v", status)
	}
}

func TestBlockedSnapshotDoesNotBlockStatusDelivery(t *testing.T) {
	delivered := make(chan struct{}, 1)
	printer := config.Printer{Name: "P2S", ID: "p2s", EventQueueSize: 10, DeliveryAttempts: 1, Notifications: map[string]bool{}}
	monitor, err := NewMonitor(printer, blockingSnapshotPrinter{}, notifyingProgressMessenger{delivered: delivered})
	if err != nil {
		t.Fatal(err)
	}
	ctx, cancel := context.WithCancel(context.Background())
	monitor.wg.Add(2)
	go monitor.eventWorker(ctx)
	go monitor.statusWorker(ctx)
	t.Cleanup(func() {
		cancel()
		monitor.wg.Wait()
	})

	monitor.events <- Event{Key: "finished", Label: "99 % erreicht"}
	monitor.statusEvents <- statusEvent{key: "p2s:job", status: messenger.PrintStatus{State: "RUNNING"}}
	select {
	case <-delivered:
	case <-time.After(time.Second):
		t.Fatal("status delivery was blocked by snapshot capture")
	}
}
func TestMilestonesAndNoDuplicateFinished(t *testing.T) {
	r := testMonitor(t)
	_ = r.Evaluate(map[string]any{"gcode_state": "IDLE", "mc_percent": 0})
	_ = r.Evaluate(map[string]any{"gcode_state": "RUNNING", "task_id": "one", "mc_percent": 1})
	if e := <-r.events; e.Key != "started" {
		t.Fatalf("got %s", e.Key)
	}
	_ = r.Evaluate(map[string]any{"gcode_state": "RUNNING", "task_id": "one", "mc_percent": 98})
	if e := <-r.events; e.Key != "progress50" {
		t.Fatalf("got %s", e.Key)
	}
	_ = r.Evaluate(map[string]any{"gcode_state": "RUNNING", "task_id": "one", "mc_percent": 99})
	if e := <-r.events; e.Key != "finished" {
		t.Fatalf("got %s", e.Key)
	}
	_ = r.Evaluate(map[string]any{"gcode_state": "FINISH", "task_id": "one", "mc_percent": 100})
	select {
	case e := <-r.events:
		t.Fatalf("duplicate event: %s", e.Key)
	default:
	}
}
func TestLayerAndPauseTransitions(t *testing.T) {
	r := testMonitor(t)
	_ = r.Evaluate(map[string]any{"gcode_state": "IDLE"})
	_ = r.Evaluate(map[string]any{"gcode_state": "RUNNING", "task_id": "one", "mc_percent": 1})
	<-r.events
	_ = r.Evaluate(map[string]any{"gcode_state": "RUNNING", "layer_num": 2, "mc_percent": 2})
	if e := <-r.events; e.Key != "layer1" {
		t.Fatalf("got %s", e.Key)
	}
	_ = r.Evaluate(map[string]any{"gcode_state": "PAUSE", "mc_percent": 3})
	if e := <-r.events; e.Key != "pause" {
		t.Fatalf("got %s", e.Key)
	}
	_ = r.Evaluate(map[string]any{"gcode_state": "RUNNING", "mc_percent": 4})
	_ = r.Evaluate(map[string]any{"gcode_state": "PAUSE", "mc_percent": 5})
	if e := <-r.events; e.Key != "pause" {
		t.Fatalf("got %s", e.Key)
	}
}

func TestNewJobStatusDoesNotReusePreviousJobProgress(t *testing.T) {
	printer := config.Printer{Name: "P1S", ID: "p1s", EventQueueSize: 10, DeliveryAttempts: 1, Notifications: map[string]bool{}}
	monitor, err := NewMonitor(printer, fakePrinter{}, progressMessenger{})
	if err != nil {
		t.Fatal(err)
	}
	_ = monitor.Evaluate(map[string]any{"gcode_state": "FINISH", "task_id": "old", "mc_percent": 100, "layer_num": 30, "total_layer_num": 30})
	_ = monitor.Evaluate(map[string]any{"gcode_state": "PREPARE", "task_id": "new", "subtask_name": "Next part", "mc_percent": 100, "layer_num": 30, "total_layer_num": 30, "mc_remaining_time": 0})
	status := <-monitor.statusEvents
	if status.status.Progress == nil || *status.status.Progress != 0 {
		t.Fatalf("start progress = %v, want 0", status.status.Progress)
	}
	if status.status.Layer != nil || status.status.TotalLayers != nil || status.status.RemainingMinutes != nil {
		t.Fatalf("start status reused stale job details: %#v", status.status)
	}
}
