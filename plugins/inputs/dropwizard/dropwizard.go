package dropwizard

import (
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"strconv"
	"strings"
	"sync"
	"time"
	
	"github.com/influxdata/telegraf"
	"github.com/influxdata/telegraf/internal"
	"github.com/influxdata/telegraf/plugins/inputs"
)

type Dropwizard struct {
	URLs []string `toml:"urls"`
	// Path to CA file
	SSLCA string `toml:"ssl_ca"`
	// Path to host cert file
	SSLCert string `toml:"ssl_cert"`
	// Path to cert key file
	SSLKey string `toml:"ssl_key"`
	// Use SSL but skip chain & host verification
	InsecureSkipVerify bool

	Timeout internal.Duration

	client *http.Client
}

func (*Dropwizard) Description() string {
	return "Read Dropwizard-formatted JSON metrics from one or more HTTP endpoints"
}

func (*Dropwizard) SampleConfig() string {
	return `
  ## Works with Dropwizard metrics endpoint out of the box

  ## Multiple URLs from which to read Dropwizard-formatted JSON
  ## Default is "http://localhost:8081/metrics".
  urls = [
    "http://localhost:8081/metrics"
  ]

  ## Optional SSL Config
  # ssl_ca = "/etc/telegraf/ca.pem"
  # ssl_cert = "/etc/telegraf/cert.pem"
  # ssl_key = "/etc/telegraf/key.pem"
  ## Use SSL but skip chain & host verification
  # insecure_skip_verify = false

  ## http request & header timeout
  timeout = "10s"

  ## exclude some built-in metrics
  # namedrop = [
  #  "jvm.classloader*",
  #  "jvm.buffers*", 
  #  "jvm.gc*",
  #  "jvm.memory.heap*",
  #  "jvm.memory.non-heap*",
  #  "jvm.memory.pools*",
  #  "jvm.threads*",
  #  "jvm.attribute.uptime",
  #  "jvm.filedescriptor",
  #  "io.dropwizard.jetty.MutableServletContextHandler*",
  #  "org.eclipse.jetty.util*" 
  # ]

  ## include only the required fields (applies to all metrics types)
  # fieldpass = [
  #  "count",
  #  "max",
  #  "p999",
  #  "m5_Rate",
  #  "value" 
  # ]
`
}

func (d *Dropwizard) Gather(acc telegraf.Accumulator) error {
	if len(d.URLs) == 0 {
		d.URLs = []string{"http://localhost:8081/metrics"}
	}

	if d.client == nil {
		tlsCfg, err := internal.GetTLSConfig(
			d.SSLCert, d.SSLKey, d.SSLCA, d.InsecureSkipVerify)
		if err != nil {
			return err
		}
		d.client = &http.Client{
			Transport: &http.Transport{
				ResponseHeaderTimeout: d.Timeout.Duration,
				TLSClientConfig:       tlsCfg,
			},
			Timeout: d.Timeout.Duration,
		}
	}

	var wg sync.WaitGroup
	for _, u := range d.URLs {
		wg.Add(1)
		go func(url string) {
			defer wg.Done()
			if err := d.gatherURL(acc, url); err != nil {
				acc.AddError(fmt.Errorf("[url=%s]: %s", url, err))
			}
		}(u)
	}

	wg.Wait()

	return nil
}

// Gauge values can be of different types 

type gaugeValueType int

const (
        IntType gaugeValueType = iota
        FloatType
        StringType
)

type gaugeValue struct {
	IntValue int64
	FloatValue float64
	StringValue string
	Type gaugeValueType
}

func (v *gaugeValue) UnmarshalJSON(b []byte) error {
	jsonString := string(b)
	if intValue, err := strconv.ParseInt(jsonString, 10, 64); err == nil {
		*v = gaugeValue{ 
			IntValue: intValue,
			Type: IntType,
		}
	} else if floatValue, err := strconv.ParseFloat(jsonString, 64); err == nil {
		*v = gaugeValue{ 
			FloatValue: floatValue,
			Type: FloatType,
		}
	} else {
		*v = gaugeValue{ 
			StringValue: strings.Trim(jsonString, "\""),
			Type: StringType,
		}
	}

	return nil
}

type gauge struct {
	Value gaugeValue `json:"value"`
}

type counter struct {
	Count int64 `json:"count"`
}

type histogram struct {
	Count  int64   `json:"count"`
	Max    int64   `json:"max"`
	Mean   float64 `json:"mean"`
	Min    int64   `json:"min"`
	P50    float64 `json:"p50"`
	P75    float64 `json:"p75"`
	P95    float64 `json:"p95"`
	P98    float64 `json:"p98"`
	P99    float64 `json:"p99"`
	P999   float64 `json:"p999"`
	Stddev float64 `json:"stddev"`
}

type meter struct {
	Count    int64   `json:"count"`
	M15Rate  float64 `json:"m15_rate"`
	M1Rate   float64 `json:"m1_rate"`
	M5Rate   float64 `json:"m5_rate"`
	MeanRate float64 `json:"mean_rate"`
	Units    string  `json:"units"`
}

type timer struct {
	Count         int64   `json:"count"`
	Max           float64 `json:"max"`
	Mean          float64 `json:"mean"`
	Min           float64 `json:"min"`
	P50           float64 `json:"p50"`
	P75           float64 `json:"p75"`
	P95           float64 `json:"p95"`
	P98           float64 `json:"p98"`
	P99           float64 `json:"p99"`
	P999          float64 `json:"p999"`
	Stddev        float64 `json:"stddev"`
	M15Rate       float64 `json:"m15_rate"`
	M1Rate        float64 `json:"m1_rate"`
	M5Rate        float64 `json:"m5_rate"`
	MeanRate      float64 `json:"mean_rate"`
	DurationUnits string  `json:"duration_units"`
	RateUnits     string  `json:"rate_units"`
}

type metrics struct {
	Version    string               `json:"version"`
	Gauges     map[string]gauge     `json:"gauges"`
	Counters   map[string]counter   `json:"counters"`
	Histograms map[string]histogram `json:"histograms"`
	Meters     map[string]meter     `json:"meters"`
	Timers     map[string]timer     `json:"timers"`
}

// Gathers data from a particular URL
// Parameters:
//     acc    : The telegraf Accumulator to use
//     url    : endpoint to send request to
//
// Returns:
//     error: Any error that may have occurred
func (d *Dropwizard) gatherURL(
	acc telegraf.Accumulator,
	url string,
) error {
	now := time.Now()

	resp, err := d.client.Get(url)
	if err != nil {
		return err
	}
	defer resp.Body.Close()

	metrics, err := d.DecodeJSONMetrics(resp.Body)
	if err != nil {
		return err
	}

	// tags can be specified in config either globally or per input
	// through built-in functionality
	var tags map[string]string = nil

	//TODO do we need to call SetPricision and is this related to units?
	
	for name, g := range metrics.Gauges {
		if g.Value.Type == IntType {
			acc.AddGauge(name,
				map[string]interface{}{ "value": g.Value.IntValue },
				tags,
				now)
		} else if g.Value.Type == FloatType {
			acc.AddGauge(name,
				map[string]interface{}{ "value": g.Value.FloatValue },
				tags,
				now)
		}
	}

	for name, c := range metrics.Counters {
		acc.AddCounter(name,
			map[string]interface{}{ "count": c.Count },
			tags,
			now)
	}

	for name, h := range metrics.Histograms {
		acc.AddHistogram(name,
			map[string]interface{}{ 
				"count": h.Count,
				"max": h.Max,
				"mean": h.Mean,
				"min": h.Min,
				"p50": h.P50,
				"p75": h.P75,
				"p95": h.P95,
				"p98": h.P98,
				"p99": h.P99,
				"p999": h.P999,
				"stddev": h.Stddev,
			},
			tags,
			now)
	}

	//TODO what to do with the Units?
	for name, m := range metrics.Meters {
		acc.AddHistogram(name,
			map[string]interface{}{ 
				"count": m.Count,
				"m15_rate": m.M15Rate,
				"m1_rate": m.M1Rate,
				"m5_rate": m.M5Rate,
				"mean_rate": m.MeanRate,
			},
			tags,
			now)
	}

	//TODO what to do with duration and rate units?
	for name, t := range metrics.Timers {
		acc.AddFields(name,
			map[string]interface{}{ 
				"count": t.Count,
				"max": t.Max,
				"mean": t.Mean,
				"min": t.Min,
				"p50": t.P50,
				"p75": t.P75,
				"p95": t.P95,
				"p98": t.P98,
				"p99": t.P99,
				"p999": t.P999,
				"stddev": t.Stddev,
				"m15_rate": t.M15Rate,
				"m1_rate": t.M1Rate,
				"m5_rate": t.M5Rate,
				"mean_rate": t.MeanRate,
			},
			tags,
			now)
	}

	return nil
}

func init() {
	inputs.Add("dropwizard", func() telegraf.Input {
		return &Dropwizard{
			Timeout: internal.Duration{Duration: time.Second * 5},
		}
	})
}

func (*Dropwizard) DecodeJSONMetrics(r io.Reader) (metrics, error) {
	var decodedMetrics metrics
	err := json.NewDecoder(r).Decode(&decodedMetrics)
	if err != nil {
		return decodedMetrics, err
	}
	return decodedMetrics, nil
}