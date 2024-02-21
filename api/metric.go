package api

import (
	"fmt"
	"github.com/mylxsw/asteria/log"
	"github.com/prometheus/client_golang/prometheus"
	"github.com/prometheus/client_golang/prometheus/promhttp"
	"net/http"
	"strings"
	"sync"
)

type PrometheusHandler struct {
	token string
}

func (h PrometheusHandler) ServeHTTP(writer http.ResponseWriter, request *http.Request) {
	authHeader := request.Header.Get("Authorization")
	if authHeader == "" || !strings.HasPrefix(authHeader, "Bearer ") {
		writer.WriteHeader(http.StatusUnauthorized)
		return
	}

	tokenStr := strings.TrimPrefix(authHeader, "Bearer ")
	if h.token != "" && tokenStr != h.token {
		writer.WriteHeader(http.StatusUnauthorized)
		return
	}

	promhttp.Handler().ServeHTTP(writer, request)
}

type HealthCheck struct{}

func (h HealthCheck) ServeHTTP(writer http.ResponseWriter, request *http.Request) {
	writer.Header().Add("Content-Type", "application/json")
	writer.WriteHeader(http.StatusOK)
	_, _ = writer.Write([]byte(`{"status": "UP"}`))
}

var counterVecs = make(map[string]*prometheus.CounterVec)
var lock sync.Mutex

func BuildCounterVec(namespace, name, help string, tags []string) *prometheus.CounterVec {
	lock.Lock()
	defer lock.Unlock()

	cacheKey := fmt.Sprintf("%s:%s:%s", namespace, name, help)
	if sv, ok := counterVecs[cacheKey]; ok {
		return sv
	}
	// prometheus metric
	counterVec := prometheus.NewCounterVec(prometheus.CounterOpts{
		Namespace: namespace,
		Name:      name,
		Help:      help,
	}, tags)

	if err := prometheus.Register(counterVec); err != nil {
		log.Errorf("register prometheus metric failed: %v", err)
	}

	counterVecs[cacheKey] = counterVec

	return counterVec
}
