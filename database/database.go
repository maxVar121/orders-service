package database

import (
	"context"
	"fmt"
	"log"
	"orders-service/model"
	"os"

	"github.com/jackc/pgx/v5/pgxpool"
	"github.com/joho/godotenv"
)

var ctx = context.Background()

type Database struct {
	Pool *pgxpool.Pool
}

// New initializes a connection pool to PostgreSQL using DATABASE_URL
func New() (*Database, error) {
	if err := godotenv.Load(); err != nil {
		if !os.IsNotExist(err) {
			log.Printf("Warning: failed to load .env file: %v", err)
		} else {
			log.Println("No .env file found, using environment variables from Docker")
		}
	}

	database_url := os.Getenv("DATABASE_URL")
	if database_url == "" {
		return nil, fmt.Errorf("database_url is not set")
	}

	pool, err := pgxpool.New(ctx, database_url)
	if err != nil {
		return nil, fmt.Errorf("unable to connect to database: %w", err)
	}

	if err = pool.Ping(ctx); err != nil {
		pool.Close()
		return nil, fmt.Errorf("unable to ping database: %w", err)
	}

	fmt.Println("Connected to PostgreSQL database!")

	return &Database{
		Pool: pool,
	}, nil
}

// MakeOrder inserts a complete order (with delivery, payment, items) in a single transaction
func (db *Database) MakeOrder(order model.Order) error {
	tx, err := db.Pool.Begin(ctx)
	if err != nil {
		return fmt.Errorf("cannot start transaction: %w", err)
	}
	defer tx.Rollback(ctx)

	// Проверка на дубль
	var exists bool
	err = tx.QueryRow(ctx, "SELECT EXISTS(SELECT 1 FROM orders WHERE order_uid = $1)", order.OrderUID).
		Scan(&exists)
	if err != nil {
		return fmt.Errorf("failed to check duplicate: %w", err)
	}
	if exists {
		return model.ErrOrderExists
	}

	_, err = tx.Exec(ctx, `
		INSERT INTO orders (
			order_uid, track_number, entry, locale, internal_signature,
			customer_id, delivery_service, shardkey, sm_id, date_created, oof_shard
		) VALUES ($1, $2, $3, $4, $5, $6, $7, $8, $9, $10, $11)
	`, order.OrderUID, order.TrackNumber, order.Entry, order.Locale, order.InternalSignature,
		order.CustomerID, order.DeliveryService, order.Shardkey, order.SmID, order.DateCreated, order.OofShard)
	if err != nil {
		return fmt.Errorf("failed to create order: %w", err)
	}

	_, err = tx.Exec(ctx, `
		INSERT INTO delivery (
			order_uid, name, phone, zip, city, address, region, email
		) VALUES ($1, $2, $3, $4, $5, $6, $7, $8)
	`, order.OrderUID, order.Delivery.Name, order.Delivery.Phone, order.Delivery.Zip,
		order.Delivery.City, order.Delivery.Address, order.Delivery.Region, order.Delivery.Email)
	if err != nil {
		return fmt.Errorf("failed to create delivery: %w", err)
	}

	_, err = tx.Exec(ctx, `
		INSERT INTO payment (
			transaction, order_uid, request_id, currency, provider,
			amount, payment_dt, bank, delivery_cost, goods_total, custom_fee
		) VALUES ($1, $2, $3, $4, $5, $6, $7, $8, $9, $10, $11)
	`, order.Payment.Transaction, order.OrderUID, order.Payment.RequestID, order.Payment.Currency,
		order.Payment.Provider, order.Payment.Amount, order.Payment.PaymentDt,
		order.Payment.Bank, order.Payment.DeliveryCost, order.Payment.GoodsTotal, order.Payment.CustomFee)
	if err != nil {
		return fmt.Errorf("failed to create payment: %w", err)
	}

	for _, item := range order.Items {
		_, err = tx.Exec(ctx, `
			INSERT INTO items (
				chrt_id, track_number, price, rid, name, sale, size, total_price,
				nm_id, brand, status, order_uid
			) VALUES ($1, $2, $3, $4, $5, $6, $7, $8, $9, $10, $11, $12)
		`, item.ChrtID, item.TrackNumber, item.Price, item.RID, item.Name,
			item.Sale, item.Size, item.TotalPrice, item.NmID, item.Brand, item.Status, order.OrderUID)
		if err != nil {
			return fmt.Errorf("failed to create item: %w", err)
		}
	}

	if err = tx.Commit(ctx); err != nil {
		return fmt.Errorf("failed to commit transaction: %w", err)
	}

	return nil
}

// ItemsInfo retrieves item data for a given order_uid from the database
func (db *Database) ItemsInfo(order_uid string) ([]model.ItemInfo, error) {
	sql := `
	SELECT track_number, name, price,sale, size, total_price, brand
	FROM items WHERE order_uid = $1
	`

	rows, err := db.Pool.Query(ctx, sql, order_uid)
	if err != nil {
		return nil, fmt.Errorf("failed to query items: %w", err)
	}
	defer rows.Close()

	var items []model.ItemInfo
	for rows.Next() {
		var item model.ItemInfo
		err := rows.Scan(
			&item.TrackNumber, &item.Name, &item.Price, &item.Sale,
			&item.Size, &item.TotalPrice, &item.Brand,
		)
		if err != nil {
			return nil, fmt.Errorf("failed to scan item row: %w", err)
		}
		items = append(items, item)
	}

	if err := rows.Err(); err != nil {
		return nil, fmt.Errorf("row iteration error: %w", err)
	}
	if len(items) == 0 {
		return nil, model.ErrOrderNotFound
	}

	return items, nil
}

// DeleteOrder removes an order
func (db *Database) DeleteOrder(order_uid string) error {
	sql := `DELETE FROM orders WHERE order_uid = $1`

	commandTag, err := db.Pool.Exec(ctx, sql, order_uid)
	if err != nil {
		return fmt.Errorf("failed to delete order: %w", err)
	}

	if commandTag.RowsAffected() == 0 {
		return fmt.Errorf("no task found with id %v", order_uid)
	}

	fmt.Println("Successfully deleted!")

	return nil
}

// GetAllOrders loads all orders from the database
func (db *Database) GetAllOrders() (map[string]model.Order, error) {
	sql := `
		SELECT 
			o.order_uid, o.track_number, o.entry, o.locale, o.internal_signature,
			o.customer_id, o.delivery_service, o.shardkey, o.sm_id, o.date_created, o.oof_shard,
			d.name, d.phone, d.zip, d.city, d.address, d.region, d.email,
			p.transaction, p.request_id, p.currency, p.provider, p.amount, p.payment_dt,
			p.bank, p.delivery_cost, p.goods_total, p.custom_fee
		FROM orders o
		LEFT JOIN delivery d ON o.order_uid = d.order_uid
		LEFT JOIN payment p ON o.order_uid = p.order_uid
	`

	rows, err := db.Pool.Query(ctx, sql)
	if err != nil {
		return nil, fmt.Errorf("failed to query all orders: %w", err)
	}
	defer rows.Close()

	orders := make(map[string]model.Order)

	for rows.Next() {
		var order model.Order
		var delivery model.Delivery
		var payment model.Payment

		err := rows.Scan(
			&order.OrderUID, &order.TrackNumber, &order.Entry, &order.Locale, &order.InternalSignature,
			&order.CustomerID, &order.DeliveryService, &order.Shardkey, &order.SmID, &order.DateCreated, &order.OofShard,
			&delivery.Name, &delivery.Phone, &delivery.Zip, &delivery.City, &delivery.Address, &delivery.Region, &delivery.Email,
			&payment.Transaction, &payment.RequestID, &payment.Currency, &payment.Provider, &payment.Amount, &payment.PaymentDt,
			&payment.Bank, &payment.DeliveryCost, &payment.GoodsTotal, &payment.CustomFee,
		)
		if err != nil {
			return nil, fmt.Errorf("failed to scan order row: %w", err)
		}

		items, err := db.ItemsInfo(order.OrderUID)
		if err != nil {
			return nil, fmt.Errorf("failed to load items for order %s: %w", order.OrderUID, err)
		}

		order.Delivery = delivery
		order.Payment = payment
		order.Items = make([]model.Item, 0, len(items))
		for _, item := range items {
			order.Items = append(order.Items, model.Item{
				TrackNumber: item.TrackNumber,
				Name:        item.Name,
				Price:       item.Price,
				Sale:        item.Sale,
				Size:        item.Size,
				TotalPrice:  item.TotalPrice,
				Brand:       item.Brand,
			})
		}

		orders[order.OrderUID] = order
	}

	if err := rows.Err(); err != nil {
		return nil, fmt.Errorf("row iteration error: %w", err)
	}

	return orders, nil
}