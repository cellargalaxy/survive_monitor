package probe

import (
	"context"
	"encoding/json"
	"net/http"
	"sync"
	"time"

	"github.com/cellargalaxy/go_common/util"
	"github.com/cellargalaxy/survive_monitor/model"
	"github.com/go-resty/resty/v2"
	"github.com/sirupsen/logrus"
)

var clientLock = &sync.RWMutex{}
var clients = make(map[time.Duration]*resty.Client)

// Probe 探测单个URL。alive是该URL是否存活；view非空说明响应体解析出了本服务的全局视图，可以合并
func Probe(ctx context.Context, url string, timeout time.Duration) (bool, *model.View) {
	response, err := getClient(ctx, timeout).R().SetContext(ctx).Get(url)
	if err != nil {
		logrus.WithContext(ctx).WithFields(logrus.Fields{"url": url, "err": err}).Warn("探测URL，请求异常")
		return false, nil
	}
	if response == nil {
		logrus.WithContext(ctx).WithFields(logrus.Fields{"url": url}).Warn("探测URL，响应为空")
		return false, nil
	}

	statusCode := response.StatusCode()
	if statusCode <= 0 || statusCode >= http.StatusInternalServerError {
		logrus.WithContext(ctx).WithFields(logrus.Fields{"url": url, "statusCode": statusCode}).Warn("探测URL，响应码失败")
		return false, nil
	}

	view, ok := ParseView(ctx, response.Body())
	if !ok {
		logrus.WithContext(ctx).WithFields(logrus.Fields{"url": url, "statusCode": statusCode}).Info("探测URL，响应")
		return true, nil
	}
	logrus.WithContext(ctx).WithFields(logrus.Fields{"url": url, "statusCode": statusCode, "len": len(view.Snapshots)}).Info("探测URL，解析到全局视图")
	return true, view
}

// ParseView 从响应体里认领全局视图，是GetView的镜像，两边的验收标准是往返恒等。
// 不靠URL长什么样判断对端是不是本服务，只看内容能不能解出来；
// 认领条件要严到不会误认：外壳是本家的响应体、业务码成功、快照非空，缺一条就当普通URL
func ParseView(ctx context.Context, data []byte) (*model.View, bool) {
	if len(data) == 0 {
		return nil, false
	}

	var envelope struct {
		Code int        `json:"code"`
		Data model.View `json:"data"`
	}
	//这里不能用util.JsonData2Struct：它解析失败会把整个响应体打到日志里，
	//而绝大多数被探测的URL返回的本来就是网页，每轮每个URL都要刷一遍整页HTML
	err := json.Unmarshal(data, &envelope)
	if err != nil {
		return nil, false
	}
	if envelope.Code != http.StatusOK || len(envelope.Data.Snapshots) == 0 {
		return nil, false
	}
	return &envelope.Data, true
}

// getClient 按超时缓存客户端。不能用util.NewHttpClientReq，那个共享客户端带重试且4xx会把地址短时封禁，
// 探测要的是「一次请求、一个结论」，重试会把单轮拖长，封禁更会让下一轮凭空多出一个失败
func getClient(ctx context.Context, timeout time.Duration) *resty.Client {
	clientLock.RLock()
	client := clients[timeout]
	clientLock.RUnlock()
	if client != nil {
		return client
	}

	clientLock.Lock()
	defer clientLock.Unlock()
	client = clients[timeout]
	if client != nil {
		return client
	}
	client = util.CreateHttpClient(timeout, nil, nil, true)
	clients[timeout] = client
	return client
}
