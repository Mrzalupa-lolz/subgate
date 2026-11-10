package xray

import (
	"context"
	"io"
	"log/slog"
	"net"
	"strings"
	"sync"
	"testing"
	"time"

	"google.golang.org/grpc"
	"google.golang.org/protobuf/reflect/protoreflect"
	"google.golang.org/protobuf/types/dynamicpb"
)

type statsTestService interface {
	Query(context.Context, *dynamicpb.Message) (*dynamicpb.Message, error)
}
type statsFixture struct {
	t        *testing.T
	mu       sync.Mutex
	values   map[string]int64
	patterns []string
}

func (f *statsFixture) Query(ctx context.Context, req *dynamicpb.Message) (*dynamicpb.Message, error) {
	if req.Get(req.Descriptor().Fields().ByName("reset")).Bool() {
		f.t.Error("stats query resets counters owned by panel")
	}
	pattern := req.Get(req.Descriptor().Fields().ByName("pattern")).String()
	f.mu.Lock()
	defer f.mu.Unlock()
	f.patterns = append(f.patterns, pattern)
	d := descriptors()
	resp := dynamicpb.NewMessage(d.Messages().ByName("QueryStatsResponse"))
	list := resp.Mutable(resp.Descriptor().Fields().ByName("stat")).List()
	for name, value := range f.values {
		if !strings.HasPrefix(name, pattern) {
			continue
		}
		stat := dynamicpb.NewMessage(d.Messages().ByName("Stat"))
		stat.Set(stat.Descriptor().Fields().ByName("name"), protoreflect.ValueOfString(name))
		stat.Set(stat.Descriptor().Fields().ByName("value"), protoreflect.ValueOfInt64(value))
		list.Append(protoreflect.ValueOfMessage(stat))
	}
	return resp, nil
}
func queryStatsHandler(service any, ctx context.Context, decode func(any) error, interceptor grpc.UnaryServerInterceptor) (any, error) {
	req := dynamicpb.NewMessage(descriptors().Messages().ByName("QueryStatsRequest"))
	if err := decode(req); err != nil {
		return nil, err
	}
	if interceptor == nil {
		return service.(statsTestService).Query(ctx, req)
	}
	return interceptor(ctx, req, &grpc.UnaryServerInfo{Server: service, FullMethod: "/xray.app.stats.command.StatsService/QueryStats"}, func(ctx context.Context, r any) (any, error) {
		return service.(statsTestService).Query(ctx, r.(*dynamicpb.Message))
	})
}

func TestRealGRPCStatsContractDeltaAndCounterReset(t *testing.T) {
	listener, err := net.Listen("tcp", "127.0.0.1:0")
	if err != nil {
		t.Fatal(err)
	}
	fixture := &statsFixture{t: t, values: map[string]int64{"user>>>alice@example.com>>>traffic>>>uplink": 100, "user>>>alice@example.com>>>traffic>>>downlink": 200, "user>>>bob@example.com>>>traffic>>>uplink": 50, "user>>>ignored>>>traffic>>>other": 1, "user>>>bad>>>traffic>>>downlink": -1, "inbound>>>reality>>>traffic>>>uplink": 77, "inbound>>>reality>>>traffic>>>downlink": 88}}
	server := grpc.NewServer()
	server.RegisterService(&grpc.ServiceDesc{ServiceName: "xray.app.stats.command.StatsService", HandlerType: (*statsTestService)(nil), Methods: []grpc.MethodDesc{{MethodName: "QueryStats", Handler: queryStatsHandler}}}, fixture)
	go server.Serve(listener)
	defer server.Stop()
	c, err := NewStatsClient(listener.Addr().String(), slog.New(slog.NewTextHandler(io.Discard, nil)))
	if err != nil {
		t.Fatal(err)
	}
	defer c.Close()
	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()
	stats, err := c.QueryStats(ctx)
	if err != nil {
		t.Fatal(err)
	}
	if len(stats) != 2 || stats[0].Email != "alice@example.com" || stats[0].UplinkBytes != 100 || stats[0].DownlinkBytes != 200 || stats[0].UplinkDelta != 0 || stats[0].UplinkBps != 0 || stats[1].Email != "bob@example.com" {
		t.Fatalf("first baseline not correct: %+v", stats)
	}
	// Set the baseline timestamp directly to make rates deterministic without wall-clock waits.
	c.mu.Lock()
	for key, old := range c.last {
		old.at = time.Now().Add(-2 * time.Second)
		c.last[key] = old
	}
	c.mu.Unlock()
	fixture.mu.Lock()
	fixture.values["user>>>alice@example.com>>>traffic>>>uplink"] = 1100
	fixture.values["user>>>alice@example.com>>>traffic>>>downlink"] = 2200
	fixture.mu.Unlock()
	stats, err = c.QueryStats(ctx)
	if err != nil {
		t.Fatal(err)
	}
	if stats[0].UplinkDelta != 1000 || stats[0].DownlinkDelta != 2000 || stats[0].UplinkBps < 3500 || stats[0].UplinkBps > 4000 || stats[0].DownlinkBps < 7000 || stats[0].DownlinkBps > 8000 {
		t.Fatalf("delta/rate calculation wrong: %+v", stats[0])
	}
	fixture.mu.Lock()
	fixture.values["user>>>alice@example.com>>>traffic>>>uplink"] = 10
	fixture.mu.Unlock()
	c.mu.Lock()
	for key, old := range c.last {
		old.at = time.Now().Add(-time.Second)
		c.last[key] = old
	}
	c.mu.Unlock()
	stats, err = c.QueryStats(ctx)
	if err != nil {
		t.Fatal(err)
	}
	if stats[0].UplinkDelta != 10 || stats[0].UplinkBps > 80 {
		t.Fatalf("restart counter produced traffic spike: %+v", stats[0])
	}
	fixture.mu.Lock()
	defer fixture.mu.Unlock()
	if len(fixture.patterns) != 3 {
		t.Fatalf("RPC query patterns: %v", fixture.patterns)
	}
}

func TestStatsLoopbackBoundary(t *testing.T) {
	for _, address := range []string{"192.0.2.1:10085", "0.0.0.0:10085", "example.com:10085", "invalid"} {
		if _, err := NewStatsClient(address, slog.Default()); err == nil {
			t.Errorf("nonlocal insecure service accepted: %s", address)
		}
	}
	for _, address := range []string{"127.0.0.1:10085", "[::1]:10085", "localhost:10085"} {
		c, err := NewStatsClient(address, slog.Default())
		if err != nil {
			t.Errorf("loopback rejected: %s %v", address, err)
		} else {
			c.Close()
		}
	}
}
