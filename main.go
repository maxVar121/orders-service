package main

import (
	"log"
	"orders-service/app"
)

func main() {
	db, err := app.InitializeDatabase()
	if err != nil {
		log.Fatal("Failed to connect to PostgreSQL:", err)
	}
	defer db.Pool.Close()

	c, err := app.InitializeCache(db)
	if err != nil {
		log.Printf("Failed to initialize cache: %v", err)
	}

	reader := app.InitializeReader()

	log.Println("Service started. Waiting for messages from Kafka...")

	app.RunHTTPServer(c, db)

	app.RunKafkaReader(reader, c, db)

	app.SetupGracefulShutdown(c, reader)

	select{}
}