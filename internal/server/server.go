package server

import (
	"distributed-realtime-chat/internal/chat"
	"distributed-realtime-chat/pkg/utils"
	"google.golang.org/grpc/codes"
	"google.golang.org/grpc/status"
	"log"
	"sync"
	"time"
)

type ChatServer struct {
	chat.UnimplementedChatServiceServer
	mu            sync.RWMutex
	clients       map[string]map[string]*clientInfo
	workerPool    *utils.WorkerPool
	broadcastChan chan broadcastJob
	config        ServerConfig
}

type ServerConfig struct {
	WorkerCount   int
	BatchInterval time.Duration
	BatchSize     int
}

type broadcastJob struct {
	msg    *chat.ChatMessage
	roomID string
}

type MessageBatch struct {
	Messages []*chat.ChatMessage
	RoomID   string
}

type clientInfo struct {
	stream chat.ChatService_ChatServer
	done   chan struct{}
}

var messagePool = sync.Pool{
	New: func() interface{} {
		return &chat.ChatMessage{}
	},
}

func NewChatServer(config ServerConfig) *ChatServer {
	s := &ChatServer{
		clients:       make(map[string]map[string]*clientInfo),
		workerPool:    utils.NewWorkerPool(config.WorkerCount),
		broadcastChan: make(chan broadcastJob, 1000000),
		config:        config,
	}
	s.workerPool.Start()
	go s.batchAndBroadcast()
	return s
}

func (s *ChatServer) batchAndBroadcast() {
	batch := make(map[string][]*chat.ChatMessage)
	ticker := time.NewTicker(s.config.BatchInterval)
	defer ticker.Stop()

	for {
		select {
		case job := <-s.broadcastChan:
			batch[job.roomID] = append(batch[job.roomID], job.msg)
			if len(batch[job.roomID]) >= s.config.BatchSize {
				s.doBroadcastBatch(batch[job.roomID], job.roomID)
				batch[job.roomID] = batch[job.roomID][:0]
			}
		case <-ticker.C:
			for roomID, messages := range batch {
				if len(messages) > 0 {
					s.doBroadcastBatch(messages, roomID)
					batch[roomID] = batch[roomID][:0]
				}
			}
		}
	}
}

func (s *ChatServer) doBroadcastBatch(messages []*chat.ChatMessage, roomID string) {
	s.mu.RLock()
	clients := make([]*clientInfo, 0, len(s.clients[roomID]))
	for _, client := range s.clients[roomID] {
		clients = append(clients, client)
	}
	s.mu.RUnlock()

	for _, client := range clients {
		client := client
		s.workerPool.Submit(func() {
			select {
			case <-client.done:
				return
			default:
				for _, msg := range messages {
					err := client.stream.Send(msg)
					if err != nil {
						log.Printf("Error sending message: %v", err)
						return
					}
				}
			}
		})
	}
}

func (s *ChatServer) Chat(stream chat.ChatService_ChatServer) error {
	var userID, roomID string
	done := make(chan struct{})

	for {
		msg, err := stream.Recv()
		if err != nil {
			s.removeClient(userID, roomID)
			close(done)
			return status.Errorf(codes.Unavailable, "Error receiving message: %v", err)
		}

		if userID == "" {
			userID = msg.UserId
			roomID = msg.RoomId
			s.addClient(userID, roomID, stream, done)
		}

		s.broadcastMessage(msg, roomID)
	}
}

func (s *ChatServer) broadcastMessage(msg *chat.ChatMessage, roomID string) {
	s.broadcastChan <- broadcastJob{msg: msg, roomID: roomID}
}

func (s *ChatServer) addClient(userID, roomID string, stream chat.ChatService_ChatServer, done chan struct{}) {
	s.mu.Lock()
	defer s.mu.Unlock()

	if _, ok := s.clients[roomID]; !ok {
		s.clients[roomID] = make(map[string]*clientInfo)
	}
	s.clients[roomID][userID] = &clientInfo{stream: stream, done: done}
}

func (s *ChatServer) removeClient(userID, roomID string) {
	s.mu.Lock()
	defer s.mu.Unlock()

	if _, ok := s.clients[roomID]; ok {
		if client, ok := s.clients[roomID][userID]; ok {
			close(client.done)
			delete(s.clients[roomID], userID)
		}
	}
}

func (s *ChatServer) Stop() {
	s.workerPool.Stop()
}
