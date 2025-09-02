package server

import (
	"encoding/json"
	"html/template"
	"log"
	"net/http"
	"orders-service/cache"
	"orders-service/database"
	"orders-service/model"
	"path/filepath"
	"regexp"
)

type Server struct {
	Cache     *cache.Cache
	Database  *database.Database
	templates *template.Template
}

// New creates a new HTTP server with access to cache and database
func New(cache *cache.Cache, db *database.Database) *Server {
	// Load templates from the templates directory
	templates, err := template.ParseFiles(filepath.Join("templates/index.html"))
	if err != nil {
		log.Fatal("Failed to load template: ", err)
	}

	return &Server{
		Cache:     cache,
		Database:  db,
		templates: templates,
	}
}

// Start launches the HTTP server on the specified address
func (s *Server) Start(addr string) {
	// Route handlers
	http.HandleFunc("/", s.indexHandler)
	http.HandleFunc("/order/", s.orderAPIHandler)

	log.Printf("HTTP server started on %s", addr)
	log.Fatal(http.ListenAndServe(addr, nil))
}

// indexHandler serves the main HTML page
func (s *Server) indexHandler(w http.ResponseWriter, r *http.Request) {
	if r.URL.Path != "/" {
		http.NotFound(w, r)
		return
	}
	s.renderTemplate(w, "index.html", nil)
}

// orderAPIHandler handles GET /order/{id}: returns order from cache or DB
func (s *Server) orderAPIHandler(w http.ResponseWriter, r *http.Request) {
    if r.Method != "GET" {
        http.Error(w, "Method not supported", http.StatusMethodNotAllowed)
        return
    }

    // Extract order_id from /order/123
    path := r.URL.Path
    re := regexp.MustCompile(`^/order/([a-zA-Z0-9\-]+)$`)
    matches := re.FindStringSubmatch(path)

    if len(matches) < 2 {
        http.Error(w, "Invalid order ID", http.StatusBadRequest)
        return
    }
    orderID := matches[1]

    log.Printf("HTTP: requested order %s", orderID)

    // 1. Check cache
    if order, found := s.Cache.Get(orderID); found {
        log.Printf("Order %s found in cache", orderID)
        s.sendJSON(w, order)
        return
    }

    // 2. If not in cache, query database
    items, err := s.Database.ItemsInfo(orderID)
    if err != nil {
        log.Printf("Error retrieving items: %v", err)
        http.Error(w, "Order not found", http.StatusNotFound)
        return
    }
    if len(items) == 0 {
        log.Printf("No items found for order %s", orderID)
        http.Error(w, "Order not found", http.StatusNotFound)
        return
    }

    log.Printf("Found %d items for order %s", len(items), orderID)

    // Construct simplified response
    response := struct {
        OrderUID string         `json:"order_uid"`
        Items    []ItemResponse `json:"items"`
    }{
        OrderUID: orderID,
        Items:    make([]ItemResponse, 0, len(items)),
    }

    for _, item := range items {
        response.Items = append(response.Items, ItemResponse{
            Name:       item.Name,
            Price:      item.Price,
            Size:       item.Size,
            TotalPrice: item.TotalPrice,
            Brand:      item.Brand,
        })
    }

    // Cache the response
    s.Cache.Set(orderToModel(response), cache.DefaultTTL)
    log.Printf("Order %s loaded from DB and added to cache", orderID)

    s.sendJSON(w, response)
}

type ItemResponse struct {
	Name       string `json:"name"`
	Price      int    `json:"price"`
	Size       string `json:"size"`
	TotalPrice int    `json:"total_price"`
	Brand      string `json:"brand"`
}

// sendJSON serializes and sends a JSON response with proper headers
func (s *Server) sendJSON(w http.ResponseWriter, data interface{}) {
	w.Header().Set("Content-Type", "application/json")
	json.NewEncoder(w).Encode(data)
}

// renderTemplate executes an HTML template
func (s *Server) renderTemplate(w http.ResponseWriter, tmpl string, data interface{}) {
	err := s.templates.ExecuteTemplate(w, tmpl, data)
	if err != nil {
		http.Error(w, "Template error: "+err.Error(), http.StatusInternalServerError)
		log.Printf("Template rendering error: %v", err)
	}
}

// orderToModel converts a simplified JSON response back into a model.Order for caching
func orderToModel(r struct {
	OrderUID string         `json:"order_uid"`
	Items    []ItemResponse `json:"items"`
}) model.Order {
	items := make([]model.Item, len(r.Items))
	for i, it := range r.Items {
		items[i] = model.Item{
			Name:       it.Name,
			Price:      it.Price,
			Size:       it.Size,
			TotalPrice: it.TotalPrice,
			Brand:      it.Brand,
		}
	}
	return model.Order{
		OrderUID: r.OrderUID,
		Items:    items,
	}
}