// Package bambu contains communication infrastructure shared by Bambu printers.
package bambu

import (
	"context"
	"crypto/tls"
	"fmt"
	"os"
	"sync"
	"time"

	mqtt "github.com/eclipse/paho.mqtt.golang"
	"github.com/sebastianrau/3d-printer-monitor/pkg/config"
	"github.com/sebastianrau/3d-printer-monitor/pkg/logger"
	"github.com/sebastianrau/3d-printer-monitor/pkg/printermonitor"
)

var log = logger.WithPackage("bambu-mqtt")

// Protocol supplies model-specific topics, initial requests, and report decoding.
// MQTT connection management remains entirely inside the Bambu layer.
type Protocol interface {
	ReportTopic() string
	RequestTopic() string
	InitialRequest() []byte
	DecodeReport([]byte, map[string]any) (map[string]any, error)
}

type MQTTConnection struct {
	config   config.Printer
	settings config.BambuPrinter
	protocol Protocol
	mu       sync.Mutex
	state    map[string]any
	client   mqtt.Client
}

func NewMQTTConnection(c config.Printer, settings config.BambuPrinter, protocol Protocol) *MQTTConnection {
	return &MQTTConnection{config: c, settings: settings, protocol: protocol, state: map[string]any{}}
}

func (m *MQTTConnection) Start(ctx context.Context, handler printermonitor.ReportHandler) error {
	o := mqtt.NewClientOptions().SetClientID(fmt.Sprintf("bambu-go-%s-%d", tail(m.settings.Serial, 8), os.Getpid())).SetOrderMatters(false)
	o.AddBroker(fmt.Sprintf("ssl://%s:8883", m.settings.Host))
	o.SetUsername("bblp")
	o.SetPassword(m.settings.AccessCode)
	o.SetTLSConfig(&tls.Config{InsecureSkipVerify: true})
	o.SetKeepAlive(30 * time.Second)
	o.SetConnectRetry(true)
	o.SetConnectRetryInterval(time.Duration(m.settings.MQTTReconnectMinSeconds) * time.Second)
	o.SetAutoReconnect(true)
	o.SetMaxReconnectInterval(time.Duration(m.settings.MQTTReconnectMaxSeconds) * time.Second)
	o.SetOnConnectHandler(func(c mqtt.Client) {
		log.Infof("[%s] MQTT connected", m.config.DisplayName())
		if token := c.Subscribe(m.protocol.ReportTopic(), 0, nil); token.Wait() && token.Error() != nil {
			log.Errorf("[%s] MQTT subscribe failed: %v", m.config.DisplayName(), token.Error())
			return
		}
		token := c.Publish(m.protocol.RequestTopic(), 0, false, m.protocol.InitialRequest())
		token.Wait()
		if token.Error() != nil {
			log.Errorf("[%s] MQTT initial request failed: %v", m.config.DisplayName(), token.Error())
		}
	})
	o.SetConnectionLostHandler(func(_ mqtt.Client, err error) {
		if ctx.Err() == nil {
			log.Warnf("[%s] MQTT disconnected; reconnecting automatically: %v", m.config.DisplayName(), err)
		}
	})
	o.SetDefaultPublishHandler(func(_ mqtt.Client, message mqtt.Message) {
		m.mu.Lock()
		report, err := m.protocol.DecodeReport(message.Payload(), m.state)
		m.mu.Unlock()
		if err != nil {
			log.Errorf("[%s] MQTT report decode failed: %v", m.config.DisplayName(), err)
			return
		}
		handler(report)
	})
	m.client = mqtt.NewClient(o)
	token := m.client.Connect()
	select {
	case <-ctx.Done():
		m.Stop()
		return ctx.Err()
	case <-token.Done():
	}
	if token.Error() != nil {
		return token.Error()
	}
	go func() { <-ctx.Done(); m.Stop() }()
	return nil
}

func (m *MQTTConnection) Stop() {
	m.mu.Lock()
	client := m.client
	m.client = nil
	m.mu.Unlock()
	if client != nil && client.IsConnected() {
		client.Disconnect(500)
	}
}

func tail(s string, n int) string {
	if len(s) > n {
		return s[len(s)-n:]
	}
	return s
}
