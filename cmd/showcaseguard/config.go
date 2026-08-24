package main

import (
	"flag"
	"fmt"
	"io"
	"net"
	"strconv"
	"strings"
)

const defaultAddress = "127.0.0.1:19081"

type config struct {
	address   string
	dataDir   string
	selfCheck bool
}

func parseConfig(arguments []string, getenv func(string) string) (config, error) {
	configuredDefault, err := addressFromEnvironment(getenv("PORT"))
	if err != nil {
		return config{}, err
	}
	flags := flag.NewFlagSet("showcaseguard", flag.ContinueOnError)
	flags.SetOutput(io.Discard)
	address := flags.String("addr", configuredDefault, "HTTP 监听地址")
	dataDir := flags.String("data", "", "数据存储目录")
	selfCheck := flags.Bool("self-check", false, "启动服务并完成回环自检后退出")
	if err := flags.Parse(arguments); err != nil {
		return config{}, err
	}
	if flags.NArg() != 0 {
		return config{}, fmt.Errorf("不支持的位置参数: %s", strings.Join(flags.Args(), " "))
	}
	validated, err := validateAddress(*address)
	if err != nil {
		return config{}, err
	}
	return config{address: validated, dataDir: strings.TrimSpace(*dataDir), selfCheck: *selfCheck}, nil
}

func addressFromEnvironment(port string) (string, error) {
	port = strings.TrimSpace(port)
	if port == "" {
		return defaultAddress, nil
	}
	number, err := strconv.Atoi(port)
	if err != nil || number < 1 || number > 65535 {
		return "", fmt.Errorf("PORT 必须是 1-65535 的端口号")
	}
	return net.JoinHostPort("127.0.0.1", strconv.Itoa(number)), nil
}

func validateAddress(address string) (string, error) {
	host, port, err := net.SplitHostPort(strings.TrimSpace(address))
	if err != nil {
		return "", fmt.Errorf("-addr 必须为 host:port: %w", err)
	}
	if host == "0.0.0.0" || host == "" || host == "::" {
		return "", fmt.Errorf("禁止监听非特定地址 %q，请使用 127.0.0.1", host)
	}
	ip := net.ParseIP(host)
	if host != "localhost" && (ip == nil || !ip.IsLoopback()) {
		return "", fmt.Errorf("本服务仅允许监听回环地址")
	}
	number, err := strconv.Atoi(port)
	if err != nil || number < 1 || number > 65535 {
		return "", fmt.Errorf("监听端口必须为 1-65535")
	}
	return net.JoinHostPort(host, strconv.Itoa(number)), nil
}
