package handlers

import (
	"log"
	"net/http"
	"sync"

	"github.com/gin-gonic/gin"
	"github.com/gorilla/websocket"
)

// Hub maintains the set of connected browser clients and broadcasts messages to them.
type Hub struct {
	mu         sync.RWMutex
	clients    map[*client]struct{}
	broadcast  chan []byte
	register   chan *client
	unregister chan *client
}

type client struct {
	conn *websocket.Conn
	send chan []byte
}

// allowedOrigins restricts which browser origins may open a WebSocket.
// CSRF/WS safety: do not blindly accept all origins in production.
var allowedOrigins = map[string]bool{
	"http://localhost:5173": true, // SvelteKit dev server
	"http://localhost:4173": true, // SvelteKit preview
	"http://localhost:8080": true, // backend-served
}

var upgrader = websocket.Upgrader{
	ReadBufferSize:  1024,
	WriteBufferSize: 1024,
	CheckOrigin: func(r *http.Request) bool {
		origin := r.Header.Get("Origin")
		if origin == "" {
			return true // non-browser clients (e.g. tests) have no Origin
		}
		return allowedOrigins[origin]
	},
}

// NewHub creates a hub ready to be started with Run.
func NewHub() *Hub {
	return &Hub{
		clients:    make(map[*client]struct{}),
		broadcast:  make(chan []byte, 256),
		register:   make(chan *client),
		unregister: make(chan *client),
	}
}

// Run processes register/unregister/broadcast events. Call it in a goroutine.
func (h *Hub) Run() {
	for {
		select {
		case c := <-h.register:
			h.mu.Lock()
			h.clients[c] = struct{}{}
			h.mu.Unlock()
		case c := <-h.unregister:
			h.mu.Lock()
			if _, ok := h.clients[c]; ok {
				delete(h.clients, c)
				close(c.send)
			}
			h.mu.Unlock()
		case msg := <-h.broadcast:
			h.mu.RLock()
			for c := range h.clients {
				select {
				case c.send <- msg:
				default:
					// Slow client: drop it rather than blocking the whole hub.
					close(c.send)
					delete(h.clients, c)
				}
			}
			h.mu.RUnlock()
		}
	}
}

// Broadcast queues a raw message to be sent to all connected clients.
func (h *Hub) Broadcast(msg []byte) {
	select {
	case h.broadcast <- msg:
	default:
		log.Println("hub: broadcast buffer full, dropping message")
	}
}

// HandleWS is the Gin handler that upgrades an HTTP request to a WebSocket.
func (h *Hub) HandleWS(c *gin.Context) {
	conn, err := upgrader.Upgrade(c.Writer, c.Request, nil)
	if err != nil {
		// Generic client-facing error; details stay server-side only.
		log.Printf("hub: websocket upgrade failed: %v", err)
		return
	}

	cl := &client{conn: conn, send: make(chan []byte, 256)}
	h.register <- cl

	go cl.writePump()
	go cl.readPump(h)
}

// writePump forwards messages from the send channel to the WebSocket.
func (c *client) writePump() {
	defer c.conn.Close()
	for msg := range c.send {
		if err := c.conn.WriteMessage(websocket.TextMessage, msg); err != nil {
			return
		}
	}
	// Channel closed by hub: send a clean close frame.
	_ = c.conn.WriteMessage(websocket.CloseMessage, websocket.FormatCloseMessage(websocket.CloseNormalClosure, ""))
}

// readPump drains incoming messages (we don't expect any) and detects disconnects.
func (c *client) readPump(h *Hub) {
	defer func() {
		h.unregister <- c
		c.conn.Close()
	}()
	for {
		if _, _, err := c.conn.ReadMessage(); err != nil {
			return
		}
	}
}
