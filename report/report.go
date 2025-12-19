package report

import (
	"bufio"
	"fmt"
	"os"
	"regexp"
	"strconv"
	"strings"
	"time"
)

// Summary 描述一次压测的关键统计信息，用于渲染 HTML 报告。
type Summary struct {
	Title       string
	TestURL     string
	Concurrency int
	DurationSec int

	TotalRequests    int // 成功请求数（仅 HTTP 200）
	TotalRequestsAll int // 总请求数（包括错误）
	TotalErrors      int
	ErrorCounts      map[string]int
	ErrorRate        float64 // 错误率（百分比）

	ReqPerSec           float64
	OverallReqPerSec    float64
	OverallReqPerSecAll float64 // 总请求数（包括错误）的 RPS
	BytesPerSec         float64
	OverallBytesPerSec  float64

	AvgLatency    time.Duration // 仅成功请求的平均延迟
	AvgLatencyAll time.Duration // 全量请求（成功 + 失败）的平均延迟，错误/超时按实际等待时间计入
	Fastest       time.Duration
	Slowest       time.Duration
	P10           time.Duration
	P50           time.Duration
	P75           time.Duration
	P99           time.Duration
	P999          time.Duration
	P9999         time.Duration
	P99999        time.Duration
	StdDev        time.Duration

	SlowRequestsCount int     // 请求时间 > 1s 的请求数
	SlowRequestsRate  float64 // 请求时间 > 1s 的请求占比（百分比）

	// LatencyBuckets 记录各时间段内的请求数量和占比
	LatencyBuckets map[string]LatencyBucket

	// NetStatusCounts 按 netStatus（HTTP 状态码或错误类别）分组的请求数量
	NetStatusCounts map[string]int

	// TimeSeries 记录每个时间段的请求数量，key 是时间段索引（从0开始，每1秒一个时间段）
	TimeSeries map[int]int

	GeneratedAt time.Time
}

// LatencyBucket 记录某个延迟范围内的请求统计
type LatencyBucket struct {
	Count int     // 请求数量
	Rate  float64 // 占比（百分比）
}

// GenerateHTMLFromSummary 根据 Summary 直接生成 HTML 报告。
func GenerateHTMLFromSummary(s Summary, out string) error {
	// 使用传入的 ErrorRate，如果没有则计算
	errorRate := s.ErrorRate
	if errorRate == 0.0 && s.TotalRequestsAll > 0 {
		errorRate = float64(s.TotalErrors) / float64(s.TotalRequestsAll) * 100
	}

	stripMs := func(d time.Duration) string {
		if d <= 0 {
			return "-"
		}
		// 统一用毫秒带 3 位小数
		return fmt.Sprintf("%.3fms", float64(d.Microseconds())/1000.0)
	}

	toMsNum := func(d time.Duration) string {
		if d <= 0 {
			return "0"
		}
		return fmt.Sprintf("%.3f", float64(d.Microseconds())/1000.0)
	}

	// 处理时间序列数据，转换为 JavaScript 数组格式
	var timeSeriesLabels []string
	var timeSeriesData []string
	if s.TimeSeries != nil && len(s.TimeSeries) > 0 {
		// 找到最大时间段索引
		maxBucket := 0
		for k := range s.TimeSeries {
			if k > maxBucket {
				maxBucket = k
			}
		}
		// 生成从 0 到 maxBucket 的所有时间段标签和数据
		for i := 0; i <= maxBucket; i++ {
			// 标签需要加引号，因为 JavaScript 数组中的字符串需要引号。
			// 这里的 bucket 单位是 5 秒，因此用 i*5 作为时间刻度。
			timeSeriesLabels = append(timeSeriesLabels, fmt.Sprintf(`"%ds"`, i*5))
			count := s.TimeSeries[i]
			// 将每个 5s 窗口内的请求数换算成 RPS（平均每秒请求数）
			rps := float64(count) / 5.0
			timeSeriesData = append(timeSeriesData, fmt.Sprintf("%.2f", rps))
		}
	} else {
		// 如果没有时间序列数据，至少生成一个空数组
		timeSeriesLabels = []string{}
		timeSeriesData = []string{}
	}
	timeSeriesLabelsStr := "[]"
	timeSeriesDataStr := "[]"
	if len(timeSeriesLabels) > 0 {
		timeSeriesLabelsStr = "[" + strings.Join(timeSeriesLabels, ",") + "]"
		timeSeriesDataStr = "[" + strings.Join(timeSeriesData, ",") + "]"
	}

	// 把错误按 "msg × count" 展示
	var errLines []string
	for k, v := range s.ErrorCounts {
		errLines = append(errLines, fmt.Sprintf("<li>%s × %d</li>", htmlEscape(k), v))
	}
	errList := "None"
	if len(errLines) > 0 {
		errList = "<ul>" + strings.Join(errLines, "") + "</ul>"
	}

	// 按 netStatus 构造表格行（HTTP 状态码 / error 分组）
	var nsLines []string
	for k, v := range s.NetStatusCounts {
		nsLines = append(nsLines, fmt.Sprintf("<tr><td>%s</td><td>%d</td></tr>", htmlEscape(k), v))
	}
	netStatusRows := "<tr><td colspan=\"2\">None</td></tr>"
	if len(nsLines) > 0 {
		netStatusRows = strings.Join(nsLines, "")
	}

	title := s.Title
	if title == "" {
		title = "go-wrk Performance Report"
	}

	html := fmt.Sprintf(template,
		title,
		title,
		htmlEscape(s.TestURL),
		s.Concurrency,
		s.DurationSec,
		s.GeneratedAt.Format("2006-01-02 15:04:05"),
		s.OverallReqPerSec,
		s.OverallReqPerSecAll,
		s.TotalRequests,
		s.TotalRequestsAll,
		s.TotalErrors, errorRate,
		stripMs(s.AvgLatencyAll),
		stripMs(s.Fastest),
		stripMs(s.P10),
		stripMs(s.P50),
		stripMs(s.P75),
		stripMs(s.P99),
		stripMs(s.P999),
		stripMs(s.P99999),
		stripMs(s.Slowest),
		stripMs(s.StdDev),
		errList,
		netStatusRows,
		getBucketCount(s.LatencyBuckets, "100ms"),
		getBucketRate(s.LatencyBuckets, "100ms"),
		getBucketCount(s.LatencyBuckets, "200ms"),
		getBucketRate(s.LatencyBuckets, "200ms"),
		getBucketCount(s.LatencyBuckets, "300ms"),
		getBucketRate(s.LatencyBuckets, "300ms"),
		getBucketCount(s.LatencyBuckets, "500ms"),
		getBucketRate(s.LatencyBuckets, "500ms"),
		getBucketCount(s.LatencyBuckets, "700ms"),
		getBucketRate(s.LatencyBuckets, "700ms"),
		getBucketCount(s.LatencyBuckets, "1s"),
		getBucketRate(s.LatencyBuckets, "1s"),
		getBucketCount(s.LatencyBuckets, ">1s"),
		getBucketRate(s.LatencyBuckets, ">1s"),
		stripMs(s.Fastest), stripMs(s.Slowest), stripMs(s.AvgLatency), stripMs(s.StdDev),
		toMsNum(s.P10),
		toMsNum(s.P50),
		toMsNum(s.P75),
		toMsNum(s.P99),
		timeSeriesLabelsStr,
		timeSeriesDataStr,
	)

	return os.WriteFile(out, []byte(html), 0644)
}

// GenerateHTMLFromRawLog 保持原始 report.go 的用法：从 stdout 日志解析数据生成 HTML。
// 仍然方便你单独对一份 go-wrk 的运行结果做离线渲染。
func GenerateHTMLFromRawLog(raw, out string) error {
	file, err := os.Open(raw)
	if err != nil {
		return err
	}
	defer file.Close()

	var rps, avg, p50, p90, p99 string

	// 针对 go-wrk 实际输出格式：
	// Requests/sec:		64991.98
	// Avg Req Time:	30.772ms
	// 50%:			15.372ms
	// 90%:			xx.xms
	// 99%:			xx.xms
	// 注意：这里只匹配首行的 Requests/sec:，不会误匹配 Overall Requests/sec:
	reRps := regexp.MustCompile(`(?i)^\s*Requests/sec:\s*([\d.]+)`)
	reAvg := regexp.MustCompile(`(?i)Avg\s+Req\s+Time:\s*([\d.]+ms)`)
	reP := regexp.MustCompile(`^\s*(50|90|99)%:\s*([\d.]+ms)`)

	scanner := bufio.NewScanner(file)
	for scanner.Scan() {
		line := scanner.Text()

		if m := reRps.FindStringSubmatch(line); m != nil {
			rps = m[1]
		}
		if m := reAvg.FindStringSubmatch(line); m != nil {
			avg = m[1]
		}
		if m := reP.FindStringSubmatch(line); m != nil {
			switch m[1] {
			case "50":
				p50 = m[2]
			case "90":
				p90 = m[2]
			case "99":
				p99 = m[2]
			}
		}
	}

	// 这里为了兼容旧格式，仅用解析出的数据生成一个最简 Summary。
	s := Summary{
		Title:       "go-wrk Performance Report",
		ReqPerSec:   parseFloatOrZero(rps),
		AvgLatency:  parseMsDuration(avg),
		P50:         parseMsDuration(p50),
		P75:         parseMsDuration(p90), // 这里无法区分 P75，只能用 P90 填充
		P99:         parseMsDuration(p99),
		GeneratedAt: time.Now(),
	}
	return GenerateHTMLFromSummary(s, out)
}

func stripSuffix(s, suffix string) string {
	s = strings.TrimSpace(s)
	return strings.TrimSuffix(s, suffix)
}

func parseMsDuration(s string) time.Duration {
	if s == "" {
		return 0
	}
	v := stripSuffix(s, "ms")
	f := parseFloatOrZero(v)
	return time.Duration(f * float64(time.Millisecond))
}

func parseFloatOrZero(s string) float64 {
	if s == "" {
		return 0
	}
	f, err := strconv.ParseFloat(strings.TrimSpace(s), 64)
	if err != nil {
		return 0
	}
	return f
}

func htmlEscape(s string) string {
	replacer := strings.NewReplacer(
		"&", "&amp;",
		"<", "&lt;",
		">", "&gt;",
		`"`, "&quot;",
		"'", "&#39;",
	)
	return replacer.Replace(s)
}

func getBucketCount(buckets map[string]LatencyBucket, key string) int {
	if bucket, ok := buckets[key]; ok {
		return bucket.Count
	}
	return 0
}

func getBucketRate(buckets map[string]LatencyBucket, key string) float64 {
	if bucket, ok := buckets[key]; ok {
		return bucket.Rate
	}
	return 0.0
}
