package main

import (
	"context"
	"encoding/json"
	"log"
	"net/http"
	"os/signal"
	"syscall"
	"time"

	"github.com/gin-gonic/gin"

	"github.com/ishraqb/Watchtower/backend/internal/config"
	"github.com/ishraqb/Watchtower/backend/internal/congress"
	"github.com/ishraqb/Watchtower/backend/internal/db"
	"github.com/ishraqb/Watchtower/backend/internal/finnhub"
	"github.com/ishraqb/Watchtower/backend/internal/handlers"
	rediscache "github.com/ishraqb/Watchtower/backend/internal/redis"
)

// wsTickPayload is the JSON shape broadcast to browser clients.
type wsTickPayload struct {
	Type   string  `json:"type"`
	Symbol string  `json:"symbol"`
	Price  float64 `json:"price"`
	Volume int     `json:"volume"`
	Time   int64   `json:"time"`
}

func main() {
	cfg := config.Load()

	ctx, stop := signal.NotifyContext(context.Background(), syscall.SIGINT, syscall.SIGTERM)
	defer stop()

	database, err := db.New(ctx, cfg.DatabaseURL)
	if err != nil {
		log.Fatalf("startup: %v", err)
	}
	defer database.Close()
	log.Println("startup: connected to TimescaleDB")

	redisClient, err := rediscache.New(ctx, cfg.RedisURL)
	if err != nil {
		log.Fatalf("startup: %v", err)
	}
	defer redisClient.Close()
	finnhubLimiter := rediscache.NewLimiter(redisClient, rediscache.FinnhubCallLimit, time.Minute)
	log.Println("startup: connected to Redis")

	hub := handlers.NewHub()
	go hub.Run()

	fh := finnhub.NewClient(cfg.FinnhubAPIKey, finnhub.DefaultSymbols)
	go fh.Run(ctx)

	go consumeTicks(ctx, database, hub, fh)

	congressPoller := congress.NewPoller(database)
	go congressPoller.Start(ctx)

	ipoPoller := finnhub.NewIPOPoller(database, cfg.FinnhubAPIKey, finnhubLimiter)
	go ipoPoller.Start(ctx)

	api := handlers.NewAPI(database)

	router := gin.Default()
	router.GET("/healthz", func(c *gin.Context) {
		c.JSON(http.StatusOK, gin.H{"status": "ok"})
	})
	router.GET("/ws", hub.HandleWS)
	router.GET("/api/congress/:symbol", api.GetCongressBySymbol)

	srv := &http.Server{
		Addr:    ":" + cfg.ServerPort,
		Handler: router,
	}

	go func() {
		log.Printf("startup: HTTP server listening on :%s", cfg.ServerPort)
		if err := srv.ListenAndServe(); err != nil && err != http.ErrServerClosed {
			log.Fatalf("server: %v", err)
		}
	}()

	<-ctx.Done()
	log.Println("shutdown: signal received, draining...")

	shutdownCtx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()
	_ = srv.Shutdown(shutdownCtx)
}

// consumeTicks fans each tick out to: a batched DB writer and the browser hub.
func consumeTicks(ctx context.Context, database *db.DB, hub *handlers.Hub, fh *finnhub.Client) {
	const batchSize = 100
	const flushInterval = 2 * time.Second

	batch := make([]db.Tick, 0, batchSize)
	ticker := time.NewTicker(flushInterval)
	defer ticker.Stop()

	flush := func() {
		if len(batch) == 0 {
			return
		}
		writeCtx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
		if _, err := database.BatchInsertTicks(writeCtx, batch); err != nil {
			log.Printf("ingest: batch insert failed: %v", err)
		}
		cancel()
		batch = batch[:0]
	}

	for {
		select {
		case <-ctx.Done():
			flush()
			return
		case <-ticker.C:
			flush()
		case tick := <-fh.Ticks:
			batch = append(batch, tick)
			if len(batch) >= batchSize {
				flush()
			}

			payload, err := json.Marshal(wsTickPayload{
				Type:   "tick",
				Symbol: tick.Symbol,
				Price:  tick.Price,
				Volume: tick.Volume,
				Time:   tick.Time.UnixMilli(),
			})
			if err == nil {
				hub.Broadcast(payload)
			}
		}
	}
}
