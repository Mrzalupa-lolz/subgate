package health

import (
	"log/slog"
	"net"
	"testing"
	"time"
)

func TestQuarantineRemainsAndRecovers(t *testing.T) {
	engine := NewEngine(slog.Default())
	engine.AddNode("a")
	start := time.Now()
	for i := 0; i < 5; i++ {
		engine.RecordProbeResult(ProbeResult{NodeID: "a", Timestamp: start.Add(time.Duration(i) * time.Second)})
	}
	status, _ := engine.GetNodeHealth("a")
	if status != StatusQuarantined {
		t.Fatal(status)
	}
	engine.RecordProbeResult(ProbeResult{NodeID: "a", Success: true, Timestamp: start.Add(5 * time.Second)})
	status, _ = engine.GetNodeHealth("a")
	if status != StatusRecovering {
		t.Fatal(status)
	}
	engine.RecordProbeResult(ProbeResult{NodeID: "a", Success: true, Timestamp: start.Add(6 * time.Second)})
	status, _ = engine.GetNodeHealth("a")
	if status != StatusRecovering {
		t.Fatal("minimum quarantine was bypassed", status)
	}
	engine.RecordProbeResult(ProbeResult{NodeID: "a", Success: true, Timestamp: start.Add(63 * time.Second)})
	status, _ = engine.GetNodeHealth("a")
	if status != StatusHealthy {
		t.Fatal(status)
	}
}
func TestTCPDoesNotCountAsTLS(t *testing.T) {
	listener, err := net.Listen("tcp", "127.0.0.1:0")
	if err != nil {
		t.Fatal(err)
	}
	defer listener.Close()
	go func() {
		for {
			conn, err := listener.Accept()
			if err != nil {
				return
			}
			conn.Close()
		}
	}()
	engine := NewEngine(slog.Default())
	result := engine.ProbeNode("a", listener.Addr().String(), ProbeTLS, time.Second)
	if result.Success {
		t.Fatal("TCP server must not pass TLS handshake")
	}
	udp := engine.ProbeNode("a", "127.0.0.1:1", ProbeQUIC, time.Millisecond)
	if udp.Success || udp.Verifiable {
		t.Fatal("QUIC without handshake must be unverified")
	}
}
