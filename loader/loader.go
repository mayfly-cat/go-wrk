package loader

import (
	"bytes"
	"errors"
	"fmt"
	"io"
	"log"
	"net/http"
	"net/url"
	"strings"
	"sync/atomic"
	"time"

	histo "github.com/HdrHistogram/hdrhistogram-go"
	"github.com/tsliwowicz/go-wrk/util"
)

const (
	USER_AGENT = "go-wrk"
)

type LoadCfg struct {
	duration           int // seconds
	goroutines         int
	testUrl            string
	reqBody            string
	method             string
	host               string
	header             map[string]string
	statsAggregator    chan *RequesterStats
	timeoutms          int
	allowRedirects     bool
	disableCompression bool
	disableKeepAlive   bool
	skipVerify         bool
	interrupted        int32
	clientCert         string
	clientKey          string
	caCert             string
	http2              bool
}

// RequesterStats used for collecting aggregate statistics
type RequesterStats struct {
	TotRespSize     int64
	TotDuration     time.Duration // 成功请求的总耗时
	TotDurationAll  time.Duration // 全部请求（成功 + 失败）的总耗时（超时按实际等待时间计算）
	NumRequests     int           // 成功请求数
	NumRequestsAll  int           // 全部请求数（成功 + 失败）
	SlowRequestsAll int           // 全部请求中，耗时 > 1s 的请求数（成功 + 失败）
	NumErrs         int
	ErrMap          map[string]int
	Histogram       *histo.Histogram
	// TimeSeries 记录每个时间段的请求数量，key 是时间段索引（从0开始，每1秒一个时间段）
	// 包括成功和失败的请求
	TimeSeries map[int]int
}

func NewLoadCfg(duration int, // seconds
	goroutines int,
	testUrl string,
	reqBody string,
	method string,
	host string,
	header map[string]string,
	statsAggregator chan *RequesterStats,
	timeoutms int,
	allowRedirects bool,
	disableCompression bool,
	disableKeepAlive bool,
	skipVerify bool,
	clientCert string,
	clientKey string,
	caCert string,
	http2 bool) (rt *LoadCfg) {
	rt = &LoadCfg{duration, goroutines, testUrl, reqBody, method, host, header, statsAggregator, timeoutms,
		allowRedirects, disableCompression, disableKeepAlive, skipVerify, 0, clientCert, clientKey, caCert, http2}
	return
}

func escapeUrlStr(in string) string {
	qm := strings.Index(in, "?")
	if qm != -1 {
		qry := in[qm+1:]
		qrys := strings.Split(qry, "&")
		var query string = ""
		var qEscaped string = ""
		var first bool = true
		for _, q := range qrys {
			qSplit := strings.Split(q, "=")
			if len(qSplit) == 2 {
				qEscaped = qSplit[0] + "=" + url.QueryEscape(qSplit[1])
			} else {
				qEscaped = qSplit[0]
			}
			if first {
				first = false
			} else {
				query += "&"
			}
			query += qEscaped

		}
		return in[:qm] + "?" + query
	} else {
		return in
	}
}

// DoRequest single request implementation. Returns the size of the response and its duration.
// On error - duration 记录从发起请求到出错的实际耗时（例如超时会接近 timeout 设置），respSize 为 0。
func DoRequest(httpClient *http.Client, header map[string]string, method, host, loadUrl, reqBody string) (respSize int, duration time.Duration, err error) {
	respSize = -1
	duration = -1

	loadUrl = escapeUrlStr(loadUrl)

	var buf io.Reader
	if len(reqBody) > 0 {
		buf = bytes.NewBufferString(reqBody)
	}

	req, err := http.NewRequest(method, loadUrl, buf)
	if err != nil {
		return 0, 0, err
	}

	for hk, hv := range header {
		req.Header.Add(hk, hv)
	}

	req.Header.Add("User-Agent", USER_AGENT)
	if host != "" {
		req.Host = host
	}
	start := time.Now()
	defer func() {
		// 确保无论成功还是失败，duration 都至少记录从发起请求到当前的耗时
		if duration < 0 {
			duration = time.Since(start)
		}
	}()
	resp, err := httpClient.Do(req)
	if err != nil {
		// this is a bit weird. When redirection is prevented, a url.Error is retuned. This creates an issue to distinguish
		// between an invalid URL that was provided and and redirection error.
		_, ok := err.(*url.Error)
		if !ok {
			return 0, duration, err
		}
		return 0, duration, err
	}
	if resp == nil {
		return 0, duration, errors.New("empty response")
	}
	defer func() {
		if resp != nil && resp.Body != nil {
			resp.Body.Close()
		}
	}()
	body, err := io.ReadAll(resp.Body)
	if err != nil {
		return 0, duration, err
	}
	if resp.StatusCode/100 == 2 { // Treat all 2XX as successful
		duration = time.Since(start)
		respSize = len(body) + int(util.EstimateHttpHeadersSize(resp.Header))
	} else if resp.StatusCode == http.StatusMovedPermanently || resp.StatusCode == http.StatusTemporaryRedirect {
		duration = time.Since(start)
		respSize = int(resp.ContentLength) + int(util.EstimateHttpHeadersSize(resp.Header))
	} else {
		return 0, duration, errors.New(fmt.Sprint("received status code ", resp.StatusCode))
	}

	return
}

func unwrap(err error) error {
	for errors.Unwrap(err) != nil {
		err = errors.Unwrap(err)
	}
	return err
}

// Requester a go function for repeatedly making requests and aggregating statistics as long as required
// When it is done, it sends the results using the statsAggregator channel
func (cfg *LoadCfg) RunSingleLoadSession() {
	stats := &RequesterStats{
		ErrMap:     make(map[string]int),
		Histogram:  histo.New(1, int64(cfg.duration*1000000), 4),
		TimeSeries: make(map[int]int),
	}
	start := time.Now()

	httpClient, err := client(cfg.disableCompression, cfg.disableKeepAlive, cfg.skipVerify,
		cfg.timeoutms, cfg.allowRedirects, cfg.clientCert, cfg.clientKey, cfg.caCert, cfg.http2)
	if err != nil {
		log.Fatal(err)
	}

	for time.Since(start).Seconds() <= float64(cfg.duration) && atomic.LoadInt32(&cfg.interrupted) == 0 {
		// 记录请求发生的时间段（每 5 秒一个时间段），用于折线图做 5s 粗粒度统计
		timeBucket := int(time.Since(start).Seconds()) / 5
		respSize, reqDur, err := DoRequest(httpClient, cfg.header, cfg.method, cfg.host, cfg.testUrl, cfg.reqBody)
		// 所有请求（无论成功还是失败）都计入全量统计
		stats.NumRequestsAll++
		if reqDur > 0 {
			stats.TotDurationAll += reqDur
		}
		// 无论成功/失败，只要单次请求耗时 > 1s，就计入慢请求（all）
		if reqDur > time.Second {
			stats.SlowRequestsAll++
		}
		if err != nil {
			stats.ErrMap[unwrap(err).Error()] += 1
			stats.NumErrs++
			// 记录失败的请求到时间序列
			stats.TimeSeries[timeBucket]++
		} else if respSize > 0 {
			stats.TotRespSize += int64(respSize)
			stats.TotDuration += reqDur
			stats.Histogram.RecordValue(reqDur.Microseconds())
			stats.NumRequests++
			// 记录成功的请求到时间序列
			stats.TimeSeries[timeBucket]++
		} else {
			stats.NumErrs++
			// 记录失败的请求到时间序列
			stats.TimeSeries[timeBucket]++
		}
	}
	cfg.statsAggregator <- stats
}

func (cfg *LoadCfg) Stop() {
	atomic.StoreInt32(&cfg.interrupted, 1)
}
