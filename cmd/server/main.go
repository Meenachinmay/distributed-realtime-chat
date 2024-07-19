package main

import (
	"distributed-realtime-chat/internal/chat"
	"distributed-realtime-chat/internal/server"
	"google.golang.org/grpc"
	"google.golang.org/grpc/keepalive"
	"log"
	"net"
	"os"
	"os/signal"
	"runtime"
	"syscall"
	"time"
)

func main() {
	// Increase file descriptor limit
	var rLimit syscall.Rlimit
	if err := syscall.Getrlimit(syscall.RLIMIT_NOFILE, &rLimit); err != nil {
		log.Println("Error getting Rlimit:", err)
	}
	rLimit.Cur = rLimit.Max
	if err := syscall.Setrlimit(syscall.RLIMIT_NOFILE, &rLimit); err != nil {
		log.Println("Error setting Rlimit:", err)
	}

	// Increase the number of operating system threads
	runtime.GOMAXPROCS(runtime.NumCPU())

	lis, err := net.Listen("tcp", ":50051")
	if err != nil {
		log.Fatalf("Failed to listen: %v", err)
	}

	// Configure keepalive and other options
	opts := []grpc.ServerOption{
		grpc.KeepaliveParams(keepalive.ServerParameters{
			MaxConnectionIdle: 15 * time.Minute,
			Time:              5 * time.Second,
			Timeout:           1 * time.Second,
		}),
		grpc.KeepaliveEnforcementPolicy(keepalive.EnforcementPolicy{
			MinTime:             5 * time.Second,
			PermitWithoutStream: true,
		}),
		grpc.MaxConcurrentStreams(100000),
		grpc.InitialWindowSize(1 << 26),
		grpc.InitialConnWindowSize(1 << 26),
		grpc.WriteBufferSize(1 << 20),
		grpc.ReadBufferSize(1 << 20),
	}

	s := grpc.NewServer(opts...)
	chatServer := server.NewChatServer(runtime.NumCPU() * 4) // Use 100 workers
	chat.RegisterChatServiceServer(s, chatServer)

	go func() {
		log.Println("Starting gRPC server on :50051")
		if err := s.Serve(lis); err != nil {
			log.Fatalf("Failed to serve: %v", err)
		}
	}()

	// Wait for interrupt signal
	sigChan := make(chan os.Signal, 1)
	signal.Notify(sigChan, syscall.SIGINT, syscall.SIGTERM)
	<-sigChan

	log.Println("Shutting down...")
	s.GracefulStop()
	chatServer.Stop()
}
