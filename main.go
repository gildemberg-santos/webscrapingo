package main

import (
	"encoding/json"
	"io"
	"log"
	"net/http"
	"net/url"
	"strings"
	"sync"
	"time"

	"github.com/PuerkitoBio/goquery"
)

type ListUrl struct {
	Domain     string   `json:"domain"`
	UrlsInput  string   `json:"url"`
	UrlsOutput []string `json:"urls"`
}

func (m *ListUrl) Normalize() []string {
	for i := range m.UrlsOutput {
		domain, _ := url.Parse(m.Domain)
		url, err := url.Parse(m.UrlsOutput[i])
		if err == nil {
			url.Scheme = "https"
			url.Fragment = ""
			url.RawQuery = ""

			extension := strings.LastIndex(url.Path, ".")
			mailto := strings.LastIndex(url.Path, "mailto:")
			tel := strings.LastIndex(url.Path, "tel:")
			javascript := strings.LastIndex(url.Path, "javascript:")
			window := strings.LastIndex(url.Path, "window.")

			if extension != -1 || mailto != -1 || tel != -1 || javascript != -1 || window != -1 || url.Path == "" {
				url.Path = "/"
			}

			if url.Host == "" {
				url.Host = domain.Host

			} else if url.Host != domain.Host {
				url.Host = domain.Host
				url.Path = "/"
			}

			m.UrlsOutput[i] = url.String()
		} else {
			m.UrlsOutput[i] = ""
		}
	}
	return m.UrlsOutput
}

func (m *ListUrl) Unique() []string {
	m.Normalize()
	result := []string{}
	encountered := map[string]bool{}
	for v := range m.UrlsOutput {
		encountered[m.UrlsOutput[v]] = true
	}
	for key := range encountered {
		result = append(result, key)
	}
	m.UrlsOutput = result
	return result
}

func ScannerPage(domain string, url []string) (ListUrl, error) {
	var mapsite = ListUrl{Domain: domain, UrlsOutput: make([]string, 1)}

	var wg sync.WaitGroup
	for i := range url {
		wg.Add(1)
		go func(url string) {
			defer wg.Done()
			urls, _ := ResquestionURL(url)
			mapsite.UrlsOutput = append(mapsite.UrlsOutput, urls...)
		}(url[i])
	}
	wg.Wait()

	return mapsite, nil
}

func ResquestionURL(url string) ([]string, error) {
	var urls = make([]string, 1)

	res, err := http.Get(url)
	if err != nil {
		return urls, err
	}

	doc, err := goquery.NewDocumentFromReader(res.Body)
	if err != nil {
		return urls, err
	}

	doc.Find("a").Each(func(i int, s *goquery.Selection) {
		href, success := s.Attr("href")
		if success {
			urls = append(urls, href)
		}
	})

	return urls, nil
}

func WebScrapinGo(w http.ResponseWriter, r *http.Request) {
	var message struct {
		Domain string   `json:"domain"`
		Urls   []string `json:"urls"`
		Error  string   `json:"error"`
		Status string   `json:"status"`
		Total  int      `json:"total"`
	}

	if err := json.NewDecoder(r.Body).Decode(&message); err != nil {
		switch err {
		case io.EOF:
			message.Error = "No data received"
			message.Status = "Error"
			message.Urls = nil
			message.Domain = ""
			w.WriteHeader(http.StatusBadRequest)
			json.NewEncoder(w).Encode(message)
			log.Printf("json.NewDecoder: %v", err)
			return
		default:
			message.Error = "%v" + err.Error()
			message.Status = "Error"
			message.Urls = nil
			message.Domain = ""
			w.WriteHeader(http.StatusBadRequest)
			json.NewEncoder(w).Encode(message)
			log.Printf("json.NewDecoder: %v", err)
			return
		}
	}

	depthLevelFirst, _ := ScannerPage(message.Domain, []string{message.Domain})
	depthLevelSecond, _ := ScannerPage(message.Domain, depthLevelFirst.Unique())
	depthLevelThird, _ := ScannerPage(message.Domain, depthLevelSecond.Unique())

	var urls = make([]string, 1)
	urls = append(urls, depthLevelFirst.UrlsOutput...)
	urls = append(urls, depthLevelSecond.UrlsOutput...)
	urls = append(urls, depthLevelThird.UrlsOutput...)

	var mapurl = ListUrl{
		Domain:     message.Domain,
		UrlsInput:  message.Domain,
		UrlsOutput: urls,
	}

	message.Urls = mapurl.Unique()

	if message.Urls == nil {
		message.Error = "No urls received"
		message.Status = "Error"
		w.WriteHeader(http.StatusBadRequest)
		json.NewEncoder(w).Encode(message)
		log.Printf("json.NewDecoder: %v", message.Error)
		return
	}

	message.Total = len(message.Urls)
	message.Status = "Success"
	message.Error = ""
	w.WriteHeader(http.StatusOK)
	json.NewEncoder(w).Encode(message)
}

func main() {
	http.DefaultClient.Timeout = time.Second * 15
	http.HandleFunc("/", WebScrapinGo)
	http.ListenAndServe(":8080", nil)
}
