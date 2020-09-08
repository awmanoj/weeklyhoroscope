package main

import (
	"errors"
	"fmt"
	"log"
	"net/http"
	"os"
	"sort"
	"strings"
	"time"

	"github.com/PuerkitoBio/goquery"
	"github.com/gorilla/mux"
	"github.com/koding/cache"
	"github.com/robfig/cron"
)

var baseURL = "https://www.4to40.com/astrology/"
var mapping map[string]string = map[string]string{
	"aquarius":    "aquarius-weekly-horoscope/",
	"libra":       "libra-weekly-horoscope/",
	"sagittarius": "sagittarius-weekly-horoscope/",
	"capricorn":   "capricorn-weekly-horoscope/",
	"scorpio":     "scorpio-weekly-horoscope/",
	"pisces":      "pisces-weekly-horoscope/",
	"virgo":       "virgo-weekly-horoscope/",
	"leo":         "leo-weekly-horoscope/",
	"cancer":      "cancer-weekly-horoscope/",
	"gemini":      "gemini-weekly-horoscope/",
	"taurus":      "taurus-weekly-horoscope/",
	"aries":       "aries-weekly-horoscope/",
}

var gCache = &cache.MemoryTTL{}

func main() {
	port := os.Getenv("PORT")

	gCache = cache.NewMemoryWithTTL(12 * time.Hour)
	r := mux.NewRouter()

	// routes
	r.HandleFunc("/", handleIndex)
	r.HandleFunc("/forecast/{sunsign}", handleForecast)

	// warm the cache
	go updateCaches()

	// cron to update the cache daily
	c := cron.New()
	c.AddFunc("@daily", updateCaches)

	// yaay!! start the server!
	log.Printf("Starting server at port %s\n", port)
	if err := http.ListenAndServe(":"+port, r); err != nil {
		log.Fatal(err)
	}
}

func handleIndex(w http.ResponseWriter, r *http.Request) {
	keys := make([]string, 0, len(mapping))
	for k := range mapping {
		keys = append(keys, k)
	}

	sort.Strings(keys)

	var helpText string = "Weekly Sunsign based Horoscope Forecasts by Anupam Kapil" + "<br/><br/>"
	for _, k := range keys {
		v := mapping[k]
		pv := strings.Replace(strings.Replace(v, "/", "", -1), "-", " ", -1)
		helpText += "Click <a href='/forecast/" + k + "'>" + k + "</a> to get <b>" + pv + "</b>" + "<br/>"
	}

	fmt.Fprintf(w, "<html><head><title>Weekly Sunsign based Horoscope Forecasts by Anupam Kapil</title></head><body>"+helpText+"</body></html>")
	return
}

func handleForecast(w http.ResponseWriter, r *http.Request) {
	if r.Method != "GET" {
		http.Error(w, "Method is not supported", http.StatusNotFound)
		return
	}

	// extract sunsign from variable
	vars := mux.Vars(r)
	sunsign := vars["sunsign"]

	title, forecast, err := fetchForecast(sunsign)
	if err != nil {
		http.Error(w, err.Error(), http.StatusNotFound)
		return
	}

	var url = baseURL + mapping[sunsign]

	fmt.Fprintf(w, "<html><head><title>Weekly Sunsign based Horoscope Forecasts by Anupam Kapil</title></head><body><h1>"+title+"</h1><p>"+forecast+"</p> <br/><br/><a href='"+url+"'>"+url+"</a></body></html>")
}

func fetchForecast(sunsign string) (string, string, error) {
	var title, forecast string

	// the keys for cache
	titleKey := sunsign + ":title"
	forecastKey := sunsign + ":forecast"

	// is data available in the global cache
	titleIntf, err1 := gCache.Get(titleKey)
	forecastIntf, err2 := gCache.Get(forecastKey)

	// the url for fetching the data
	var url = baseURL + mapping[sunsign]
	if err1 != nil || err2 != nil {
		_, ok := mapping[sunsign]
		if ok {
			// TODO: using http client with timeout instead.
			// this may timeout in 20 minutes or so and lead to too many open files
			// crashing the server.
			resp, err := http.Get(url)
			if err != nil {
				return title, forecast, errors.New("error connecting with " + url)
			}

			defer resp.Body.Close()

			// parse the HTML
			doc, err := goquery.NewDocumentFromReader(resp.Body)
			if err != nil {
				return title, forecast, errors.New("error parsing the html")
			}

			title, _ = doc.Find("h4").First().Html()
			forecast, _ = doc.Find("blockquote p").First().Html()

			title = strings.Replace(title, "<nil>", "", -1)

			// update the cache
			_ = gCache.Set(titleKey, title)
			_ = gCache.Set(forecastKey, forecast)
		} else {
			return title, forecast, errors.New("Invalid sunsign. Accepted values: aquarius, libra, sagittarius, capricorn, scorpio, pisces, virgo, leo, cancer, gemini, taurus, aries")
		}
	} else {
		title = titleIntf.(string)
		forecast = forecastIntf.(string)
	}

	return title, forecast, nil
}

func updateCaches() {
	for k := range mapping {
		fetchForecast(k)
		log.Println("cache warmed for forecast for " + k)
	}
	log.Println("cache warming finished successfully!")
}
