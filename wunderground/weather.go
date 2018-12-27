package wunderground

import (
	"strconv"
	"time"
)

type ForecastResponse struct {
	Forecast *Forecast `json:"forecast,omitempty"`
	Response Response  `json:"response,omitempty"`
}

type Response struct {
	Features       Features `json:"features"`
	TermsOfService string   `json:"termsofService"`
	Version        string   `json:"version"`
}

type Features struct {
	Forecast int `json:"forecast,omitempty"`
}

type Forecast struct {
	SimpleForecast *SimpleForecast `json:"simpleforecast,omitempty"`
	TextForecast   *TextForecast   `json:"txt_forecast,omitempty"`
}

type SimpleForecast struct {
	ForecastDay []ForecastDay `json:"forecastday,omitempty"`
}

type ForecastDay struct {
	AvererageHumidity float32       `json:"avehumidity"`
	AverageWind       Wind          `json:"avewind"`
	Conditions        string        `json:"conditions"`
	DateTime          DateTime      `json:"date"`
	High              Tempurature   `json:"high"`
	Icon              string        `json:"icon"`
	IconURL           string        `json:"icon_url"`
	Low               Tempurature   `json:"low"`
	MaxHumidity       float32       `json:"maxhumidity"`
	MaxWind           Wind          `json:"maxwind"`
	MinHumidity       float32       `json:"minhumidity"`
	Period            int           `json:"period"`
	Pop               float32       `json:"pop"`
	QPFAllDay         Precipitation `json:"qpf_allday"`
	QPFDay            Precipitation `json:"qpf_day"`
	QPFNight          Precipitation `json:"qpf_night"`
	SkyIcon           string        `json:"skyicon"`
	SnowAllDay        Precipitation `json:"snow_allday"`
	SnowDay           Precipitation `json:"snow_day"`
	SnowNight         Precipitation `json:"snow_night"`
}

type TextForecast struct {
	DateString  string            `json:"date"`
	ForecastDay []TextForecastDay `json:"forecastday"`
}

type TextForecastDay struct {
	ForecastText       string `json:"fcttext"`
	ForecastTextMetric string `json:"fcttext_metric"`
	Icon               string `json:"icon"`
	IconURL            string `json:"icon_url"`
	Period             int    `json:"period"`
	Pop                string `json:"pop"`
	Title              string `json:"title"`
}

func (forecastDay ForecastDay) Date() time.Time {
	sec, _ := strconv.ParseInt(forecastDay.DateTime.Epoch, 10, 64)
	return time.Unix(sec, 0)
}

type Precipitation struct {
	In float32 `json:"in"`
	MM float32 `json:"mm"`
	CM float32 `json:"cm"`
}

type Tempurature struct {
	Celsius    string `json:"celsius"`
	Fahrenheit string `json:"fahrenheit"`
}

type Wind struct {
	Degrees   float32 `json:"degrees"`
	Direction string  `json:"dir"`
	KPH       float32 `json:"kph"`
	MPH       float32 `json:"mph"`
}

type DateTime struct {
	AMPM           string `json:"ampm"`
	Day            int    `json:"day"`
	Epoch          string `json:"epoch"`
	Hour           int    `json:"hour"`
	IsDST          string `json:"isdst"`
	Min            string `json:"min"`
	Month          int    `json:"month"`
	MonthName      string `json:"monthname"`
	MonthNameShort string `json:"monthname_short"`
	Pretty         string `json:"pretty"`
	Sec            int    `json:"sec"`
	TZLong         string `json:"tz_long"`
	TZShort        string `json:"tz_short"`
	WeekDay        string `json:"weekday"`
	YDay           int    `json:"yday"`
	Year           int    `json:"year"`
}
