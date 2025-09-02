package app

import (
	"log"
	"orders-service/cache"
	"orders-service/database"
	"time"

	"github.com/segmentio/kafka-go"
)

// InitializeDatabase connects to PostgreSQL and returns a new Database instance
func InitializeDatabase() (*database.Database, error) {
	db, err := database.New()
	if err != nil {
		return nil, err
	}
	return db, nil
}

// InitializeCache loads cached orders from file or falls back to DB
func InitializeCache(db *database.Database) (*cache.Cache, error) {
	c := cache.New("order_cache.gob")

	if err := c.LoadFromFile(); err != nil {
		log.Printf("No cache file found, loading from DB: %v", err)
	}

	ordersFromDB, err := db.GetAllOrders()
	if err != nil {
		log.Printf("Failed to load orders from DB: %v", err)
	} else {
		loaded := 0
		for _, order := range ordersFromDB {
			if _, found := c.Get(order.OrderUID); !found {
				c.Set(order, cache.NoExpiration)
				loaded++
			}
		}
		log.Printf("Loaded %d orders from DB into cache", loaded)
	}

	return c, nil
}

// InitializeReader creates a Kafka reader for the "orders" topic
func InitializeReader() *kafka.Reader {
	reader := kafka.NewReader(kafka.ReaderConfig{
		Brokers:        []string{"kafka:9092"},
		Topic:          "orders",
		GroupID:        "order-service-group",
		CommitInterval: 0,
		MaxWait:        1 * time.Second,
	})

	return reader
}