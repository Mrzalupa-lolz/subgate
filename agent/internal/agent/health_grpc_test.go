package agent

import (
	"context"
	"encoding/json"
	"io"
	"net"
	"net/http"
	"net/http/httptest"
	"sync/atomic"
	"testing"

	"github.com/subgate/subgate/pkg/wire"
	"google.golang.org/grpc"
	"google.golang.org/protobuf/types/known/emptypb"
)

type syntheticProbeRPC interface{}

func TestAgentReportsGRPCTransportProbe(t *testing.T) {
	l, err := net.Listen("tcp", "127.0.0.1:0")
	if err != nil {
		t.Fatal(err)
	}
	var accepted atomic.Int32
	rpc := grpc.NewServer()
	rpc.RegisterService(&grpc.ServiceDesc{ServiceName: "synthetic.Agent", HandlerType: (*syntheticProbeRPC)(nil), Streams: []grpc.StreamDesc{{StreamName: "Tun", ServerStreams: true, ClientStreams: true, Handler: func(_ any, stream grpc.ServerStream) error {
		var empty emptypb.Empty
		if err := stream.RecvMsg(&empty); err != nil || len(empty.ProtoReflect().GetUnknown()) != 0 {
			t.Error("nonempty probe envelope", err)
		}
		if err := stream.RecvMsg(&empty); err != io.EOF {
			t.Error("additional payload sent", err)
		}
		accepted.Add(1)
		return nil
	}}}}, struct{}{})
	go rpc.Serve(l)
	defer rpc.Stop()
	target := wire.HealthTarget{NodeID: "grpc", Address: "127.0.0.1", Inbounds: []wire.HealthInbound{{Protocol: "vless", Network: "grpc", Port: l.Addr().(*net.TCPAddr).Port, HTTP: &wire.HealthHTTP{Path: "/synthetic.Agent/Tun", Host: "probe.example.test"}}}}
	var report wire.HealthReport
	core := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.Header.Get("Authorization") != "Bearer synthetic-agent-token" {
			t.Error("missing Agent auth")
		}
		if r.Method == "GET" {
			json.NewEncoder(w).Encode(map[string]any{"data": []wire.HealthTarget{target}})
			return
		}
		if err := json.NewDecoder(r.Body).Decode(&report); err != nil {
			t.Error(err)
		}
	}))
	defer core.Close()
	a := testAgent(t, core.URL, "", "synthetic-agent-token")
	if err := a.collectHealth(context.Background()); err != nil {
		t.Fatal(err)
	}
	if len(report.Results) != 1 || !report.Results[0].Success || report.Results[0].Verified != "grpc" || accepted.Load() != 1 {
		t.Fatal("Agent gRPC not observed", report)
	}
	target.Inbounds[0].HTTP.Path = "/missing/Tun"
	if err := a.collectHealth(context.Background()); err != nil {
		t.Fatal(err)
	}
	if len(report.Results) != 1 || report.Results[0].Success || report.Results[0].Verified != "grpc" {
		t.Fatal("Agent unknown gRPC method accepted", report)
	}
}
