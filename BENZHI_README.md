# BENZHI_README

## 项目说明
- 项目：benzhi-project-e637c091-666b-42ba-9bd3-4fcf4c6f464d
- 项目用途：已完整实现面向博物馆藏品保管人员的展柜微环境异常闭环处置台，包含异常指纹去重、影响分级、方案审批、现场证据、连续复测、自动归档、乐观并发、请求幂等、本地原子快照、追加审计日志及原生浏览器工作台。
- Go 工具链：`golang:1.23`
- 前端工具链：原生 HTML、CSS 和 JavaScript，由 Go 服务直接提供

## 项目描述
- 项目名称：展柜微环境异常处置台
- 项目概述：面向博物馆藏品保管人员的微环境异常闭环工作台：从展柜传感器或人工发现异常开始，经过分级、方案审批、现场处置、复测验证，最终安全关闭并沉淀可检索档案。项目根目录提供简体中文 README.md，说明标准构建、运行和测试方式。
- 核心工作流：展柜异常登记→影响分级→处置方案审批→现场动作记录→复测验证→关闭归档
- 对外接口：由 Go 服务提供原生 HTML、CSS 和 JavaScript 浏览器工作台及 JSON 接口；监听地址支持 -addr=127.0.0.1:<port> 或 PORT 环境变量（PORT 仅端口号时绑定 127.0.0.1:<PORT>），默认使用 127.0.0.1:19081，禁止绑定 0.0.0.0；不引入 Node 构建链。

## 标准构建、运行和测试命令
进入容器后执行：
cd '/app' && GOTOOLCHAIN=local go build ./...
cd /app && GOTOOLCHAIN=local go run ./cmd/showcaseguard -addr=127.0.0.1:19081 -self-check
cd '/app' && GOTOOLCHAIN=local go test ./...

## Docker 构建和进入容器
chmod +x build_benzhi_docker.sh
./build_benzhi_docker.sh benzhi-project-e637c091-666b-42ba-9bd3-4fcf4c6f464d-amd64 linux/amd64
./build_benzhi_docker.sh benzhi-project-e637c091-666b-42ba-9bd3-4fcf4c6f464d-arm64 linux/arm64
docker run -it benzhi-project-e637c091-666b-42ba-9bd3-4fcf4c6f464d-amd64:latest

## 题目验证命令
1. 预期退出码 0：`go test ./...`
2. 预期退出码 0：`go run ./cmd/showcaseguard -addr=127.0.0.1:19081 -self-check`
