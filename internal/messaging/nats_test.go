package messaging_test

import (
	"testing"

	"github.com/zsomething/zlaw/internal/messaging"
)

func TestChanMessenger_JetStream(t *testing.T) {
	m := messaging.NewChanMessenger()
	if m.JetStream() != nil {
		t.Error("ChanMessenger.JetStream() should return nil")
	}
}

func TestJetMsg_Data(t *testing.T) {
	msg := &messaging.JetMsg{}
	if msg.Data() != nil {
		t.Error("Data() on nil msg should return nil")
	}
}
