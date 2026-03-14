package websocket

import (
	"encoding/json"
	"log"
	"net/http"
	"sync"

	"github.com/gin-gonic/gin"
	"github.com/gorilla/websocket"
)

// Message — sent to frontend in real time
type Message struct {
	Type    string      `json:"type"`    // new_review, alert, stats_update
	Payload interface{} `json:"payload"`
}

// Client — one connected browser tab
type Client struct {
	BusinessID int
	Conn       *websocket.Conn
	Send       chan Message
}

// Hub — manages all connected clients
type Hub struct {
	clients    map[int][]*Client  // businessID → clients
	mu         sync.RWMutex
	register   chan *Client
	unregister chan *Client
}

var upgrader = websocket.Upgrader{
	CheckOrigin: func(r *http.Request) bool { return true },
}

// Global hub instance
var H = &Hub{
	clients:    make(map[int][]*Client),
	register:   make(chan *Client, 100),
	unregister: make(chan *Client, 100),
}

// Run — starts the hub event loop
func (h *Hub) Run() {
	for {
		select {
		case client := <-h.register:
			h.mu.Lock()
			h.clients[client.BusinessID] = append(
				h.clients[client.BusinessID], client,
			)
			h.mu.Unlock()
			log.Printf("✅ WS Client connected: business %d", client.BusinessID)

		case client := <-h.unregister:
			h.mu.Lock()
			clients := h.clients[client.BusinessID]
			for i, c := range clients {
				if c == client {
					h.clients[client.BusinessID] = append(clients[:i], clients[i+1:]...)
					close(client.Send)
					break
				}
			}
			h.mu.Unlock()
			log.Printf("❌ WS Client disconnected: business %d", client.BusinessID)
		}
	}
}

// Broadcast — sends a message to ALL clients of a business
func (h *Hub) Broadcast(businessID int, msg Message) {
	h.mu.RLock()
	defer h.mu.RUnlock()

	for _, client := range h.clients[businessID] {
		select {
		case client.Send <- msg:
		default:
			// Client too slow — skip
		}
	}
}

// ServeWS — HTTP handler that upgrades to WebSocket
func ServeWS(c *gin.Context) {
	businessID := c.GetInt("business_id")

	conn, err := upgrader.Upgrade(c.Writer, c.Request, nil)
	if err != nil {
		log.Println("WS upgrade error:", err)
		return
	}

	client := &Client{
		BusinessID: businessID,
		Conn:       conn,
		Send:       make(chan Message, 256),
	}

	H.register <- client

	// Write pump — sends messages to browser
	go func() {
		defer func() {
			H.unregister <- client
			conn.Close()
		}()
		for msg := range client.Send {
			data, _ := json.Marshal(msg)
			if err := conn.WriteMessage(websocket.TextMessage, data); err != nil {
				return
			}
		}
	}()

	// Read pump — keeps connection alive
	go func() {
		defer func() {
			H.unregister <- client
			conn.Close()
		}()
		for {
			_, _, err := conn.ReadMessage()
			if err != nil {
				return
			}
		}
	}()
}