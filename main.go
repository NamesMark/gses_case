package main

import (
	"crypto/tls"
	"encoding/json"
	"fmt"
	"log"
	"net/http"
	"os"
	"strconv"
	"time"

	"github.com/joho/godotenv"

	"github.com/jmoiron/sqlx"
	_ "github.com/mattn/go-sqlite3"

	docs "github.com/NamesMark/gses_case/docs"
	"github.com/gin-gonic/gin"
	swaggerFiles "github.com/swaggo/files"
	ginSwagger "github.com/swaggo/gin-swagger"

	"github.com/go-gomail/gomail"
	"github.com/robfig/cron/v3"
)

var db *sqlx.DB
var mailer *gomail.Dialer

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

func getAllSubscribers() ([]Subscription, error) {
	var emails []Subscription
	err := db.Select(&emails, "SELECT email FROM subscription")
	if err != nil {
		return nil, err
	}
	return emails, nil
}

// @Summary Відправити актуальний курс USD до UAH на всі електронні адреси, які були підписані раніше.
// @Description Відправити e-mail з поточним курсом на всі підписані електронні пошти.
// @Tags subscription
// @Produce json
// @Success 200 {string} string "E-mail'и відправлено"
// @Failure 500 {string} string "Internal server error"
// @Router /sendEmails [post]
func sendRateToAll() error {
	updateRate()
	var rate, err = getLastRate()
	if err != nil {
		return err
	}
	date := time.Now().Format("Monday, January 2, 2006")
	var contents = fmt.Sprintf("Hi! Today is %s The current rate is %.2f", date, rate.Value)

	emails, err := getAllSubscribers()
	if err != nil {
		return err
	}

	for _, email := range emails {
		err := sendEmail(email.Email, contents)
		if err != nil {
			log.Printf("Failed to send email to %s: %v", email.Email, err)
		}
	}
	return nil
}

func sendEmail(email string, contents string) error {
	message := gomail.NewMessage()
	message.SetHeader("From", "noreply@gses2.app")
	message.SetHeader("To", email)
	message.SetHeader("Subject", "Today's USD to UAH Rate")
	message.SetBody("text/plain", contents)
	return mailer.DialAndSend(message)
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
	// go to exchange rate API
	resp, err := http.Get("https://api.exchangerate-api.com/v4/latest/USD")
	if err != nil {
		return err
	}

	defer resp.Body.Close()

	var decoded_result ExchangeRateResp
	if err := json.NewDecoder(resp.Body).Decode(&decoded_result); err != nil {
		return err
	}

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
	db_file := os.Getenv("DATABASE_FILE")
	db, err := sqlx.Connect("sqlite3", db_file)
	if err != nil {
		log.Fatalln(err)
	}
	return db
}

func setupMailer() *gomail.Dialer {
	host := os.Getenv("SMTP_HOST")
	portString := os.Getenv("SMTP_PORT")
	username := os.Getenv("SMTP_USERNAME")
	password := os.Getenv("SMTP_PASSWORD")

	port, err := strconv.Atoi(portString)
	if err != nil {
		log.Fatalf("Invalid port number: %v", err)
	}

	d := gomail.NewDialer(host, port, username, password)
	d.TLSConfig = &tls.Config{InsecureSkipVerify: true}
	return d
}

func setupCron() *cron.Cron {
	c := cron.New()
	//c.AddFunc("@daily", func() {
	c.AddFunc("@hourly", func() { // for testing purposes; TODO: change back to daily
		if err := sendRateToAll(); err != nil {
			log.Printf("Error sending rate to all: %v", err)
		}
	})
	c.Start()
	return c
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

	r.POST("/sendEmails", func(c *gin.Context) {
		err := sendRateToAll()
		if err != nil {
			c.JSON(http.StatusInternalServerError, gin.H{"status": "internal server error", "error": err.Error()})
		} else {
			c.JSON(http.StatusOK, gin.H{"status": "sent emails"})
		}
	})

	docs.SwaggerInfo.BasePath = "/api"

	r.GET("/swagger/*any", ginSwagger.WrapHandler(swaggerFiles.Handler))

	return r
}

func main() {
	err := godotenv.Load()
	if err != nil {
		log.Fatalf("Error loading .env file")
	}
	db = setupDatabase()
	mailer = setupMailer()
	setupCron()
	r := setupRouter()
	r.Run(":8080")
}
