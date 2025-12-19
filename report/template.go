package report

var template = `<!doctype html>
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
.raw-metrics {
	margin-top: 12px;
	font-size: 15px;
	color: #9ca3af;
	font-weight: 500;
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
		<div class="card-label">Total Requests (success)</div>
		<div class="card-value">%d</div>
		<div class="card-note">HTTP 200 only</div>
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
		<div class="card-label">Avg Latency (all)</div>
		<div class="card-value">%s</div>
		<div class="card-note">All requests (incl. errors/timeouts)</div>
	</div>
</div>

<div class="stat-note">
	<strong>Note:</strong> 
	<ul>
		<li><strong>Success requests</strong> = HTTP 200 status only</li>
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

<div class="panel" style="margin-bottom: 16px;">
	<h3>Net Status Breakdown</h3>
	<table>
		<tr><th>Net Status</th><th>Count</th></tr>
		%s
	</table>
</div>

<div class="panel" style="margin-bottom: 24px;">
	<h3>Latency Distribution by Time Range</h3>
	<table>
		<tr><th>Time Range</th><th>Count</th><th>Percentage</th></tr>
		<tr><td>&le; 100ms</td><td>%d</td><td>%.2f%%</td></tr>
		<tr><td>100ms - 200ms</td><td>%d</td><td>%.2f%%</td></tr>
		<tr><td>200ms - 300ms</td><td>%d</td><td>%.2f%%</td></tr>
		<tr><td>300ms - 500ms</td><td>%d</td><td>%.2f%%</td></tr>
		<tr><td>500ms - 700ms</td><td>%d</td><td>%.2f%%</td></tr>
		<tr><td>700ms - 1s</td><td>%d</td><td>%.2f%%</td></tr>
		<tr><td>&gt; 1s</td><td>%d</td><td>%.2f%%</td></tr>
	</table>
	<div class="card-note" style="margin-top: 8px;">Success requests only</div>
	<div class="raw-metrics">
		Raw metrics: fastest=%s, slowest=%s, avg=%s, stddev=%s.
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
</html>`
