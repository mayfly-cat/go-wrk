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

	TotalRequests    int // 成功请求数（2XX/301/307）
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

	// TimeSeries 记录每个时间段的请求数量，key 是时间段索引（从0开始，每1秒一个时间段）
	TimeSeries map[int]int

	GeneratedAt time.Time
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

	title := s.Title
	if title == "" {
		title = "go-wrk Performance Report"
	}

	html := fmt.Sprintf(`<!doctype html>
<html>
<head>
<meta charset="utf-8">
<title>%s</title>
<script src="https://cdn.jsdelivr.net/npm/chart.js"></script>
<style>
body {
	font-family: -apple-system, BlinkMacSystemFont, "Segoe UI", Arial, sans-serif;
	padding: 24px;
	background: #0f172a;
	color: #e5e7eb;
	max-width: 1200px;
	margin: 0 auto;
}
h1 {
	margin-bottom: 4px;
	font-size: 28px;
}
.subtitle {
	color: #9ca3af;
	margin-bottom: 20px;
	font-size: 18px;
}
.meta-row {
	font-size: 14px;
	color: #9ca3af;
	margin-bottom: 18px;
}
.meta-row span {
	margin-right: 16px;
}
.grid {
	display: grid;
	grid-template-columns: repeat(auto-fit, minmax(180px, 1fr));
	gap: 16px;
	margin-bottom: 24px;
}
.card {
	background: radial-gradient(circle at top left, #1d4ed8 0, #020617 55%%, #020617 100%%);
	border-radius: 14px;
	padding: 16px 18px;
	box-shadow: 0 18px 45px rgba(15, 23, 42, 0.9);
	border: 1px solid rgba(148, 163, 184, 0.2);
}
.card-label {
	font-size: 12px;
	letter-spacing: 0.08em;
	text-transform: uppercase;
	color: #9ca3af;
	margin-bottom: 6px;
}
.card-value {
	font-size: 22px;
	font-weight: 600;
}
.card-unit {
	font-size: 13px;
	color: #9ca3af;
	margin-left: 4px;
}
.card-note {
	font-size: 10px;
	color: #6b7280;
	margin-top: 4px;
	font-style: italic;
}
.stat-note {
	background: rgba(31, 41, 55, 0.6);
	border-left: 3px solid rgba(59, 130, 246, 0.8);
	border-radius: 8px;
	padding: 12px 16px;
	margin-bottom: 24px;
	font-size: 14px;
	color: #9ca3af;
}
.stat-note strong {
	color: #e5e7eb;
}
.stat-note ul {
	margin: 8px 0 0 20px;
	padding: 0;
}
.stat-note li {
	margin-bottom: 4px;
}
.chart-card {
	background: #020617;
	border-radius: 16px;
	padding: 20px;
	border: 1px solid rgba(148, 163, 184, 0.35);
	box-shadow: 0 24px 60px rgba(15, 23, 42, 0.95);
}
.chart-card-compact {
	/* 用在直方图上，放在三列布局中，避免过宽 */
}
.chart-card-wide {
	/* 用在折线图上，尽量铺满版心宽度 */
	max-width: 100%%;
	margin: 0 auto 18px;
}
.metrics-row {
	display: grid;
	grid-template-columns: 2fr 1fr 1fr;
	gap: 16px;
	align-items: stretch;
	margin-bottom: 24px;
}
.chart-header {
	display: flex;
	justify-content: space-between;
	align-items: baseline;
	margin-bottom: 12px;
}
.chart-title {
	font-size: 16px;
	font-weight: 500;
}
.chart-subtitle {
	font-size: 12px;
	color: #9ca3af;
}
.badge {
	font-size: 11px;
	text-transform: uppercase;
	letter-spacing: 0.08em;
	padding: 4px 9px;
	border-radius: 999px;
	background: rgba(52, 211, 153, 0.1);
	color: #6ee7b7;
	border: 1px solid rgba(16, 185, 129, 0.4);
}
.meta {
	margin-top: 10px;
	font-size: 13px;
	color: #6b7280;
}
.two-col {
	display: grid;
	grid-template-columns: 2fr 1fr;
	gap: 16px;
}
.panel {
	background: #020617;
	border-radius: 14px;
	padding: 14px 16px;
	border: 1px solid rgba(148, 163, 184, 0.3);
}
.panel h3 {
	margin: 0 0 8px 0;
	font-size: 14px;
}
.panel table {
	width: 100%%;
	border-collapse: collapse;
	font-size: 13px;
}
.panel th, .panel td {
	padding: 4px 6px;
	text-align: left;
}
.panel th {
	color: #9ca3af;
	font-weight: 500;
	border-bottom: 1px solid rgba(55, 65, 81, 0.7);
}
.panel td {
	border-bottom: 1px dashed rgba(31, 41, 55, 0.6);
}
.panel ul {
	margin: 4px 0 0 18px;
	padding: 0;
}
.panel li {
	margin-bottom: 2px;
}
.footer {
	margin-top: 18px;
	font-size: 13px;
	color: #6b7280;
}
</style>
</head>
<body>

<h1>%s</h1>
<div class="subtitle">Generated from go-wrk benchmark result</div>
<div class="meta-row">
	<span>Target: <strong>%s</strong></span>
	<span>Concurrency: <strong>%d</strong></span>
	<span>Duration: <strong>%ds</strong></span>
	<span>Generated at: %s</span>
</div>

<div class="grid">
	<div class="card">
		<div class="card-label">RPS (overall, success)</div>
		<div class="card-value">%.2f<span class="card-unit">req/s</span></div>
		<div class="card-note">Success requests only</div>
	</div>
	<div class="card">
		<div class="card-label">RPS (overall, all)</div>
		<div class="card-value">%.2f<span class="card-unit">req/s</span></div>
		<div class="card-note">All requests (incl. errors)</div>
	</div>
	<div class="card">
		<div class="card-label">RPS (thread avg)</div>
		<div class="card-value">%.2f<span class="card-unit">req/s</span></div>
		<div class="card-note">Success requests only</div>
	</div>
	<div class="card">
		<div class="card-label">Total Requests (success)</div>
		<div class="card-value">%d</div>
		<div class="card-note">2XX/301/307 only</div>
	</div>
	<div class="card">
		<div class="card-label">Total Requests (all)</div>
		<div class="card-value">%d</div>
		<div class="card-note">Including errors</div>
	</div>
	<div class="card">
		<div class="card-label">Errors / Error Rate</div>
		<div class="card-value">%d<span class="card-unit">(%.2f%%)</span></div>
	</div>
	<div class="card">
		<div class="card-label">Slow Requests (>1s)</div>
		<div class="card-value">%d<span class="card-unit">(%.2f%%)</span></div>
		<div class="card-note">Of success requests</div>
	</div>
	<div class="card">
		<div class="card-label">Avg Latency</div>
		<div class="card-value">%s</div>
		<div class="card-note">Success requests only</div>
	</div>
	<div class="card">
		<div class="card-label">Avg Latency (all)</div>
		<div class="card-value">%s</div>
		<div class="card-note">All requests (incl. errors/timeouts)</div>
	</div>
	<div class="card">
		<div class="card-label">Throughput (overall)</div>
		<div class="card-value">%.2f<span class="card-unit">bytes/s</span></div>
	</div>
</div>

<div class="stat-note">
	<strong>Note:</strong> 
	<ul>
		<li><strong>Success requests</strong> = HTTP 2XX status codes and 301/307 redirects</li>
		<li><strong>All requests</strong> = Success requests + errors (4XX, 5XX, timeouts, etc.)</li>
		<li><strong>Latency metrics</strong> are calculated from success requests only</li>
		<li>If your service monitoring shows different RPS, check if it counts all requests or only success requests</li>
	</ul>
</div>

<div class="metrics-row">
<div class="chart-card chart-card-compact">
	<div class="chart-header">
		<div>
			<div class="chart-title">Latency Distribution</div>
			<div class="chart-subtitle">Percentile latency in milliseconds</div>
		</div>
		<div class="badge">P10 / P50 / P75 / P99</div>
	</div>
	<canvas id="latency" height="220"></canvas>
	<div class="meta">Tip: hover over the bars to see exact values.</div>
</div>

<div class="panel">
	<h3>Latency Percentiles</h3>
	<table>
		<tr><th>Percentile</th><th>Latency</th></tr>
		<tr><td>Min</td><td>%s</td></tr>
		<tr><td>P10</td><td>%s</td></tr>
		<tr><td>P50</td><td>%s</td></tr>
		<tr><td>P75</td><td>%s</td></tr>
		<tr><td>P99</td><td>%s</td></tr>
		<tr><td>P99.9</td><td>%s</td></tr>
		<tr><td>P99.99</td><td>%s</td></tr>
		<tr><td>Max</td><td>%s</td></tr>
		<tr><td>StdDev</td><td>%s</td></tr>
	</table>
</div>
<div class="panel">
	<h3>Error Breakdown</h3>
	%s
</div>
</div>

<div class="chart-card chart-card-wide">
	<div class="chart-header">
		<div>
			<div class="chart-title">Requests Over Time</div>
			<div class="chart-subtitle">Requests/sec (5s moving window)</div>
		</div>
		<div class="badge">Time Series</div>
	</div>
	<canvas id="timeSeries" height="260"></canvas>
	<div class="meta">Tip: hover over the line to see exact request counts at each time point.</div>
</div>

<div class="footer">
Raw metrics: fastest=%s, slowest=%s, avg=%s, stddev=%s.
</div>

<script>
const ctx = document.getElementById('latency').getContext('2d');
const chart = new Chart(ctx, {
	type: 'bar',
	data: {
		labels: ['P10', 'P50', 'P75', 'P99'],
		datasets: [{
			label: 'Latency (ms)',
			data: [
				parseFloat('%s'),
				parseFloat('%s'),
				parseFloat('%s'),
				parseFloat('%s')
			],
			backgroundColor: [
				'rgba(59, 130, 246, 0.9)',
				'rgba(96, 165, 250, 0.9)',
				'rgba(147, 197, 253, 0.95)',
				'rgba(202, 227, 255, 0.95)'
			],
			borderColor: [
				'rgba(191, 219, 254, 1)',
				'rgba(191, 219, 254, 1)',
				'rgba(191, 219, 254, 1)',
				'rgb(225, 238, 253)'
			],
			borderWidth: 1.4,
			borderRadius: 8,
			hoverBackgroundColor: 'rgba(248, 250, 252, 0.95)',
			barPercentage: 0.6,
			categoryPercentage: 0.6
		}]
	},
	options: {
		plugins: {
			legend: {
				labels: {
					color: '#9ca3af',
					font: { size: 11 }
				}
			},
			tooltip: {
				callbacks: {
					label: function(context) {
						return context.parsed.y.toFixed(3) + ' ms';
					}
				}
			}
		},
		scales: {
			x: {
				grid: { display: false },
				ticks: { color: '#9ca3af' }
			},
			y: {
				beginAtZero: true,
				grid: { color: 'rgba(31, 41, 55, 0.7)' },
				ticks: { color: '#6b7280' }
			}
		}
	}
});

const ctxTimeSeries = document.getElementById('timeSeries').getContext('2d');
const timeSeriesChart = new Chart(ctxTimeSeries, {
	type: 'line',
	data: {
		labels: %s,
		datasets: [{
			label: 'Requests per second (5s avg)',
			data: %s,
			borderColor: 'rgba(59, 130, 246, 0.9)',
			backgroundColor: 'rgba(59, 130, 246, 0.1)',
			borderWidth: 2,
			fill: true,
			tension: 0.4,
			pointRadius: 3,
			pointHoverRadius: 5,
			pointBackgroundColor: 'rgba(59, 130, 246, 0.9)',
			pointBorderColor: '#fff',
			pointHoverBackgroundColor: '#fff',
			pointHoverBorderColor: 'rgba(59, 130, 246, 0.9)'
		}]
	},
	options: {
		plugins: {
			legend: {
				labels: {
					color: '#9ca3af',
					font: { size: 11 }
				}
			},
			tooltip: {
				callbacks: {
					label: function(context) {
						return context.parsed.y.toFixed(2) + ' req/s';
					}
				}
			}
		},
		scales: {
			x: {
				title: {
					display: true,
					text: 'Time (seconds, 5s step)',
					color: '#9ca3af',
					font: { size: 12 }
				},
				grid: { color: 'rgba(31, 41, 55, 0.7)' },
				ticks: { color: '#9ca3af' }
			},
			y: {
				title: {
					display: true,
					text: 'Requests per second',
					color: '#9ca3af',
					font: { size: 12 }
				},
				beginAtZero: true,
				grid: { color: 'rgba(31, 41, 55, 0.7)' },
				ticks: { color: '#6b7280' }
			}
		}
	}
});
</script>

</body>
</html>`,
		title,
		title,
		htmlEscape(s.TestURL),
		s.Concurrency,
		s.DurationSec,
		s.GeneratedAt.Format("2006-01-02 15:04:05"),
		s.OverallReqPerSec,
		s.OverallReqPerSecAll,
		s.ReqPerSec,
		s.TotalRequests,
		s.TotalRequestsAll,
		s.TotalErrors, errorRate,
		s.SlowRequestsCount, s.SlowRequestsRate,
		stripMs(s.AvgLatency),
		stripMs(s.AvgLatencyAll),
		s.OverallBytesPerSec,
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
