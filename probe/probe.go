package probe

import (
	"context"
	"encoding/json"
	"io"
	"net/http"
	"sync"
	"time"

	"github.com/cellargalaxy/go_common/util"
	"github.com/cellargalaxy/survive_monitor/model"
	"github.com/go-resty/resty/v2"
	"github.com/sirupsen/logrus"
)

// maxBodySize 响应体最多读这么多。判活只看响应码，响应体只用来认领全局视图，视图也就几KB到几十KB；
// 不设上限的话，探到一个大文件下载地址就是每轮整个下下来，既占内存又给被探测的服务添压力
const maxBodySize = 4 << 20

var clientLock = &sync.RWMutex{}
var clients = make(map[time.Duration]*resty.Client)

// Probe 探测单个URL。alive是该URL是否存活；view非空说明响应体解析出了本服务的全局视图，可以合并
func Probe(ctx context.Context, url string, timeout time.Duration) (bool, *model.View) {
	//不让resty自己读响应体，它会不设上限地整个读进内存，改由这里限量读
	response, err := getClient(ctx, timeout).R().SetContext(ctx).SetDoNotParseResponse(true).Get(url)
	if response != nil && response.RawBody() != nil {
		defer response.RawBody().Close()
	}
	if err != nil {
		logrus.WithContext(ctx).WithFields(logrus.Fields{"url": url, "err": err}).Warn("探测URL，请求异常")
		return false, nil
	}
	if response == nil || response.RawBody() == nil {
		logrus.WithContext(ctx).WithFields(logrus.Fields{"url": url}).Warn("探测URL，响应为空")
		return false, nil
	}

	statusCode := response.StatusCode()
	if statusCode <= 0 || statusCode >= http.StatusInternalServerError {
		logrus.WithContext(ctx).WithFields(logrus.Fields{"url": url, "statusCode": statusCode}).Warn("探测URL，响应码失败")
		return false, nil
	}

	//读响应体也算在单次超时里，读不完(超时、连接中断)跟原来交给resty读时一样按失败计；读到上限就截断，截断的解析不出视图，当普通URL
	data, err := io.ReadAll(io.LimitReader(response.RawBody(), maxBodySize))
	if err != nil {
		logrus.WithContext(ctx).WithFields(logrus.Fields{"url": url, "statusCode": statusCode, "err": err}).Warn("探测URL，读取响应体异常")
		return false, nil
	}

	view, ok := ParseView(ctx, data)
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
// 探测要的是「一次请求、一个结论」，重试会把单轮拖长，封禁更会让下一轮凭空多出一个失败。
// 跳过TLS证书校验是有意的：只关心服务有没有响应，证书过期、自签名都不算离线
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
