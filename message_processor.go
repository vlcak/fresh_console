package main

import (
	"encoding/json"
	"fmt"
	"io"
	"log"
	"strconv"
	"strings"
	"time"

	"github.com/newrelic/go-agent/v3/newrelic"
	"github.com/vlcak/fresh_console/fresh_client"
	"github.com/vlcak/groupme_qr_bot/groupme"
)

type MessageProcessor struct {
	newRelicApp    *newrelic.Application
	messageService *groupme.MessageService
	botId          string
	freshClient    *fresh_client.FreshClient
}

func NewMessageProcessor(newRelicApp *newrelic.Application, messageService *groupme.MessageService, botID string, freshClient *fresh_client.FreshClient) *MessageProcessor {
	return &MessageProcessor{
		newRelicApp:    newRelicApp,
		messageService: messageService,
		botId:          botID,
		freshClient:    freshClient,
	}
}

func (mp *MessageProcessor) ProcessMessage(body io.ReadCloser) {
	m := groupme.GroupmeMessage{}
	if err := json.NewDecoder(body).Decode(&m); err != nil {
		log.Printf("ERROR: %v\n", err)
		return
	}

	// Ignore own messages
	if m.SenderId == mp.botId {
		return
	}

	parsedMessage := strings.Split(m.Text, " ")
	if len(parsedMessage) < 1 {
		return
	}

	switch parsedMessage[0] {
	case "LOGIN":
		date := time.Now().AddDate(0, 0, 7).Format("2006-01-02")
		start := "7:00"
		location := 13
		if len(parsedMessage) > 1 {
			start = parsedMessage[1]
		}
		if len(parsedMessage) > 2 {
			date = parsedMessage[2]
		}
		if len(parsedMessage) > 3 {
			loc, err := strconv.Atoi(parsedMessage[3])
			if err != nil {
				location = loc
			} else {
				log.Printf("Error parsing location ID: %v", err)
				mp.messageService.SendMessage("Invalid location ID", "")
				return
			}
		}
		startTime, err := time.Parse("2006-01-02 15:04", date+" "+start)
		if err != nil {
			log.Printf("Error parsing time: %v", err)
			mp.messageService.SendMessage("Invalid time format", "")
			return
		}
		err = mp.freshClient.Login(location, startTime)
		if err != nil {
			log.Printf("Error logging in: %v", err)
			mp.messageService.SendMessage("Failed to login: "+err.Error(), "")
			return
		}

		mp.messageService.SendMessage("Logged in for "+date+" "+start, "")

	case "FIND":
		if len(parsedMessage) < 2 {
			log.Printf("Invalid command format")
			mp.messageService.SendMessage("Invalid command format", "")
			return
		}
		name := strings.Join(parsedMessage[1:], " ")

		mp.freshClient.FetchTypes()
		locations, err := mp.freshClient.FetchLocations()
		found := make([]fresh_client.Training, 0)
		if err != nil {
			log.Printf("Error fetching locations: %v", err)
			mp.messageService.SendMessage("Failed to fetch locations: "+err.Error(), "")
			return
		}
		for _, loc := range locations {
			trainings, err := mp.freshClient.GetNextTraining(loc.ID)
			if err != nil {
				log.Printf("Error getting next training: %v", err)
				mp.messageService.SendMessage("Failed to get next training: "+err.Error(), "")
				return
			}
			for _, t := range trainings {
				details, err := mp.freshClient.FetchTrainingDetails(t.ID)
				if err != nil {
					log.Printf("Error fetching training details: %v", err)
					mp.messageService.SendMessage("Failed to fetch training details: "+err.Error(), "")
					return
				}
				for _, p := range details.Users {
					if strings.Contains(p.Name, name) {
						found = append(found, t)
						break
					}
				}
			}
		}

		if len(found) == 0 {
			mp.messageService.SendMessage("No results found", "")
			return
		}
		message := ""
		for _, t := range found {
			trainingType := mp.freshClient.GetType(t.TrainingTypeID)
			trainingStart := time.UnixMilli(t.StartTime)
			message += fmt.Sprintf("%s %s - %s - %d/%d\n", trainingType.Name, trainingStart.Format("2006-01-02 15:04"), t.Trainer, t.Occuppancy, trainingType.Capacity)
		}
		mp.messageService.SendMessage(message, "")
	}
}
