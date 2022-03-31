// Copyright (c) Alex Ellis 2017. All rights reserved.
// Copyright 2020 OpenFaaS Author(s)
// Licensed under the MIT license. See LICENSE file in the project root for full license information.

package handlers

import (
	"encoding/json"
	"io/ioutil"
	"log"
	"net/http"
	"net/url"
	"time"

	"github.com/gorilla/mux"
	"github.com/openfaas/faas-provider/httputil"
	"github.com/openfaas/faas-provider/proxy"
	types "github.com/openfaas/faas-provider/types"
)

// MakeReplicaReader reads the amount of replicas for a deployment
//func MakeReplicaReader(defaultNamespace string, lister v1.DeploymentLister) http.HandlerFunc {
func MakeReplicaReader(config types.FaaSConfig, resolver proxy.BaseURLResolver) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {

		vars := mux.Vars(r)

		functionName := vars["name"]
		/*q := r.URL.Query()
		namespace := q.Get("namespace")

		lookupNamespace := defaultNamespace

		if len(namespace) > 0 {
			lookupNamespace = namespace
		}*/
		var function *types.FunctionStatus

		proxyClient := NewProxyClientFromConfig(config)

		tmpAddr, resolveErr := resolver.Resolve(functionName)
		if resolveErr != nil {
			// TODO: Should record the 404/not found error in Prometheus.
			log.Printf("resolver error: no endpoints for %s: %s\n", functionName, resolveErr.Error())
			httputil.Errorf(w, http.StatusServiceUnavailable, "No endpoints available for: %s.", functionName)
			return
		}
		addrStr := tmpAddr.String()
		addrStr += "/scale-reader"
		functionAddr, _ := url.Parse(addrStr)

		proxyReq, err := buildProxyRequest(r, *functionAddr)
		if err != nil {
			httputil.Errorf(w, http.StatusInternalServerError, "Failed to resolve service: %s.", functionName)
			return
		}
		if proxyReq.Body != nil {
			defer proxyReq.Body.Close()
		}
		ctx := r.Context()

		s := time.Now()
		response, err := proxyClient.Do(proxyReq.WithContext(ctx)) //send request to watchdog
		if err != nil {
			log.Printf("error with proxy request to: %s, %s\n", proxyReq.URL.String(), err.Error())

			httputil.Errorf(w, http.StatusInternalServerError, "Can't reach service for: %s.", functionName)
			return
		}
		if response.Body != nil {
			defer response.Body.Close()
			bytesIn, _ := ioutil.ReadAll(r.Body)
			marshalErr := json.Unmarshal(bytesIn, &function)
			if marshalErr != nil {
				w.WriteHeader(http.StatusBadRequest)
				msg := "Cannot parse watchdog read replica response. Please pass valid JSON."
				w.Write([]byte(msg))
				log.Println(msg, marshalErr)
				return
			}
		}
		function.Name = functionName

		/*function, err := getService(lookupNamespace, functionName, lister)
		if err != nil {
			log.Printf("Unable to fetch service: %s %s\n", functionName, namespace)
			w.WriteHeader(http.StatusInternalServerError)
			return
		}

		if function == nil {
			w.WriteHeader(http.StatusNotFound)
			return
		}*/
		d := time.Since(s)
		log.Printf("Replicas: %s, (%d/%d) %dms\n", functionName, function.AvailableReplicas, function.Replicas, d.Milliseconds())

		functionBytes, err := json.Marshal(function)
		if err != nil {
			log.Printf("Failed to marshal function: %s", err.Error())
			w.WriteHeader(http.StatusInternalServerError)
			w.Write([]byte("Failed to marshal function"))
			return
		}

		w.Header().Set("Content-Type", "application/json")
		w.WriteHeader(http.StatusOK)
		w.Write(functionBytes)
	}
}

// getService returns a function/service or nil if not found
/*func getService(functionNamespace string, functionName string, lister v1.DeploymentLister) (*types.FunctionStatus, error) {

	item, err := lister.Deployments(functionNamespace).
		Get(functionName)

	if err != nil {
		if errors.IsNotFound(err) {
			return nil, nil
		}

		return nil, err
	}

	if item != nil {
		function := k8s.AsFunctionStatus(*item)
		if function != nil {
			return function, nil
		}
	}

	return nil, fmt.Errorf("function: %s not found", functionName)
}*/
