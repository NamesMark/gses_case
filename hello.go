package main

import (
	"fmt"
	"log"
	"net/http"
	"time"

	"github.com/gin-gonic/gin"
	"github.com/jmoiron/sqlx"
	_ "github.com/mattn/go-sqlite3"
)

var db *sqlx.DB

type RateMeasurement struct {
	Timestamp time.Time `db:"timestamp" json:"timestamp"`
	Value     float64   `db:"value" json:"value"`
}

type Subscription struct {
	Timestamp time.Time `db:"timestamp" json:"timestamp"`
	Email     string    `db:"email" json:"email"`
}

type ExchangeRateResp struct {
	RatesMap map[string]float64 `json:"ratesmap"`
	Date     time.Time          `json:"date"`
}

func getRate() (RateMeasurement, error) {
	rate, err := getLastRate()
	if err != nil || time.Since(rate.Timestamp) > time.Hour {
		err := updateRate()
		if err != nil {
			return RateMeasurement{}, err
		}
		rate, err = getRate()
		if err != nil {
			return RateMeasurement{}, err
		}
	}
	return rate, nil
}

func trySubscribe(email string) error {
	// if database has email -> return error
	// otherwise add a new email to the database
}

func getLastRate() (RateMeasurement, error) {
	var rate RateMeasurement
	err := db.Get(&rate, "SELECT timestamp, value FROM usd_uah_rates ORDER BY timestamp DESC LIMIT 1")
	if err != nil {
		return RateMeasurement{}, err
	}
	return rate, nil
}

func updateRate() error {
	// go to USD/UAH exchange API and add a new rate measurement to the database
	resp, err := http.Get("https://api.exchangerate-api.com/v4/latest/USD")
	if err != nil {
		return err
	}

	defer resp.Body.Close()
}

func setupDatabase() *sqlx.DB {
	db, err := sqlx.Connect("sqlite3", "exchange.db")
	if err != nil {
		log.Fatalln(err)
	}
	return db
}

func setupRouter() *gin.Engine {
	r := gin.Default()

	r.GET("/ping", func(c *gin.Context) {
		c.String(http.StatusOK, "pong")
	})

	r.GET("/rate", func(c *gin.Context) {
		var rate, err = getRate()
		if err != nil {
			c.String(http.StatusBadRequest, fmt.Sprintf("bad request, error: %s", err.Error()))
		}
		c.String(http.StatusOK, fmt.Sprintf("%.2f", rate.Value))
	})

	r.POST("/subscribe/:email", func(c *gin.Context) {
		err := trySubscribe(c.Param("email"))
		if err != nil {
			c.JSON(http.StatusConflict, gin.H{"status": "subscription conflict", "error": err.Error()})
		} else {
			c.JSON(http.StatusOK, gin.H{"status": "subscribed"})
		}
	})

	return r
}

func main() {
	r := setupRouter()
	r.Run(":8080")
}
