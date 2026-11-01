package tier

import (
	"log/slog"
	"testing"
)

func TestFallbackCycleReturnsError(t *testing.T) {
	a := NewTier("a", "A", StrategyRandom, 1, slog.Default())
	b := NewTier("b", "B", StrategyRandom, 1, slog.Default())
	a.FallbackTier = b
	b.FallbackTier = a
	if _, err := a.SelectNode(); err == nil {
		t.Fatal("expected fallback cycle error")
	}
}
func TestRoundRobinAndFallback(t *testing.T) {
	a := NewTier("a", "A", StrategyRoundRobin, 1, slog.Default())
	a.AddNode(NodeInfo{ID: "one", Healthy: true})
	a.AddNode(NodeInfo{ID: "two", Healthy: true})
	first, _ := a.SelectNode()
	second, _ := a.SelectNode()
	if first.ID == second.ID {
		t.Fatal("round robin repeated node")
	}
	a.UpdateNodeHealth("one", false)
	a.UpdateNodeHealth("two", false)
	b := NewTier("b", "B", StrategyLeastLoad, 1, slog.Default())
	b.AddNode(NodeInfo{ID: "three", Healthy: true})
	a.FallbackTier = b
	node, err := a.SelectNode()
	if err != nil || node.ID != "three" {
		t.Fatal(node, err)
	}
}
