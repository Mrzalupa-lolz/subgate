package server

import (
	"context"
	"encoding/json"
	"fmt"
	"io"
	"net"
	"strings"
	"sync/atomic"
	"testing"
	"time"

	"github.com/subgate/subgate/core/internal/panel"
	"github.com/subgate/subgate/pkg/wire"
	"google.golang.org/grpc"
	"google.golang.org/grpc/metadata"
	"google.golang.org/protobuf/types/known/emptypb"
)

type syntheticGRPCService interface{}

func TestGRPCTransportProbe(t *testing.T) {
	l, err := net.Listen("tcp", "127.0.0.1:0")
	if err != nil {
		t.Fatal(err)
	}
	var accepted atomic.Int32
	rpc := grpc.NewServer()
	rpc.RegisterService(&grpc.ServiceDesc{ServiceName: "synthetic.Service", HandlerType: (*syntheticGRPCService)(nil), Streams: []grpc.StreamDesc{{StreamName: "Tun", ServerStreams: true, ClientStreams: true, Handler: func(_ any, stream grpc.ServerStream) error {
		md, _ := metadata.FromIncomingContext(stream.Context())
		if values := md.Get(":authority"); len(values) != 1 || values[0] != "probe.example.test" {
			t.Error("configured authority not sent")
		}
		var message emptypb.Empty
		if err := stream.RecvMsg(&message); err != nil || len(message.ProtoReflect().GetUnknown()) != 0 {
			t.Error("probe did not send empty transport envelope", err)
		}
		if err := stream.RecvMsg(&message); err != io.EOF {
			t.Error("probe sent additional application data", err)
		}
		accepted.Add(1)
		return nil
	}}}}, struct{}{})
	go rpc.Serve(l)
	defer rpc.Stop()
	port := l.Addr().(*net.TCPAddr).Port
	raw := fmt.Sprintf(`{"type":"vless","network":"grpc","port":%d,"rawInbound":{"settings":{"clients":[{"id":"synthetic-credential"}]},"streamSettings":{"network":"grpc","grpcSettings":{"serviceName":"synthetic.Service","authority":"probe.example.test"}}}}`, port)
	var inbound panel.Inbound
	if err := json.Unmarshal([]byte(raw), &inbound); err != nil {
		t.Fatal(err)
	}
	s := newTestServer(t)
	n := panel.Node{ID: "grpc", IPAddress: "127.0.0.1", Inbounds: []panel.Inbound{inbound}}
	if err := s.saveObject("nodes", n.ID, n); err != nil {
		t.Fatal(err)
	}
	if err := s.scheduledNodeProbe(context.Background(), n); err != nil {
		t.Fatal(err)
	}
	var probe nodeProbe
	if err := s.object("health", n.ID, &probe); err != nil || !probe.Success || probe.Verified != "grpc" || accepted.Load() != 1 {
		t.Fatal("configured gRPC method not verified", probe, accepted.Load(), err)
	}
	w := perform(s, "POST", "/api/v1/nodes/grpc/probe", nil, loginTest(t, s), "", true)
	if w.Code != 200 || accepted.Load() != 2 || !strings.Contains(w.Body.String(), `"verified":"grpc"`) {
		t.Fatal("manual gRPC probe differs", w.Code, accepted.Load())
	}
	data, _ := json.Marshal(healthTarget(n))
	if strings.Contains(string(data), "synthetic-credential") || !strings.Contains(string(data), "/synthetic.Service/Tun") {
		t.Fatal("sanitized method not exposed")
	}
	token, _ := enrollTest(t, s, loginTest(t, s), "demo-node-1")
	report := wire.HealthReport{Results: []wire.HealthResult{{NodeID: n.ID, Timestamp: time.Now(), Verified: "grpc", Success: true}}}
	if w := perform(s, "PUT", "/api/v1/agents/health-results", report, nil, token, false); w.Code != 200 {
		t.Fatal("Agent gRPC rejected", w.Code)
	}
	s.config.Health.Quorum = 2
	if err := s.scheduledNodeProbe(context.Background(), n); err != nil {
		t.Fatal(err)
	}
	if err := s.object("health", n.ID, &probe); err != nil || !probe.QuorumMet || probe.Vantages != 2 {
		t.Fatal("gRPC quorum not met", probe, err)
	}
	s.config.Health.Quorum = 1
	raw = strings.Replace(raw, "synthetic.Service", "unknown.Service", 1)
	if err := json.Unmarshal([]byte(raw), &n.Inbounds[0]); err != nil {
		t.Fatal(err)
	}
	if err := s.scheduledNodeProbe(context.Background(), n); err != nil {
		t.Fatal(err)
	}
	if err := s.object("health", n.ID, &probe); err != nil || probe.Success || probe.Verified != "grpc" {
		t.Fatal("unknown method masked by TCP", probe, err)
	}
}
