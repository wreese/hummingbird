//  Copyright (c) 2015 Rackspace
//
//  Licensed under the Apache License, Version 2.0 (the "License");
//  you may not use this file except in compliance with the License.
//  You may obtain a copy of the License at
//
//     http://www.apache.org/licenses/LICENSE-2.0
//
//  Unless required by applicable law or agreed to in writing, software
//  distributed under the License is distributed on an "AS IS" BASIS,
//  WITHOUT WARRANTIES OR CONDITIONS OF ANY KIND, either express or
//  implied.
//  See the License for the specific language governing permissions and
//  limitations under the License.

package proxyserver

import (
	"flag"
	"fmt"
	"net/http"

	"github.com/troubling/hummingbird/client"
	"github.com/troubling/hummingbird/common/conf"
	"github.com/troubling/hummingbird/common/ring"
	"github.com/troubling/hummingbird/common/srv"
	"github.com/troubling/hummingbird/proxyserver/middleware"

	"github.com/justinas/alice"
)

type ProxyServer struct {
	C      client.ProxyClient
	logger srv.LowLevelLogger
	mc     ring.MemcacheRing
}

func (server *ProxyServer) Finalize() {
}

func (server *ProxyServer) HealthcheckHandler(writer http.ResponseWriter, request *http.Request) {
	writer.Header().Set("Content-Length", "2")
	writer.WriteHeader(http.StatusOK)
	writer.Write([]byte("OK"))
	return
}

func (server *ProxyServer) GetHandler(config conf.Config) http.Handler {
	router := srv.NewRouter()
	router.Get("/healthcheck", http.HandlerFunc(server.HealthcheckHandler))

	router.Get("/v1/:account/:container/*obj", http.HandlerFunc(server.ObjectGetHandler))
	router.Head("/v1/:account/:container/*obj", http.HandlerFunc(server.ObjectHeadHandler))
	router.Put("/v1/:account/:container/*obj", http.HandlerFunc(server.ObjectPutHandler))
	router.Delete("/v1/:account/:container/*obj", http.HandlerFunc(server.ObjectDeleteHandler))

	router.Get("/v1/:account/:container", http.HandlerFunc(server.ContainerGetHandler))
	router.Get("/v1/:account/:container/", http.HandlerFunc(server.ContainerGetHandler))
	router.Head("/v1/:account/:container", http.HandlerFunc(server.ContainerHeadHandler))
	router.Head("/v1/:account/:container/", http.HandlerFunc(server.ContainerHeadHandler))
	router.Put("/v1/:account/:container", http.HandlerFunc(server.ContainerPutHandler))
	router.Put("/v1/:account/:container/", http.HandlerFunc(server.ContainerPutHandler))
	router.Delete("/v1/:account/:container", http.HandlerFunc(server.ContainerDeleteHandler))
	router.Delete("/v1/:account/:container/", http.HandlerFunc(server.ContainerDeleteHandler))
	router.Post("/v1/:account/:container", http.HandlerFunc(server.ContainerPutHandler))
	router.Post("/v1/:account/:container/", http.HandlerFunc(server.ContainerPutHandler))

	router.Get("/v1/:account", http.HandlerFunc(server.AccountGetHandler))
	router.Get("/v1/:account/", http.HandlerFunc(server.AccountGetHandler))
	router.Head("/v1/:account", http.HandlerFunc(server.AccountHeadHandler))
	router.Head("/v1/:account/", http.HandlerFunc(server.AccountHeadHandler))
	router.Put("/v1/:account", http.HandlerFunc(server.AccountPutHandler))
	router.Put("/v1/:account/", http.HandlerFunc(server.AccountPutHandler))
	router.Delete("/v1/:account", http.HandlerFunc(server.AccountDeleteHandler))
	router.Delete("/v1/:account/", http.HandlerFunc(server.AccountDeleteHandler))
	router.Post("/v1/:account", http.HandlerFunc(server.AccountPutHandler))
	router.Post("/v1/:account/", http.HandlerFunc(server.AccountPutHandler))

	// TODO: make this all dynamical and stuff
	middlewares := []struct {
		construct func(config conf.Section) (func(http.Handler) http.Handler, error)
		section   string
	}{
		{middleware.NewHealthcheck, "filter:healthcheck"},
		{middleware.NewRequestLogger, "filter:proxy-logging"},
		{middleware.NewTempURL, "filter:tempurl"},
		{middleware.NewTempAuth, "filter:tempauth"},
		{middleware.NewRatelimiter, "filter:ratelimit"},
	}
	pipeline := alice.New(middleware.NewContext(server.mc, server.C, server.logger))
	for _, m := range middlewares {
		mid, err := m.construct(config.GetSection(m.section))
		if err != nil {
			// TODO: propagate error upwards instead of panicking
			panic("Unable to construct middleware")
		}
		pipeline = pipeline.Append(mid)
	}
	return pipeline.Then(router)
}

func GetServer(serverconf conf.Config, flags *flag.FlagSet) (string, int, srv.Server, srv.LowLevelLogger, error) {
	var err error
	server := &ProxyServer{}
	server.C, err = client.NewProxyDirectClient()
	if err != nil {
		return "", 0, nil, nil, err
	}
	server.mc, err = ring.NewMemcacheRingFromConfig(serverconf)
	if err != nil {
		return "", 0, nil, nil, err
	}

	bindIP := serverconf.GetDefault("DEFAULT", "bind_ip", "0.0.0.0")
	bindPort := serverconf.GetInt("DEFAULT", "bind_port", 8080)
	if server.logger, err = srv.SetupLogger(serverconf, flags, "app:proxy-server", "proxy-server"); err != nil {
		return "", 0, nil, nil, fmt.Errorf("Error setting up logger: %v", err)
	}

	return bindIP, int(bindPort), server, server.logger, nil
}
