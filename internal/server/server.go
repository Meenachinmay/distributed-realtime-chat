package server

import (
	"distributed-realtime-chat/internal/chat"
	"distributed-realtime-chat/pkg/utils"
	"google.golang.org/grpc/codes"
	"google.golang.org/grpc/status"
	"log"
	"sync"
)

type ChatServer struct {
	chat.UnimplementedChatServiceServer
	mu            sync.RWMutex
	clients       map[string]map[string]*clientInfo
	workerPool    *utils.WorkerPool
	broadcastChan chan broadcastJob
}

type broadcastJob struct {
	msg    *chat.ChatMessage
	roomID string
}

type clientInfo struct {
	stream chat.ChatService_ChatServer
	done   chan struct{}
}

func NewChatServer(workerCount int) *ChatServer {
	s := &ChatServer{
		clients:       make(map[string]map[string]*clientInfo),
		workerPool:    utils.NewWorkerPool(workerCount),
		broadcastChan: make(chan broadcastJob, 1000000),
	}
	s.workerPool.Start()
	for i := 0; i < workerCount; i++ {
		go s.broadcastWorker()
	}
	return s
}

func (s *ChatServer) broadcastWorker() {
	for job := range s.broadcastChan {
		s.doBroadcastMessage(job.msg, job.roomID)
	}
}

func (s *ChatServer) broadcastMessage(msg *chat.ChatMessage, roomID string) {
	select {
	case s.broadcastChan <- broadcastJob{msg: msg, roomID: roomID}:
	default:
		// If the channel is full, process the message immediately
		s.doBroadcastMessage(msg, roomID)
	}
}

func (s *ChatServer) doBroadcastMessage(msg *chat.ChatMessage, roomID string) {
	s.mu.RLock()
	clients := make([]*clientInfo, 0, len(s.clients[roomID]))
	for userID, client := range s.clients[roomID] {
		if userID != msg.UserId {
			clients = append(clients, client)
		}
	}
	s.mu.RUnlock()

	for _, client := range clients {
		client := client
		s.workerPool.Submit(func() {
			select {
			case <-client.done:
				return
			default:
				err := client.stream.Send(msg)
				if err != nil {
					log.Printf("Error sending message: %v", err)
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
