package ws

import (
	"encoding/json"
	"net/http"
	"sync"

	"github.com/gin-gonic/gin"
	"github.com/gorilla/websocket"
)

type Message struct {
	Stage   string `json:"stage"`
	Percent int    `json:"percent,omitempty"`
	Error   string `json:"error,omitempty"`
}

var upgrader = websocket.Upgrader{
	CheckOrigin: func(r *http.Request) bool { return true },
}

type Hub struct {
	mu   sync.RWMutex
	subs map[string][]*websocket.Conn
}

func NewHub() *Hub { return &Hub{subs: make(map[string][]*websocket.Conn)} }

func (h *Hub) Broadcast(docID string, msg Message) {
	b, _ := json.Marshal(msg)
	h.mu.RLock()
	conns := h.subs[docID]
	h.mu.RUnlock()
	for _, c := range conns {
		c.WriteMessage(websocket.TextMessage, b) //nolint:errcheck
	}
}

func (h *Hub) Handler(c *gin.Context) {
	docID := c.Param("id")
	conn, err := upgrader.Upgrade(c.Writer, c.Request, nil)
	if err != nil {
		return
	}
	h.mu.Lock()
	h.subs[docID] = append(h.subs[docID], conn)
	h.mu.Unlock()

	defer func() {
		h.mu.Lock()
		conns := h.subs[docID]
		for i, co := range conns {
			if co == conn {
				h.subs[docID] = append(conns[:i], conns[i+1:]...)
				break
			}
		}
		h.mu.Unlock()
		conn.Close()
	}()

	for {
		if _, _, err := conn.ReadMessage(); err != nil {
			break
		}
	}
}
