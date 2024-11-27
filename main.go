package main

import (
	"errors"
	"flag"
	"fmt"
	"log"
	"net/http"
	"os"
	"time"

	"github.com/newrelic/go-agent/v3/newrelic"
	"github.com/robfig/cron"
	"github.com/vlcak/fresh_console/fresh_client"
	"github.com/vlcak/groupme_qr_bot/groupme"
)

var (
	flagBotToken        = flag.String("bot-token", "", "Bot TOKEN")
	flagBotID           = flag.String("bot-id", "", "Bot ID")
	flagPort            = flag.String("port", ":80", "Service address (e.g. :80)")
	flagFreshToken      = flag.String("fresh-token", "", "Fresh token")
	flagNewRelicLicense = flag.String("newrelic-license", "", "NewRelic license")
)

func main() {
	flag.Parse()
	newRelicApp, err := newrelic.NewApplication(
		newrelic.ConfigAppName("FreshConsole"),
		newrelic.ConfigLicense(*flagNewRelicLicense),
		newrelic.ConfigAppLogForwardingEnabled(true),
	)
	if err != nil {
		log.Printf("Can't initialize NewRelic: %v", err)
	}

	flag.Parse()
	if *flagBotToken == "" {
		log.Fatal("Bot token is required")
	}
	if *flagBotID == "" {
		log.Fatal("Bot ID is required")
	}
	if *flagPort == "" {
		log.Fatal("Port is required")
	}
	if *flagFreshToken == "" {
		log.Fatal("Fresh token is required")
	}
	messageService := groupme.NewMessageService(*flagBotToken)
	freshClient := fresh_client.NewFreshClient(*flagFreshToken)

	messageProcessor := NewMessageProcessor(newRelicApp, messageService, *flagBotID, freshClient)

	locationPrague, err := time.LoadLocation("Europe/Prague")
	if err != nil {
		log.Printf("Error loading timezone: %v", err)
	}
	c := cron.NewWithLocation(locationPrague)
	c.AddFunc("0 0 * * * *", func() {
		credit, expired, err := freshClient.GetCredit()
		if err != nil {
			log.Printf("Error getting credit: %v", err)
			messageService.SendMessage(fmt.Sprintf("Error getting credit: %v", err), "")
			return
		}

		log.Printf("Credit: %d, Expired: %d", credit, expired)
		if credit < 100 {
			messageService.SendMessage(fmt.Sprintf("Credit is low: %d", credit), "")
		}
		newRelicApp.RecordCustomMetric("Credit", float64(credit))
		newRelicApp.RecordCustomMetric("ExpiredCredit", float64(expired))
	})
	c.AddFunc("0 5 0 * * 2", func() {
		date := time.Now().In(locationPrague).AddDate(0, 0, 6).Format("2006-01-02")
		startTime, err := time.ParseInLocation("2006-01-02 15:04", date+" 7:00", locationPrague)
		if err != nil {
			messageService.SendMessage(fmt.Sprintf("Error parsing time: %v", err), "")
			return
		}
		err = freshClient.Login(13, startTime)
		if err != nil {
			messageService.SendMessage(fmt.Sprintf("Error logging in: %s", err.Error()), "")
			return
		}
		messageService.SendMessage(fmt.Sprintf("Logged in for %s", startTime.Format("2006-01-02 15:04")), "")
	})
	c.Start()
	defer c.Stop()

	handler := NewHandler(newRelicApp, messageProcessor)
	fmt.Printf("Starting server...")
	err = http.ListenAndServe(*flagPort, handler.Mux())
	if errors.Is(err, http.ErrServerClosed) {
		log.Printf("server closed")
	} else if err != nil {
		log.Printf("error starting server: %v", err)
		os.Exit(1)
	} else {
		log.Printf("Server exited, err: %v", err)
	}
}
