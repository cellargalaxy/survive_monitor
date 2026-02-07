package service

import (
	"context"
	"fmt"
	"net/url"
	"strings"
	"sync"
	"time"

	"github.com/cellargalaxy/go_common/util"
	"github.com/cellargalaxy/msg_gateway/sdk"
	"github.com/cellargalaxy/survive_monitor/config"
	"github.com/sirupsen/logrus"
)

// StatusRecord 记录单次检测结果
type StatusRecord struct {
	URL       string    `json:"url"`
	CheckTime time.Time `json:"check_time"`
	Alive     bool      `json:"alive"`
}

// statusStore 全局存活记录存储
var statusStore = &StatusStore{
	records: make(map[string][]StatusRecord),
}

// StatusStore 线程安全的存活记录存储
type StatusStore struct {
	mu      sync.RWMutex
	records map[string][]StatusRecord // key: url
}

// AddRecord 添加一条检测记录
func (s *StatusStore) AddRecord(urlStr string, checkTime time.Time, alive bool) {
	s.mu.Lock()
	defer s.mu.Unlock()

	record := StatusRecord{
		URL:       urlStr,
		CheckTime: checkTime,
		Alive:     alive,
	}
	s.records[urlStr] = append(s.records[urlStr], record)
}

// CleanOldRecords 清除7天以前的数据
func (s *StatusStore) CleanOldRecords() {
	s.mu.Lock()
	defer s.mu.Unlock()

	cutoff := time.Now().AddDate(0, 0, -7)
	for urlStr, records := range s.records {
		var filtered []StatusRecord
		for _, r := range records {
			if r.CheckTime.After(cutoff) {
				filtered = append(filtered, r)
			}
		}
		if len(filtered) == 0 {
			delete(s.records, urlStr)
		} else {
			s.records[urlStr] = filtered
		}
	}
}

// URLStatus 返回给前端的单个URL状态
type URLStatus struct {
	URL     string         `json:"url"`
	Domain  string         `json:"domain"`
	Path    string         `json:"path"`
	Records []RecordOutput `json:"records"`
}

// RecordOutput 返回给前端的单条记录
type RecordOutput struct {
	CheckTime string `json:"check_time"`
	Alive     bool   `json:"alive"`
}

// GetAllStatus 获取全部URL的存活情况
func (s *StatusStore) GetAllStatus() []URLStatus {
	s.mu.RLock()
	defer s.mu.RUnlock()

	var result []URLStatus
	for urlStr, records := range s.records {
		domain, path := parseDomainAndPath(urlStr)
		var outputs []RecordOutput
		for _, r := range records {
			outputs = append(outputs, RecordOutput{
				CheckTime: r.CheckTime.Format("2006-01-02 15:04:05"),
				Alive:     r.Alive,
			})
		}
		result = append(result, URLStatus{
			URL:     urlStr,
			Domain:  domain,
			Path:    path,
			Records: outputs,
		})
	}
	return result
}

// GetStatusStore 获取全局存储实例（供handler调用）
func GetStatusStore() *StatusStore {
	return statusStore
}

// parseDomainAndPath 从URL中解析域名和路径
func parseDomainAndPath(rawURL string) (domain string, path string) {
	u, err := url.Parse(rawURL)
	if err != nil {
		return rawURL, "/"
	}
	domain = u.Scheme + "://" + u.Host
	path = u.Path
	if path == "" {
		path = "/"
	}
	return domain, path
}

// groupURLsByDomain 按域名对URL进行分组
func groupURLsByDomain(urls []string) map[string][]string {
	groups := make(map[string][]string)
	for _, rawURL := range urls {
		domain, _ := parseDomainAndPath(rawURL)
		groups[domain] = append(groups[domain], rawURL)
	}
	return groups
}

func MonitorConfig(ctx context.Context) {
	// 先清理过期数据
	statusStore.CleanOldRecords()

	urls := config.Config.Urls

	// 按域名分组
	domainGroups := groupURLsByDomain(urls)

	// 按域名维度遍历
	for domain, domainURLs := range domainGroups {
		var offlinePaths []string

		for _, u := range domainURLs {
			ok := MonitorAndAlarmCollect(ctx, u)
			if !ok {
				_, path := parseDomainAndPath(u)
				offlinePaths = append(offlinePaths, path)
			}
		}

		// 如果该域名下有离线的path，统一发送一条告警消息
		if len(offlinePaths) > 0 {
			msg := fmt.Sprintf("服务离线 [%s]\n%s", domain, strings.Join(offlinePaths, "\n"))
			sdk.SendTemplateText(ctx, "通用消息", config.Config.BoardUrl, msg)
		}
	}
}

// MonitorAndAlarmCollect 检测存活并收集结果（不再单独发送告警）
func MonitorAndAlarmCollect(ctx context.Context, url string) bool {
	ok := MonitorSurvive(ctx, url)
	return ok
}
func MonitorSurvive(ctx context.Context, url string) bool {
	checkTime := time.Now()
	for i := 0; i < 1; i++ {
		ok := monitorSurvive(ctx, url)
		if ok {
			statusStore.AddRecord(url, checkTime, true)
			return true
		}
		time.Sleep(time.Second)
	}
	statusStore.AddRecord(url, checkTime, false)
	return false
}
func monitorSurvive(ctx context.Context, url string) bool {
	response, err := util.GetHttpSpiderRequest(ctx).Get(url)
	if err != nil {
		logrus.WithContext(ctx).WithFields(logrus.Fields{"url": url, "err": err}).Error("检测存活，请求异常")
		return false
	}
	if response == nil {
		logrus.WithContext(ctx).WithFields(logrus.Fields{"url": url, "err": err}).Error("检测存活，响应为空")
		return false
	}
	statusCode := response.StatusCode()
	logrus.WithContext(ctx).WithFields(logrus.Fields{"url": url, "statusCode": statusCode}).Info("检测存活，响应")
	return statusCode > 0 && statusCode < 500
}
