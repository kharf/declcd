package prometheus

import (
	"github.com/kharf/cuepkgs/modules/k8s/k8s.io/api/core/v1"
	"encoding/json"
)

_goDashboard: {
	annotations: list: [{
		builtIn: 1
		datasource: {
			type: "datasource"
			uid:  "grafana"
		}
		enable:    true
		hide:      true
		iconColor: "rgba(0, 211, 255, 1)"
		name:      "Annotations & Alerts"
		type:      "dashboard"
	}]
	description:          "A quickstart to setup the Prometheus Go runtime exporter with preconfigured dashboards, alerting rules, and recording rules."
	editable:             true
	fiscalYearStartMonth: 0
	gnetId:               14061
	graphTooltip:         0
	links: []
	liveNow: false
	panels: [{
		aliasColors: {}
		bars:       false
		dashLength: 10
		dashes:     false
		datasource: {
			type: "prometheus"
			uid:  "prometheus"
		}
		description: "Average total bytes of memory reserved across all process instances of a job."
		fieldConfig: {
			defaults: links: []
			overrides: []
		}
		fill:         1
		fillGradient: 0
		gridPos: {
			h: 8
			w: 12
			x: 0
			y: 0
		}
		hiddenSeries: false
		id:           16
		legend: {
			avg:     false
			current: false
			max:     false
			min:     false
			show:    true
			total:   false
			values:  false
		}
		lines:     true
		linewidth: 1
		links: []
		nullPointMode: "null"
		options: alertThreshold: true
		percentage:    false
		pluginVersion: "10.2.3"
		pointradius:   2
		points:        false
		renderer:      "flot"
		seriesOverrides: []
		spaceLength: 10
		stack:       false
		steppedLine: false
		targets: [{
			datasource: {}
			editorMode:   "code"
			expr:         "avg by(job)(go_memstats_sys_bytes{job=\"$job\"})"
			interval:     ""
			legendFormat: "{{job}} (avg)"
			range:        true
			refId:        "A"
		}]
		thresholds: []
		timeRegions: []
		title: "Total Reserved Memory"
		tooltip: {
			shared:     true
			sort:       0
			value_type: "individual"
		}
		type: "graph"
		xaxis: {
			mode: "time"
			show: true
			values: []
		}
		yaxes: [{
			format:  "decbytes"
			logBase: 1
			show:    true
		}, {
			format:  "short"
			logBase: 1
			show:    true
		}]
		yaxis: align: false
	}, {
		aliasColors: {}
		bars:       false
		dashLength: 10
		dashes:     false
		datasource: {
			type: "prometheus"
			uid:  "prometheus"
		}
		description: "Average stack memory usage across all instances of a job."
		fieldConfig: {
			defaults: links: []
			overrides: []
		}
		fill:         1
		fillGradient: 0
		gridPos: {
			h: 8
			w: 12
			x: 12
			y: 0
		}
		hiddenSeries: false
		id:           24
		legend: {
			avg:     false
			current: false
			max:     false
			min:     false
			show:    true
			total:   false
			values:  false
		}
		lines:     true
		linewidth: 1
		links: []
		nullPointMode: "null"
		options: alertThreshold: true
		percentage:    false
		pluginVersion: "10.2.3"
		pointradius:   2
		points:        false
		renderer:      "flot"
		seriesOverrides: []
		spaceLength: 10
		stack:       false
		steppedLine: false
		targets: [{
			datasource: {}
			editorMode:   "code"
			expr:         "avg by (job) (go_memstats_stack_sys_bytes{job=\"$job\"})"
			interval:     ""
			legendFormat: "{{job}}: stack inuse (avg)"
			range:        true
			refId:        "A"
		}]
		thresholds: []
		timeRegions: []
		title: "Stack Memory Use"
		tooltip: {
			shared:     true
			sort:       0
			value_type: "individual"
		}
		type: "graph"
		xaxis: {
			mode: "time"
			show: true
			values: []
		}
		yaxes: [{
			format:  "decbytes"
			logBase: 1
			show:    true
		}, {
			format:  "short"
			logBase: 1
			show:    true
		}]
		yaxis: align: false
	}, {
		aliasColors: {}
		bars:       false
		dashLength: 10
		dashes:     false
		datasource: {
			type: "prometheus"
			uid:  "prometheus"
		}
		description: "Average memory reservations by the runtime, not for stack or heap, across all instances of a job."
		fieldConfig: {
			defaults: links: []
			overrides: []
		}
		fill:         1
		fillGradient: 0
		gridPos: {
			h: 8
			w: 12
			x: 0
			y: 8
		}
		hiddenSeries: false
		id:           26
		legend: {
			avg:     false
			current: false
			max:     false
			min:     false
			show:    true
			total:   false
			values:  false
		}
		lines:     true
		linewidth: 1
		links: []
		nullPointMode: "null"
		options: alertThreshold: true
		percentage:    false
		pluginVersion: "10.2.3"
		pointradius:   2
		points:        false
		renderer:      "flot"
		seriesOverrides: []
		spaceLength: 10
		stack:       false
		steppedLine: false
		targets: [{
			datasource: {}
			editorMode:   "code"
			expr:         "avg by (job)(go_memstats_mspan_sys_bytes{job=\"$job\"})"
			interval:     ""
			legendFormat: "{{instance}}: mspan (avg)"
			range:        true
			refId:        "B"
		}, {
			datasource: {}
			editorMode:   "code"
			expr:         "avg by (job)(go_memstats_mcache_sys_bytes{job=\"$job\"})"
			interval:     ""
			legendFormat: "{{instance}}: mcache (avg)"
			range:        true
			refId:        "D"
		}, {
			datasource: {}
			editorMode:   "code"
			expr:         "avg by (job)(go_memstats_buck_hash_sys_bytes{job=\"$job\"})"
			interval:     ""
			legendFormat: "{{instance}}: buck hash (avg)"
			range:        true
			refId:        "E"
		}, {
			datasource: {}
			editorMode:   "code"
			expr:         "avg by (job)(go_memstats_gc_sys_bytes{job=\"$job\"})"
			interval:     ""
			legendFormat: "{{job}}: gc (avg)"
			range:        true
			refId:        "F"
		}]
		thresholds: []
		timeRegions: []
		title: "Other Memory Reservations"
		tooltip: {
			shared:     true
			sort:       0
			value_type: "individual"
		}
		type: "graph"
		xaxis: {
			mode: "time"
			show: true
			values: []
		}
		yaxes: [{
			format:  "decbytes"
			logBase: 1
			show:    true
		}, {
			format:  "short"
			logBase: 1
			show:    false
		}]
		yaxis: align: false
	}, {
		aliasColors: {}
		bars:       false
		dashLength: 10
		dashes:     false
		datasource: {
			type: "prometheus"
			uid:  "prometheus"
		}
		description: "Average memory reserved, and actually in use, by the heap, across all instances of a job."
		fieldConfig: {
			defaults: links: []
			overrides: []
		}
		fill:         1
		fillGradient: 0
		gridPos: {
			h: 8
			w: 12
			x: 12
			y: 8
		}
		hiddenSeries: false
		id:           12
		legend: {
			avg:     false
			current: false
			max:     false
			min:     false
			show:    true
			total:   false
			values:  false
		}
		lines:     true
		linewidth: 1
		links: []
		nullPointMode: "null"
		options: alertThreshold: true
		percentage:    false
		pluginVersion: "10.2.3"
		pointradius:   2
		points:        false
		renderer:      "flot"
		seriesOverrides: []
		spaceLength: 10
		stack:       false
		steppedLine: false
		targets: [{
			datasource: {}
			editorMode:   "code"
			expr:         "avg by (job)(go_memstats_heap_sys_bytes{job=\"$job\"})"
			interval:     ""
			legendFormat: "{{job}}: heap reserved (avg)"
			range:        true
			refId:        "B"
		}, {
			datasource: {}
			editorMode:   "code"
			expr:         "avg by (job)(go_memstats_heap_inuse_bytes{job=\"$job\"})"
			interval:     ""
			legendFormat: "{{job}}: heap in use (avg)"
			range:        true
			refId:        "A"
		}, {
			datasource: {}
			editorMode:   "code"
			expr:         "avg by (job)(go_memstats_heap_alloc_bytes{job=\"$job\"})"
			interval:     ""
			legendFormat: "{{job}}: heap alloc (avg)"
			range:        true
			refId:        "C"
		}, {
			datasource: {}
			editorMode:   "code"
			expr:         "avg by (job)(go_memstats_heap_idle_bytes{job=\"$job\"})"
			interval:     ""
			legendFormat: "{{job}}: heap idle (avg)"
			range:        true
			refId:        "D"
		}, {
			datasource: {}
			editorMode:   "code"
			expr:         "avg by (job)(go_memstats_heap_released_bytes{job=\"$job\"})"
			interval:     ""
			legendFormat: "{{job}}: heap released (avg)"
			range:        true
			refId:        "E"
		}]
		thresholds: []
		timeRegions: []
		title: "Heap Memory"
		tooltip: {
			shared:     true
			sort:       0
			value_type: "individual"
		}
		type: "graph"
		xaxis: {
			mode: "time"
			show: true
			values: []
		}
		yaxes: [{
			format:  "decbytes"
			logBase: 1
			show:    true
		}, {
			format:  "short"
			logBase: 1
			show:    true
		}]
		yaxis: align: false
	}, {
		aliasColors: {}
		bars:       false
		dashLength: 10
		dashes:     false
		datasource: {
			type: "prometheus"
			uid:  "prometheus"
		}
		description: "Average allocation rate in bytes per second, across all instances of a job."
		fieldConfig: {
			defaults: links: []
			overrides: []
		}
		fill:         1
		fillGradient: 0
		gridPos: {
			h: 8
			w: 12
			x: 0
			y: 16
		}
		hiddenSeries: false
		id:           14
		legend: {
			avg:     false
			current: false
			max:     false
			min:     false
			show:    true
			total:   false
			values:  false
		}
		lines:     true
		linewidth: 1
		links: []
		nullPointMode: "null"
		options: alertThreshold: true
		percentage:    false
		pluginVersion: "10.2.3"
		pointradius:   1
		points:        true
		renderer:      "flot"
		seriesOverrides: []
		spaceLength: 10
		stack:       false
		steppedLine: false
		targets: [{
			datasource: {}
			editorMode:   "code"
			expr:         "avg by (job)(rate(go_memstats_alloc_bytes_total{job=\"$job\"}[$__rate_interval]))"
			interval:     ""
			legendFormat: "{{job}}: bytes malloced/s (avg)"
			range:        true
			refId:        "A"
		}]
		thresholds: []
		timeRegions: []
		title: "Allocation Rate, Bytes"
		tooltip: {
			shared:     true
			sort:       0
			value_type: "individual"
		}
		type: "graph"
		xaxis: {
			mode: "time"
			show: true
			values: []
		}
		yaxes: [{
			format:  "Bps"
			logBase: 1
			show:    true
		}, {
			format:  "short"
			logBase: 1
			show:    false
		}]
		yaxis: align: false
	}, {
		aliasColors: {}
		bars:       false
		dashLength: 10
		dashes:     false
		datasource: {
			type: "prometheus"
			uid:  "prometheus"
		}
		description: "Average rate of heap object allocation, across all instances of a job."
		fieldConfig: {
			defaults: links: []
			overrides: []
		}
		fill:         1
		fillGradient: 0
		gridPos: {
			h: 8
			w: 12
			x: 12
			y: 16
		}
		hiddenSeries: false
		id:           20
		legend: {
			avg:     false
			current: false
			max:     false
			min:     false
			show:    true
			total:   false
			values:  false
		}
		lines:     true
		linewidth: 1
		links: []
		nullPointMode: "null"
		options: alertThreshold: true
		percentage:    false
		pluginVersion: "10.2.3"
		pointradius:   2
		points:        false
		renderer:      "flot"
		seriesOverrides: []
		spaceLength: 10
		stack:       false
		steppedLine: false
		targets: [{
			datasource: {}
			editorMode:   "code"
			expr:         "rate(go_memstats_mallocs_total{job=\"$job\"}[$__rate_interval])"
			interval:     ""
			legendFormat: "{{job}}: obj mallocs/s (avg)"
			range:        true
			refId:        "A"
		}]
		thresholds: []
		timeRegions: []
		title: "Heap Object Allocation Rate"
		tooltip: {
			shared:     true
			sort:       0
			value_type: "individual"
		}
		type: "graph"
		xaxis: {
			mode: "time"
			show: true
			values: []
		}
		yaxes: [{
			format:  "short"
			logBase: 1
			show:    true
		}, {
			format:  "short"
			logBase: 1
			show:    true
		}]
		yaxis: align: false
	}, {
		aliasColors: {}
		bars:       false
		dashLength: 10
		dashes:     false
		datasource: {
			type: "prometheus"
			uid:  "prometheus"
		}
		description: "Average number of live memory objects across all instances of a job."
		fieldConfig: {
			defaults: links: []
			overrides: []
		}
		fill:         1
		fillGradient: 0
		gridPos: {
			h: 8
			w: 12
			x: 0
			y: 24
		}
		hiddenSeries: false
		id:           22
		legend: {
			alignAsTable: false
			avg:          false
			current:      false
			max:          false
			min:          false
			rightSide:    false
			show:         true
			total:        false
			values:       false
		}
		lines:     true
		linewidth: 1
		links: []
		nullPointMode: "null"
		options: alertThreshold: true
		percentage:    false
		pluginVersion: "10.2.3"
		pointradius:   2
		points:        false
		renderer:      "flot"
		seriesOverrides: []
		spaceLength: 10
		stack:       false
		steppedLine: false
		targets: [{
			datasource: {}
			editorMode:   "code"
			expr:         "avg by(job)(go_memstats_mallocs_total{job=\"$job\"} - go_memstats_frees_total{job=\"$job\"})"
			interval:     ""
			legendFormat: "{{job}}: object count (avg)"
			range:        true
			refId:        "A"
		}]
		thresholds: []
		timeRegions: []
		title: "Number of Live Objects"
		tooltip: {
			shared:     true
			sort:       0
			value_type: "individual"
		}
		type: "graph"
		xaxis: {
			mode: "time"
			show: true
			values: []
		}
		yaxes: [{
			format:  "short"
			logBase: 1
			show:    true
		}, {
			format:  "short"
			logBase: 1
			show:    false
		}]
		yaxis: align: false
	}, {
		aliasColors: {}
		bars:       false
		dashLength: 10
		dashes:     false
		datasource: {
			type: "prometheus"
			uid:  "prometheus"
		}
		description: "Average number of goroutines across instances of a job."
		fieldConfig: {
			defaults: links: []
			overrides: []
		}
		fill:         1
		fillGradient: 0
		gridPos: {
			h: 8
			w: 12
			x: 12
			y: 24
		}
		hiddenSeries: false
		id:           8
		legend: {
			avg:     false
			current: false
			max:     false
			min:     false
			show:    true
			total:   false
			values:  false
		}
		lines:     true
		linewidth: 1
		links: []
		nullPointMode: "null"
		options: alertThreshold: true
		percentage:    false
		pluginVersion: "10.2.3"
		pointradius:   2
		points:        false
		renderer:      "flot"
		seriesOverrides: []
		spaceLength: 10
		stack:       false
		steppedLine: false
		targets: [{
			datasource: {}
			editorMode:   "code"
			expr:         "avg by (job)(go_goroutines{job=\"$job\"})"
			interval:     ""
			legendFormat: "{{job}}: goroutine count (avg)"
			range:        true
			refId:        "A"
		}]
		thresholds: []
		timeRegions: []
		title: "Goroutines"
		tooltip: {
			shared:     true
			sort:       0
			value_type: "individual"
		}
		type: "graph"
		xaxis: {
			mode: "time"
			show: true
			values: []
		}
		yaxes: [{
			decimals: 0
			format:   "short"
			logBase:  1
			show:     true
		}, {
			format:  "short"
			logBase: 1
			show:    true
		}]
		yaxis: align: false
	}, {
		aliasColors: {}
		bars:       false
		dashLength: 10
		dashes:     false
		datasource: {
			type: "prometheus"
			uid:  "prometheus"
		}
		fieldConfig: {
			defaults: links: []
			overrides: []
		}
		fill:         1
		fillGradient: 0
		gridPos: {
			h: 8
			w: 12
			x: 0
			y: 32
		}
		hiddenSeries: false
		id:           4
		legend: {
			alignAsTable: false
			avg:          false
			current:      false
			max:          false
			min:          false
			show:         true
			total:        false
			values:       false
		}
		lines:     true
		linewidth: 1
		links: []
		nullPointMode: "null"
		options: alertThreshold: true
		percentage:    false
		pluginVersion: "10.2.3"
		pointradius:   2
		points:        false
		renderer:      "flot"
		seriesOverrides: []
		spaceLength: 10
		stack:       false
		steppedLine: false
		targets: [{
			datasource: {}
			editorMode:   "code"
			expr:         "avg by (job)(go_gc_duration_seconds{quantile=\"0\", job=\"$job\"})"
			interval:     ""
			legendFormat: "{{job}}: min gc time (avg)"
			range:        true
			refId:        "A"
		}, {
			datasource: {}
			editorMode:   "code"
			expr:         "avg by (job)(go_gc_duration_seconds{quantile=\"1\", job=\"$job\"})"
			interval:     ""
			legendFormat: "{{job}}: max gc time (avg)"
			range:        true
			refId:        "B"
		}]
		thresholds: []
		timeRegions: []
		title: "GC min & max duration"
		tooltip: {
			shared:     true
			sort:       0
			value_type: "individual"
		}
		type: "graph"
		xaxis: {
			mode: "time"
			show: true
			values: []
		}
		yaxes: [{
			format:  "ms"
			logBase: 1
			show:    true
		}, {
			format:  "short"
			logBase: 1
			show:    true
		}]
		yaxis: align: false
	}, {
		aliasColors: {}
		bars:       false
		dashLength: 10
		dashes:     false
		datasource: {
			type: "prometheus"
			uid:  "prometheus"
		}
		description: "The number used bytes at which the runtime plans to perform the next GC, averaged across all instances of a job."
		fieldConfig: {
			defaults: links: []
			overrides: []
		}
		fill:         1
		fillGradient: 0
		gridPos: {
			h: 8
			w: 12
			x: 12
			y: 32
		}
		hiddenSeries: false
		id:           27
		legend: {
			avg:     false
			current: false
			max:     false
			min:     false
			show:    true
			total:   false
			values:  false
		}
		lines:     true
		linewidth: 1
		links: []
		nullPointMode: "null"
		options: alertThreshold: true
		percentage:    false
		pluginVersion: "10.2.3"
		pointradius:   2
		points:        false
		renderer:      "flot"
		seriesOverrides: []
		spaceLength: 10
		stack:       false
		steppedLine: false
		targets: [{
			datasource: {}
			editorMode:   "code"
			expr:         "avg by (job)(go_memstats_next_gc_bytes{job=\"$job\"})"
			interval:     ""
			legendFormat: "{{job}} next gc bytes (avg)"
			range:        true
			refId:        "A"
		}]
		thresholds: []
		timeRegions: []
		title: "Next GC, Bytes"
		tooltip: {
			shared:     true
			sort:       0
			value_type: "individual"
		}
		type: "graph"
		xaxis: {
			mode: "time"
			show: true
			values: []
		}
		yaxes: [{
			format:  "decbytes"
			logBase: 1
			show:    true
		}, {
			format:  "s"
			logBase: 1
			show:    true
		}]
		yaxis: align: false
	}]
	refresh:       "10s"
	schemaVersion: 39
	tags: [
		"go",
		"golang",
	]
	templating: list: [{
		current: {
			selected: false
			text:     "Prometheus"
			value:    "prometheus"
		}
		hide:       0
		includeAll: false
		multi:      false
		name:       "datasource"
		options: []
		query:       "prometheus"
		queryValue:  ""
		refresh:     1
		regex:       ""
		skipUrlSync: false
		type:        "datasource"
	}, {
		current: {
			selected: false
			text:     "gitops-controller"
			value:    "gitops-controller"
		}
		datasource: uid: ""
		definition: "label_values(go_info, job)"
		hide:       0
		includeAll: false
		label:      "job"
		multi:      false
		name:       "job"
		options: []
		query:          "label_values(go_info, job)"
		refresh:        2
		regex:          ""
		skipUrlSync:    false
		sort:           0
		tagValuesQuery: ""
		tagsQuery:      ""
		type:           "query"
		useTags:        false
	}]
	time: {
		from: "now-5m"
		to:   "now"
	}
	timepicker: {
		refresh_intervals: ["10s", "30s", "1m", "5m", "15m", "30m", "1h", "2h", "1d"]
		time_options: ["5m", "15m", "1h", "6h", "12h", "24h", "2d", "7d", "30d"]
	}
	timezone:  ""
	title:     "Go Runtime"
	uid:       "CgCw8jKZz3"
	version:   1
	weekStart: ""
}

_goDashboardConfigMap: v1.#ConfigMap & {
	apiVersion: "v1"
	kind:       "ConfigMap"
	metadata: {
		name:      "go-metrics-dashboard"
		namespace: #namespace.metadata.name
		labels: "grafana_dashboard": "1"
	}
	data: {
		"go-metrics-dashboard.json": json.Marshal(_goDashboard)
	}
}
