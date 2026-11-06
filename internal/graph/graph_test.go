package graph

import "testing"

func buildTestGraph() *Graph {
	g := New()
	g.AddNode(Node{ID: "room", Name: "Room"})
	g.AddNode(Node{ID: "stairsA", Name: "Stairs A"})
	g.AddNode(Node{ID: "stairsB", Name: "Stairs B"})
	g.AddNode(Node{ID: "exitA", Name: "Exit A", IsExit: true})
	g.AddNode(Node{ID: "exitB", Name: "Exit B", IsExit: true})
	g.AddEdge("room", "stairsA", 5)
	g.AddEdge("room", "stairsB", 10)
	g.AddEdge("stairsA", "exitA", 5)
	g.AddEdge("stairsB", "exitB", 5)
	return g
}

func TestNormalRoute(t *testing.T) {
	g := buildTestGraph()
	path, _, err := g.SafestRoute("room")
	if err != nil {
		t.Fatal(err)
	}
	if path[len(path)-1] != "exitA" {
		t.Errorf("expected exitA, got %v", path)
	}
}

func TestRerouteWhenBlocked(t *testing.T) {
	g := buildTestGraph()
	g.SetBlocked("stairsA", true)
	path, _, err := g.SafestRoute("room")
	if err != nil {
		t.Fatal(err)
	}
	if path[len(path)-1] != "exitB" {
		t.Errorf("expected exitB after blocking, got %v", path)
	}
}

func TestNoRoute(t *testing.T) {
	g := buildTestGraph()
	g.SetBlocked("stairsA", true)
	g.SetBlocked("stairsB", true)
	if _, _, err := g.SafestRoute("room"); err == nil {
		t.Error("expected an error when everything is blocked")
	}
}
