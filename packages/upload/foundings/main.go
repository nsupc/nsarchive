package main

import (
	"bytes"
	"context"
	"encoding/json"
	"encoding/xml"
	"fmt"
	"io"
	"log"
	"net/http"
	"os"
	"regexp"
	"strconv"
	"strings"
	"time"

	"github.com/Backblaze/blazer/b2"
)

var bucketName = "nsarchive"

var filenameTemplate = "foundings/%s-foundings.json"

var happeningsUrl = "https://www.nationstates.net/cgi-bin/api.cgi?q=happenings;filter=founding;limit=100;sincetime=%d;beforetime=%d;sinceid=%s;beforeid=%s;"

var nation_re = regexp.MustCompile("\\@@(.*)@@")
var region_re = regexp.MustCompile("\\%%(.*)%%")

type World struct {
	Happenings Happenings `xml:"HAPPENINGS"`
}

type Happenings struct {
	Events []RawEvent `xml:"EVENT"`
}

type RawEvent struct {
	Id        int64  `xml:"id,attr"`
	Timestamp int64  `xml:"TIMESTAMP"`
	Text      string `xml:"TEXT"`
}

type Founding struct {
	Id        int64  `json:"id"`
	Timestamp int64  `json:"timestamp"`
	Nation    string `json:"nation"`
	Region    string `json:"region"`
}

func NewFounding(event RawEvent) Founding {
	nation := strings.ReplaceAll(nation_re.FindString(event.Text), "@", "")
	region := strings.ReplaceAll(region_re.FindString(event.Text), "%", "")

	return Founding{
		Id:        event.Id,
		Timestamp: event.Timestamp,
		Nation:    nation,
		Region:    region,
	}
}

func getHappenings(client *http.Client, sincetime int64, beforetime int64, sinceid string, beforeid string) (Happenings, error) {
	world := World{}

	requestUrl := fmt.Sprintf(happeningsUrl, sincetime, beforetime, sinceid, beforeid)

	req, err := http.NewRequest("GET", requestUrl, nil)
	if err != nil {
		return world.Happenings, err
	}

	req.Header.Add("User-Agent", "upc")

	resp, err := client.Do(req)
	if err != nil {
		return world.Happenings, err
	}
	defer resp.Body.Close()

	body, err := io.ReadAll(resp.Body)
	if err != nil {
		return world.Happenings, err
	}

	err = xml.Unmarshal(body, &world)
	if err != nil {
		return world.Happenings, err
	}

	return world.Happenings, nil
}

func getFoundings(client *http.Client, yesterday time.Time, today time.Time) ([]Founding, error) {
	var foundings []Founding

	sinceid := ""
	beforeid := ""

	for {
		if beforeid != "" {
			log.Printf("Checking for events prior to event %s\n", beforeid)
		}

		happenings, err := getHappenings(client, yesterday.Unix(), today.Unix(), sinceid, beforeid)
		if err != nil {
			log.Fatal(err)
		}

		count := len(happenings.Events)

		for _, event := range happenings.Events {
			founding := NewFounding(event)

			foundings = append(foundings, founding)
		}

		if count > 0 {
			beforeid = strconv.FormatInt(foundings[len(foundings)-1].Id, 10)
		} else {
			break
		}

		time.Sleep(2 * time.Second)
	}

	return foundings, nil
}

func upload(bucket *b2.Bucket, date time.Time, data []byte) error {
	filename := fmt.Sprintf(filenameTemplate, date.Format("2006-01-02"))

	log.Printf("Uploading %d bytes to bucket %s as file %s\n", len(data), bucketName, filename)

	ctx := context.Background()

	writer := bucket.Object(filename).NewWriter(ctx)
	defer writer.Close()

	fmt.Println("Uploading to B2...")
	if _, err := io.Copy(writer, bytes.NewReader(data)); err != nil {
		writer.Close()
		return err
	}

	return writer.Close()
}

func Main() {
	accessKeyID, present := os.LookupEnv("ACCESS_KEY_ID")
	if !present {
		log.Fatal("Set ACCESS_KEY_ID ENV var")
	}

	secretAccessKey, present := os.LookupEnv("SECRET_ACCESS_KEY")
	if !present {
		log.Fatal("Set SECRET_ACCESS_KEY ENV var")
	}

	ctx := context.Background()

	bzClient, err := b2.NewClient(ctx, accessKeyID, secretAccessKey)
	if err != nil {
		log.Fatalf("Error creating backblaze client: %s", err)
	}

	bucket, err := bzClient.Bucket(ctx, bucketName)
	if err != nil {
		log.Fatalf("Error retrieving bucket: %s", err)
	}

	client := &http.Client{}

	year, month, day := time.Now().UTC().Date()

	yesterday := time.Date(year, month, day-1, 0, 0, 0, 0, time.UTC)
	today := time.Date(year, month, day, 0, 0, 0, 0, time.UTC)

	foundings, err := getFoundings(client, yesterday, today)
	if err != nil {
		log.Fatal(err)
	}

	data, err := json.Marshal(foundings)
	if err != nil {
		log.Fatal(err)
	}

	err = upload(bucket, yesterday, data)
	if err != nil {
		log.Fatal(err)
	}
}
