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

var cssStyle = `

`

var indexHTMLTemplate = `<html>
	<head>
		<title>Weekly Sunsign based Horoscope Forecasts by Anupam Kapil</title>
		<style>
			%%cssStyle%%
		</style>
	</head>
	<body>
		%%helpText%%
	<br/><br/>
	<center>
		<hr/>
		Manoj Awasthi | <a href="https://awmanoj.github.io">https://awmanoj.github.io</a> | 2020  
	</center>

	</body>
	</html>`

var forecastHTMLTemplate = `<html>
	<head>
		<title>Weekly Sunsign based Horoscope Forecasts by Anupam Kapil</title>
	</head>
	<body>
		<h1>%%title%%</h1>
		<p>%%forecast%%</p>
		<br/><br/>
		<p style='display: inline'>Source: </p><a style='display: inline' href='%%url%%'>%%url%%</a>
		<br/><br/>
		<p style='display: inline'>Back to </p><a href='/'>Home</a>
		<br/><br/>
		<center>
		<hr/>
		Manoj Awasthi | <a href="https://awmanoj.github.io">https://awmanoj.github.io</a> | 2020  
	</center>

	</body>
	</html>`

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

	var helpText string = "Weekly Sunsign based Horoscope Forecasts by <a href='http://anupamkapil.com/'>Anupam Kapil</a>" + "<br/><br/>"
	for _, k := range keys {
		v := mapping[k]
		pv := strings.Replace(strings.Replace(v, "/", "", -1), "-", " ", -1)
		helpText += "Click <a href='/forecast/" + k + "'>" + k + "</a> to get <b>" + pv + "</b>" + "<br/>"
	}

	indexHTML := strings.Replace(indexHTMLTemplate, "%%helpText%%", helpText, 1)
	indexHTML = strings.Replace(indexHTML, "%%cssStyle%%", cssStyle, -1)

	fmt.Fprintf(w, indexHTML)
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

	forecastHTML := strings.Replace(forecastHTMLTemplate, "%%title%%", title, 1)
	forecastHTML = strings.Replace(forecastHTML, "%%forecast%%", forecast, 1)
	forecastHTML = strings.Replace(forecastHTML, "%%url%%", url, 2)
	forecastHTML = strings.Replace(forecastHTML, "%%cssStyle%%", cssStyle, -1)

	fmt.Fprintf(w, forecastHTML)
}

func fetchForecast(sunsign string) (string, string, error) {
	var title, forecast string

	titleKey := sunsign + ":title"
	forecastKey := sunsign + ":forecast"

	// is data available in the global cache
	titleIntf, err1 := gCache.Get(titleKey)
	forecastIntf, err2 := gCache.Get(forecastKey)

	// the url for fetching the data
	if err1 != nil || err2 != nil {
		_, ok := mapping[sunsign]
		if ok {
			err := forceUpdate(sunsign)
			if err != nil {
				return title, forecast, errors.New("err" + err.Error())
			}
		} else {
			return title, forecast, errors.New("Invalid sunsign. Accepted values: aquarius, libra, sagittarius, capricorn, scorpio, pisces, virgo, leo, cancer, gemini, taurus, aries")
		}
	} else {
		title = titleIntf.(string)
		forecast = forecastIntf.(string)

		// still go and update the cache
		go forceUpdate(sunsign)
	}

	return title, forecast, nil
}

func forceUpdate(sunsign string) error {
	var url = baseURL + mapping[sunsign]

	// the keys for cache
	titleKey := sunsign + ":title"
	forecastKey := sunsign + ":forecast"

	// TODO: using http client with timeout instead.
	// this may timeout in 20 minutes or so and lead to too many open files
	// crashing the server.
	resp, err := http.Get(url)
	if err != nil {
		return errors.New("error connecting with " + url)
	}

	defer resp.Body.Close()

	// parse the HTML
	doc, err := goquery.NewDocumentFromReader(resp.Body)
	if err != nil {
		return errors.New("error parsing the html")
	}

	title, err1 := doc.Find("h4").First().Html()
	forecast, err2 := doc.Find("blockquote p").First().Html()

	title = strings.Replace(title, "<nil>", "", -1)

	// update the cache
	if err1 != nil {
		log.Println("err", "problem fetching title from ", url)
	} else {
		_ = gCache.Set(titleKey, title)
	}

	if err2 != nil {
		log.Println("err", "problem fetching forecast from ", url)
	} else {
		_ = gCache.Set(forecastKey, forecast)
	}

	return nil
}

func updateCaches() {
	for k := range mapping {
		fetchForecast(k)
		log.Println("cache warmed for forecast for " + k)
	}
	log.Println("cache warming finished successfully!")
}
