package helper

import (
	"encoding/json"
	"bufio"
	"context"
	"fmt"
	"io"
	"net/http"
	"one-api/common"
	"one-api/constant"
	relaycommon "one-api/relay/common"
	"one-api/setting/operation_setting"
	"strings"
	"sync"
	"time"

	"github.com/bytedance/gopkg/util/gopool"

	"github.com/gin-gonic/gin"
)

const (
	InitialScannerBufferSize = 64 << 10  // 64KB (64*1024)
	MaxScannerBufferSize     = 10 << 20  // 10MB (10*1024*1024)
	DefaultPingInterval      = 10 * time.Second
)

func StreamScannerHandler(c *gin.Context, resp *http.Response, info *relaycommon.RelayInfo, dataHandler func(data string) bool) {

	if resp == nil || dataHandler == nil {
		return
	}

	// 确保响应体总是被关闭
	defer func() {
		if resp.Body != nil {
			resp.Body.Close()
		}
	}()

	streamingTimeout := time.Duration(constant.StreamingTimeout) * time.Second
	if strings.HasPrefix(info.UpstreamModelName, "o") {
		// twice timeout for thinking model
		streamingTimeout *= 2
	}

	var (
		stopChan   = make(chan bool, 3) // 增加缓冲区避免阻塞
		scanner    = bufio.NewScanner(resp.Body)
		ticker     = time.NewTicker(streamingTimeout)
		pingTicker *time.Ticker
		writeMutex sync.Mutex // Mutex to protect concurrent writes
		wg         sync.WaitGroup // 用于等待所有 goroutine 退出
	)

	generalSettings := operation_setting.GetGeneralSetting()
	pingEnabled := generalSettings.PingIntervalEnabled
	pingInterval := time.Duration(generalSettings.PingIntervalSeconds) * time.Second
	if pingInterval <= 0 {
		pingInterval = DefaultPingInterval
	}

	if pingEnabled {
		pingTicker = time.NewTicker(pingInterval)
	}

	// 改进资源清理，确保所有 goroutine 正确退出
	defer func() {
		// 通知所有 goroutine 停止
		common.SafeSendBool(stopChan, true)
		
		ticker.Stop()
		if pingTicker != nil {
			pingTicker.Stop()
		}
		
		// 等待所有 goroutine 退出，最多等待5秒
		done := make(chan struct{})
		go func() {
			wg.Wait()
			close(done)
		}()
		
		select {
		case <-done:
		case <-time.After(5 * time.Second):
			common.LogError(c, "timeout waiting for goroutines to exit")
		}
		
		close(stopChan)
	}()
	
	scanner.Buffer(make([]byte, InitialScannerBufferSize), MaxScannerBufferSize)
	scanner.Split(bufio.ScanLines)
	SetEventStreamHeaders(c)

	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()

	ctx = context.WithValue(ctx, "stop_chan", stopChan)

	// Handle ping data sending with improved error handling
	if pingEnabled && pingTicker != nil {
		wg.Add(1)
		gopool.Go(func() {
			defer func() {
				wg.Done()
				if r := recover(); r != nil {
					common.LogError(c, fmt.Sprintf("ping goroutine panic: %v", r))
					common.SafeSendBool(stopChan, true)
				}
				if common.DebugEnabled {
					println("ping goroutine exited")
				}
			}()
			
			// 添加超时保护，防止 goroutine 无限运行
			maxPingDuration := 30 * time.Minute // 最大 ping 持续时间
			pingTimeout := time.NewTimer(maxPingDuration)
			defer pingTimeout.Stop()
			
			for {
				select {
				case <-pingTicker.C:
					// 使用超时机制防止写操作阻塞
					done := make(chan error, 1)
					go func() {
						writeMutex.Lock()
						defer writeMutex.Unlock()
						done <- PingData(c)
					}()
					
					select {
					case err := <-done:
						if err != nil {
							common.LogError(c, "ping data error: "+err.Error())
							return
						}
						if common.DebugEnabled {
							println("ping data sent")
						}
					case <-time.After(10 * time.Second):
						common.LogError(c, "ping data send timeout")
						return
					case <-ctx.Done():
						return
					case <-stopChan:
						return
					}
				case <-ctx.Done():
					return
				case <-stopChan:
					return
				case <-c.Request.Context().Done():
					// 监听客户端断开连接
					return
				case <-pingTimeout.C:
					common.LogError(c, "ping goroutine max duration reached")
					return
				}
			}
		})
	}

	// Scanner goroutine with improved error handling
	wg.Add(1)
	common.RelayCtxGo(ctx, func() {
		defer func() {
			wg.Done()
			if r := recover(); r != nil {
				common.LogError(c, fmt.Sprintf("scanner goroutine panic: %v", r))
			}
			common.SafeSendBool(stopChan, true)
			if common.DebugEnabled {
				println("scanner goroutine exited")
			}
		}()
		
		for scanner.Scan() {
			// 检查是否需要停止
			select {
			case <-stopChan:
				return
			case <-ctx.Done():
				return
			case <-c.Request.Context().Done():
				return
			default:
			}
			
			ticker.Reset(streamingTimeout)
			data := scanner.Text()
			if common.DebugEnabled {
				println(data)
			}

			if len(data) < 6 {
				continue
			}
			if data[:5] != "data:" && data[:6] != "[DONE]" {
				continue
			}
			data = data[5:]
			data = strings.TrimLeft(data, " ")
			data = strings.TrimSuffix(data, "\r")
			if !strings.HasPrefix(data, "[DONE]") {
				info.SetFirstResponseTime()
				
				var jsonData map[string]interface{}
				if err := json.Unmarshal([]byte(data), &jsonData); err == nil {
					if choices, ok := jsonData["choices"].([]interface{}); ok && len(choices) == 0 {
						// ✅ 替换 jsonData 中的 choices 字段为伪内容，保留其他字段
						jsonData["choices"] = []interface{}{
							map[string]interface{}{
								"delta": map[string]interface{}{
									"content": "",
								},
							},
						}

						if patchedBytes, err := json.Marshal(jsonData); err == nil {
							data = string(patchedBytes) // ✅ 替换原始 data
							if common.DebugEnabled {
								println("🛠️ Patched empty choices, kept full structure:", data)
							}
						}
					}
				}
				
				// 使用超时机制防止写操作阻塞
				done := make(chan bool, 1)
				go func() {
					writeMutex.Lock()
					defer writeMutex.Unlock()
					done <- dataHandler(data)
				}()
				
				select {
				case success := <-done:
					if !success {
						return
					}
				case <-time.After(10 * time.Second):
					common.LogError(c, "data handler timeout")
					return
				case <-ctx.Done():
					return
				case <-stopChan:
					return
				}
			}
		}

		if err := scanner.Err(); err != nil {
			if err != io.EOF {
				common.LogError(c, "scanner error: "+err.Error())
			}
		}
	})

	// 主循环等待完成或超时
	select {
	case <-ticker.C:
		// 超时处理逻辑
		common.LogError(c, "streaming timeout")
	case <-stopChan:
		// 正常结束
		common.LogInfo(c, "streaming finished")
	case <-c.Request.Context().Done():
		// 客户端断开连接
		common.LogInfo(c, "client disconnected")
	}
}
