package ws

import (
	"encoding/json"
	"log"
	"net/http"
	"rollcall-pro/internal/models"
	"sync"

	"github.com/gin-gonic/gin"
	"github.com/gorilla/websocket"
)

var upgrader = websocket.Upgrader{
	CheckOrigin: func(r *http.Request) bool {
		return true // 允许所有来源，方便学生端访问
	},
	ReadBufferSize:  1024,
	WriteBufferSize: 1024,
}

type Hub struct {
	mu           sync.RWMutex
	clients      map[*Client]bool
	state        *models.InteractionState
	teacherConn  *Client
	broadcast    chan []byte
	register     chan *Client
	unregister   chan *Client
}

type Client struct {
	hub    *Hub
	conn   *websocket.Conn
	send   chan []byte
	isTeacher bool
}

func NewHub() *Hub {
	return &Hub{
		clients:   make(map[*Client]bool),
		state:     nil,
		broadcast: make(chan []byte),
		register:  make(chan *Client),
		unregister: make(chan *Client),
	}
}

func (h *Hub) Run() {
	for {
		select {
		case client := <-h.register:
			h.mu.Lock()
			h.clients[client] = true
			if client.isTeacher {
				h.teacherConn = client
			}
			h.mu.Unlock()
			// 发送当前状态给新连接
			if h.state != nil {
				msg := models.WSMessage{Type: "state", Payload: h.state}
				data, _ := json.Marshal(msg)
				client.send <- data
			}

		case client := <-h.unregister:
			h.mu.Lock()
			if _, ok := h.clients[client]; ok {
				delete(h.clients, client)
				close(client.send)
				if client == h.teacherConn {
					h.teacherConn = nil
				}
			}
			h.mu.Unlock()

		case message := <-h.broadcast:
			h.mu.RLock()
			for client := range h.clients {
				select {
				case client.send <- message:
				default:
					close(client.send)
					delete(h.clients, client)
				}
			}
			h.mu.RUnlock()
		}
	}
}

func (h *Hub) UpdateState(state *models.InteractionState) {
	h.mu.Lock()
	h.state = state
	h.mu.Unlock()
	msg := models.WSMessage{Type: "state", Payload: state}
	data, _ := json.Marshal(msg)
	h.broadcast <- data
}

func (h *Hub) BroadcastTask(task *models.Task) {
	msg := models.WSMessage{Type: "new_task", Payload: task}
	data, _ := json.Marshal(msg)
	h.broadcast <- data
}

func (h *Hub) BroadcastFeedback(taskID string, feedbacks []models.TaskFeedback, total int) {
	payload := map]interface{}{
		"task_id":    taskID,
		"feedbacks": feedbacks,
		"total":      total,
	}
	msg := models.WSMessage{Type: "feedback_update", Payload: payload}
	data, _ := json.Marshal(msg)
	h.broadcast <- data
}

func (h *Hub) HandleWebSocket(c *gin.Context, isTeacher bool) {
	conn, err := upgrader.Upgrade(c.Writer, c.Request, nil)
	if err != nil {
		log.Printf("WebSocket upgrade error: %v", err)
		return
	}
	client := &Client{hub: h, conn: conn, send: make(chan []byte, 256), isTeacher: isTeacher}
	h.register <- client

	go client.writePump()
	go client.readPump()
}

func (c *Client) readPump() {
	defer func() {
		c.hub.unregister <- c
		c.conn.Close()
	}()
	for {
		_, message, err := c.conn.ReadMessage()
		if err != nil {
			break
		}
		// 处理学生反馈
		var msg models.WSMessage
		if err := json.Unmarshal(message, &msg); err == nil {
			if msg.Type == "task_feedback" {
				// 转发给教师端处理
				if c.hub.teacherConn != nil {
					c.hub.teacherConn.send <- message
				}
			}
		}
	}
}

func (c *Client) writePump() {
	defer c.conn.Close()
	for {
		message, ok := <-c.send
		if !ok {
			c.conn.WriteMessage(websocket.CloseMessage, []byte{})
			return
		}
		if err := c.conn.WriteMessage(websocket.TextMessage, message); err != nil {
			return
		}
	}
}
