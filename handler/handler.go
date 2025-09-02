package handler

import (
	"encoding/json"
	"errors"
	"fmt"
	"log"
	"orders-service/cache"
	"orders-service/database"
	"orders-service/model"

	"github.com/segmentio/kafka-go"
)

// HandleOrder processes an incoming Kafka message with order data
func HandleOrder(msg kafka.Message, db *database.Database, c *cache.Cache) error {
    log.Printf("Received message: key=%s, value=%s", string(msg.Key), string(msg.Value))
    if len(msg.Value) == 0 {
        log.Printf("Empty message received, skipping")
        return nil // Commit to avoid re-reading
    }

    var order model.Order
    if err := json.Unmarshal(msg.Value, &order); err != nil {
        return fmt.Errorf("failed to unmarshal json: %w", err)
    }

    log.Printf("Order parsed: order_uid=%s", order.OrderUID)

    if order.OrderUID == "" {
        return fmt.Errorf("empty order_uid")
    }

    // Check for duplicate in cache
    if _, found := c.Get(order.OrderUID); found {
        log.Printf("Order %s already exists, skipping", order.OrderUID)
        return nil // Commit
    }

    // Save to database
    if err := db.MakeOrder(order); err != nil {
        if errors.Is(err, model.ErrOrderExists) {
            log.Printf("Order %s already exists, skipping", order.OrderUID)
            return nil
        }
        return fmt.Errorf("failed to save order to DB: %w", err)
    }

    // Cache order
    c.Set(order, cache.DefaultTTL)
    log.Printf("Order %s saved and cached", order.OrderUID)

    return nil
}