package main

import (
	"flag"
	"fmt"
	"io"
	"os"
	"os/signal"
	"runtime"
	"strings"
	"time"

	histo "github.com/HdrHistogram/hdrhistogram-go"
	"github.com/tsliwowicz/go-wrk/loader"
	"github.com/tsliwowicz/go-wrk/report"
	"github.com/tsliwowicz/go-wrk/util"
)

const APP_VERSION = "0.10"

// default that can be overridden from the command line
var versionFlag bool = false
var helpFlag bool = false
var duration int = 10 //seconds
var goroutines int = 2
var testUrl string
var method string = "GET"
var host string
var headerFlags util.HeaderList
var header map[string]string
var statsAggregator chan *loader.RequesterStats
var timeoutms int
var allowRedirectsFlag bool = false
var disableCompression bool
var disableKeepAlive bool
var skipVerify bool
var playbackFile string
var reqBody string
var clientCert string
var clientKey string
var caCert string
var http2 bool
var cpus int = 0
var htmlReport string
var debug bool

func init() {
	flag.BoolVar(&versionFlag, "v", false, "Print version details")
	flag.BoolVar(&allowRedirectsFlag, "redir", false, "Allow Redirects")
	flag.BoolVar(&helpFlag, "help", false, "Print help")
	flag.BoolVar(&disableCompression, "no-c", false, "Disable Compression - Prevents sending the \"Accept-Encoding: gzip\" header")
	flag.BoolVar(&disableKeepAlive, "no-ka", false, "Disable KeepAlive - prevents re-use of TCP connections between different HTTP requests")
	flag.BoolVar(&skipVerify, "no-vr", false, "Skip verifying SSL certificate of the server")
	flag.IntVar(&goroutines, "c", 10, "Number of goroutines to use (concurrent connections)")
	flag.IntVar(&duration, "d", 10, "Duration of test in seconds")
	flag.IntVar(&timeoutms, "T", 1000, "Socket/request timeout in ms")
	flag.IntVar(&cpus, "cpus", 0, "Number of cpus, i.e. GOMAXPROCS. 0 = system default.")
	flag.StringVar(&method, "M", "GET", "HTTP method")
	flag.StringVar(&host, "host", "", "Host Header")
	flag.Var(&headerFlags, "H", "Header to add to each request (you can define multiple -H flags)")
	flag.StringVar(&playbackFile, "f", "<empty>", "Playback file name")
	flag.StringVar(&reqBody, "body", "", "request body string or @filename")
	flag.StringVar(&clientCert, "cert", "", "CA certificate file to verify peer against (SSL/TLS)")
	flag.StringVar(&clientKey, "key", "", "Private key file name (SSL/TLS")
	flag.StringVar(&caCert, "ca", "", "CA file to verify peer against (SSL/TLS)")
	flag.BoolVar(&http2, "http", true, "Use HTTP/2")
	flag.StringVar(&htmlReport, "html-report", "", "HTML report output file path")
	flag.BoolVar(&debug, "debug", false, "Enable debug logs for request statistics")
}

// printDefaults a nicer format for the defaults
func printDefaults() {
	fmt.Println("Usage: go-wrk <options> <url>")
	fmt.Println("Options:")
	flag.VisitAll(func(flag *flag.Flag) {
		fmt.Println("\t-"+flag.Name, "\t", flag.Usage, "(Default "+flag.DefValue+")")
	})
}

func mapToString(m map[string]int) string {
	s := make([]string, 0, len(m))
	for k, v := range m {
		s = append(s, fmt.Sprint(k, "=", v))
	}
	return strings.Join(s, ",")
}

func main() {

	statsAggregator = make(chan *loader.RequesterStats, goroutines)
	sigChan := make(chan os.Signal, 1)

	signal.Notify(sigChan, os.Interrupt)

	flag.Parse() // Scan the arguments list
	header = make(map[string]string)
	for _, hdr := range headerFlags {
		hp := strings.SplitN(hdr, ":", 2)
		header[hp[0]] = hp[1]
	}

	if playbackFile != "<empty>" {
		file, err := os.Open(playbackFile) // For read access.
		if err != nil {
			fmt.Println(err)
			os.Exit(1)
		}
		defer file.Close()
		url, err := io.ReadAll(file)
		if err != nil {
			fmt.Println(err)
			os.Exit(1)
		}
		testUrl = string(url)
	} else {
		testUrl = flag.Arg(0)
	}

	if versionFlag {
		fmt.Println("Version:", APP_VERSION)
		return
	} else if helpFlag || len(testUrl) == 0 {
		printDefaults()
		return
	}

	if cpus > 0 {
		runtime.GOMAXPROCS(cpus)
	}

	fmt.Printf("Running %vs test @ %v\n  %v goroutine(s) running concurrently\n", duration, testUrl, goroutines)

	if len(reqBody) > 0 && reqBody[0] == '@' {
		bodyFilename := reqBody[1:]
		data, err := os.ReadFile(bodyFilename)
		if err != nil {
			fmt.Println(fmt.Errorf("could not read file %q: %v", bodyFilename, err))
			os.Exit(1)
		}
		reqBody = string(data)
	}

	loadGen := loader.NewLoadCfg(duration, goroutines, testUrl, reqBody, method, host, header, statsAggregator, timeoutms,
		allowRedirectsFlag, disableCompression, disableKeepAlive, skipVerify, clientCert, clientKey, caCert, http2)

	start := time.Now()

	for i := 0; i < goroutines; i++ {
		go loadGen.RunSingleLoadSession()
	}

	responders := 0
	aggStats := loader.RequesterStats{
		ErrMap:     make(map[string]int),
		Histogram:  histo.New(1, int64(duration*1000000), 4),
		TimeSeries: make(map[int]int),
	}

	for responders < goroutines {
		select {
		case <-sigChan:
			loadGen.Stop()
			fmt.Printf("stopping...\n")
		case stats := <-statsAggregator:
			if debug {
				fmt.Printf("[debug] worker finished: requests=%d, errors=%d, bytes=%d, duration=%v\n",
					stats.NumRequests, stats.NumErrs, stats.TotRespSize, stats.TotDuration)
			}
			aggStats.NumErrs += stats.NumErrs
			aggStats.NumRequests += stats.NumRequests
			aggStats.NumRequestsAll += stats.NumRequestsAll
			aggStats.SlowRequestsAll += stats.SlowRequestsAll
			aggStats.TotRespSize += stats.TotRespSize
			aggStats.TotDuration += stats.TotDuration
			aggStats.TotDurationAll += stats.TotDurationAll
			responders++
			for k, v := range stats.ErrMap {
				aggStats.ErrMap[k] += v
			}
			aggStats.Histogram.Merge(stats.Histogram)
			// 合并时间序列数据
			for k, v := range stats.TimeSeries {
				aggStats.TimeSeries[k] += v
			}
		}
	}

	wallClockDuration := time.Since(start)

	if aggStats.NumRequests == 0 {
		fmt.Println("Error: No statistics collected / no requests found")
		fmt.Printf("Number of Errors:\t%v\n", aggStats.NumErrs)
		if aggStats.NumErrs > 0 {
			fmt.Printf("Error Counts:\t\t%v\n", mapToString(aggStats.ErrMap))
		}
		return
	}

	avgThreadDur := aggStats.TotDuration / time.Duration(responders) // need to average the aggregated duration

	reqRate := float64(aggStats.NumRequests) / avgThreadDur.Seconds()
	bytesRate := float64(aggStats.TotRespSize) / avgThreadDur.Seconds()

	// 使用「配置的压测时长」来计算整体 RPS，更贴近业务视角，不受汇总/输出耗时影响
	testDurationSec := float64(duration)
	if testDurationSec <= 0 {
		testDurationSec = wallClockDuration.Seconds()
	}

	overallReqRate := float64(aggStats.NumRequests) / testDurationSec
	overallBytesRate := float64(aggStats.TotRespSize) / testDurationSec

	// 计算总请求数（包括错误）和错误率
	totalRequestsAll := aggStats.NumRequestsAll
	if totalRequestsAll == 0 {
		// 兼容旧数据结构：如果新的 NumRequestsAll 还没被填充，就退回到成功+错误的和
		totalRequestsAll = aggStats.NumRequests + aggStats.NumErrs
	}
	errorRate := 0.0
	if totalRequestsAll > 0 {
		errorRate = float64(aggStats.NumErrs) / float64(totalRequestsAll) * 100.0
	}
	overallReqRateAll := float64(totalRequestsAll) / testDurationSec

	// 全量请求（成功 + 失败）的平均延迟（包含超时等错误，按真实等待时长计入）
	var avgLatencyAll time.Duration
	if totalRequestsAll > 0 && aggStats.TotDurationAll > 0 {
		avgLatencyAll = aggStats.TotDurationAll / time.Duration(totalRequestsAll)
	}

	fmt.Printf("=== Request Statistics ===\n")
	fmt.Printf("Total Requests (all):\t%d (success: %d, errors: %d, error rate: %.2f%%)\n",
		totalRequestsAll, aggStats.NumRequests, aggStats.NumErrs, errorRate)
	fmt.Printf("Test Duration (config):\t%ds\n", duration)
	fmt.Printf("Test Duration (wall):\t%v\n", wallClockDuration)
	fmt.Printf("Avg Thread Duration:\t%v (average per goroutine)\n", avgThreadDur)
	fmt.Printf("\n=== Throughput (Success Requests Only) ===\n")
	fmt.Printf("Requests/sec (thread avg):\t%.2f req/s\n", reqRate)
	fmt.Printf("Requests/sec (overall):\t\t%.2f req/s\n", overallReqRate)
	fmt.Printf("Transfer/sec:\t\t\t%v\n", util.ByteSize{Size: bytesRate})
	fmt.Printf("Overall Transfer/sec:\t\t%v\n", util.ByteSize{Size: overallBytesRate})
	fmt.Printf("\n=== Throughput (All Requests) ===\n")
	fmt.Printf("Requests/sec (all, overall):\t%.2f req/s\n", overallReqRateAll)
	fmt.Printf("\n=== Latency (Success Requests Only) ===\n")
	fmt.Printf("Fastest Request:\t%v\n", toDuration(aggStats.Histogram.Min()))
	fmt.Printf("Avg Req Time:\t\t%v\n", toDuration(int64(aggStats.Histogram.Mean())))
	fmt.Printf("Slowest Request:\t%v\n", toDuration(aggStats.Histogram.Max()))
	if avgLatencyAll > 0 {
		fmt.Printf("Avg Req Time (all):\t%v (including errors/timeouts)\n", avgLatencyAll)
	}
	if aggStats.NumErrs > 0 {
		fmt.Printf("\n=== Error Details ===\n")
		fmt.Printf("Number of Errors:\t%v\n", aggStats.NumErrs)
		fmt.Printf("Error Counts:\t\t%v\n", mapToString(aggStats.ErrMap))
	}
	fmt.Printf("\n=== Latency Percentiles (Success Requests Only) ===\n")
	fmt.Printf("10%%:\t\t\t%v\n", toDuration(aggStats.Histogram.ValueAtPercentile(.10)))
	fmt.Printf("50%%:\t\t\t%v\n", toDuration(aggStats.Histogram.ValueAtPercentile(.50)))
	fmt.Printf("75%%:\t\t\t%v\n", toDuration(aggStats.Histogram.ValueAtPercentile(.75)))
	fmt.Printf("99%%:\t\t\t%v\n", toDuration(aggStats.Histogram.ValueAtPercentile(.99)))
	fmt.Printf("99.9%%:\t\t\t%v\n", toDuration(aggStats.Histogram.ValueAtPercentile(.999)))
	fmt.Printf("99.9999%%:\t\t%v\n", toDuration(aggStats.Histogram.ValueAtPercentile(.999999)))
	fmt.Printf("99.99999%%:\t\t%v\n", toDuration(aggStats.Histogram.ValueAtPercentile(.9999999)))
	fmt.Printf("stddev:\t\t\t%v\n", toDuration(int64(aggStats.Histogram.StdDev())))
	// aggStats.Histogram.PercentilesPrint(os.Stdout,1,1)

	// 可选：生成 HTML 报告
	if htmlReport != "" {
		// 使用精确计数的方式计算 >1s 的请求数和占比（成功 + 失败，超时也计入）
		slowRequestsCount := aggStats.SlowRequestsAll
		slowRequestsRate := 0.0
		if totalRequestsAll > 0 {
			slowRequestsRate = float64(slowRequestsCount) / float64(totalRequestsAll) * 100.0
		}

		// 计算延迟分组统计（仅成功请求）
		latencyBuckets := make(map[string]report.LatencyBucket)
		totalSuccessRequests := aggStats.NumRequests
		if totalSuccessRequests > 0 && aggStats.Histogram.TotalCount() > 0 {
			// 定义时间阈值范围（微秒）
			ranges := []struct {
				label string
				minUs int64
				maxUs int64
			}{
				{"100ms", 0, 100 * 1000},
				{"200ms", 100 * 1000, 200 * 1000},
				{"300ms", 200 * 1000, 300 * 1000},
				{"500ms", 300 * 1000, 500 * 1000},
				{"700ms", 500 * 1000, 700 * 1000},
				{"1s", 700 * 1000, 1000 * 1000},
			}

			// 初始化每个区间的计数
			bucketCounts := make(map[string]int64)
			for _, r := range ranges {
				bucketCounts[r.label] = 0
			}
			bucketCounts[">1s"] = 0 // 初始化 >1s 的计数

			// 遍历 histogram 的分布，累加每个区间的计数
			bars := aggStats.Histogram.Distribution()
			oneSecondUs := int64(1000 * 1000) // 1秒 = 1000000微秒
			for _, bar := range bars {
				// bar.From 和 bar.To 是区间的边界（微秒），bar.Count 是该区间的请求数
				// 检查这个 bar 是否与我们的时间范围有交集
				for _, r := range ranges {
					// 如果 bar 与范围 r 有交集，累加计数
					// 交集条件：bar.From < r.maxUs && bar.To > r.minUs
					if bar.From < r.maxUs && bar.To > r.minUs {
						// 计算交集部分的计数（简化处理：如果 bar 完全在范围内，使用全部计数；否则按比例）
						intersectMin := bar.From
						if intersectMin < r.minUs {
							intersectMin = r.minUs
						}
						intersectMax := bar.To
						if intersectMax > r.maxUs {
							intersectMax = r.maxUs
						}
						// 按比例分配计数（简化：如果 bar 跨度很小，直接使用全部计数）
						if bar.To-bar.From > 0 {
							ratio := float64(intersectMax-intersectMin) / float64(bar.To-bar.From)
							bucketCounts[r.label] += int64(float64(bar.Count) * ratio)
						} else {
							bucketCounts[r.label] += bar.Count
						}
					}
				}
				// 统计 >1s 的请求（bar.From >= 1s 的所有请求）
				if bar.From >= oneSecondUs {
					// 如果整个 bar 都在 >1s 范围内，使用全部计数
					bucketCounts[">1s"] += bar.Count
				} else if bar.To > oneSecondUs {
					// 如果 bar 跨越 1s 边界，按比例分配 >1s 部分的计数
					intersectMin := oneSecondUs
					intersectMax := bar.To
					if bar.To-bar.From > 0 {
						ratio := float64(intersectMax-intersectMin) / float64(bar.To-bar.From)
						bucketCounts[">1s"] += int64(float64(bar.Count) * ratio)
					}
				}
			}

			// 计算每个区间的占比
			for _, r := range ranges {
				bucketCount := int(bucketCounts[r.label])
				rate := 0.0
				if totalSuccessRequests > 0 {
					rate = float64(bucketCount) / float64(totalSuccessRequests) * 100.0
				}
				latencyBuckets[r.label] = report.LatencyBucket{
					Count: bucketCount,
					Rate:  rate,
				}
			}
			// 计算 >1s 的占比
			bucketCount := int(bucketCounts[">1s"])
			rate := 0.0
			if totalSuccessRequests > 0 {
				rate = float64(bucketCount) / float64(totalSuccessRequests) * 100.0
			}
			latencyBuckets[">1s"] = report.LatencyBucket{
				Count: bucketCount,
				Rate:  rate,
			}
		}

		summary := report.Summary{
			TestURL:     testUrl,
			Concurrency: goroutines,
			// HTML 报告展示配置的压测时长（秒）
			DurationSec:         duration,
			TotalRequests:       aggStats.NumRequests,
			TotalRequestsAll:    totalRequestsAll,
			TotalErrors:         aggStats.NumErrs,
			ErrorCounts:         aggStats.ErrMap,
			ErrorRate:           errorRate,
			ReqPerSec:           reqRate,
			OverallReqPerSec:    overallReqRate,
			OverallReqPerSecAll: overallReqRateAll,
			BytesPerSec:         bytesRate,
			OverallBytesPerSec:  overallBytesRate,
			AvgLatency:          toDuration(int64(aggStats.Histogram.Mean())),
			AvgLatencyAll:       avgLatencyAll,
			Fastest:             toDuration(aggStats.Histogram.Min()),
			Slowest:             toDuration(aggStats.Histogram.Max()),
			P10:                 toDuration(aggStats.Histogram.ValueAtPercentile(.10)),
			P50:                 toDuration(aggStats.Histogram.ValueAtPercentile(.50)),
			P75:                 toDuration(aggStats.Histogram.ValueAtPercentile(.75)),
			P99:                 toDuration(aggStats.Histogram.ValueAtPercentile(.99)),
			P999:                toDuration(aggStats.Histogram.ValueAtPercentile(.999)),
			P9999:               toDuration(aggStats.Histogram.ValueAtPercentile(.9999)),
			P99999:              toDuration(aggStats.Histogram.ValueAtPercentile(.99999)),
			StdDev:              toDuration(int64(aggStats.Histogram.StdDev())),
			SlowRequestsCount:   slowRequestsCount,
			SlowRequestsRate:    slowRequestsRate,
			LatencyBuckets:      latencyBuckets,
			TimeSeries:          aggStats.TimeSeries,
			GeneratedAt:         time.Now(),
		}
		if err := report.GenerateHTMLFromSummary(summary, htmlReport); err != nil {
			fmt.Printf("failed to generate HTML report: %v\n", err)
		} else if debug {
			fmt.Printf("[debug] HTML report written to %s\n", htmlReport)
		}
	}
}

func toDuration(usecs int64) time.Duration {
	return time.Duration(usecs * 1000)
}
