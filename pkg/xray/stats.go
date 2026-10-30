// Package xray speaks the Xray StatsService protobuf contract without depending
// on the Xray runtime or resetting counters used by the panel.
package xray

import (
	"context"
	"fmt"
	"log/slog"
	"math"
	"net"
	"os"
	"sort"
	"strings"
	"sync"
	"time"

	"google.golang.org/grpc"
	"google.golang.org/grpc/credentials/insecure"
	"google.golang.org/protobuf/proto"
	"google.golang.org/protobuf/reflect/protodesc"
	"google.golang.org/protobuf/reflect/protoreflect"
	"google.golang.org/protobuf/types/descriptorpb"
	"google.golang.org/protobuf/types/dynamicpb"
)

type PerUserStats struct {
	Email                                                                          string
	UplinkBytes, DownlinkBytes, UplinkDelta, DownlinkDelta, UplinkBps, DownlinkBps int64
	LastUpdate                                                                     time.Time
	IntervalMS                                                                     int64
	Gap, Warming                                                                   bool
	Scope                                                                          string
}
type previous struct {
	value int64
	at    time.Time
}
type StatsClient struct {
	address     string
	conn        *grpc.ClientConn
	logger      *slog.Logger
	mu          sync.Mutex
	last        map[string]previous
	lastInbound map[string]previous
}

var descriptorOnce sync.Once
var statsDescriptor protoreflect.FileDescriptor

func descriptors() protoreflect.FileDescriptor {
	descriptorOnce.Do(func() {
		optional := descriptorpb.FieldDescriptorProto_LABEL_OPTIONAL
		repeated := descriptorpb.FieldDescriptorProto_LABEL_REPEATED
		str := descriptorpb.FieldDescriptorProto_TYPE_STRING
		i64 := descriptorpb.FieldDescriptorProto_TYPE_INT64
		boolean := descriptorpb.FieldDescriptorProto_TYPE_BOOL
		message := descriptorpb.FieldDescriptorProto_TYPE_MESSAGE
		field := func(name string, n int32, typ descriptorpb.FieldDescriptorProto_Type) *descriptorpb.FieldDescriptorProto {
			return &descriptorpb.FieldDescriptorProto{Name: proto.String(name), Number: proto.Int32(n), Type: &typ, Label: &optional}
		}
		statField := field("stat", 1, message)
		statField.Label = &repeated
		statField.TypeName = proto.String(".xray.app.stats.command.Stat")
		d, err := protodesc.NewFile(&descriptorpb.FileDescriptorProto{Name: proto.String("subgate-xray-stats.proto"), Package: proto.String("xray.app.stats.command"), Syntax: proto.String("proto3"), MessageType: []*descriptorpb.DescriptorProto{
			{Name: proto.String("QueryStatsRequest"), Field: []*descriptorpb.FieldDescriptorProto{field("pattern", 1, str), field("reset", 2, boolean)}},
			{Name: proto.String("Stat"), Field: []*descriptorpb.FieldDescriptorProto{field("name", 1, str), field("value", 2, i64)}},
			{Name: proto.String("QueryStatsResponse"), Field: []*descriptorpb.FieldDescriptorProto{statField}},
		}}, nil)
		if err != nil {
			panic(err)
		}
		statsDescriptor = d
	})
	return statsDescriptor
}
func NewStatsClient(address string, logger *slog.Logger) (*StatsClient, error) {
	if address == "" || address == "auto" {
		address = discoverAddress()
	}
	if strings.HasPrefix(address, "unix:") {
		path := strings.TrimPrefix(address, "unix:")
		if path == "" || (!strings.HasPrefix(path, "/") && !strings.HasPrefix(path, "@")) {
			return nil, fmt.Errorf("Unix stats address must be unix:/absolute/path or unix:@abstract-name")
		}
		return &StatsClient{address: address, logger: logger, last: map[string]previous{}, lastInbound: map[string]previous{}}, nil
	}
	host, _, err := net.SplitHostPort(address)
	if err != nil {
		return nil, fmt.Errorf("stats address must be host:port")
	}
	ip := net.ParseIP(host)
	if host != "localhost" && (ip == nil || !ip.IsLoopback()) {
		return nil, fmt.Errorf("insecure Xray StatsService must use a loopback address")
	}
	return &StatsClient{address: address, logger: logger, last: map[string]previous{}, lastInbound: map[string]previous{}}, nil
}

// Remnanode uses an abstract Unix socket in current Xray releases. The socket
// appears in the host network namespace even when Xray runs in a container.
func discoverAddress() string {
	raw, err := os.ReadFile("/proc/net/unix")
	if err == nil {
		found := []string{}
		seen := map[string]bool{}
		for _, line := range strings.Split(string(raw), "\n") {
			fields := strings.Fields(line)
			if len(fields) >= 8 {
				path := fields[len(fields)-1]
				if strings.HasPrefix(path, "@xtls-api-") && !seen[path] {
					found = append(found, path)
					seen[path] = true
				}
			}
		}
		if len(found) == 1 {
			return "unix:" + found[0]
		}
	}
	return "127.0.0.1:10085"
}
func (s *StatsClient) Connect(ctx context.Context) error {
	s.mu.Lock()
	defer s.mu.Unlock()
	return s.connectLocked()
}
func (s *StatsClient) connectLocked() error {
	if s.conn != nil {
		return nil
	}
	options := []grpc.DialOption{grpc.WithTransportCredentials(insecure.NewCredentials()), grpc.WithDefaultCallOptions(grpc.MaxCallRecvMsgSize(16 << 20))}
	address := s.address
	if strings.HasPrefix(address, "unix:") {
		path := strings.TrimPrefix(address, "unix:")
		options = append(options, grpc.WithContextDialer(func(ctx context.Context, _ string) (net.Conn, error) {
			return (&net.Dialer{}).DialContext(ctx, "unix", path)
		}))
		address = "passthrough:///xray-local-unix"
	}
	conn, err := grpc.NewClient(address, options...)
	if err != nil {
		return err
	}
	s.conn = conn
	return nil
}
func (s *StatsClient) queryLocked(ctx context.Context, pattern string) (map[string]int64, error) {
	if err := s.connectLocked(); err != nil {
		return nil, err
	}
	d := descriptors()
	req := dynamicpb.NewMessage(d.Messages().ByName("QueryStatsRequest"))
	req.Set(req.Descriptor().Fields().ByName("pattern"), protoreflect.ValueOfString(pattern))
	req.Set(req.Descriptor().Fields().ByName("reset"), protoreflect.ValueOfBool(false))
	resp := dynamicpb.NewMessage(d.Messages().ByName("QueryStatsResponse"))
	ctx, cancel := context.WithTimeout(ctx, 3*time.Second)
	defer cancel()
	if err := s.conn.Invoke(ctx, "/xray.app.stats.command.StatsService/QueryStats", req, resp); err != nil {
		return nil, fmt.Errorf("Xray stats query failed: %w", err)
	}
	list := resp.Get(resp.Descriptor().Fields().ByName("stat")).List()
	result := map[string]int64{}
	for i := 0; i < list.Len(); i++ {
		item := list.Get(i).Message()
		name := item.Get(item.Descriptor().Fields().ByName("name")).String()
		value := item.Get(item.Descriptor().Fields().ByName("value")).Int()
		if value >= 0 {
			result[name] = value
		}
	}
	return result, nil
}
func (s *StatsClient) QueryStats(ctx context.Context) ([]PerUserStats, error) {
	s.mu.Lock()
	defer s.mu.Unlock()
	counters, err := s.queryLocked(ctx, "user>>>")
	if err != nil {
		return nil, err
	}
	now := time.Now()
	users := map[string]*PerUserStats{}
	next := map[string]previous{}
	for name, value := range counters {
		parts := strings.Split(name, ">>>")
		if len(parts) != 4 || parts[0] != "user" || parts[1] == "" || parts[2] != "traffic" || (parts[3] != "uplink" && parts[3] != "downlink") {
			continue
		}
		email := parts[1]
		entry := users[email]
		if entry == nil {
			entry = &PerUserStats{Email: email, LastUpdate: now}
			users[email] = entry
		}
		delta, bps := int64(0), int64(0)
		if old, ok := s.last[name]; ok {
			delta = value - old.value
			if delta < 0 {
				delta = value
			}
			interval := now.Sub(old.at)
			entry.IntervalMS = interval.Milliseconds()
			entry.Gap = interval > 10*time.Second
			bps = ComputeSpeed(delta, interval.Seconds())
		} else {
			entry.Warming = true
		}
		next[name] = previous{value, now}
		if parts[3] == "uplink" {
			entry.UplinkBytes = value
			entry.UplinkDelta = delta
			entry.UplinkBps = bps
		} else {
			entry.DownlinkBytes = value
			entry.DownlinkDelta = delta
			entry.DownlinkBps = bps
		}
	}
	s.last = next
	result := make([]PerUserStats, 0, len(users))
	for _, entry := range users {
		result = append(result, *entry)
	}
	sort.Slice(result, func(i, j int) bool { return result[i].Email < result[j].Email })
	return result, nil
}

// QueryInboundSpeeds is used only when there are no per-user counters. Its
// records are marked inbound and must never be presented as identified users.
func (s *StatsClient) QueryInboundSpeeds(ctx context.Context) ([]PerUserStats, error) {
	s.mu.Lock()
	defer s.mu.Unlock()
	values, err := s.queryLocked(ctx, "inbound>>>")
	if err != nil {
		return nil, err
	}
	now := time.Now()
	next := map[string]previous{}
	entries := map[string]*PerUserStats{}
	for name, value := range values {
		parts := strings.Split(name, ">>>")
		if len(parts) != 4 || parts[0] != "inbound" || parts[1] == "" || parts[2] != "traffic" || (parts[3] != "uplink" && parts[3] != "downlink") {
			continue
		}
		e := entries[parts[1]]
		if e == nil {
			e = &PerUserStats{Email: parts[1], Scope: "inbound", LastUpdate: now}
			entries[parts[1]] = e
		}
		delta, bps := int64(0), int64(0)
		if old, ok := s.lastInbound[name]; ok {
			delta = value - old.value
			if delta < 0 {
				delta = value
			}
			e.IntervalMS = now.Sub(old.at).Milliseconds()
			e.Gap = now.Sub(old.at) > 10*time.Second
			bps = ComputeSpeed(delta, now.Sub(old.at).Seconds())
		} else {
			e.Warming = true
		}
		next[name] = previous{value, now}
		if parts[3] == "uplink" {
			e.UplinkBytes, e.UplinkDelta, e.UplinkBps = value, delta, bps
		} else {
			e.DownlinkBytes, e.DownlinkDelta, e.DownlinkBps = value, delta, bps
		}
	}
	s.lastInbound = next
	result := make([]PerUserStats, 0, len(entries))
	for _, entry := range entries {
		result = append(result, *entry)
	}
	sort.Slice(result, func(i, j int) bool { return result[i].Email < result[j].Email })
	return result, nil
}
func (s *StatsClient) GetUserSpeed(ctx context.Context, email string) (int64, int64, error) {
	stats, err := s.QueryStats(ctx)
	if err != nil {
		return 0, 0, err
	}
	for _, v := range stats {
		if v.Email == email {
			return v.UplinkBps, v.DownlinkBps, nil
		}
	}
	return 0, 0, fmt.Errorf("user counters not found")
}
func (s *StatsClient) GetInboundStats(ctx context.Context, tag string) (int64, int64, error) {
	s.mu.Lock()
	defer s.mu.Unlock()
	values, err := s.queryLocked(ctx, "inbound>>>"+tag+">>>traffic>>>")
	if err != nil {
		return 0, 0, err
	}
	return values["inbound>>>"+tag+">>>traffic>>>uplink"], values["inbound>>>"+tag+">>>traffic>>>downlink"], nil
}
func (s *StatsClient) Health(ctx context.Context) error {
	s.mu.Lock()
	defer s.mu.Unlock()
	_, err := s.queryLocked(ctx, "user>>>")
	return err
}
func (s *StatsClient) Close() error {
	s.mu.Lock()
	defer s.mu.Unlock()
	if s.conn != nil {
		err := s.conn.Close()
		s.conn = nil
		return err
	}
	return nil
}
func ExtractUserEmail(name string) string {
	p := strings.Split(name, ">>>")
	if len(p) == 4 && p[0] == "user" && p[2] == "traffic" {
		return p[1]
	}
	return ""
}
func ExtractInboundTag(name string) string {
	p := strings.Split(name, ">>>")
	if len(p) == 4 && p[0] == "inbound" && p[2] == "traffic" {
		return p[1]
	}
	return ""
}
func ComputeSpeed(delta int64, seconds float64) int64 {
	if delta <= 0 || seconds <= 0 || math.IsNaN(seconds) || math.IsInf(seconds, 0) {
		return 0
	}
	value := float64(delta) * 8 / seconds
	if value >= float64(math.MaxInt64) {
		return math.MaxInt64
	}
	return int64(value)
}
