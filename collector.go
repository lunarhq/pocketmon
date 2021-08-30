package main

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"io/ioutil"
	"log"
	"net/http"
	"runtime"
	"strconv"
	"time"

	"github.com/shirou/gopsutil/cpu"
	"github.com/shirou/gopsutil/disk"
	"github.com/shirou/gopsutil/host"
	"github.com/shirou/gopsutil/mem"
)

const (
	sleepTime = 3 * time.Minute
	endpoint  = "https://injest.lunar.dev"
)

var (
	client = &http.Client{}
)

type NodeStats struct {
	Chain           string   `json:"chain"`
	AppVersion      string   `json:"app_version"`
	Moniker         string   `json:"moniker"`
	Height          int64    `json:"height"`
	LatestBlockTime string   `json:latest_block_time`
	CatchingUp      bool     `json:"catching_up"`
	Balance         float64  `json:"balance"`
	Chains          []string `json:"chains"`
	Jailed          bool     `json:"jailed"`
	ServiceUrl      string   `json:"service_url"`
	Address         string   `json:"address"`
	PublicKey       string   `json:"public_key"`
}

func (s NodeStats) String() string {
	return fmt.Sprintf("%s (%s), height:%d", s.Chain, s.AppVersion, s.Height)
}

func bytesHumanize(b uint64) string {
	const unit = 1000
	if b < unit {
		return fmt.Sprintf("%d B", b)
	}
	div, exp := int64(unit), 0
	for n := b / unit; n >= unit; n /= unit {
		div *= unit
		exp++
	}
	return fmt.Sprintf("%.1f %cB",
		float64(b)/float64(div), "kMGTPE"[exp])
}

//mem,disk in bytes
type HostStats struct {
	MemoryTotal     uint64
	MemoryFree      uint64
	DiskTotal       uint64
	DiskFree        uint64
	Uptime          uint64
	Platform        string
	CPUUsagePercent float64
}

func (s HostStats) String() string {
	memP := 100 * (float64(s.MemoryFree) / float64(s.MemoryTotal))
	diskP := 100 * (float64(s.DiskFree) / float64(s.DiskTotal))
	return fmt.Sprintf("Mem Free:%s(%.0f%%), Disk Free:%s(%.0f%%), CPU Usage: %.0f%%", bytesHumanize(s.MemoryFree), memP, bytesHumanize(s.DiskFree), diskP, s.CPUUsagePercent)
}

type Stats struct {
	Version   string    `json:"version"`
	Timestamp string    `json:"timestamp"`
	Node      NodeStats `json:"node"`
	Host      HostStats `json:"host"`
}

func (s Stats) String() string {
	return fmt.Sprintf(`
	Host: %s
	Node: %s`, s.Host.String(), s.Node.String())
}

func getUrl(node string) string {
	return fmt.Sprintf("%s/%s", endpoint, node)
}

func sendStats(node string, key string, s Stats) error {
	url := getUrl(node)

	data, err := json.Marshal(s)
	if err != nil {
		return err
	}

	req, err := http.NewRequest("POST", url, bytes.NewBuffer(data))
	req.Header.Set("x-api-key", key)
	req.Header.Set("Content-Type", "application/json")

	resp, err := client.Do(req)
	if err != nil {
		return err
	}

	if resp.StatusCode > 399 {
		defer resp.Body.Close()
		body, _ := ioutil.ReadAll(resp.Body)
		return errors.New(fmt.Sprintf("%d %s\n", resp.StatusCode, string(body)))
	}
	return nil
}

func queryBalance(addr string) (map[string]interface{}, error) {
	url := "http://localhost:8082/v1/query/balance"

	s := map[string]interface{}{
		"address": addr,
	}
	data, err := json.Marshal(s)
	if err != nil {
		//
		return nil, err
	}

	req, err := http.NewRequest("POST", url, bytes.NewBuffer(data))
	req.Header.Set("Content-Type", "application/json")

	r, err := client.Do(req)
	if err != nil {
		return nil, err
	}
	defer r.Body.Close()

	var result map[string]interface{}
	err = json.NewDecoder(r.Body).Decode(&result)
	if err != nil {
		return nil, err
	}

	return result, nil
}
func queryNode(addr string) (map[string]interface{}, error) {
	url := "http://localhost:8082/v1/query/node"

	s := map[string]interface{}{
		"address": addr,
	}
	data, err := json.Marshal(s)
	if err != nil {
		return nil, err
	}

	req, err := http.NewRequest("POST", url, bytes.NewBuffer(data))
	req.Header.Set("Content-Type", "application/json")

	r, err := client.Do(req)
	if err != nil {
		return nil, err
	}
	defer r.Body.Close()

	var result map[string]interface{}
	err = json.NewDecoder(r.Body).Decode(&result)
	if err != nil {
		return nil, err
	}

	return result, nil
}

func queryStatus() (map[string]interface{}, error) {
	url := "http://localhost:26657/status"
	r, err := client.Get(url)
	if err != nil {
		return nil, err
	}

	defer r.Body.Close()

	var result map[string]interface{}
	err = json.NewDecoder(r.Body).Decode(&result)
	if err != nil {
		return nil, err
	}

	return result, nil
}
func queryVersion() (string, error) {
	url := "http://localhost:8082/v1"
	r, err := client.Get(url)
	if err != nil {
		return "", err
	}

	defer r.Body.Close()

	var result string
	err = json.NewDecoder(r.Body).Decode(&result)
	if err != nil {
		return "", err
	}

	return result, nil
}

func collectNodeStats() (NodeStats, error) {
	s := NodeStats{
		Chain: "pocket",
	}

	//Get version
	ver, err := queryVersion()
	if err != nil {
		return s, err
	}
	s.AppVersion = ver

	statusResp, err := queryStatus()
	if err != nil {
		return s, err
	}
	status, ok := statusResp["result"].(map[string]interface{})
	if !ok {
		log.Println(statusResp)
		return s, errors.New("Invalid data from /status call")
	}

	nodeInfo := status["node_info"].(map[string]interface{})
	s.Address = nodeInfo["id"].(string)
	s.Moniker = nodeInfo["moniker"].(string)
	syncInfo := status["sync_info"].(map[string]interface{})
	h, err := strconv.ParseInt(syncInfo["latest_block_height"].(string), 10, 64)
	if err != nil {
		return s, err
	}
	s.Height = h
	s.LatestBlockTime = syncInfo["latest_block_time"].(string)
	s.CatchingUp = syncInfo["catching_up"].(bool)

	nodeResp, err := queryNode(s.Address)
	if err != nil {
		return s, err
	}
	// s.Chains = nodeResp["chains"].([]string)
	s.PublicKey = nodeResp["public_key"].(string)
	s.Jailed = nodeResp["jailed"].(bool)
	s.ServiceUrl = nodeResp["service_url"].(string)

	balResp, err := queryBalance(s.Address)
	if err != nil {
		return s, err
	}
	s.Balance = balResp["balance"].(float64)

	return s, nil
}

func collectHostStats() (HostStats, error) {
	s := HostStats{}

	runtimeOS := runtime.GOOS
	vmStat, err := mem.VirtualMemory()
	if err != nil {
		return s, err
	}
	s.MemoryTotal = vmStat.Total
	s.MemoryFree = vmStat.Available

	path := "/"
	if runtimeOS == "windows" {
		path = "\\"
	}
	diskStat, err := disk.Usage(path)
	if err != nil {
		return s, err
	}
	s.DiskTotal = diskStat.Total
	s.DiskFree = diskStat.Free

	percentage, err := cpu.Percent(0, false)
	if err != nil {
		return s, err
	}
	s.CPUUsagePercent = percentage[0]

	hostStat, err := host.Info()
	if err != nil {
		return s, err
	}
	s.Platform = hostStat.Platform
	s.Uptime = hostStat.Uptime

	return s, nil
}

func collectStats() (Stats, error) {
	s := Stats{
		Version:   "v1",
		Timestamp: time.Now().Format(time.RFC3339),
	}
	hs, err := collectHostStats()
	if err != nil {
		log.Println("err host stats:")
		return s, err
	}
	ns, err := collectNodeStats()
	if err != nil {
		log.Println("err node stats:")
		return s, err
	}

	s.Host = hs
	s.Node = ns

	log.Printf("%s", s.String())

	return s, nil
}

func collectAndSend(node, key string) {
	stats, err := collectStats()
	if err != nil {
		log.Println("Err collecting stats:", err)
		return
	}

	err = sendStats(node, key, stats)
	if err != nil {
		log.Println("Err sending stats:", err)
		return
	}
}

func start(ctx context.Context, node, key string, daemon bool) {
	fmt.Printf(`Started monitoring node: %s
You can view health status at https://lunar.dev/app
`, node)
	ticker := time.NewTicker(sleepTime)
	collectAndSend()
	for {
		select {
		case <-ctx.Done():
			return
		case <-ticker.C:
			collectAndSend(node, key)
		}
	}
}
