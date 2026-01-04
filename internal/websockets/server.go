package websockets

import (
	"encoding/binary"
	"encoding/json"
	"log"
	"net/http"
	"sync"
	"time"

	"can-service/internal/canbus"
	"can-service/internal/repository"

	"github.com/gorilla/websocket"
)

var upgrader = websocket.Upgrader{
	ReadBufferSize:  1024,
	WriteBufferSize: 1024,
	CheckOrigin:     func(r *http.Request) bool { return true },
}

type Server struct {
	repo        repository.CarRepository
	canListener *canbus.CanListener

	mu    sync.RWMutex
	state map[string]any

	dirtyMu sync.Mutex
	dirty   bool
}

func NewServer(repo repository.CarRepository, listener *canbus.CanListener) *Server {
	return &Server{
		repo:        repo,
		canListener: listener,
		state:       make(map[string]any),
	}
}

func crc32IEEE(b []byte) uint32 {
	const poly uint32 = 0xEDB88320
	var crc uint32 = 0xFFFFFFFF
	for _, v := range b {
		crc ^= uint32(v)
		for i := 0; i < 8; i++ {
			if crc&1 == 1 {
				crc = (crc >> 1) ^ poly
			} else {
				crc >>= 1
			}
		}
	}
	return ^crc
}

func (s *Server) markDirty() {
	s.dirtyMu.Lock()
	s.dirty = true
	s.dirtyMu.Unlock()
}

func (s *Server) takeDirty() bool {
	s.dirtyMu.Lock()
	d := s.dirty
	s.dirty = false
	s.dirtyMu.Unlock()
	return d
}

func (s *Server) ServeHTTP(w http.ResponseWriter, r *http.Request) {
	conn, err := upgrader.Upgrade(w, r, nil)
	if err != nil {
		log.Println(err)
		return
	}
	defer conn.Close()

	_, message, err := conn.ReadMessage()
	if err != nil {
		log.Println(err)
		return
	}
	if len(message) < 9 {
		_ = conn.WriteMessage(websocket.CloseMessage, []byte("Invalid Length"))
		return
	}

	cmd := message[0]
	modelID := binary.LittleEndian.Uint32(message[1:5])
	recvCRC := binary.LittleEndian.Uint32(message[5:9])

	if cmd != 0x01 {
		_ = conn.WriteMessage(websocket.CloseMessage, []byte("Invalid CMD"))
		return
	}
	if recvCRC != crc32IEEE(message[:5]) {
		_ = conn.WriteMessage(websocket.CloseMessage, []byte("Invalid CRC"))
		return
	}

	config, err := s.repo.GetConfigByModelID(modelID)
	if err != nil {
		_ = conn.WriteMessage(websocket.TextMessage, []byte("Unknown Model"))
		return
	}

	log.Printf("handshake ok model=%s id=0x%X", config.Name, modelID)
	s.canListener.Start(config)

	ticker := time.NewTicker(50 * time.Millisecond)
	defer ticker.Stop()

	for range ticker.C {
		st := s.canListener.Status()

		s.mu.Lock()
		s.state["can_ok"] = boolToNum(st.OK)
		if st.Err != "" {
			s.state["can_err"] = st.Err
		} else {
			delete(s.state, "can_err")
		}
		s.mu.Unlock()

		if !s.takeDirty() {
			continue
		}

		s.mu.RLock()
		if len(s.state) == 0 {
			s.mu.RUnlock()
			continue
		}
		jsonData, _ := json.Marshal(s.state)
		s.mu.RUnlock()

		if err := conn.WriteMessage(websocket.TextMessage, jsonData); err != nil {
			return
		}
	}
}

func boolToNum(b bool) int {
	if b {
		return 1
	}
	return 0
}

func (s *Server) ConsumeReadings(readings <-chan canbus.Reading) {
	go func() {
		for r := range readings {
			s.mu.Lock()
			s.state[r.Name] = r.Value
			s.mu.Unlock()
			s.markDirty()
		}
	}()
}
