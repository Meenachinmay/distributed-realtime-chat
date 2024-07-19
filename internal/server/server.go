package server

import (
	"distributed-realtime-chat/internal/chat"
	"distributed-realtime-chat/pkg/utils"
	"log"
	"sync"

	"google.golang.org/grpc/codes"
	"google.golang.org/grpc/status"
)

type ChatServer struct {
	chat.UnimplementedChatServiceServer
	mu         sync.RWMutex
	clients    map[string]map[string]chat.ChatService_ChatServer
	workerPool *utils.WorkerPool
}

func NewChatServer(workerCount int) *ChatServer {
	s := &ChatServer{
		clients:    make(map[string]map[string]chat.ChatService_ChatServer),
		workerPool: utils.NewWorkerPool(workerCount),
	}
	s.workerPool.Start()
	return s
}

func (s *ChatServer) Chat(stream chat.ChatService_ChatServer) error {
	var userID, roomID string

	for {
		msg, err := stream.Recv()
		if err != nil {
			s.removeClient(userID, roomID)
			return status.Errorf(codes.Unavailable, "Error receiving message: %v", err)
		}

		if userID == "" {
			userID = msg.UserId
			roomID = msg.RoomId
			s.addClient(userID, roomID, stream)
		}

		s.broadcastMessage(msg, roomID)
	}
}

func (s *ChatServer) addClient(userID, roomID string, stream chat.ChatService_ChatServer) {
	s.mu.Lock()
	defer s.mu.Unlock()

	if _, ok := s.clients[roomID]; !ok {
		s.clients[roomID] = make(map[string]chat.ChatService_ChatServer)
	}
	s.clients[roomID][userID] = stream
	log.Printf("New client connected: User ID: %s, Room ID: %s", userID, roomID)
}

func (s *ChatServer) removeClient(userID, roomID string) {
	s.mu.Lock()
	defer s.mu.Unlock()

	if _, ok := s.clients[roomID]; ok {
		delete(s.clients[roomID], userID)
		log.Printf("Client disconnected: User ID: %s, Room ID: %s", userID, roomID)
	}
}

func (s *ChatServer) broadcastMessage(msg *chat.ChatMessage, roomID string) {
	s.mu.RLock()
	defer s.mu.RUnlock()

	for userID, stream := range s.clients[roomID] {
		if userID != msg.UserId {
			userID := userID
			stream := stream
			s.workerPool.Submit(func() {
				err := stream.Send(msg)
				if err != nil {
					log.Printf("Error sending message to user %s: %v", userID, err)
				}
			})
		}
	}
}

func (s *ChatServer) Stop() {
	s.workerPool.Stop()
}
