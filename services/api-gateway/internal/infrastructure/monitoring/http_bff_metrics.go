package monitoring

import (
	"sort"
	"strconv"
	"strings"
	"sync"
	"time"
)

type HTTPBFFMetrics struct {
	mu     sync.Mutex
	routes map[string]*httpBFFRouteMetrics
}

type httpBFFRouteMetrics struct {
	count          int64
	errorCount     int64
	totalLatencyMS int64
	maxLatencyMS   int64
	statusCodes    map[string]int64
}

func NewHTTPBFFMetrics() *HTTPBFFMetrics {
	return &HTTPBFFMetrics{routes: make(map[string]*httpBFFRouteMetrics)}
}

func (metrics *HTTPBFFMetrics) RecordHTTPBFF(route string, method string, statusCode int, latency time.Duration) {
	if metrics == nil {
		return
	}
	route = sanitizeHTTPBFFLabel(route)
	if route == "" {
		route = "unknown"
	}
	method = sanitizeHTTPBFFLabel(method)
	if method == "" {
		method = "UNKNOWN"
	}
	code := strconv.Itoa(statusCode)
	key := method + " " + route
	latencyMS := latency.Milliseconds()
	if latencyMS < 0 {
		latencyMS = 0
	}

	metrics.mu.Lock()
	defer metrics.mu.Unlock()
	routeMetrics := metrics.routes[key]
	if routeMetrics == nil {
		routeMetrics = &httpBFFRouteMetrics{statusCodes: make(map[string]int64)}
		metrics.routes[key] = routeMetrics
	}
	routeMetrics.count++
	routeMetrics.statusCodes[code]++
	if statusCode >= 400 {
		routeMetrics.errorCount++
	}
	routeMetrics.totalLatencyMS += latencyMS
	if latencyMS > routeMetrics.maxLatencyMS {
		routeMetrics.maxLatencyMS = latencyMS
	}
}

func (metrics *HTTPBFFMetrics) Snapshot() HTTPBFFSnapshot {
	if metrics == nil {
		return HTTPBFFSnapshot{}
	}
	metrics.mu.Lock()
	defer metrics.mu.Unlock()

	snapshot := HTTPBFFSnapshot{Routes: make([]HTTPBFFRouteSnapshot, 0, len(metrics.routes))}
	for key, routeMetrics := range metrics.routes {
		method, route := splitHTTPBFFRouteKey(key)
		statusCodes := make(map[string]int64, len(routeMetrics.statusCodes))
		for code, count := range routeMetrics.statusCodes {
			statusCodes[code] = count
		}
		snapshot.TotalRequests += routeMetrics.count
		snapshot.TotalErrors += routeMetrics.errorCount
		snapshot.Routes = append(snapshot.Routes, HTTPBFFRouteSnapshot{
			Route:        route,
			Method:       method,
			Count:        routeMetrics.count,
			ErrorCount:   routeMetrics.errorCount,
			LatencyAvgMS: averageLatency(routeMetrics.totalLatencyMS, routeMetrics.count),
			LatencyMaxMS: routeMetrics.maxLatencyMS,
			StatusCodes:  statusCodes,
		})
	}
	sort.Slice(snapshot.Routes, func(i, j int) bool {
		if snapshot.Routes[i].Route == snapshot.Routes[j].Route {
			return snapshot.Routes[i].Method < snapshot.Routes[j].Method
		}
		return snapshot.Routes[i].Route < snapshot.Routes[j].Route
	})
	return snapshot
}

type HTTPBFFSnapshot struct {
	TotalRequests int64                  `json:"total_requests"`
	TotalErrors   int64                  `json:"total_errors"`
	Routes        []HTTPBFFRouteSnapshot `json:"routes"`
}

type HTTPBFFRouteSnapshot struct {
	Route        string           `json:"route"`
	Method       string           `json:"method"`
	Count        int64            `json:"count"`
	ErrorCount   int64            `json:"error_count"`
	LatencyAvgMS int64            `json:"latency_avg_ms"`
	LatencyMaxMS int64            `json:"latency_max_ms"`
	StatusCodes  map[string]int64 `json:"status_codes"`
}

func sanitizeHTTPBFFLabel(value string) string {
	value = strings.TrimSpace(value)
	if value == "" {
		return ""
	}
	if len(value) > 96 {
		value = value[:96]
	}
	for _, r := range value {
		if (r >= 'a' && r <= 'z') ||
			(r >= 'A' && r <= 'Z') ||
			(r >= '0' && r <= '9') ||
			r == '.' ||
			r == '_' ||
			r == '-' {
			continue
		}
		return ""
	}
	return value
}

func splitHTTPBFFRouteKey(key string) (string, string) {
	parts := strings.SplitN(key, " ", 2)
	if len(parts) != 2 {
		return "UNKNOWN", "unknown"
	}
	return parts[0], parts[1]
}
