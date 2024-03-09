package prometheus

import (
	"github.com/kharf/cuepkgs/modules/k8s/k8s.io/api/core/v1"
	"encoding/json"
)

_declcdDashboard: {
	annotations: list: [{
		builtIn: 1
		datasource: {
			type: "grafana"
			uid:  "-- Grafana --"
		}
		enable:    true
		hide:      true
		iconColor: "rgba(0, 211, 255, 1)"
		name:      "Annotations & Alerts"
		type:      "dashboard"
	}]
	editable:             true
	fiscalYearStartMonth: 0
	graphTooltip:         0
	id:                   29
	links: []
	liveNow: false
	panels: [{
		datasource: {
			type: "prometheus"
			uid:  "prometheus"
		}
		description: ""
		fieldConfig: {
			defaults: {
				color: mode: "continuous-BlPu"
				custom: {
					axisBorderShow:   false
					axisCenteredZero: false
					axisColorMode:    "text"
					axisGridShow:     false
					axisLabel:        ""
					axisPlacement:    "left"
					barAlignment:     0
					drawStyle:        "line"
					fillOpacity:      100
					gradientMode:     "opacity"
					hideFrom: {
						legend:  false
						tooltip: false
						viz:     false
					}
					insertNulls:       false
					lineInterpolation: "linear"
					lineStyle: fill: "solid"
					lineWidth: 5
					pointSize: 5
					scaleDistribution: type: "linear"
					showPoints: "auto"
					spanNulls:  true
					stacking: {
						group: "A"
						mode:  "none"
					}
					thresholdsStyle: mode: "off"
				}
				mappings: []
				thresholds: {
					mode: "absolute"
					steps: [{
						color: "green"
						value: null
					}, {
						color: "red"
						value: 80
					}]
				}
				unit: "s"
			}
			overrides: []
		}
		gridPos: {
			h: 8
			w: 12
			x: 0
			y: 0
		}
		id: 1
		options: {
			legend: {
				calcs: []
				displayMode: "list"
				placement:   "bottom"
				showLegend:  true
			}
			tooltip: {
				mode: "single"
				sort: "none"
			}
		}
		targets: [{
			datasource: {
				type: "prometheus"
				uid:  "prometheus"
			}
			disableTextWrap:     false
			editorMode:          "code"
			exemplar:            false
			expr:                "rate(declcd_reconciliation_duration_seconds_sum[$__rate_interval]) / rate(declcd_reconciliation_duration_seconds_count[$__rate_interval])"
			fullMetaSearch:      false
			includeNullMetadata: false
			instant:             false
			legendFormat:        "{{project}} - {{url}}"
			range:               true
			refId:               "A"
			useBackend:          false
		}]
		title:       "Reconciliations"
		transparent: true
		type:        "timeseries"
	}]
	refresh:       ""
	schemaVersion: 39
	tags: []
	templating: list: []
	time: {
		from: "now-5m"
		to:   "now"
	}
	timepicker: {}
	timezone:  ""
	title:     "Declcd"
	uid:       "cef24a07-a94e-4d1f-9833-089263e23d37"
	version:   2
	weekStart: ""
}

_declcdDashboardConfigMap: v1.#ConfigMap & {
	apiVersion: "v1"
	kind:       "ConfigMap"
	metadata: {
		name:      "declcd-dashboard"
		namespace: #namespace.metadata.name
		labels: "grafana_dashboard": "1"
	}
	data: {
		"declcd-dashboard.json": json.Marshal(_declcdDashboard)
	}
}
