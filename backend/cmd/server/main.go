// Command server is the Watchtower backend. It wires everything together:
// streams ticks from Finnhub, batches them into TimescaleDB, runs the anomaly
// detector, hands spikes off to Kafka for sentiment analysis, and pushes
// everything to the browser over a websocket. The cron pollers (congress, IPO)
// run alongside it.
package main

import (
	"context"
	"encoding/json"
	"log"
	"net/http"
	"os/signal"
	"regexp"
	"strings"
	"syscall"
	"time"

	"github.com/gin-gonic/gin"

	"github.com/ishraqb/Watchtower/backend/internal/anomaly"
	"github.com/ishraqb/Watchtower/backend/internal/config"
	"github.com/ishraqb/Watchtower/backend/internal/congress"
	"github.com/ishraqb/Watchtower/backend/internal/db"
	"github.com/ishraqb/Watchtower/backend/internal/finnhub"
	"github.com/ishraqb/Watchtower/backend/internal/handlers"
	"github.com/ishraqb/Watchtower/backend/internal/kafka"
	rediscache "github.com/ishraqb/Watchtower/backend/internal/redis"
)

// devSymbol validates the symbol path param on the dev simulate endpoint.
var devSymbol = regexp.MustCompile(`^[A-Z.]{1,10}$`)

// wsTickPayload is the JSON shape broadcast to browser clients.
type wsTickPayload struct {
	Type   string  `json:"type"`
	Symbol string  `json:"symbol"`
	Price  float64 `json:"price"`
	Volume int     `json:"volume"`
	Time   int64   `json:"time"`
}

// wsSentimentPayload is broadcast when the worker returns a sentiment result.
type wsSentimentPayload struct {
	Type           string  `json:"type"`
	EventID        int     `json:"event_id"`
	Symbol         string  `json:"symbol"`
	SentimentScore float64 `json:"sentiment_score"`
	ArticleCount   int     `json:"article_count"`
	TopHeadline    string  `json:"top_headline"`
}

// wsAnomalyPayload is broadcast the moment a volume spike is detected.
type wsAnomalyPayload struct {
	Type          string  `json:"type"`
	EventID       int     `json:"event_id"`
	Symbol        string  `json:"symbol"`
	TriggerVolume int     `json:"trigger_volume"`
	AvgVolume     float64 `json:"avg_volume"`
	Time          int64   `json:"time"`
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

	producer, err := kafka.NewProducer(cfg.KafkaBrokers)
	if err != nil {
		log.Fatalf("startup: %v", err)
	}
	defer producer.Close()
	detector := anomaly.NewDetector()
	log.Println("startup: connected to Kafka producer")

	sentimentConsumer, err := kafka.NewConsumer(cfg.KafkaBrokers, "watchtower-backend", func(msg kafka.SentimentMessage) {
		payload, err := json.Marshal(wsSentimentPayload{
			Type:           "sentiment",
			EventID:        msg.EventID,
			Symbol:         msg.Symbol,
			SentimentScore: msg.SentimentScore,
			ArticleCount:   msg.ArticleCount,
			TopHeadline:    msg.TopHeadline,
		})
		if err == nil {
			hub.Broadcast(payload)
		}
	})
	if err != nil {
		log.Fatalf("startup: %v", err)
	}
	defer sentimentConsumer.Close()
	go sentimentConsumer.Run(ctx)
	log.Println("startup: connected to Kafka consumer")

	go consumeTicks(ctx, database, hub, fh, detector, producer)

	congressPoller := congress.NewPoller(database)
	go congressPoller.Start(ctx)

	ipoPoller := finnhub.NewIPOPoller(database, cfg.FinnhubAPIKey, finnhubLimiter)
	go ipoPoller.Start(ctx)

	api := handlers.NewAPI(database, cfg.FinnhubAPIKey, finnhubLimiter, fh)

	router := gin.Default()
	router.Use(handlers.CORS())
	router.GET("/healthz", func(c *gin.Context) {
		c.JSON(http.StatusOK, gin.H{"status": "ok"})
	})
	router.GET("/ws", hub.HandleWS)
	router.GET("/api/congress/:symbol", api.GetCongressBySymbol)
	router.GET("/api/ipo", api.GetIPOs)
	router.GET("/api/quote/:symbol", api.GetQuote)
	router.GET("/api/history/:symbol", api.GetHistory)
	router.POST("/api/watch/:symbol", api.Watch)

	// Dev-only: simulate a volume spike to exercise the full pipeline off-hours.
	// Guarded by an env flag so it is never reachable in production.
	if cfg.EnableDevEndpoints {
		router.POST("/api/dev/simulate-anomaly/:symbol", func(c *gin.Context) {
			symbol := strings.ToUpper(c.Param("symbol"))
			if !devSymbol.MatchString(symbol) {
				c.JSON(http.StatusBadRequest, gin.H{"error": "invalid symbol"})
				return
			}
			synthetic := db.Tick{Time: time.Now(), Symbol: symbol, Price: 0, Volume: 100000}
			det := anomaly.Detection{Symbol: symbol, TriggerVolume: 100000, AvgVolume: 1000}
			handleAnomaly(database, producer, hub, synthetic, det)
			c.JSON(http.StatusAccepted, gin.H{"status": "anomaly simulated", "symbol": symbol})
		})
		log.Println("startup: dev endpoints ENABLED (POST /api/dev/simulate-anomaly/:symbol)")
	}

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

// consumeTicks fans each tick out to: a batched DB writer, the browser hub,
// and the anomaly detector (which publishes spikes to Kafka).
func consumeTicks(ctx context.Context, database *db.DB, hub *handlers.Hub, fh *finnhub.Client, detector *anomaly.Detector, producer *kafka.Producer) {
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

			if det, fired := detector.Observe(tick); fired {
				handleAnomaly(database, producer, hub, tick, det)
			}
		}
	}
}

// handleAnomaly persists a detected spike and publishes it to Kafka for the
// sentiment worker. Failures are logged but never crash the tick stream.
func handleAnomaly(database *db.DB, producer *kafka.Producer, hub *handlers.Hub, tick db.Tick, det anomaly.Detection) {
	writeCtx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()

	eventID, err := database.InsertAnomaly(writeCtx, tick.Time, det.Symbol, "VOLUME_SPIKE", float64(det.TriggerVolume))
	if err != nil {
		log.Printf("anomaly: insert failed: %v", err)
		return
	}

	// Broadcast immediately so the UI shows the spike before sentiment returns.
	if payload, err := json.Marshal(wsAnomalyPayload{
		Type:          "anomaly",
		EventID:       eventID,
		Symbol:        det.Symbol,
		TriggerVolume: det.TriggerVolume,
		AvgVolume:     det.AvgVolume,
		Time:          tick.Time.UnixMilli(),
	}); err == nil {
		hub.Broadcast(payload)
	}

	if err := producer.PublishAnomaly(kafka.AnomalyMessage{
		EventID:       eventID,
		Symbol:        det.Symbol,
		Time:          tick.Time,
		TriggerVolume: det.TriggerVolume,
		AvgVolume:     det.AvgVolume,
	}); err != nil {
		log.Printf("anomaly: publish failed: %v", err)
		return
	}
	log.Printf("anomaly: %s volume %d vs avg %.0f -> event %d published", det.Symbol, det.TriggerVolume, det.AvgVolume, eventID)
}
