package main

// http://tsdb.kronos.d:4242/#start=2016/02/09-08:00:00&end=2016/02/09-11:00:00&m=mimmax:php.timers.mongo.p95&o=&yrange=%5B0:%5D&wxh=1420x608&style=linespoint
// http://tsdb.kronos.d:4242/api/query/?start=2016/02/09-08:00:00&end=2016/02/09-11:00:00&m=mimmax:php.timers.mongo.p95

// http://tsdb.kronos.d:4242/#start=2016/02/03-09:23:00&end=2016/02/10-09:23:58&m=mimmax:gauges.do.sphinx.lag&o=&yrange=%5B0:%5D&wxh=1380x636&style=linespoint
import (
	"encoding/json"
	"fmt"
	"log"
	"math"
	"net/http"
	"net/url"
	"os"
	"sort"
	"strconv"
	"time"
)

const tsdbHost = "http://tsdb.kronos.d:4242"

const tpl = `<!DOCTYPE html>
<html>
	<head>
		<title>chart</title>
		<script type="text/javascript" src="http://www.amcharts.com/lib/3/amcharts.js"></script>
		<script type="text/javascript" src="http://www.amcharts.com/lib/3/serial.js"></script>
		<script type="text/javascript">
			AmCharts.makeChart("chartdiv", {
				"type": "serial",
				"categoryField": "date",
				"dataDateFormat": "YYYY-MM-DD HH:NN:SS",
				"precision": 4,
				"categoryAxis": { "minPeriod": "ss", "parseDates": true },
				"chartCursor": {"enabled": true, "categoryBalloonDateFormat": "JJ:NN:SS" },
				"chartScrollbar": { "enabled": true },
				"trendLines": [],
				"graphs": [
					{"id": "value", "title": "Value", "valueField": "val"},
					{"id": "average", "title": "Average", "valueField": "avg"},
					{"id": "sigma", "title": "Sigma", "valueField": "sigma", "valueAxis": "ValueAxis-2"}
				],
				"guides": %s,
				"valueAxes": [{
					"id": "ValueAxis-1",
					"title": "Values"
				},{
					"id": "ValueAxis-2",
					"position": "right",
					"title": "Sigma"
				}],
				"allLabels": [],
				"balloon": {},
				"legend": { "enabled": true, "useGraphSettings": true },
				"titles": [],
				"dataProvider": %s
			});
		</script>
	</head>
	<body>
		<div id="chartdiv" style="width: 100%%; height: 400px; background-color: #FFFFFF;" ></div>
	</body>
</html>
`

type tsdbResponse struct {
	Metric        string             `json:"metric"`
	Tags          map[string]string  `json:"Tags"`
	AggregateTags []string           `json:"aggregateTags"`
	DataPoints    map[string]float64 `json:"dps"`
}

type DataPoint struct {
	Time  time.Time
	Value float64
}

type DataPoints []DataPoint

func (dps DataPoints) Len() int           { return len(dps) }
func (dps DataPoints) Swap(i, j int)      { dps[i], dps[j] = dps[j], dps[i] }
func (dps DataPoints) Less(i, j int) bool { return dps[i].Time.Before(dps[j].Time) }

type chartData struct {
	Date    string  `json:"date"`
	Value   float64 `json:"val"`
	Average float64 `json:"avg"`
	Sigma   float64 `json:"sigma"`
}

/*
	{
		"date": "2016-02-09 09:20:00",
		"id": "anomaly-01",
		"lineAlpha": 0.5,
		"lineColor": "#FF0000",
		"lineThickness": 5
	}
*/
type anomalyData struct {
	ID        string  `json:"id"`
	Date      string  `json:"date",omitempty`
	Alpha     float64 `json:"lineAlpha",omitempty`
	Color     string  `json:"lineColor"`
	Thickness int64   `json:"lineThickness"`
	Value     int64   `json:"value",omitempty`
	Axis      string  `json:"valueAxis",omitempty`
}

func main() {
	port := "80"
	if os.Getenv("PORT") != "" {
		port = os.Getenv("PORT")
	}

	http.HandleFunc("/", func(w http.ResponseWriter, r *http.Request) {
		t := time.Now()

		start := r.URL.Query().Get("start")
		end := r.URL.Query().Get("end")
		m := r.URL.Query().Get("m")
		periodRaw := r.URL.Query().Get("period")

		if m == "" {
			fmt.Fprintln(w, "Берём урл от tsdb, заменяем хост и порт на текущий, вместо '/#start' делаем '/?start'")
			return
		}

		if periodRaw == "" {
			periodRaw = "120"
		}
		period, err := strconv.Atoi(periodRaw)
		if err != nil {
			http.Error(w, fmt.Sprintf("Failed to atoi: %v", err), http.StatusInternalServerError)
		}

		chart, anomalyes, err := detectAnomalyes(start, end, m, period)
		if err != nil {
			http.Error(w, fmt.Sprintf("Failed to detect: %v", err), http.StatusInternalServerError)
		}

		anomalyes = append(anomalyes, anomalyData{
			ID:        "guide-1",
			Color:     "#CC0000",
			Thickness: 3,
			Value:     1,
			Axis:      "ValueAxis-2",
		})

		anomalyes = append(anomalyes, anomalyData{
			ID:        "guide-1",
			Color:     "#CC0000",
			Thickness: 3,
			Value:     -1,
			Axis:      "ValueAxis-2",
		})

		chartBytes, err := json.Marshal(chart)
		if err != nil {
			http.Error(w, fmt.Sprintf("Failed to Marshal: %v", err), http.StatusInternalServerError)
		}

		anomalyesBytes, err := json.Marshal(anomalyes)
		if err != nil {
			http.Error(w, fmt.Sprintf("Failed to Marshal: %v", err), http.StatusInternalServerError)
		}

		fmt.Fprintf(w, tpl, string(anomalyesBytes), string(chartBytes))

		log.Printf("%s %q %v\n", r.Method, r.URL.String(), time.Since(t))
	})
	log.Fatal(http.ListenAndServe(":"+port, nil))
}

func detectAnomalyes(start, end, m string, period int) ([]chartData, []anomalyData, error) {
	path := url.Values{}
	// path.Add("start", "2016/02/09-08:00:00")
	// path.Add("end", "2016/02/09-11:00:00")
	// path.Add("m", "mimmax:php.timers.mongo.p95")
	path.Add("start", start)
	path.Add("end", end)
	path.Add("m", m)

	resp, err := http.Get(fmt.Sprintf("%s/api/query/?%s", tsdbHost, path.Encode()))
	if err != nil {
		return nil, nil, fmt.Errorf("failed to get data from TSDB: %v", err)
	}

	var data []tsdbResponse
	if err := json.NewDecoder(resp.Body).Decode(&data); err != nil {
		return nil, nil, fmt.Errorf("failed to decode TSDB response: %v", err)
	}

	dps := make(DataPoints, 0)
	for ts, val := range data[0].DataPoints {
		timestamp, err := strconv.Atoi(ts)
		if err != nil {
			continue
		}
		dp := DataPoint{
			Time:  time.Unix(int64(timestamp), 0),
			Value: val,
		}
		dps = append(dps, dp)
	}

	sort.Sort(dps)

	var movingAverage float64
	var sigma float64
	vals := make([]float64, period)

	chart := make([]chartData, 0)
	anomalyes := make([]anomalyData, 0)

	loc, _ := time.LoadLocation("Asia/Novosibirsk")
	for idx, cdp := range dps {
		if idx <= period {
			continue
		}
		points := dps[idx-period : idx]
		for i, dp := range points {
			vals[i] = dp.Value
		}

		movingAverage = average(vals)
		sigma = test3Sigma(vals)
		anomalyIdx := 0
		if sigma > 1.2 {
			anomalyIdx++
			anomalyes = append(anomalyes, anomalyData{
				ID:        fmt.Sprintf("anomaly-%d", anomalyIdx),
				Date:      cdp.Time.In(loc).Format("2006-01-02 15:04:05"),
				Alpha:     0.5,
				Color:     "#FF0000",
				Thickness: 5,
			})
		}
		//fmt.Printf("%v,%v,%v,%v\n", cdp.Time.Format("2006-01-02 15:04:05"), cdp.Value, movingAverage, sigma)

		chart = append(chart, chartData{
			cdp.Time.In(loc).Format("2006-01-02 15:04:05"),
			cdp.Value,
			movingAverage,
			sigma,
		})
	}

	return chart, anomalyes, nil
}

func average(vals []float64) float64 {
	var sum float64
	for i := 0; i < len(vals); i++ {
		sum += vals[i]
	}
	return sum / float64(len(vals))
}

// Get the standard deviation of float64 values, with
// an input average.
func stddev(vals []float64, avg float64) float64 {
	var sum float64
	for i := 0; i < len(vals); i++ {
		dis := vals[i] - avg
		sum += dis * dis
	}
	return math.Sqrt(sum / float64(len(vals)))
}

// Calculate metric score with 3-sigma rule.
//
// What's the 3-sigma rule?
//
//	states that nearly all values (99.7%) lie within the 3 standard deviations
//	of the mean in a normal distribution.
//
// Also like z-score, defined as
//
//	(val - mean) / stddev
//
// And we name the below as metric score, yet 1/3 of z-score
//
//	(val - mean) / (3 * stddev)
//
// The score has
//
//	score > 0   => values is trending up
//	score < 0   => values is trending down
//	score > 1   => values is anomalously trending up
//	score < -1  => values is anomalously trending down
//
// The following function will set the metric score and also the average.
//
func test3Sigma(vals []float64) float64 {
	if len(vals) == 0 {
		return 0
	}

	// Values average and standard deviation.
	avg := average(vals)
	std := stddev(vals, avg)
	last := average(vals[len(vals)-3:])

	if std == 0 {
		switch {
		case last == avg:
			return 0
		case last > avg:
			return 1
		case last < avg:
			return -1
		}
		return 0
	}
	// 3-sigma
	return math.Abs(last-avg) / (3 * std)
}
