package handlers

import (
	"encoding/json"
	"io/ioutil"
	"log"
	"net"
	"net/http"
	"net/url"
	"time"

	"github.com/openfaas/faas-provider/proxy"
	"github.com/openfaas/faas-provider/types"
)

const (
	watchdogPort       = "8080"
	defaultContentType = "text/plain"
)

type replicaFuncStatus struct {

	// Name is the name of the function deployment
	Name string `json:"name,omitempty"`

	// Image is a fully-qualified container image
	Image string `json:"image,omitempty"`

	// Namespace for the function, if supported by the faas-provider
	Namespace string `json:"namespace,omitempty"`

	// EnvProcess overrides the fprocess environment variable and can be used
	// with the watchdog
	EnvProcess string `json:"envProcess,omitempty"`

	// EnvVars set environment variables for the function runtime
	EnvVars map[string]string `json:"envVars,omitempty"`

	// InvocationCount count of invocations
	InvocationCount float64 `json:"invocationCount,omitempty"`

	// Replicas desired within the cluster
	Replicas uint64 `json:"replicas,omitempty"`

	// AvailableReplicas is the count of replicas ready to receive
	// invocations as reported by the faas-provider
	AvailableReplicas uint64 `json:"availableReplicas,omitempty"`
}

func NewProxyClientFromConfig(config types.FaaSConfig) *http.Client {
	return NewProxyClient(config.GetReadTimeout(), config.GetMaxIdleConns(), config.GetMaxIdleConnsPerHost())
}
func NewProxyClient(timeout time.Duration, maxIdleConns int, maxIdleConnsPerHost int) *http.Client {
	return &http.Client{
		// these Transport values ensure that the http Client will eventually timeout and prevents
		// infinite retries. The default http.Client configure these timeouts.  The specific
		// values tuned via performance testing/benchmarking
		//
		// Additional context can be found at
		// - https://medium.com/@nate510/don-t-use-go-s-default-http-client-4804cb19f779
		// - https://blog.cloudflare.com/the-complete-guide-to-golang-net-http-timeouts/
		//
		// Additionally, these overrides for the default client enable re-use of connections and prevent
		// CoreDNS from rate limiting under high traffic
		//
		// See also two similar projects where this value was updated:
		// https://github.com/prometheus/prometheus/pull/3592
		// https://github.com/minio/minio/pull/5860
		Transport: &http.Transport{
			Proxy: http.ProxyFromEnvironment,
			DialContext: (&net.Dialer{
				Timeout:   timeout,
				KeepAlive: 1 * time.Second,
				DualStack: true,
			}).DialContext,
			MaxIdleConns:          maxIdleConns,
			MaxIdleConnsPerHost:   maxIdleConnsPerHost,
			IdleConnTimeout:       120 * time.Millisecond,
			TLSHandshakeTimeout:   10 * time.Second,
			ExpectContinueTimeout: 1500 * time.Millisecond,
		},
		Timeout: timeout,
		CheckRedirect: func(req *http.Request, via []*http.Request) error {
			return http.ErrUseLastResponse
		},
	}
}
func buildProxyRequest(originalReq *http.Request, baseURL url.URL) (*http.Request, error) {

	host := baseURL.Host
	if baseURL.Port() == "" {
		host = baseURL.Host + ":" + watchdogPort
	}

	url := url.URL{
		Scheme:   baseURL.Scheme,
		Host:     host,
		RawQuery: originalReq.URL.RawQuery,
		Path:     baseURL.Path,
	}
	//log.Println("this is buildProxy url: ", url)

	upstreamReq, err := http.NewRequest(originalReq.Method, url.String(), nil)
	if err != nil {
		return nil, err
	}
	copyHeaders(upstreamReq.Header, &originalReq.Header)

	if len(originalReq.Host) > 0 && upstreamReq.Header.Get("X-Forwarded-Host") == "" {
		upstreamReq.Header["X-Forwarded-Host"] = []string{originalReq.Host}
	}
	if upstreamReq.Header.Get("X-Forwarded-For") == "" {
		upstreamReq.Header["X-Forwarded-For"] = []string{originalReq.RemoteAddr}
	}

	if originalReq.Body != nil {
		upstreamReq.Body = originalReq.Body
	}

	return upstreamReq, nil
}

// copyHeaders clones the header values from the source into the destination.
func copyHeaders(destination http.Header, source *http.Header) {
	for k, v := range *source {
		vClone := make([]string, len(v))
		copy(vClone, v)
		destination[k] = vClone
	}
}

func updateReplica(functionName string, config types.FaaSConfig, resolver proxy.BaseURLResolver, r *http.Request) (*replicaFuncStatus, error) {
	var function *replicaFuncStatus
	proxyClient := NewProxyClientFromConfig(config)
	tmpAddr, resolveErr := resolver.Resolve(functionName)
	if resolveErr != nil {
		// TODO: Should record the 404/not found error in Prometheus.
		log.Printf("resolver error: no endpoints for %s: %s\n", functionName, resolveErr.Error())
		return nil, resolveErr
	}
	addrStr := tmpAddr.String()
	addrStr += "/scale-reader"
	functionAddr, _ := url.Parse(addrStr)
	proxyReq, err := buildProxyRequest(r, *functionAddr)
	if err != nil {
		return nil, err
	}
	if proxyReq.Body != nil {
		defer proxyReq.Body.Close()
	}
	ctx := r.Context()
	s := time.Now()
	response, err := proxyClient.Do(proxyReq.WithContext(ctx)) //send request to watchdog
	if err != nil {
		log.Printf("error with proxy request to: %s, %s\n", proxyReq.URL.String(), err.Error())
		return nil, err
	}
	if response.Body != nil {
		defer response.Body.Close()
		bytesIn, _ := ioutil.ReadAll(response.Body)
		marshalErr := json.Unmarshal(bytesIn, &function)
		if marshalErr != nil {
			msg := "Cannot parse watchdog read replica response. Please pass valid JSON."
			log.Println(msg, marshalErr)
			return nil, marshalErr
		}
	}
	function.Name = functionName
	d := time.Since(s)
	log.Printf("Replicas: %s, (%d/%d) %dms\n", functionName, function.AvailableReplicas, function.Replicas, d.Milliseconds())
	return function, nil
}
