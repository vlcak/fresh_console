package main

import (
	"encoding/json"
	"fmt"
	"io"
	"log"
	"slices"
	"strconv"
	"strings"
	"sync"
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
		log.Printf("Finding user %s", name)

		mp.freshClient.FetchTypes()
		locations, err := mp.freshClient.FetchLocations()
		if err != nil {
			log.Printf("Error fetching locations: %v", err)
			mp.messageService.SendMessage("Failed to fetch locations: "+err.Error(), "")
			return
		}
		participants := make([]fresh_client.Training, 0)
		benches := make([]fresh_client.Training, 0)
		var mu sync.Mutex
		var wg sync.WaitGroup
		for _, loc := range locations {
			wg.Add(1)
			go func(loc fresh_client.Location) {
				defer wg.Done()
				trainings, err := mp.freshClient.GetNextTrainings(loc.ID)
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
					for _, u := range details.Users {
						if u.Name == name {
							idx := slices.IndexFunc(details.Participants, func(u fresh_client.User) bool {
								return u.ID == u.ID
							})
							if idx != -1 {
								mu.Lock()
								participants = append(participants, t)
								mu.Unlock()
								break
							}
							idx = slices.IndexFunc(details.Bench, func(u fresh_client.User) bool {
								return u.ID == u.ID
							})
							if idx != -1 {
								mu.Lock()
								benches = append(benches, t)
								mu.Unlock()
								break
							}
						}
					}
				}
			}(loc)
		}

		wg.Wait()

		if len(participants) == 0 && len(benches) == 0 {
			log.Printf("No results found")
			mp.messageService.SendMessage("No results found", "")
			return
		}
		message := ""
		if len(participants) > 0 {
			message += "Participate:\n"
		}
		for _, t := range participants {
			trainingType := mp.freshClient.GetType(t.TrainingTypeID)
			message += fmt.Sprintf("%s %s - %s - %d/%d\n", trainingType.Name, t.StartTime.Format("2006-01-02 15:04"), t.Trainer, t.Occuppancy, trainingType.Capacity)
		}
		if len(benches) > 0 {
			message += "Bench:\n"
		}
		for _, t := range benches {
			trainingType := mp.freshClient.GetType(t.TrainingTypeID)
			message += fmt.Sprintf("%s %s - %s - %d/%d\n", trainingType.Name, t.StartTime.Format("2006-01-02 15:04"), t.Trainer, t.Occuppancy, trainingType.Capacity)
		}
		log.Printf(message)
		mp.messageService.SendMessage(message, "")
	}
}
