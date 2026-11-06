package graph

import (
	"container/heap"
	"errors"
	"sync"
)

type Node struct {
	ID        string  `json:"id"`
	Name      string  `json:"name"`
	X         float64 `json:"x"`
	Y         float64 `json:"y"`
	IsExit    bool    `json:"isExit"`
	Blocked   bool    `json:"blocked"`
	Occupancy int     `json:"occupancy"`
}

type Edge struct {
	From     string  `json:"from"`
	To       string  `json:"to"`
	Distance float64 `json:"distance"`
}

type Graph struct {
	mu    sync.RWMutex
	nodes map[string]*Node
	edges []Edge
	adj   map[string][]Edge
	order []string
}

var ErrNoRoute = errors.New("no safe route available")

func New() *Graph {
	return &Graph{nodes: map[string]*Node{}, adj: map[string][]Edge{}}
}

func (g *Graph) AddNode(n Node) {
	g.mu.Lock()
	defer g.mu.Unlock()
	g.nodes[n.ID] = &n
	g.order = append(g.order, n.ID)
}

// AddEdge adds a two-way corridor between two nodes.
func (g *Graph) AddEdge(a, b string, distance float64) {
	g.mu.Lock()
	defer g.mu.Unlock()
	g.edges = append(g.edges, Edge{a, b, distance})
	g.adj[a] = append(g.adj[a], Edge{a, b, distance})
	g.adj[b] = append(g.adj[b], Edge{b, a, distance})
}

func (g *Graph) GetNode(id string) (Node, bool) {
	g.mu.RLock()
	defer g.mu.RUnlock()
	n, ok := g.nodes[id]
	if !ok {
		return Node{}, false
	}
	return *n, true
}

func (g *Graph) SetBlocked(id string, blocked bool) error {
	g.mu.Lock()
	defer g.mu.Unlock()
	n, ok := g.nodes[id]
	if !ok {
		return errors.New("unknown location")
	}
	n.Blocked = blocked
	return nil
}

func (g *Graph) SetOccupancy(id string, count int) {
	g.mu.Lock()
	defer g.mu.Unlock()
	if n, ok := g.nodes[id]; ok {
		n.Occupancy = count
	}
}

// Snapshot returns copies of all nodes and edges (safe to send as JSON).
func (g *Graph) Snapshot() ([]Node, []Edge) {
	g.mu.RLock()
	defer g.mu.RUnlock()
	outNodes := make([]Node, 0, len(g.order))
	for _, id := range g.order {
		outNodes = append(outNodes, *g.nodes[id])
	}
	outEdges := make([]Edge, len(g.edges))
	copy(outEdges, g.edges)
	return outNodes, outEdges
}

// ---- Priority queue used by Dijkstra ----

type item struct {
	id   string
	cost float64
}
type priorityQueue []item

func (p priorityQueue) Len() int            { return len(p) }
func (p priorityQueue) Less(i, j int) bool  { return p[i].cost < p[j].cost }
func (p priorityQueue) Swap(i, j int)       { p[i], p[j] = p[j], p[i] }
func (p *priorityQueue) Push(x any)         { *p = append(*p, x.(item)) }
func (p *priorityQueue) Pop() any {
	old := *p
	n := len(old)
	it := old[n-1]
	*p = old[:n-1]
	return it
}

// SafestRoute finds the cheapest path from start to the nearest exit.
// Blocked nodes are never used. Crowded nodes cost extra.
func (g *Graph) SafestRoute(start string) ([]string, float64, error) {
	g.mu.RLock()
	defer g.mu.RUnlock()

	if _, ok := g.nodes[start]; !ok {
		return nil, 0, errors.New("unknown start location")
	}

	dist := map[string]float64{start: 0}
	prev := map[string]string{}
	visited := map[string]bool{}
	q := &priorityQueue{}
	heap.Push(q, item{start, 0})

	for q.Len() > 0 {
		cur := heap.Pop(q).(item)
		if visited[cur.id] {
			continue
		}
		visited[cur.id] = true

		// The first exit we pull out of the queue is the cheapest one.
		if g.nodes[cur.id].IsExit {
			path := []string{cur.id}
			for path[0] != start {
				path = append([]string{prev[path[0]]}, path...)
			}
			return path, cur.cost, nil
		}

		for _, e := range g.adj[cur.id] {
			next := g.nodes[e.To]
			if next.Blocked || visited[e.To] {
				continue
			}
			newCost := cur.cost + e.Distance + float64(next.Occupancy)*0.5
			if old, seen := dist[e.To]; !seen || newCost < old {
				dist[e.To] = newCost
				prev[e.To] = cur.id
				heap.Push(q, item{e.To, newCost})
			}
		}
	}
	return nil, 0, ErrNoRoute
}