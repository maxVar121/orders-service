package app

import (
	"context"
	"log"
	"orders-service/cache"
	"orders-service/database"
	"orders-service/handler"
	"orders-service/server"
	"os"
	"os/signal"
	"syscall"

	"github.com/segmentio/kafka-go"
)

// RunHTTPServer starts the HTTP server in a goroutine
func RunHTTPServer(c *cache.Cache, db *database.Database) {
	go func() {
		httpServer := server.New(c, db)
		httpServer.Start(":8080")
	}()
}

// RunKafkaReader starts consuming Kafka messages in a goroutine
func RunKafkaReader(reader *kafka.Reader, c *cache.Cache, db *database.Database) {
	ctx := context.Background()
	go func() {
		for {
			msg, err := reader.ReadMessage(ctx)
			if err != nil {
				log.Printf("Error reading message: %v", err)
				continue
			}

			if err := handler.HandleOrder(msg, db, c); err != nil {
				log.Printf("Failed to process message: %v", err)
				continue
			}

			if err := reader.CommitMessages(ctx, msg); err != nil {
				log.Printf("Failed to commit message: %v", err)
			} else {
				log.Printf("Committed message for order %s", msg.Key)
			}
		}
	}()
}

// SetupGracefulShutdown handles SIGTERM to save cache and close resources
func SetupGracefulShutdown(c *cache.Cache, reader *kafka.Reader) {
	go func() {
		ch := make(chan os.Signal, 1)
		signal.Notify(ch, os.Interrupt, syscall.SIGTERM)
		<-ch
		log.Println("Shutting down... Saving cache to file")
		c.Stop()
		if err := c.SaveToFile(); err != nil {
			log.Printf("Failed to save cache: %v", err)
		}
		reader.Close()
		os.Exit(0)
	}()
}