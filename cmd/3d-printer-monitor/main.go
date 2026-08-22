package main

import (
	"context"
	"flag"
	"fmt"
	"os"
	"os/signal"
	"strconv"
	"strings"
	"syscall"
	"time"

	"github.com/sebastianrau/3d-printer-monitor/pkg/config"
	"github.com/sebastianrau/3d-printer-monitor/pkg/logger"
	"github.com/sebastianrau/3d-printer-monitor/pkg/messenger"
	messengerregistry "github.com/sebastianrau/3d-printer-monitor/pkg/messenger/registry"
	"github.com/sebastianrau/3d-printer-monitor/pkg/printermonitor"
	printerregistry "github.com/sebastianrau/3d-printer-monitor/pkg/printermonitor/registry"
)

var log = logger.WithPackage("main")

func main() {
	if err := run(); err != nil {
		log.Errorf("3d-printer-monitor stopped: %v", err)
		os.Exit(1)
	}
}

func run() error {
	options, err := parseOptions()
	if err != nil {
		return err
	}
	c, err := config.Load(options.configPath, options.requiresMessenger(), options.requiresChatID())
	if err != nil {
		return err
	}
	if err := logger.Configure(c.LogLevel); err != nil {
		return err
	}
	if handled, err := handleOneShotMode(context.Background(), c, options); handled {
		return err
	}
	return runMonitoring(c)
}

type options struct {
	configPath         string
	testPrinter        string
	testTimeout        time.Duration
	findTelegramChatID bool
	telegramWait       time.Duration
	telegramIncludeOld bool
}

func parseOptions() (options, error) {
	var result options
	var testTimeout = secondsFlag(10 * time.Second)
	var telegramWait = secondsFlag(30 * time.Second)
	flag.StringVar(&result.configPath, "config", "/etc/3d-printer-monitor/config.yaml", "YAML config file")
	flag.StringVar(&result.testPrinter, "test-printer", "", "diagnose one enabled printer by its configured name, then exit")
	flag.Var(&testTimeout, "test-timeout", "timeout in seconds for each connection test")
	flag.BoolVar(&result.findTelegramChatID, "find-telegram-chat-id", false, "wait for /id and report the Telegram chat ID")
	flag.Var(&telegramWait, "telegram-wait", "seconds to wait for /id")
	flag.BoolVar(&result.telegramIncludeOld, "telegram-include-old", false, "include pending updates when finding chat IDs")
	flag.Parse()
	result.testPrinter = strings.TrimSpace(result.testPrinter)
	result.testTimeout = time.Duration(testTimeout)
	result.telegramWait = time.Duration(telegramWait)
	return result, result.validate()
}

func (o options) validate() error {
	if o.testPrinter != "" && o.findTelegramChatID {
		return fmt.Errorf("--test-printer and --find-telegram-chat-id are mutually exclusive")
	}
	if o.testTimeout <= 0 {
		return fmt.Errorf("--test-timeout must be greater than zero")
	}
	if o.telegramWait < 0 {
		return fmt.Errorf("--telegram-wait must not be negative")
	}
	return nil
}

func (o options) requiresMessenger() bool { return o.testPrinter == "" }
func (o options) requiresChatID() bool    { return !o.findTelegramChatID }

type secondsFlag time.Duration

func (s *secondsFlag) String() string { return time.Duration(*s).String() }
func (s *secondsFlag) Set(raw string) error {
	if strings.ContainsAny(raw, "hmsµns") {
		duration, err := time.ParseDuration(raw)
		if err != nil {
			return err
		}
		*s = secondsFlag(duration)
		return nil
	}
	seconds, err := strconv.ParseFloat(raw, 64)
	if err != nil {
		return err
	}
	*s = secondsFlag(time.Duration(seconds * float64(time.Second)))
	return nil
}

func handleOneShotMode(ctx context.Context, c config.Config, options options) (bool, error) {
	switch {
	case options.testPrinter != "":
		return true, testPrinter(ctx, c, options)
	case options.findTelegramChatID:
		return true, findTelegramChatID(ctx, c, options)
	default:
		return false, nil
	}
}

func testPrinter(ctx context.Context, c config.Config, options options) error {
	return printerregistry.New().Diagnose(ctx, c.Printers, options.testPrinter, options.testTimeout)
}

func findTelegramChatID(ctx context.Context, c config.Config, options options) error {
	service, err := messengerregistry.New().Create(c.Messaging)
	if err != nil {
		return err
	}
	discoverer, ok := service.(messenger.TargetDiscoverer)
	if !ok {
		return fmt.Errorf("messaging provider %q does not support target discovery", c.Messaging.Provider)
	}
	found, err := discoverer.FindTargets(ctx, options.telegramWait, options.telegramIncludeOld)
	if err != nil {
		return err
	}
	if !found {
		return fmt.Errorf("no /id command found or reply failed")
	}
	return nil
}

func runMonitoring(c config.Config) error {
	ctx, stop := signal.NotifyContext(context.Background(), os.Interrupt, syscall.SIGTERM)
	defer stop()
	messageService, err := messengerregistry.New().Create(c.Messaging)
	if err != nil {
		return err
	}
	if err := messageService.Validate(); err != nil {
		return err
	}
	registry := printerregistry.New()
	var monitors []*printermonitor.Monitor
	var selectable []messenger.SnapshotSource

	for _, printerConfig := range c.Printers {
		if !printerConfig.IsEnabled() {
			continue
		}
		implementation, err := registry.Create(printerConfig)
		if err != nil {
			stopMonitors(monitors)
			return err
		}
		monitor, err := printermonitor.NewMonitor(printerConfig, implementation, messageService)
		if err != nil {
			stopMonitors(monitors)
			return err
		}
		if err := monitor.Run(ctx); err != nil {
			stopMonitors(monitors)
			return fmt.Errorf("start %s: %w", printerConfig.DisplayName(), err)
		}
		monitors = append(monitors, monitor)
		selectable = append(selectable, monitor)
	}

	go messageService.Run(ctx, selectable)
	log.Infof("monitoring %d printer(s)", len(monitors))
	<-ctx.Done()
	stopMonitors(monitors)
	return nil
}

func stopMonitors(monitors []*printermonitor.Monitor) {
	for _, monitor := range monitors {
		monitor.Stop()
	}
}
