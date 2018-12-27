package main

import (
	"encoding/json"
	"fmt"
	"io/ioutil"
	"log"
	"net"
	"net/http"
	"strconv"
	"time"

	wunderground "github.com/donniet/mirror.4/wunderground"
)

type Weather struct {
	High     float64
	Low      float64
	Icon     string
	DateTime time.Time
}

type WeatherService struct {
	URL     string
	Timeout time.Duration
}

func (w WeatherService) GetWeather() (Weather, error) {
	client := &http.Client{
		Transport: &http.Transport{
			Dial: (&net.Dialer{
				Timeout:   w.Timeout,
				KeepAlive: w.Timeout,
			}).Dial,
			TLSHandshakeTimeout:   w.Timeout,
			ResponseHeaderTimeout: w.Timeout,
			ExpectContinueTimeout: w.Timeout,
		},
	}

	ret := Weather{}
	response := wunderground.ForecastResponse{}

	if res, err := client.Get(w.URL); err != nil {
		return ret, err
	} else if data, err := ioutil.ReadAll(res.Body); err != nil {
		return ret, err
	} else if err := json.Unmarshal(data, &response); err != nil {
		return ret, err
	} else if response.Forecast == nil || response.Forecast.SimpleForecast == nil || len(response.Forecast.SimpleForecast.ForecastDay) == 0 {
		return ret, fmt.Errorf("no forecast in response")
	} else {
		d := response.Forecast.SimpleForecast.ForecastDay[0]

		ret.DateTime = d.Date()

		if icon, ok := iconMap[d.Icon]; !ok {
			log.Printf("unrecognized icon from weather service: %s", d.Icon)
		} else {
			ret.Icon = icon
		}

		if ret.High, err = strconv.ParseFloat(d.High.Fahrenheit, 32); err != nil {
			log.Printf("invalid high temperature %s, %v", d.High.Fahrenheit, err)
		}

		if ret.Low, err = strconv.ParseFloat(d.Low.Fahrenheit, 32); err != nil {
			log.Printf("invalid high temperature %s, %v", d.Low.Fahrenheit, err)
		}
	}

	return ret, nil
}

var iconMap = map[string]string{
	"chanceflurries":    "Cloud-Snow-Sun-Alt",
	"chancerain":        "Cloud-Rain-Sun-Alt",
	"chancesleet":       "Cloud-Hail-Sun",
	"chancesnow":        "Cloud-Snow-Sun-Alt",
	"chancetstorms":     "Cloud-Lightning-Sun",
	"clear":             "Sun",
	"cloudy":            "Cloud",
	"flurries":          "Cloud-Snow",
	"fog":               "Cloud-Fog",
	"hazy":              "Cloud-Fog-Sun",
	"mostlycloudy":      "Cloud-Sun",
	"nt_chanceflurries": "Cloud-Snow-Moon-Alt",
	"nt_chancerain":     "Cloud-Rain-Moon-Alt",
	"nt_chancesleet":    "Cloud-Hail-Moon",
	"nt_chancesnow":     "Cloud-Snow-Moon-Alt",
	"nt_chancetstorms":  "Cloud-Lightning-Moon",
	"nt_clear":          "Moon",
	"nt_cloudy":         "Cloud-Moon",
	"nt_flurries":       "Cloud-Snow",
	"nt_fog":            "Cloud-Fog",
	"nt_hazy":           "CLoud-Fog-Moon",
	"nt_mostlysunny":    "Cloud-Moon",
	"nt_partlycloudy":   "Cloud-Moon",
	"nt_partlysunny":    "Cloud-Moon",
	"nt_rain":           "Cloud-Rain",
	"nt_sleet":          "Cloud-Hail",
	"nt_snow":           "Cloud-Snow",
	"nt_sunny":          "Cloud-Moon",
	"nt_tstorms":        "Cloud-Lightning",
	"partlycloudy":      "Cloud-Sun",
	"partlysunny":       "Cloud-Sun",
	"rain":              "Cloud-Rain",
	"sleet":             "Cloud-Hail",
	"snow":              "Cloud-Snow",
	"sunny":             "Sun",
	"tstorms":           "Cloud-Lightning",
}
