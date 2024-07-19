package main

import (
	"context"
	"distributed-realtime-chat/internal/chat"
	"distributed-realtime-chat/internal/client"
	"flag"
	"fmt"
	"log"
	"math/rand"
	"sync"
	"sync/atomic"
	"time"
)

var (
	numClients  = flag.Int("clients", 20000, "Number of clients to simulate")
	serverAddr  = flag.String("server", "localhost:50051", "Server address")
	roomID      = flag.String("room", "FUJI123", "Chat room ID")
	duration    = flag.Duration("duration", 3*time.Minute, "Duration of the load test")
	msgInterval = flag.Duration("interval", 100*time.Millisecond, "Interval between messages for each client")
	poolSize    = flag.Int("pool", 1000, "Connection pool size")
)

func main() {
	flag.Parse()

	ctx, cancel := context.WithTimeout(context.Background(), *duration)
	defer cancel()

	var wg sync.WaitGroup
	var messagesSent int64
	var errors int64
	var connectedClients int32

	var connectionErrors int64

	clientIncrement := *numClients / 20
	for currentClients := clientIncrement; currentClients <= *numClients; currentClients += clientIncrement {
		for i := currentClients - clientIncrement + 1; i <= currentClients; i++ {
			wg.Add(1)
			go func(clientID int) {
				defer wg.Done()

				chatClient, err := client.NewChatClient(*serverAddr, *poolSize)
				if err != nil {
					log.Printf("Failed to create chat client %d: %v", clientID, err)
					atomic.AddInt64(&errors, 1)
					return
				}
				defer chatClient.Close()

				err = chatClient.Connect(ctx)
				if err != nil {
					log.Printf("Failed to connect client %d: %v", clientID, err)
					atomic.AddInt64(&errors, 1)
					return
				}

				atomic.AddInt32(&connectedClients, 1)
				userID := fmt.Sprintf("user_%d", clientID)

				// Send initial message to join the room
				err = chatClient.SendMessage(&chat.ChatMessage{
					UserId:  userID,
					Content: "Joined the chat",
					RoomId:  *roomID,
				})
				if err != nil {
					log.Printf("Failed to send initial message for client %d: %v", clientID, err)
					atomic.AddInt64(&errors, 1)
					return
				}

				ticker := time.NewTicker(*msgInterval)
				defer ticker.Stop()

				for {
					select {
					case <-ctx.Done():
						return
					case <-ticker.C:
						msg := fmt.Sprintf("Message from %s: %d", userID, rand.Intn(1000))
						err := chatClient.SendMessage(&chat.ChatMessage{
							UserId:  userID,
							Content: msg,
							RoomId:  *roomID,
						})
						if err != nil {
							log.Printf("Failed to send message for client %d: %v", clientID, err)
							atomic.AddInt64(&errors, 1)
						} else {
							atomic.AddInt64(&messagesSent, 1)
						}
					}
				}
			}(i)
			time.Sleep(time.Millisecond)
		}
		time.Sleep(3 * time.Second)
		log.Printf("Connected clients: %d, Connection errors: %d", atomic.LoadInt32(&connectedClients), atomic.LoadInt64(&connectionErrors))

	}

	// Start a goroutine to periodically print statistics
	go func() {
		ticker := time.NewTicker(1 * time.Second)
		defer ticker.Stop()
		startTime := time.Now()

		for {
			select {
			case <-ctx.Done():
				return
			case <-ticker.C:
				sent := atomic.LoadInt64(&messagesSent)
				errs := atomic.LoadInt64(&errors)
				connected := atomic.LoadInt32(&connectedClients)
				elapsed := time.Since(startTime)
				rate := float64(sent) / elapsed.Seconds()
				log.Printf("Connected clients: %d, Messages sent: %d, Errors: %d, Rate: %.2f msgs/sec", connected, sent, errs, rate)
			}
		}
	}()

	wg.Wait()

	totalSent := atomic.LoadInt64(&messagesSent)
	totalErrors := atomic.LoadInt64(&errors)
	totalConnected := atomic.LoadInt32(&connectedClients)
	totalRate := float64(totalSent) / duration.Seconds()
	log.Printf("Load test completed. Total connected clients: %d, Total messages sent: %d, Total errors: %d, Average rate: %.2f msgs/sec", totalConnected, totalSent, totalErrors, totalRate)
}
