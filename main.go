package main

import (
	"encoding/json"
	"fmt"
	"log"
	"net/http"
	"time"

	"github.com/jmoiron/sqlx"
	_ "github.com/mattn/go-sqlite3"

	docs "github.com/NamesMark/gses_case/docs"

	"github.com/gin-gonic/gin"
	swaggerFiles "github.com/swaggo/files"
	ginSwagger "github.com/swaggo/gin-swagger"
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
	RatesMap map[string]float64 `json:"rates"`
	Date     string             `json:"date"`
}

// @Summary Отримати поточний курс USD до UAH
// @Description Запит має повертати поточний курс USD до UAH використовуючи будь-який third party сервіс з публічним АРІ
// @Tags rate
// @Produce json
// @Success 200 {object} RateMeasurement
// @Failure 400 {string} string "Invalid status value"
// @Router /rate [get]
func getRate() (RateMeasurement, error) {
	rate, err := getLastRate()
	if err != nil || time.Since(rate.Timestamp) > 24*time.Hour {
		err := updateRate()
		if err != nil {
			return RateMeasurement{}, err
		}
		rate, err = getLastRate()
		if err != nil {
			return RateMeasurement{}, err
		}
	}
	return rate, nil
}

// @Summary Підписати емейл на отримання поточного курсу
// @Description Запит має перевірити, чи немає данної електронної адреси в поточній базі даних і, в разі її відсутності, записати її.
// @Tags subscription
// @Produce json
// @Param email formData string true "Електронна адреса, яку потрібно підписати"
// @Success 200 {string} string "E-mail додано"
// @Failure 409 {string} string "Повертати, якщо e-mail вже є в базі даних"
// @Router /subscribe [post]
func trySubscribe(email string) error {
	// if database has email -> return error
	// otherwise add a new email to the database
	var exists bool
	err := db.Get(&exists, "SELECT EXISTS (SELECT 1 FROM subscription WHERE email = ?)", email)
	if err != nil {
		return err
	}
	if exists {
		return fmt.Errorf("email already subscribed")
	}
	_, err = db.Exec("INSERT INTO subscription (timestamp, email) VALUES (?, ?)", time.Now(), email)
	return err
}

func getLastRate() (RateMeasurement, error) {
	var rate RateMeasurement
	err := db.Get(&rate, "SELECT timestamp, value FROM usd_uah_rate ORDER BY timestamp DESC LIMIT 1")
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

	log.Println("response:", resp)

	var decoded_result ExchangeRateResp
	if err := json.NewDecoder(resp.Body).Decode(&decoded_result); err != nil {
		return err
	}

	log.Println("decoded_result:", decoded_result)

	uah_rate, exists := decoded_result.RatesMap["UAH"]
	if !exists {
		return fmt.Errorf("UAH rate not found")
	}

	date, err := time.Parse("2006-01-02", decoded_result.Date)
	if err != nil {
		return err
	}

	_, err = db.Exec("INSERT INTO usd_uah_rate (timestamp, value) VALUES (?, ?)", date, uah_rate)
	return err
}

func setupDatabase() *sqlx.DB {
	db, err := sqlx.Connect("sqlite3", "exchange.db")
	if err != nil {
		log.Fatalln(err)
	}
	return db
}

// @title Exchange Rate API
// @version 1.0
// @description Простий сервер для отримання поточного курса USD до UAH.
// @host gses2.app
// @BasePath /api
func setupRouter() *gin.Engine {
	r := gin.Default()

	r.GET("/ping", func(c *gin.Context) {
		c.String(http.StatusOK, "pong")
	})

	r.GET("/rate", func(c *gin.Context) {
		var rate, err = getRate()
		if err != nil {
			c.JSON(http.StatusBadRequest, gin.H{"status": "bad request", "error": err.Error()})
		}
		c.JSON(http.StatusOK, gin.H{"number": rate.Value})
	})

	r.POST("/subscribe/:email", func(c *gin.Context) {
		err := trySubscribe(c.Param("email"))
		if err != nil {
			c.JSON(http.StatusConflict, gin.H{"status": "subscription conflict", "error": err.Error()})
		} else {
			c.JSON(http.StatusOK, gin.H{"status": "subscribed"})
		}
	})

	docs.SwaggerInfo.BasePath = "/api"

	r.GET("/swagger/*any", ginSwagger.WrapHandler(swaggerFiles.Handler))

	return r
}

func main() {
	db = setupDatabase()
	r := setupRouter()
	r.Run(":8080")
}
