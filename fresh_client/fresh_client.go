package fresh_client

import (
	"encoding/json"
	"fmt"
	"io"
	"log"
	"net/http"
	"time"
)

const (
	baseURL      = "https://api.freshkruhac.cz"
	userPath     = "/v2/user"
	locationPath = "/v2/training/location"
	typePath     = "/v2/training/type"
	nextPath     = "/v2/training/next/%d"
	trainingPath = "/v2/training/%d"
	creditPath   = "/v2/user/credit"
	joinPath     = "/v2/training/%d/join"
)

type FreshClient struct {
	httpClient *http.Client
	baseURL    string
	token      string
	locations  map[int]Location
	types      map[int]Type
	timeZone   *time.Location
}

type Location struct {
	ID              int    `json:"id"`
	HumanReadableID string `json:"human_readable_id"`
	Name            string `json:"name"`
}

type Type struct {
	ID          int    `json:"id"`
	Name        string `json:"name"`
	Description string `json:"description"`
	Capacity    int    `json:"capacity"`
}

type Training struct {
	ID                 int   `json:"id"`
	StartTimeUNIX      int64 `json:"start_time"`
	StartTime          time.Time
	Trainer            string `json:"trainer"`
	TrainingLocationID int    `json:"training_location_id"`
	TrainingTypeID     int    `json:"training_type_id"`
	Occuppancy         int    `json:"occupancy"`
}

type User struct {
	ID int `json:"user_id"`
}

type TrainingDetails struct {
	Bench        []User `json:"bench"`
	Participants []User `json:"participants"`
	Trainers     []User `json:"trainers"`
	Users        []struct {
		ID   int    `json:"id"`
		Name string `json:"name"`
	} `json:"users"`
	userMap map[int]struct {
		Name string
	}
}

type CreditRecord struct {
	ID           int   `json:"id"`
	CreditTypeID int   `json:"credit_type_id"`
	LeftAmount   int   `json:"left_amount"`
	ExpiresAt    int64 `json:"expires_at"`
	CreatedAt    int64 `json:"created_at"`
	Type         struct {
		ID             int `json:"id"`
		Amount         int `json:"amount"`
		DurationMonths int `json:"duration_months"`
	} `json:"type"`
}

func (td *TrainingDetails) GetUserName(userID int) string {
	return td.userMap[userID].Name
}

func NewFreshClient(token string) *FreshClient {
	fc := FreshClient{
		httpClient: &http.Client{},
		baseURL:    baseURL,
		token:      token,
	}
	fc.timeZone, _ = time.LoadLocation("Europe/Prague")
	fc.FetchTypes()
	fc.FetchLocations()
	return &fc
}

func (fc *FreshClient) GetLocation(id int) Location {
	return fc.locations[id]
}

func (fc *FreshClient) GetType(id int) Type {
	return fc.types[id]
}

func (fc *FreshClient) FetchLocations() ([]Location, error) {
	resp, err := fc.get(locationPath)
	if err != nil {
		return nil, err
	}

	var locations []Location
	err = json.Unmarshal(resp, &locations)
	if err != nil {
		return nil, err
	}

	fc.locations = make(map[int]Location)
	for _, location := range locations {
		fc.locations[location.ID] = location
	}
	return locations, nil
}

func (fc *FreshClient) FetchTypes() ([]Type, error) {
	resp, err := fc.get(typePath)
	if err != nil {
		return nil, err
	}

	var types []Type
	err = json.Unmarshal(resp, &types)
	if err != nil {
		return nil, err
	}

	fc.types = make(map[int]Type)
	for _, t := range types {
		fc.types[t.ID] = t
	}
	return types, nil
}

func (fc *FreshClient) GetNextTrainings(locationID int) ([]Training, error) {
	resp, err := fc.get(fmt.Sprintf(nextPath, locationID))
	if err != nil {
		return nil, err
	}

	var trainings []Training
	err = json.Unmarshal(resp, &trainings)
	if err != nil {
		return nil, err
	}

	for i := range trainings {
		trainings[i].StartTime = time.UnixMilli(trainings[i].StartTimeUNIX).In(fc.timeZone)
	}

	return trainings, nil
}

func (fc *FreshClient) FetchTrainingDetails(trainingID int) (*TrainingDetails, error) {
	resp, err := fc.get(fmt.Sprintf(trainingPath, trainingID))
	if err != nil {
		return nil, err
	}

	var trainingDetails TrainingDetails
	err = json.Unmarshal(resp, &trainingDetails)
	if err != nil {
		return nil, err
	}

	trainingDetails.userMap = make(map[int]struct{ Name string })
	for _, user := range trainingDetails.Users {
		trainingDetails.userMap[user.ID] = struct{ Name string }{Name: user.Name}
	}

	return &trainingDetails, nil
}

func (fc *FreshClient) GetCredit() (int, int, error) {
	resp, err := fc.get(creditPath)
	if err != nil {
		return 0, 0, err
	}

	var credit []CreditRecord
	err = json.Unmarshal(resp, &credit)
	if err != nil {
		return 0, 0, err
	}

	creditSum := 0
	expiredCreditSum := 0
	now := time.Now().In(fc.timeZone)
	for _, record := range credit {
		if record.LeftAmount > 0 {
			if time.UnixMilli(record.ExpiresAt).After(now) {
				creditSum += record.LeftAmount
			} else {
				expiredCreditSum += record.LeftAmount
			}
		}
	}

	return creditSum, expiredCreditSum, nil
}

func (fc *FreshClient) Login(location int, startTime time.Time) error {
	trainings, err := fc.GetNextTrainings(location)
	if err != nil {
		log.Printf("Error getting next training: %v", err)
		return err
	}
	var trainingID int

	for _, training := range trainings {
		if training.StartTime.Unix() == startTime.Unix() {
			trainingID = training.ID
			break
		}
	}
	if trainingID == 0 {
		return fmt.Errorf("no training found")
	}

	req, err := http.NewRequest("POST", fmt.Sprintf(fc.baseURL+joinPath, trainingID), nil)
	if err != nil {
		return err
	}

	req.Header.Set("Authorization", "Bearer "+fc.token)

	resp, err := fc.httpClient.Do(req)
	if err != nil {
		return err
	}

	if resp.StatusCode != http.StatusOK {
		return fmt.Errorf("unexpected status code: %d", resp.StatusCode)
	}

	return nil
}

func (fc *FreshClient) get(path string) ([]byte, error) {
	req, err := http.NewRequest("GET", fc.baseURL+path, nil)
	if err != nil {
		return nil, err
	}

	req.Header.Set("Authorization", "Bearer "+fc.token)

	resp, err := fc.httpClient.Do(req)
	if err != nil {
		return nil, err
	}
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusOK {
		return nil, fmt.Errorf("unexpected status code: %d", resp.StatusCode)
	}

	body, err := io.ReadAll(resp.Body)
	if err != nil {
		return nil, err
	}

	return body, nil
}
