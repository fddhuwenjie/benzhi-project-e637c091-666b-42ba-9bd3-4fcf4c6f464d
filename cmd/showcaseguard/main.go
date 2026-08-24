package main

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"log"
	"net"
	"net/http"
	"os"
	"os/signal"
	"strings"
	"syscall"
	"time"

	"showcaseguard/internal/domain"
	"showcaseguard/internal/httpapi"
	"showcaseguard/internal/store"
	"showcaseguard/internal/web"
	"showcaseguard/internal/workflow"
)

const (
	readTimeout  = 10 * time.Second
	writeTimeout = 20 * time.Second
)

func main() {
	if err := run(os.Args[1:], os.Getenv, os.Stdout); err != nil {
		log.Printf("启动失败: %v", err)
		os.Exit(1)
	}
}

func run(arguments []string, getenv func(string) string, output io.Writer) error {
	configuration, err := parseConfig(arguments, getenv)
	if err != nil {
		return err
	}
	dataDirectory := configuration.dataDir
	cleanup := func() {}
	if dataDirectory == "" && configuration.selfCheck {
		dataDirectory, err = os.MkdirTemp("", "showcaseguard-self-check-")
		if err != nil {
			return fmt.Errorf("创建自检数据目录: %w", err)
		}
		cleanup = func() { _ = os.RemoveAll(dataDirectory) }
	} else if dataDirectory == "" {
		dataDirectory = "data"
	}
	defer cleanup()
	storage, err := store.Open(dataDirectory)
	if err != nil {
		return err
	}
	if err := storage.SeedShowcases(defaultShowcases()); err != nil {
		return fmt.Errorf("初始化展柜: %w", err)
	}
	service := workflow.New(storage)
	webHandler, err := web.New()
	if err != nil {
		return fmt.Errorf("加载页面资源: %w", err)
	}
	handler := httpapi.New(service, webHandler).Routes()
	server := &http.Server{Addr: configuration.address, Handler: handler, ReadHeaderTimeout: 5 * time.Second, ReadTimeout: readTimeout, WriteTimeout: writeTimeout, IdleTimeout: 60 * time.Second}
	listener, err := net.Listen("tcp", configuration.address)
	if err != nil {
		return fmt.Errorf("监听 %s: %w", configuration.address, err)
	}
	if configuration.selfCheck {
		return performSelfCheck(server, listener, output)
	}
	fmt.Fprintf(output, "展柜微环境异常处置台已启动：http://%s\n", configuration.address)
	serveErrors := make(chan error, 1)
	go func() { serveErrors <- server.Serve(listener) }()
	signals := make(chan os.Signal, 1)
	signal.Notify(signals, syscall.SIGINT, syscall.SIGTERM)
	defer signal.Stop(signals)
	select {
	case received := <-signals:
		fmt.Fprintf(output, "收到信号 %s，正在安全退出\n", received)
	case serveErr := <-serveErrors:
		if !errors.Is(serveErr, http.ErrServerClosed) {
			return serveErr
		}
		return nil
	}
	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()
	if err := server.Shutdown(ctx); err != nil {
		return fmt.Errorf("关闭 HTTP 服务: %w", err)
	}
	return nil
}

func performSelfCheck(server *http.Server, listener net.Listener, output io.Writer) error {
	serveErrors := make(chan error, 1)
	go func() { serveErrors <- server.Serve(listener) }()
	defer func() {
		ctx, cancel := context.WithTimeout(context.Background(), 2*time.Second)
		defer cancel()
		_ = server.Shutdown(ctx)
	}()
	client := &http.Client{Timeout: 2 * time.Second}
	baseURL := "http://" + listener.Addr().String()
	deadline := time.Now().Add(5 * time.Second)
	for {
		if err := probe(client, baseURL+"/healthz", "data"); err == nil {
			break
		}
		if time.Now().After(deadline) {
			return fmt.Errorf("健康探测超时")
		}
		select {
		case err := <-serveErrors:
			return fmt.Errorf("自检期间服务退出: %w", err)
		case <-time.After(50 * time.Millisecond):
		}
	}
	if err := probe(client, baseURL+"/api/showcases", "data"); err != nil {
		return fmt.Errorf("展柜接口自检: %w", err)
	}
	if err := probeHTML(client, baseURL+"/"); err != nil {
		return fmt.Errorf("工作台页面自检: %w", err)
	}
	if err := exerciseWorkflow(client, baseURL); err != nil {
		return fmt.Errorf("异常闭环自检: %w", err)
	}
	fmt.Fprintf(output, "self-check 通过：%s 的页面、接口和异常闭环均可用\n", listener.Addr().String())
	return nil
}

func probe(client *http.Client, url, requiredField string) error {
	response, err := client.Get(url)
	if err != nil {
		return err
	}
	defer response.Body.Close()
	if response.StatusCode != http.StatusOK {
		return fmt.Errorf("状态码 %d", response.StatusCode)
	}
	var payload map[string]any
	if err := json.NewDecoder(io.LimitReader(response.Body, 1<<20)).Decode(&payload); err != nil {
		return err
	}
	if _, exists := payload[requiredField]; !exists {
		return fmt.Errorf("响应缺少字段 %s", requiredField)
	}
	return nil
}

func probeHTML(client *http.Client, url string) error {
	response, err := client.Get(url)
	if err != nil {
		return err
	}
	defer response.Body.Close()
	if response.StatusCode != http.StatusOK {
		return fmt.Errorf("状态码 %d", response.StatusCode)
	}
	contentType := response.Header.Get("Content-Type")
	if !strings.Contains(contentType, "text/html") {
		return fmt.Errorf("Content-Type 不是 HTML: %s", contentType)
	}
	body, err := io.ReadAll(io.LimitReader(response.Body, 64<<10))
	if err != nil {
		return err
	}
	if !strings.Contains(string(body), "展柜微环境异常处置台") {
		return fmt.Errorf("页面缺少应用标题")
	}
	return nil
}

func defaultShowcases() []domain.Showcase {
	return []domain.Showcase{
		{ID: "SC-A01", Name: "错金银青铜器展柜", GalleryZone: "常设展二层东厅", CollectionLevel: domain.CollectionRare, SensorIDs: []string{"TH-A01-1", "TH-A01-2"}, TargetTemperatureMin: 18, TargetTemperatureMax: 22, TargetHumidityMin: 45, TargetHumidityMax: 55, Active: true},
		{ID: "SC-B07", Name: "明清书画轮换展柜", GalleryZone: "专题展一层南厅", CollectionLevel: domain.CollectionKey, SensorIDs: []string{"TH-B07-1"}, TargetTemperatureMin: 19, TargetTemperatureMax: 23, TargetHumidityMin: 48, TargetHumidityMax: 55, Active: true},
		{ID: "SC-C12", Name: "陶瓷标本展柜", GalleryZone: "常设展三层西厅", CollectionLevel: domain.CollectionGeneral, SensorIDs: []string{"TH-C12-1"}, TargetTemperatureMin: 18, TargetTemperatureMax: 24, TargetHumidityMin: 40, TargetHumidityMax: 60, Active: true},
	}
}
