package main

import (
	"context"
	_ "crypto/sha256"
	"fmt"
	"io"
	_ "io"
	"log"
	"net/http"
	"os"
	"time"

	"github.com/Backblaze/blazer/b2"
)

var nsUrlTemplate = "https://www.nationstates.net/pages/%ss.xml.gz"

var bucketName = "nsarchive"

var filenameTemplate = "%ss/%s-%ss.xml.gz"

func upload_dump(bucket *b2.Bucket, dtype string) error {
	var downloadUrl = fmt.Sprintf(nsUrlTemplate, dtype)
	log.Printf("Downloading dump from %s\n", downloadUrl)

	req, err := http.NewRequest("GET", downloadUrl, nil)
	if err != nil {
		return err
	}

	req.Header.Set("User-Agent", "nsarchive by upc")

	resp, err := http.DefaultClient.Do(req)
	if err != nil {
		return err
	}
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusOK {
		return err
	}

	var filename = fmt.Sprintf(filenameTemplate, dtype, time.Now().Format("2006-01-02"), dtype)

	log.Printf("Uploading %d bytes to bucket %s as file %s\n", resp.ContentLength, bucketName, filename)

	ctx := context.Background()

	writer := bucket.Object(filename).NewWriter(ctx)
	defer writer.Close()

	fmt.Println("Uploading to B2...")
	if _, err := io.Copy(writer, resp.Body); err != nil {
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

	client, err := b2.NewClient(ctx, accessKeyID, secretAccessKey)
	if err != nil {
		log.Fatalf("Error creating backblaze client: %s", err)
	}

	bucket, err := client.Bucket(ctx, bucketName)
	if err != nil {
		log.Fatalf("Error retrieving bucket: %s", err)
	}

	err = upload_dump(bucket, "nation")
	if err != nil {
		log.Fatalf("Error uploading nation dump: %v", err)
	}

	time.Sleep(5 * time.Second)

	err = upload_dump(bucket, "region")
	if err != nil {
		log.Fatalf("Error uploading nation dump: %v", err)
	}

	log.Println("Successfully uploaded dumps")
}
