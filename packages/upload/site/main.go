package main

import (
	"bytes"
	"context"
	"fmt"
	"io"
	"log"
	"net/http"
	"os"
	"sort"
	"time"

	"github.com/Backblaze/blazer/b2"
)

var bucketName = "nsarchive"

// nations file format: nations/YYYY-MM-DD-nations.xml.gz
// regions file format: regions/YYYY-MM-DD-regions.xml.gz
// foundings file format: foundings/YYYY-MM-DD-foundings.json

var urlPrefix = "file/nsarchive/%s"

func monthFromIndex(index int) string {
	months := []string{"January", "February", "March", "April", "May", "June", "July", "August", "September", "October", "November", "December"}

	return months[index-1]
}

type Files struct {
	Years []Year
}

func (f *Files) getDate(date time.Time) *Day {
	y := date.Year()
	m := date.Month()
	d := date.Day()

	if !f.containsYear(y) {
		f.addYear(y)
	}

	year := f.getYear(y)

	if !year.containsMonth(int(m)) {
		year.addMonth(int(m))
	}

	month := year.getMonth(int(m))

	if !month.containsDay(d) {
		month.addDay(d)
	}

	return month.getDay(d)
}

func (f *Files) generateHTML() []byte {
	var buffer bytes.Buffer

	buffer.WriteString("<html><head><title>NSArchive</title></head><style>")

	buffer.WriteString(`
		body { font-family: Helvetica, sans-serif; }
		.year { font-size: 2em; font-weight: bold; cursor: pointer; }
		.month { font-size: 1.5em; font-weight: bold; cursor: pointer; }
		h4 { margin: 5px 20px; }
		details { margin-left: 20px; }
		summary { font-weight: bold; cursor: pointer; }
		ul { margin: 5px 20px; }
	`)

	buffer.WriteString("</style></head><body>")

	buffer.WriteString(`
		<h1>NSArchive</h1>
		<p>NSArchive (suggestions for a better name are welcome) is a collection of daily snapshots of <a href="https://www.nationstates.net">NationStates</a> data. NationStates produces two daily dumps -- <a href="https://www.nationstates.net/pages/api.html#dumps">Nations and Regions</a> -- each day, which are archived here, and founding data is collected from the <a href="https://www.nationstates.net/pages/api.html#worldapi">World API</a>. Founding data is based on UTC time and is always one day behind. The source code for this project can be viewed on GitHub <a href="https://github.com/nsupc/nsarchive">here</a>.</p>
	`)

	for _, year := range f.Years {
		buffer.WriteString("<details>")
		buffer.WriteString(fmt.Sprintf("<summary class=\"year\">%d</summary>", year.Year))

		for _, month := range year.Months {
			buffer.WriteString("<details>")
			buffer.WriteString(fmt.Sprintf("<summary class=\"month\">%s</summary>", monthFromIndex(month.Month)))

			for _, day := range month.Days {
				if day.NationsUrl != "" || day.RegionsUrl != "" || day.FoundingsUrl != "" {
					buffer.WriteString(fmt.Sprintf("<h4>%d-%02d-%02d</h4>", year.Year, month.Month, day.Day))
					buffer.WriteString("<ul>")

					if day.NationsUrl != "" {
						buffer.WriteString(fmt.Sprintf("<li><a href=\"%s\">nations</a></li>", day.NationsUrl))
					}
					if day.RegionsUrl != "" {
						buffer.WriteString(fmt.Sprintf("<li><a href=\"%s\">regions</a></li>", day.RegionsUrl))
					}
					if day.FoundingsUrl != "" {
						buffer.WriteString(fmt.Sprintf("<li><a href=\"%s\" download>foundings</a></li>", day.FoundingsUrl))
					}

					buffer.WriteString("</ul>")
				}
			}

			buffer.WriteString("</details>")
		}

		buffer.WriteString("</details>")
	}

	buffer.WriteString("</body></html>")

	return buffer.Bytes()
}

func (f *Files) containsYear(year int) bool {
	for _, y := range f.Years {
		if y.Year == year {
			return true
		}
	}

	return false
}

func (f *Files) addYear(year int) {
	f.Years = append(f.Years, Year{Year: year})
}

func (f *Files) getYear(year int) *Year {
	for i, y := range f.Years {
		if y.Year == year {
			return &f.Years[i]
		}
	}

	return nil
}

type Year struct {
	Year   int
	Months []Month
}

func (y *Year) containsMonth(month int) bool {
	for _, m := range y.Months {
		if m.Month == month {
			return true
		}
	}

	return false
}

func (y *Year) addMonth(month int) {
	y.Months = append(y.Months, Month{Month: month})
}

func (y *Year) getMonth(month int) *Month {
	for i, m := range y.Months {
		if m.Month == month {
			return &y.Months[i]
		}
	}

	return nil
}

type Month struct {
	Month int
	Days  []Day
}

func (m *Month) containsDay(day int) bool {
	for _, d := range m.Days {
		if d.Day == day {
			return true
		}
	}

	return false
}

func (m *Month) addDay(day int) {
	m.Days = append(m.Days, Day{Day: day})
}

func (m *Month) getDay(day int) *Day {
	for i, d := range m.Days {
		if d.Day == day {
			return &m.Days[i]
		}
	}

	return nil
}

type Day struct {
	Day          int
	NationsUrl   string
	RegionsUrl   string
	FoundingsUrl string
}

func createFileList(bucket *b2.Bucket) (Files, error) {
	result := Files{}
	ctx := context.Background()

	nations := bucket.List(ctx, b2.ListPrefix("nations/"))
	for nations.Next() {
		date, err := time.Parse("2006-01-02", nations.Object().Name()[8:18])
		if err != nil {
			return Files{}, err
		}

		day := result.getDate(date)

		day.NationsUrl = fmt.Sprintf(urlPrefix, nations.Object().Name())
	}

	regions := bucket.List(ctx, b2.ListPrefix("regions/"))
	for regions.Next() {
		date, err := time.Parse("2006-01-02", regions.Object().Name()[8:18])
		if err != nil {
			return Files{}, err
		}

		day := result.getDate(date)

		day.RegionsUrl = fmt.Sprintf(urlPrefix, regions.Object().Name())
	}

	foundings := bucket.List(ctx, b2.ListPrefix("foundings/"))
	for foundings.Next() {
		date, err := time.Parse("2006-01-02", foundings.Object().Name()[10:20])
		if err != nil {
			return Files{}, err
		}

		day := result.getDate(date)

		day.FoundingsUrl = fmt.Sprintf(urlPrefix, foundings.Object().Name())
	}

	// sort the result (foundings mess it up sometimes)
	for i := range result.Years {
		for j := range result.Years[i].Months {
			sort.Slice(result.Years[i].Months[j].Days, func(a, b int) bool {
				return result.Years[i].Months[j].Days[a].Day < result.Years[i].Months[j].Days[b].Day
			})
		}

		sort.Slice(result.Years[i].Months, func(a, b int) bool {
			return result.Years[i].Months[a].Month < result.Years[i].Months[b].Month
		})
	}

	sort.Slice(result.Years, func(a, b int) bool {
		return result.Years[a].Year < result.Years[b].Year
	})

	return result, nil
}

func uploadIndex(bucket *b2.Bucket, data []byte) error {
	ctx := context.Background()

	log.Println("Uploading index.html")

	writer := bucket.Object("index.html").NewWriter(ctx, b2.WithAttrsOption(&b2.Attrs{
		ContentType: "text/html; charset=utf-8",
	}))
	defer writer.Close()

	log.Println("Uploading to B2...")
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

	heartbeatUrl, present := os.LookupEnv("HEARTBEAT_URL")
	if !present {
		log.Fatal("Set HEARTBEAT_URL ENV var")
	}

	ctx := context.Background()

	bzClient, err := b2.NewClient(ctx, accessKeyID, secretAccessKey)
	if err != nil {
		log.Fatalf("Error creating backblaze client: %v", err)
	}

	bucket, err := bzClient.Bucket(ctx, bucketName)
	if err != nil {
		log.Fatalf("Error retrieving bucket: %v", err)
	}

	files, err := createFileList(bucket)
	if err != nil {
		log.Fatalf("Error creating file list: %v", err)
	}

	if err := uploadIndex(bucket, files.generateHTML()); err != nil {
		log.Fatalf("Error uploading index: %v", err)
	}

	_, err = http.Get(heartbeatUrl)
	if err != nil {
		log.Fatalf("Error sending heartbeat: %v", err)
	}
}
