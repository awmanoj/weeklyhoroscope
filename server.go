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

type zodiac struct {
	title string
	daterange string
}

var baseURL = "https://www.4to40.com/astrology/"
var mapping map[string]string = map[string]string{
	"aquarius": "aquarius-weekly-horoscope/",
	"libra": "libra-weekly-horoscope/",
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

var mapping2 map[string]string = map[string]string{
	"aquarius":    "January 20 – February 18",
	"libra":       "September 21 – October 22",
	"sagittarius": "November 22 – December 22",
	"capricorn":   "December 23 – January 19",
	"scorpio":     "October 23 – November 21",
	"pisces":      "February 19 – March 19",
	"virgo":       "August 22 – September 20",
	"leo":         "July 22 – August 21",
	"cancer":      "June 21 – July 21",
	"gemini":      "May 20 – June 20",
	"taurus":      "April 19 – May 19",
	"aries":       "March 20 – April 18",
}

var cssStyle = `
	body {
		width: 60%;
	}

	h1 {
		font-family: "Geneva";
		background-color: #C0C4E4;
	}


	p {
		font-family: "Geneva";
	}	

	a {
		font-family: "Geneva"
	}

	#footer {
		background-color: #DFE1F1;
	}
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
		<div id="footer">
		<p style='display: inline'>Manoj Awasthi | </p><a href="https://awmanoj.github.io">https://awmanoj.github.io</a> | <p style='display: inline'>2020  </p>
		</div>
	</center>

	</body>
	</html>`

var forecastHTMLTemplate = `<html>
	<head>
		<title>Weekly Sunsign based Horoscope Forecasts by Anupam Kapil</title>
		<style>
			%%cssStyle%%
		</style>
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
		<div id="footer">
		<p style='display: inline'>Manoj Awasthi | </p><a href="https://awmanoj.github.io">https://awmanoj.github.io</a> | <p style='display: inline'>2020  </p>
		</div>
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

	var helpText string = "<h1>Weekly Sunsign based Horoscope Forecasts by <a href='http://anupamkapil.com/'>Anupam Kapil</a></h1>" + "<br/><br/>"
	for _, k := range keys {
		v := mapping[k]
		v2 := mapping2[k]
		pv := strings.Replace(strings.Replace(v, "/", "", -1), "-", " ", -1)
		helpText += "Click <a href='/forecast/" + k + "'>" + k + "</a> ("+ v2 + ") to get <b>" + pv + "</b>" + "<br/>"
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
