package main

import (
	"encoding/json"
	"log"
	"math/rand"
	"net/http"
	"os"
	"sync"
	"time"

	"evacx/internal/graph"

	"github.com/gorilla/websocket"
)

// ---------- Sample building ----------

func buildSampleBuilding() *graph.Graph {
	g := graph.New()
	g.AddNode(graph.Node{ID: "room101", Name: "Room 101", X: 100, Y: 80})
	g.AddNode(graph.Node{ID: "room102", Name: "Room 102", X: 100, Y: 250})
	g.AddNode(graph.Node{ID: "room103", Name: "Room 103", X: 100, Y: 420})
	g.AddNode(graph.Node{ID: "hallA", Name: "West Hall", X: 260, Y: 250})
	g.AddNode(graph.Node{ID: "mainStairs", Name: "Main Stairs", X: 420, Y: 100})
	g.AddNode(graph.Node{ID: "backStairs", Name: "Back Stairs", X: 420, Y: 400})
	g.AddNode(graph.Node{ID: "hallB", Name: "East Hall", X: 560, Y: 250})
	g.AddNode(graph.Node{ID: "office", Name: "Office", X: 700, Y: 250})
	g.AddNode(graph.Node{ID: "mainExit", Name: "Main Exit", X: 700, Y: 80, IsExit: true})
	g.AddNode(graph.Node{ID: "sideExit", Name: "Side Exit", X: 700, Y: 420, IsExit: true})

	g.AddEdge("room101", "hallA", 15)
	g.AddEdge("room102", "hallA", 10)
	g.AddEdge("room103", "hallA", 15)
	g.AddEdge("hallA", "mainStairs", 15)
	g.AddEdge("hallA", "backStairs", 20)
	g.AddEdge("mainStairs", "mainExit", 20)
	g.AddEdge("backStairs", "sideExit", 20)
	g.AddEdge("mainStairs", "hallB", 15)
	g.AddEdge("backStairs", "hallB", 15)
	g.AddEdge("hallB", "office", 10)
	return g
}

// ---------- Live alerts (WebSocket) ----------

var upgrader = websocket.Upgrader{CheckOrigin: func(r *http.Request) bool { return true }}

type Hub struct {
	mu      sync.Mutex
	clients map[*websocket.Conn]bool
}

func (h *Hub) Broadcast(msg any) {
	data, _ := json.Marshal(msg)
	h.mu.Lock()
	defer h.mu.Unlock()
	for c := range h.clients {
		if err := c.WriteMessage(websocket.TextMessage, data); err != nil {
			c.Close()
			delete(h.clients, c)
		}
	}
}

func (h *Hub) Handle(w http.ResponseWriter, r *http.Request) {
	conn, err := upgrader.Upgrade(w, r, nil)
	if err != nil {
		return
	}
	h.mu.Lock()
	h.clients[conn] = true
	h.mu.Unlock()
	for {
		if _, _, err := conn.ReadMessage(); err != nil {
			h.mu.Lock()
			delete(h.clients, conn)
			h.mu.Unlock()
			conn.Close()
			return
		}
	}
}

// ---------- Incident history (saved in incidents.json) ----------

type Incident struct {
	ID         int        `json:"id"`
	NodeID     string     `json:"nodeId"`
	NodeName   string     `json:"nodeName"`
	Type       string     `json:"type"`
	Severity   string     `json:"severity"`
	StartedAt  time.Time  `json:"startedAt"`
	ResolvedAt *time.Time `json:"resolvedAt"`
}

type Store struct {
	mu        sync.Mutex
	Incidents []Incident
}

const incidentFile = "incidents.json"

func (s *Store) load() {
	data, err := os.ReadFile(incidentFile)
	if err == nil {
		json.Unmarshal(data, &s.Incidents)
	}
	// After a restart nothing is blocked, so close any old open incidents.
	now := time.Now()
	for i := range s.Incidents {
		if s.Incidents[i].ResolvedAt == nil {
			t := now
			s.Incidents[i].ResolvedAt = &t
		}
	}
	s.save()
}

func (s *Store) save() {
	data, _ := json.MarshalIndent(s.Incidents, "", "  ")
	os.WriteFile(incidentFile, data, 0644)
}

func (s *Store) Add(nodeID, nodeName, typ, severity string) {
	s.mu.Lock()
	defer s.mu.Unlock()
	s.Incidents = append(s.Incidents, Incident{
		ID: len(s.Incidents) + 1, NodeID: nodeID, NodeName: nodeName,
		Type: typ, Severity: severity, StartedAt: time.Now(),
	})
	s.save()
}

func (s *Store) Resolve(nodeID string) {
	s.mu.Lock()
	defer s.mu.Unlock()
	now := time.Now()
	for i := range s.Incidents {
		if s.Incidents[i].NodeID == nodeID && s.Incidents[i].ResolvedAt == nil {
			t := now
			s.Incidents[i].ResolvedAt = &t
		}
	}
	s.save()
}

func (s *Store) List() []Incident {
	s.mu.Lock()
	defer s.mu.Unlock()
	out := make([]Incident, len(s.Incidents))
	copy(out, s.Incidents)
	return out
}

// ---------- Helpers ----------

func writeJSON(w http.ResponseWriter, status int, v any) {
	w.Header().Set("Content-Type", "application/json")
	w.WriteHeader(status)
	json.NewEncoder(w).Encode(v)
}

func adminOnly(next http.HandlerFunc) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		password := os.Getenv("ADMIN_PASSWORD")
		if password == "" {
			password = "admin123"
		}
		if r.Header.Get("X-Admin-Password") != password {
			writeJSON(w, http.StatusUnauthorized, map[string]string{"error": "wrong admin password"})
			return
		}
		next(w, r)
	}
}

// Simulated crowd sensors: random occupancy every 5 seconds.
func simulateCrowd(g *graph.Graph, hub *Hub) {
	for {
		time.Sleep(5 * time.Second)
		nodes, _ := g.Snapshot()
		for _, n := range nodes {
			if !n.IsExit {
				g.SetOccupancy(n.ID, rand.Intn(11))
			}
		}
		hub.Broadcast(map[string]string{"type": "crowd"})
	}
}

// ---------- Main ----------

func main() {
	g := buildSampleBuilding()
	hub := &Hub{clients: map[*websocket.Conn]bool{}}
	store := &Store{}
	store.load()
	go simulateCrowd(g, hub)

	mux := http.NewServeMux()

	mux.HandleFunc("GET /api/map", func(w http.ResponseWriter, r *http.Request) {
		nodes, edges := g.Snapshot()
		writeJSON(w, 200, map[string]any{"nodes": nodes, "edges": edges})
	})

	mux.HandleFunc("GET /api/route", func(w http.ResponseWriter, r *http.Request) {
		path, cost, err := g.SafestRoute(r.URL.Query().Get("from"))
		if err != nil {
			writeJSON(w, 404, map[string]string{"error": err.Error()})
			return
		}
		writeJSON(w, 200, map[string]any{"path": path, "cost": cost})
	})

	mux.HandleFunc("GET /api/incidents", func(w http.ResponseWriter, r *http.Request) {
		writeJSON(w, 200, store.List())
	})

	mux.HandleFunc("POST /api/emergency", adminOnly(func(w http.ResponseWriter, r *http.Request) {
		var req struct {
			NodeID   string `json:"nodeId"`
			Type     string `json:"type"`
			Severity string `json:"severity"`
		}
		if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
			writeJSON(w, 400, map[string]string{"error": "bad request"})
			return
		}
		node, ok := g.GetNode(req.NodeID)
		if !ok {
			writeJSON(w, 404, map[string]string{"error": "unknown location"})
			return
		}
		if req.Type == "" {
			req.Type = "Fire"
		}
		if req.Severity == "" {
			req.Severity = "High"
		}
		g.SetBlocked(req.NodeID, true)
		store.Add(node.ID, node.Name, req.Type, req.Severity)
		hub.Broadcast(map[string]string{
			"type":    "alert",
			"nodeId":  node.ID,
			"message": "EMERGENCY: " + req.Type + " at " + node.Name + ". Routes updated.",
		})
		writeJSON(w, 200, map[string]string{"status": "zone blocked"})
	}))

	mux.HandleFunc("DELETE /api/emergency/{id}", adminOnly(func(w http.ResponseWriter, r *http.Request) {
		id := r.PathValue("id")
		node, ok := g.GetNode(id)
		if !ok {
			writeJSON(w, 404, map[string]string{"error": "unknown location"})
			return
		}
		g.SetBlocked(id, false)
		store.Resolve(id)
		hub.Broadcast(map[string]string{
			"type":    "clear",
			"nodeId":  id,
			"message": "All clear at " + node.Name + ". Routes updated.",
		})
		writeJSON(w, 200, map[string]string{"status": "zone cleared"})
	}))

	mux.HandleFunc("/ws", hub.Handle)
	mux.Handle("/", http.FileServer(http.Dir("web")))

	log.Println("EvacX running at http://localhost:8080")
	log.Fatal(http.ListenAndServe(":8080", mux))
}
